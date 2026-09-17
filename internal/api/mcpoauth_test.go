package api

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeIdP is an issuer with a real key pair: it serves discovery and a JWKS,
// and signs whatever claims a test asks for. What the server does with a
// token is then decided by the same code path a Keycloak token takes.
type fakeIdP struct {
	*httptest.Server
	key *rsa.PrivateKey
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &fakeIdP{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": idp.URL, "authorization_endpoint": idp.URL + "/auth", "token_endpoint": idp.URL + "/token", "jwks_uri": idp.URL + "/keys",
			// Keycloak advertises the HMAC family too; the server must not take
			// that as permission to accept it.
			"id_token_signing_alg_values_supported": []string{"RS256", "HS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "kid": "test", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	idp.Server = httptest.NewServer(mux)
	t.Cleanup(idp.Close)
	return idp
}

// claims are what a Keycloak 26 access token carries, before a test bends
// them: aud holds only "account" and the client id rides in azp.
func (idp *fakeIdP) claims(subject string, override map[string]any) map[string]any {
	now := time.Now()
	claims := map[string]any{
		"iss": idp.URL, "sub": subject, "aud": []string{"account"}, "azp": "claude-mcp", "typ": "Bearer",
		"exp": now.Add(5 * time.Minute).Unix(), "iat": now.Unix(), "nbf": 0, "scope": "openid profile email",
	}
	for key, value := range override {
		if value == nil {
			delete(claims, key)
		} else {
			claims[key] = value
		}
	}
	return claims
}

func (idp *fakeIdP) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	return signJWT(t, map[string]any{"alg": "RS256", "typ": "JWT", "kid": "test"}, claims, func(signing []byte) []byte {
		digest := sha256.Sum256(signing)
		signature, err := rsa.SignPKCS1v15(rand.Reader, idp.key, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		return signature
	})
}

// signHS256 forges a token under a shared secret nobody configured, which is
// the classic downgrade a server that trusts the advertised algorithm list
// falls for.
func (idp *fakeIdP) signHS256(t *testing.T, claims map[string]any) string {
	t.Helper()
	return signJWT(t, map[string]any{"alg": "HS256", "typ": "JWT", "kid": "test"}, claims, func(signing []byte) []byte {
		mac := hmac.New(sha256.New, []byte("guessable"))
		mac.Write(signing)
		return mac.Sum(nil)
	})
}

func signJWT(t *testing.T, header, claims map[string]any, sign func([]byte) []byte) string {
	t.Helper()
	encode := func(value any) string {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	signing := encode(header) + "." + encode(claims)
	return signing + "." + base64.RawURLEncoding.EncodeToString(sign([]byte(signing)))
}

// mcpOAuthServer seeds the settings a server needs to decide about tokens
// without a database: OIDC pointing at the fake issuer, the public URL, and
// the mcp setting as given ("" for never stored).
func mcpOAuthServer(t *testing.T, idp *fakeIdP, mcp string) *Server {
	t.Helper()
	oidcRaw, _ := json.Marshal(oidcSetting{Enabled: true, Issuer: idp.URL, ClientID: "igame-web"})
	s := seedSettings(t, map[string]string{
		"oidc": string(oidcRaw), "mcp": mcp, "api_keys": "", "tracking": "",
		"service": `{"public_url":"https://games.example.test"}`,
	})
	s.HTTP = http.DefaultClient
	return s
}

func rpcCall(t *testing.T, s *Server, token string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	s.Router().ServeHTTP(recorder, request)
	var body struct {
		Error *rpcError `json:"error"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	message := ""
	if body.Error != nil {
		message = body.Error.Message
	}
	return recorder, message
}

// Off is the default, and off must look exactly like before: no metadata
// document, the old challenge, and a token refused the way any unknown
// bearer was — nothing that tells a caller SSO exists here.
func TestMCPOAuthOffLeavesEverythingAsItWas(t *testing.T) {
	idp := newFakeIdP(t)
	for name, mcp := range map[string]string{"never stored": "", "stored off": `{"oauth":{"enabled":false}}`} {
		t.Run(name, func(t *testing.T) {
			s := mcpOAuthServer(t, idp, mcp)
			for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
				recorder := httptest.NewRecorder()
				s.Router().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
				if recorder.Code != 404 {
					t.Fatalf("%s answered %d while SSO is off, want 404", path, recorder.Code)
				}
			}
			recorder, message := rpcCall(t, s, idp.sign(t, idp.claims("subject", nil)))
			if recorder.Code != 401 || message != "authentication required" {
				t.Fatalf("a JWT while off answered %d %q, want the plain 401", recorder.Code, message)
			}
			if got := recorder.Header().Get("WWW-Authenticate"); got != `Bearer realm="igame-mcp", scope="mcp:access"` {
				t.Fatalf("WWW-Authenticate = %q, want the pre-SSO challenge", got)
			}
		})
	}
}

// The switch alone is not enough: without an issuer there is nothing to
// verify against, so the save is refused and a stored value stays inactive.
func TestMCPOAuthNeedsTheIssuer(t *testing.T) {
	s := seedSettings(t, map[string]string{"oidc": "", "mcp": `{"oauth":{"enabled":true}}`, "service": ""})
	if msg := s.validateMCPSetting(t.Context(), mcpSetting{OAuth: mcpOAuthSetting{Enabled: true}}); !strings.Contains(msg, "issuer") {
		t.Fatalf("save without an issuer was accepted: %q", msg)
	}
	cfg, err := s.mcpOAuthConfig(httptest.NewRequest(http.MethodGet, "/mcp", nil))
	if err != nil || cfg.Active() {
		t.Fatalf("enabled without an issuer is active (%v, %q)", err, cfg.Inactive)
	}
}

// The metadata is the document a refused client follows: bare JSON, readable
// from a browser, naming the resource and the authorization server.
func TestProtectedResourceMetadataNamesTheServerAndKeycloak(t *testing.T) {
	idp := newFakeIdP(t)
	s := mcpOAuthServer(t, idp, `{"oauth":{"enabled":true,"audience":["claude-mcp"]}}`)
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		recorder := httptest.NewRecorder()
		s.Router().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != 200 {
			t.Fatalf("%s answered %d: %s", path, recorder.Code, recorder.Body.String())
		}
		if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
		}
		var doc map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		if doc["resource"] != "https://games.example.test/mcp" {
			t.Fatalf("resource = %v, want the public URL plus /mcp", doc["resource"])
		}
		if servers, _ := doc["authorization_servers"].([]any); len(servers) != 1 || servers[0] != idp.URL {
			t.Fatalf("authorization_servers = %v, want [%s]", doc["authorization_servers"], idp.URL)
		}
		if scopes, _ := doc["scopes_supported"].([]any); len(scopes) != len(defaultMCPOAuthScopes) {
			t.Fatalf("scopes_supported = %v, want the default read set", doc["scopes_supported"])
		}
		if _, enveloped := doc["error"]; enveloped || doc["bearer_methods_supported"] == nil {
			t.Fatalf("metadata is not the bare RFC 9728 document: %s", recorder.Body.String())
		}
	}
	// An explicit resource wins over the derived one, and the challenge
	// follows it.
	s = mcpOAuthServer(t, idp, `{"oauth":{"enabled":true,"resource":"https://mcp.example.test/mcp"}}`)
	recorder := httptest.NewRecorder()
	s.Router().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/mcp", nil))
	if !strings.Contains(recorder.Body.String(), `"resource":"https://mcp.example.test/mcp"`) {
		t.Fatalf("configured resource not advertised: %s", recorder.Body.String())
	}
	recorder, _ = rpcCall(t, s, "")
	if got := recorder.Header().Get("WWW-Authenticate"); !strings.Contains(got, `resource_metadata="https://mcp.example.test/.well-known/oauth-protected-resource/mcp"`) {
		t.Fatalf("challenge does not follow the configured resource: %q", got)
	}
}

// A 401 on /mcp points at the metadata; a 401 on REST does not, because a
// browser or a REST client sent there would go somewhere it cannot use.
func TestMCPChallengePointsAtMetadataOnlyOnMCP(t *testing.T) {
	idp := newFakeIdP(t)
	s := mcpOAuthServer(t, idp, `{"oauth":{"enabled":true}}`)
	recorder, message := rpcCall(t, s, "")
	if recorder.Code != 401 || message != "authentication required" {
		t.Fatalf("anonymous /mcp answered %d %q", recorder.Code, message)
	}
	got := recorder.Header().Get("WWW-Authenticate")
	if want := `Bearer realm="igame-mcp", resource_metadata="https://games.example.test/.well-known/oauth-protected-resource/mcp"`; got != want {
		t.Fatalf("WWW-Authenticate = %q, want %q", got, want)
	}
	rest := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.Header.Set("Authorization", "Bearer "+idp.sign(t, idp.claims("subject", map[string]any{"aud": "https://games.example.test/mcp"})))
	s.Router().ServeHTTP(rest, request)
	if rest.Code != 401 || rest.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("REST with a valid SSO token answered %d with WWW-Authenticate %q; tokens belong to /mcp only", rest.Code, rest.Header().Get("WWW-Authenticate"))
	}
}

// Every way a token can be wrong is refused before any account is looked up,
// and the audience refusal says what was seen and what to write down.
func TestMCPOAuthRefusesTokensItCannotTrust(t *testing.T) {
	idp := newFakeIdP(t)
	other := newFakeIdP(t)
	s := mcpOAuthServer(t, idp, `{"oauth":{"enabled":true}}`)
	cases := []struct {
		name, token, want string
	}{
		{"for another application", idp.sign(t, idp.claims("subject", nil)), `aud=[account], azp="claude-mcp"`},
		{"expired", idp.sign(t, idp.claims("subject", map[string]any{"exp": time.Now().Add(-time.Minute).Unix()})), "not valid"},
		{"not yet valid", idp.sign(t, idp.claims("subject", map[string]any{"nbf": time.Now().Add(time.Hour).Unix()})), "not valid"},
		{"other issuer", other.sign(t, other.claims("subject", map[string]any{"iss": other.URL})), "not valid"},
		{"forged issuer", other.sign(t, other.claims("subject", map[string]any{"iss": idp.URL})), "not valid"},
		{"HS256", idp.signHS256(t, idp.claims("subject", nil)), "not valid"},
		{"ID token", idp.sign(t, idp.claims("subject", map[string]any{"typ": "ID", "aud": "igame-web"})), "ID token"},
		{"sender-constrained", idp.sign(t, idp.claims("subject", map[string]any{"cnf": map[string]any{"jkt": "x"}, "aud": "https://games.example.test/mcp"})), "cnf"},
		{"no subject", idp.sign(t, idp.claims("", map[string]any{"aud": "https://games.example.test/mcp"})), "subject"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder, message := rpcCall(t, s, tc.token)
			if recorder.Code != 401 {
				t.Fatalf("answered %d: %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(message, tc.want) {
				t.Fatalf("message %q does not say %q", message, tc.want)
			}
			if got := recorder.Header().Get("WWW-Authenticate"); !strings.Contains(got, `error="invalid_token"`) || !strings.Contains(got, "resource_metadata=") {
				t.Fatalf("WWW-Authenticate = %q, want invalid_token beside the metadata", got)
			}
		})
	}
	// The audience refusal is the one an operator finishes the setup from.
	_, message := rpcCall(t, s, idp.sign(t, idp.claims("subject", nil)))
	for _, want := range []string{`add "claude-mcp" to mcp.oauth.audience`, `Audience mapper for "https://games.example.test/mcp"`} {
		if !strings.Contains(message, want) {
			t.Fatalf("audience refusal %q does not tell the operator %q", message, want)
		}
	}
}

// A key is a key whatever the setting says; a bearer that is neither is still
// the old refusal, and an igk_ value that merely looks like a JWT is a key.
func TestMCPOAuthLeavesKeysAlone(t *testing.T) {
	idp := newFakeIdP(t)
	s := mcpOAuthServer(t, idp, `{"oauth":{"enabled":true}}`)
	cfg, _ := s.mcpOAuthConfig(httptest.NewRequest(http.MethodGet, "/mcp", nil))
	for _, bearer := range []string{"igk_abc.def.ghi", "not-a-token", "a.b", "a..c"} {
		request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		request.Header.Set("Authorization", "Bearer "+bearer)
		if token, _ := bearerToken(request); strings.HasPrefix(token, "igk_") || !looksLikeJWT(token) {
			continue
		}
		t.Fatalf("%q would be verified as an SSO token", bearer)
	}
	if !cfg.Active() {
		t.Fatalf("configuration inactive: %s", cfg.Inactive)
	}
}

// The token's own scope narrows the administrator's list only when it speaks
// this service's vocabulary; a Keycloak default scope leaves it alone.
func TestMCPOAuthTokenScopesNarrowOnlyInThisVocabulary(t *testing.T) {
	configured := []string{"mcp:access", "games:read", "rankings:read"}
	if got := tokenScopes(configured, "openid profile email"); len(got) != 3 {
		t.Fatalf("Keycloak's default scope narrowed the list to %v", got)
	}
	if got := tokenScopes(configured, "openid mcp:access games:read"); len(got) != 2 || got[1] != "games:read" {
		t.Fatalf("scope in this vocabulary did not narrow: %v", got)
	}
	if got := tokenScopes(configured, "openid scores:write"); len(got) != 0 {
		t.Fatalf("a scope outside the list widened it: %v", got)
	}
}

// An administrator's session may do anything; an administrator's token may
// do only what it was scoped to.
func TestAdminBypassIsForSessionsOnly(t *testing.T) {
	admin := Principal{Role: "admin", AuthType: "oauth", Permissions: []string{"games:read"}}
	if admin.Can("admin:*") || admin.Can("scores:write") {
		t.Fatal("an admin's SSO token was granted more than its scopes")
	}
	if !(Principal{Role: "admin", AuthType: "session"}).Can("admin:*") {
		t.Fatal("an admin's session lost its role")
	}
}

// The setting is checked on save like every other one.
func TestMCPSettingIsValidatedOnSave(t *testing.T) {
	cases := map[string]string{
		`{"oauth":{"enabled":false}}`:                                               "",
		`{"oauth":{"enabled":true,"resource":"games.example.test/mcp"}}`:            "absolute",
		`{"oauth":{"enabled":true,"audience":["claude mcp"]}}`:                      "single words",
		`{"oauth":{"enabled":true,"scopes":["admin:*"]}}`:                           "unknown MCP OAuth scope",
		`{"oauth":{"enabled":true,"scopes":["mcp:access","games:read"]}}`:           "",
		`{"oauth":{"enabled":true,"resource":"https://games.example.test/mcp"}}`:    "",
		`{"oauth":{"enabled":true,"audience":["claude-mcp","cursor"],"scopes":[]}}`: "",
	}
	for raw, want := range cases {
		got := validateSetting("mcp", []byte(raw))
		if (want == "" && got != "") || (want != "" && !strings.Contains(got, want)) {
			t.Fatalf("validateSetting(%s) = %q, want %q", raw, got, want)
		}
	}
}
