package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// What a personal API key may do is decided by three things that live apart
// from the key: the permissions stored on the row, the api_keys policy setting,
// and the role on the owner's user row. authenticateAPIKey reloads the latter
// two on every request so that narrowing the policy or demoting the owner
// takes a key's reach away without rewriting or re-issuing its secret.
//
// The unit tests over effectiveKeyPermissions build a Principal and an
// apiKeyPolicy by hand, so they cannot see whether the server actually wires
// the policy into authentication, whether the settings cache lets a stale
// policy keep answering, or whether the middleware that maps a path to a
// required permission agrees with what the key was granted. These tests drive
// the real router against a real database: the key is created over HTTP, the
// policy is changed over HTTP, and the key is then presented as a bearer token
// exactly as a client would. They are skipped when IGAME_TEST_DSN is unset;
// `make test-db DSN=...` runs them.

type apiKeyFixture struct {
	pool        *pgxpool.Pool
	server      *httptest.Server
	adminCookie string
}

func newAPIKeyFixture(t *testing.T) apiKeyFixture {
	t.Helper()
	pool := migratedPool(t)
	f := apiKeyFixture{pool: pool}
	admin := insertTestUser(t, pool, "admin")
	f.adminCookie = insertTestSession(t, pool, admin)
	f.server = httptest.NewServer(New(pool, nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))).Router())
	t.Cleanup(f.server.Close)
	return f
}

// settings returns the settings helper over the same server, so a policy edit
// in these tests goes through the same PUT handler — and the same cache
// invalidation — an administrator would use.
func (f apiKeyFixture) settings() settingFixture {
	return settingFixture{pool: f.pool, server: f.server, cookie: f.adminCookie}
}

func (f apiKeyFixture) do(t *testing.T, request *http.Request) (int, map[string]any) {
	t.Helper()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(response.Body).Decode(&out); err != nil && err != io.EOF {
		t.Fatalf("%s %s: decode response: %v", request.Method, request.URL.Path, err)
	}
	return response.StatusCode, out
}

// withCookie calls the API as the signed-in owner of the session.
func (f apiKeyFixture) withCookie(t *testing.T, method, path, cookie, body string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}
	request, err := http.NewRequest(method, f.server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	return f.do(t, request)
}

