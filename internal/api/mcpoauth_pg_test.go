package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// insertSSOUser registers an account the way a web sign-in does: keyed by the
// subject the identity provider gave it.
func insertSSOUser(t *testing.T, pool *pgxpool.Pool, subject, status string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	username := "sso-" + uuid.NewString()
	if err := pool.QueryRow(context.Background(), `INSERT INTO users(username,display_name,role,status,oidc_subject) VALUES($1,'SSO test',$2,$3,$4) RETURNING id`, username, "user", status, subject).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, id) })
	return id
}

func insertAPIKey(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID, permissions []string) string {
	t.Helper()
	raw := "igk_" + uuid.NewString()
	hash := sha256.Sum256([]byte(raw))
	if _, err := pool.Exec(context.Background(), `INSERT INTO api_keys(user_id,name,key_hash,key_prefix,permissions) VALUES($1,'mcp test',$2,$3,$4)`, userID, hash[:], tokenPrefix(raw), permissions); err != nil {
		t.Fatal(err)
	}
	return raw
}

func mcpOAuthPGServer(t *testing.T, pool *pgxpool.Pool, idp *fakeIdP, mcp string) *Server {
	t.Helper()
	s := mcpOAuthServer(t, idp, mcp)
	s.DB = pool
	// The key policy and the rest come from the migrated database here.
	s.invalidateSetting("api_keys")
	s.invalidateSetting("tracking")
	return s
}

func countUsers(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A token minted for this server opens tools/list as the account that signed
// in through the web, by either path: the administrator's audience list
// (azp, no mapper) or the resource identifier in aud (Audience mapper).
func TestMCPOAuthOpensToolsForARegisteredAccount(t *testing.T) {
	pool := migratedPool(t)
	idp := newFakeIdP(t)
	subject := uuid.NewString()
	userID := insertSSOUser(t, pool, subject, "active")
	cases := map[string]struct {
		mcp    string
		claims map[string]any
	}{
		"azp in the audience list": {`{"oauth":{"enabled":true,"audience":["claude-mcp"]}}`, nil},
		"resource in aud":          {`{"oauth":{"enabled":true}}`, map[string]any{"aud": []string{"account", "https://games.example.test/mcp"}}},
		"web client as azp":        {`{"oauth":{"enabled":true}}`, map[string]any{"azp": "igame-web"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := mcpOAuthPGServer(t, pool, idp, tc.mcp)
			recorder, message := rpcCall(t, s, idp.sign(t, idp.claims(subject, tc.claims)))
			if recorder.Code != 200 || message != "" {
				t.Fatalf("tools/list answered %d %q: %s", recorder.Code, message, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"games_list"`) {
				t.Fatalf("tools/list did not list the tools: %s", recorder.Body.String())
			}
			// profile_get shows whom the token was mapped to.
			request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"profile_get","arguments":{}}}`))
			request.Header.Set("Authorization", "Bearer "+idp.sign(t, idp.claims(subject, tc.claims)))
			profile := httptest.NewRecorder()
			s.Router().ServeHTTP(profile, request)
			if !strings.Contains(profile.Body.String(), userID.String()) {
				t.Fatalf("profile_get did not answer as the registered account: %s", profile.Body.String())
			}
		})
	}
}

// The token's scopes are the administrator's, held to the same policy a key
// is: a tool outside them is refused, and the refusal names the setting.
func TestMCPOAuthScopesAreTheAdministrators(t *testing.T) {
	pool := migratedPool(t)
	idp := newFakeIdP(t)
	subject := uuid.NewString()
	insertSSOUser(t, pool, subject, "active")
	s := mcpOAuthPGServer(t, pool, idp, `{"oauth":{"enabled":true,"audience":["claude-mcp"],"scopes":["mcp:access","games:read"]}}`)
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"leaderboard_get","arguments":{"game_id":"snake"}}}`))
	request.Header.Set("Authorization", "Bearer "+idp.sign(t, idp.claims(subject, nil)))
	recorder := httptest.NewRecorder()
	s.Router().ServeHTTP(recorder, request)
	if !strings.Contains(recorder.Body.String(), "SSO token (mcp.oauth.scopes) requires rankings:read") {
		t.Fatalf("a tool outside the configured scopes was not refused by name: %s", recorder.Body.String())
	}
	// Without mcp:access in the list the door itself stays shut.
	s = mcpOAuthPGServer(t, pool, idp, `{"oauth":{"enabled":true,"audience":["claude-mcp"],"scopes":["games:read"]}}`)
	response, message := rpcCall(t, s, idp.sign(t, idp.claims(subject, nil)))
	if response.Code != 403 || !strings.Contains(message, "requires mcp:access") {
		t.Fatalf("scopes without mcp:access answered %d %q", response.Code, message)
	}
}

// Nobody is registered by a token. An unknown subject and a disabled account
// are both refused with the same instruction, and the users table does not
// grow.
func TestMCPOAuthNeverCreatesOrRevivesAnAccount(t *testing.T) {
	pool := migratedPool(t)
	idp := newFakeIdP(t)
	disabled := uuid.NewString()
	insertSSOUser(t, pool, disabled, "disabled")
	s := mcpOAuthPGServer(t, pool, idp, `{"oauth":{"enabled":true,"audience":["claude-mcp"]}}`)
	before := countUsers(t, pool)
	for name, subject := range map[string]string{"unknown": uuid.NewString(), "disabled": disabled} {
		recorder, message := rpcCall(t, s, idp.sign(t, idp.claims(subject, map[string]any{"preferred_username": "someone", "realm_access": map[string]any{"roles": []string{"admin"}}})))
		if recorder.Code != 401 || !strings.Contains(message, "sign in to the web portal once first") {
			t.Fatalf("%s subject answered %d %q", name, recorder.Code, message)
		}
	}
	if after := countUsers(t, pool); after != before {
		t.Fatalf("users grew from %d to %d on a token", before, after)
	}
}

// A personal key keeps working beside the new door, and an account's own key
// is not widened by the token setting.
func TestMCPOAuthLeavesPersonalKeysAsTheyWere(t *testing.T) {
	pool := migratedPool(t)
	idp := newFakeIdP(t)
	userID := insertTestUser(t, pool, "user")
	key := insertAPIKey(t, pool, userID, []string{"mcp:access", "games:read"})
	for _, mcp := range []string{"", `{"oauth":{"enabled":true,"audience":["claude-mcp"]}}`} {
		s := mcpOAuthPGServer(t, pool, idp, mcp)
		recorder, message := rpcCall(t, s, key)
		if recorder.Code != 200 || message != "" {
			t.Fatalf("key with mcp setting %q answered %d %q", mcp, recorder.Code, message)
		}
	}
	bare := insertAPIKey(t, pool, userID, []string{"games:read"})
	recorder, message := rpcCall(t, mcpOAuthPGServer(t, pool, idp, `{"oauth":{"enabled":true}}`), bare)
	if recorder.Code != 403 || message != "API key requires mcp:access" {
		t.Fatalf("key without mcp:access answered %d %q, want the old refusal", recorder.Code, message)
	}
}

// A valid token opens /mcp and nothing else: REST answers 401 as if no
// credential had been sent.
func TestMCPOAuthTokenIsRefusedOnREST(t *testing.T) {
	pool := migratedPool(t)
	idp := newFakeIdP(t)
	subject := uuid.NewString()
	insertSSOUser(t, pool, subject, "active")
	s := mcpOAuthPGServer(t, pool, idp, `{"oauth":{"enabled":true,"audience":["claude-mcp"]}}`)
	token := idp.sign(t, idp.claims(subject, nil))
	if recorder, message := rpcCall(t, s, token); recorder.Code != 200 || message != "" {
		t.Fatalf("token did not open /mcp: %d %q", recorder.Code, message)
	}
	for _, path := range []string{"/api/v1/me", "/api/v1/games", "/api/v1/admin/settings"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		s.Router().ServeHTTP(recorder, request)
		if recorder.Code != 401 {
			t.Fatalf("%s with an SSO token answered %d: %s", path, recorder.Code, recorder.Body.String())
		}
	}
}

// The setting round-trips through the admin API, and turning it on without an
// issuer is refused at save time.
func TestMCPSettingSavesThroughTheAdminAPI(t *testing.T) {
	pool := migratedPool(t)
	idp := newFakeIdP(t)
	admin := insertTestUser(t, pool, "admin")
	cookie := insertTestSession(t, pool, admin)
	s := New(pool, nil, nil)
	s.HTTP = http.DefaultClient
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM system_settings WHERE key='mcp'`) })
	put := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/mcp", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		recorder := httptest.NewRecorder()
		s.Router().ServeHTTP(recorder, request)
		return recorder
	}
	// The fresh database has no OIDC issuer stored.
	if recorder := put(`{"value":{"oauth":{"enabled":true}}}`); recorder.Code != 400 || !strings.Contains(recorder.Body.String(), "issuer") {
		t.Fatalf("enabling without an issuer answered %d: %s", recorder.Code, recorder.Body.String())
	}
	oidcRaw, _ := json.Marshal(oidcSetting{Enabled: true, Issuer: idp.URL, ClientID: "igame-web"})
	if _, err := pool.Exec(context.Background(), `UPDATE system_settings SET value=$1 WHERE key='oidc'`, oidcRaw); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE system_settings SET value='{"enabled":false,"issuer":"","client_id":"","client_secret":""}' WHERE key='oidc'`)
	})
	s.invalidateSetting("oidc")
	if recorder := put(`{"value":{"oauth":{"enabled":true,"audience":["claude-mcp"],"scopes":["mcp:access","games:read"]}}}`); recorder.Code != 200 {
		t.Fatalf("save answered %d: %s", recorder.Code, recorder.Body.String())
	}
	metadata := httptest.NewRecorder()
	s.Router().ServeHTTP(metadata, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/mcp", nil))
	if metadata.Code != 200 || !strings.Contains(metadata.Body.String(), idp.URL) {
		t.Fatalf("metadata after save answered %d: %s", metadata.Code, metadata.Body.String())
	}
}