// withKey calls the API the way a script holding only the secret does.
func (f apiKeyFixture) withKey(t *testing.T, method, path, secret string) (int, map[string]any) {
	t.Helper()
	request, err := http.NewRequest(method, f.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	return f.do(t, request)
}

// createKey issues a key over the real endpoint and returns its id and secret.
func (f apiKeyFixture) createKey(t *testing.T, cookie string, permissions ...string) (uuid.UUID, string) {
	t.Helper()
	scopes, err := json.Marshal(permissions)
	if err != nil {
		t.Fatal(err)
	}
	status, out := f.withCookie(t, http.MethodPost, "/api/v1/me/api-keys", cookie, `{"name":"scope regression","permissions":`+string(scopes)+`}`)
	if status != http.StatusCreated {
		t.Fatalf("create key %v: status %d, want 201 (%s)", permissions, status, body(out))
	}
	secret, _ := out["secret"].(string)
	if !strings.HasPrefix(secret, "igk_") {
		t.Fatalf("create key %v: secret %q does not look like a key", permissions, secret)
	}
	key, _ := out["api_key"].(map[string]any)
	raw, _ := key["id"].(string)
	id, err := uuid.Parse(raw)
	if err != nil {
		t.Fatalf("create key %v: id %q: %v", permissions, raw, err)
	}
	t.Cleanup(func() { _, _ = f.pool.Exec(context.Background(), `DELETE FROM api_keys WHERE id=$1`, id) })
	return id, secret
}

func errorCode(out map[string]any) string {
	envelope, _ := out["error"].(map[string]any)
	code, _ := envelope["code"].(string)
	return code
}

// body reports the error envelope a refusal carries, and says so plainly when
// the call was allowed instead. Printing the whole payload would bury the
// result under the seeded game catalogue.
func body(out map[string]any) string {
	if envelope, ok := out["error"].(map[string]any); ok {
		return fmt.Sprintf("error %v", envelope)
	}
	return "no error envelope; the call was allowed"
}

// TestAPIKeyScopeFollowsPolicyWithoutReissuingTheKey pins the claim in
// authenticateAPIKey: removing a permission from the api_keys policy takes it
// away from keys that already exist, and putting it back restores them, with
// the key's own secret and stored permissions untouched throughout.
func TestAPIKeyScopeFollowsPolicyWithoutReissuingTheKey(t *testing.T) {
	f := newAPIKeyFixture(t)
	user := insertTestUser(t, f.pool, "user")
	cookie := insertTestSession(t, f.pool, user)
	id, secret := f.createKey(t, cookie, "profile:read", "games:read")

	if status, out := f.withKey(t, http.MethodGet, "/api/v1/games", secret); status != http.StatusOK {
		t.Fatalf("games with a games:read key: status %d, want 200 (%s)", status, body(out))
	}
	if status, out := f.withKey(t, http.MethodGet, "/api/v1/me", secret); status != http.StatusOK {
		t.Fatalf("me with a profile:read key: status %d, want 200 (%s)", status, body(out))
	}

	policy := f.settings()
	policy.preserve(t, "api_keys")
	before, _, ok := policy.row(t, "api_keys")
	if !ok {
		t.Fatal("api_keys policy is missing; the seed should have created it")
	}
	if status, out := policy.put(t, "api_keys", `{"value":{"available_permissions":["profile:read"],"role_permissions":{"user":["profile:read"]},"max_keys":10,"max_ttl_days":365}}`); status != http.StatusOK {
		t.Fatalf("narrow policy: status %d, want 200 (%s)", status, body(out))
	}

	status, out := f.withKey(t, http.MethodGet, "/api/v1/games", secret)
	if status != http.StatusForbidden || errorCode(out) != "insufficient_scope" {
		t.Fatalf("games after games:read left the policy: status %d code %q, want 403 insufficient_scope (%s)", status, errorCode(out), body(out))
	}
	if status, out := f.withKey(t, http.MethodGet, "/api/v1/me", secret); status != http.StatusOK {
		t.Fatalf("me after an unrelated permission left the policy: status %d, want 200 (%s)", status, body(out))
	}

	// The key itself must not have been touched: the narrowing is a reading of
	// the policy, not a rewrite of the row.
	var stored []string
	if err := f.pool.QueryRow(context.Background(), `SELECT permissions FROM api_keys WHERE id=$1 AND revoked_at IS NULL`, id).Scan(&stored); err != nil {
		t.Fatalf("read stored permissions: %v", err)
	}
	if len(stored) != 2 || stored[0] != "profile:read" || stored[1] != "games:read" {
		t.Fatalf("stored permissions = %v, want the two the key was created with", stored)
	}

	if status, out := policy.put(t, "api_keys", `{"value":`+before+`}`); status != http.StatusOK {
		t.Fatalf("restore policy: status %d, want 200 (%s)", status, body(out))
	}
	if status, out := f.withKey(t, http.MethodGet, "/api/v1/games", secret); status != http.StatusOK {
		t.Fatalf("games after the policy was restored: status %d, want 200 (%s)", status, body(out))
	}
}

// TestAPIKeyLosesAdminReachWhenOwnerIsDemoted pins the other half: an admin's
// key carries admin:* only while the owner's user row still says admin, and
// the demotion is effective on the next request without revoking the key.
func TestAPIKeyLosesAdminReachWhenOwnerIsDemoted(t *testing.T) {
	f := newAPIKeyFixture(t)
	owner := insertTestUser(t, f.pool, "admin")
	cookie := insertTestSession(t, f.pool, owner)
	_, secret := f.createKey(t, cookie, "admin:*", "profile:read")

	if status, out := f.withKey(t, http.MethodGet, "/api/v1/admin/settings", secret); status != http.StatusOK {
		t.Fatalf("admin settings with an admin:* key: status %d, want 200 (%s)", status, body(out))
	}

	if _, err := f.pool.Exec(context.Background(), `UPDATE users SET role='user' WHERE id=$1`, owner); err != nil {
		t.Fatalf("demote owner: %v", err)
	}

	if status, out := f.withKey(t, http.MethodGet, "/api/v1/admin/settings", secret); status != http.StatusForbidden {
		t.Fatalf("admin settings after the owner was demoted: status %d, want 403 (%s)", status, body(out))
	}
	// Demotion narrows the key, it does not disable it.
	if status, out := f.withKey(t, http.MethodGet, "/api/v1/me", secret); status != http.StatusOK {
		t.Fatalf("me after the owner was demoted: status %d, want 200 (%s)", status, body(out))
	}
}

// TestAPIKeyCannotManageAPIKeys pins the refusal that keeps a leaked key from
// minting more of itself: key management needs a session, whatever the key was
// scoped to.
func TestAPIKeyCannotManageAPIKeys(t *testing.T) {
	f := newAPIKeyFixture(t)
	owner := insertTestUser(t, f.pool, "admin")
	cookie := insertTestSession(t, f.pool, owner)
	id, secret := f.createKey(t, cookie, "admin:*", "api:access", "profile:read")

	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/me/api-keys"},
		{http.MethodPost, "/api/v1/me/api-keys"},
		{http.MethodPost, "/api/v1/me/api-keys/" + id.String() + "/rotate"},
		{http.MethodDelete, "/api/v1/me/api-keys/" + id.String()},
	} {
		t.Run(call.method+" "+call.path, func(t *testing.T) {
			before := countAPIKeys(t, f.pool, owner)
			status, out := f.withKey(t, call.method, call.path, secret)
			if status != http.StatusForbidden || errorCode(out) != "forbidden" {
				t.Fatalf("status %d code %q, want 403 forbidden (%s)", status, errorCode(out), body(out))
			}
			if after := countAPIKeys(t, f.pool, owner); after != before {
				t.Fatalf("live key count went from %d to %d; the refusal did not come before the handler", before, after)
			}
		})
	}
}

// TestRotatedAndRevokedAPIKeysStopAuthenticating pins the lifecycle: rotation
// retires the old secret in the same transaction that issues the new one, an
// explicit revoke retires it immediately, and an elapsed expiry retires it
// without anybody doing anything.
func TestRotatedAndRevokedAPIKeysStopAuthenticating(t *testing.T) {
	f := newAPIKeyFixture(t)
	user := insertTestUser(t, f.pool, "user")
	cookie := insertTestSession(t, f.pool, user)

	t.Run("rotation retires the old secret", func(t *testing.T) {
		id, old := f.createKey(t, cookie, "profile:read")
		status, out := f.withCookie(t, http.MethodPost, "/api/v1/me/api-keys/"+id.String()+"/rotate", cookie, "")
		if status != http.StatusCreated {
			t.Fatalf("rotate: status %d, want 201 (%s)", status, body(out))
		}
		fresh, _ := out["secret"].(string)
		if fresh == "" || fresh == old {
			t.Fatalf("rotate returned secret %q, want a new one", fresh)
		}
		if status, out := f.withKey(t, http.MethodGet, "/api/v1/me", old); status != http.StatusUnauthorized {
			t.Fatalf("old secret after rotation: status %d, want 401 (%s)", status, body(out))
		}
		if status, out := f.withKey(t, http.MethodGet, "/api/v1/me", fresh); status != http.StatusOK {
			t.Fatalf("rotated secret: status %d, want 200 (%s)", status, body(out))
		}
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM api_keys WHERE user_id=$1`, user)
	})

	t.Run("revoke retires the secret", func(t *testing.T) {
		id, secret := f.createKey(t, cookie, "profile:read")
		if status, out := f.withCookie(t, http.MethodDelete, "/api/v1/me/api-keys/"+id.String(), cookie, ""); status != http.StatusNoContent {
			t.Fatalf("revoke: status %d, want 204 (%s)", status, body(out))
		}
		if status, out := f.withKey(t, http.MethodGet, "/api/v1/me", secret); status != http.StatusUnauthorized {
			t.Fatalf("revoked secret: status %d, want 401 (%s)", status, body(out))
		}
	})

	t.Run("an elapsed expiry retires the secret", func(t *testing.T) {
		id, secret := f.createKey(t, cookie, "profile:read")
		if status, out := f.withKey(t, http.MethodGet, "/api/v1/me", secret); status != http.StatusOK {
			t.Fatalf("new key: status %d, want 200 (%s)", status, body(out))
		}
		// The API refuses a past expiry on write, so age the row instead.
		if _, err := f.pool.Exec(context.Background(), `UPDATE api_keys SET expires_at=now()-interval '1 minute' WHERE id=$1`, id); err != nil {
			t.Fatalf("age key: %v", err)
		}
		if status, out := f.withKey(t, http.MethodGet, "/api/v1/me", secret); status != http.StatusUnauthorized {
			t.Fatalf("expired secret: status %d, want 401 (%s)", status, body(out))
		}
	})
}

func countAPIKeys(t *testing.T, pool *pgxpool.Pool, user uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM api_keys WHERE user_id=$1 AND revoked_at IS NULL`, user).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
