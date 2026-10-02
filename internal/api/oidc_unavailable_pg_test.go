package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The callback reads the OIDC setting only after the state row has been
// consumed, so a pool that fails every query never gets that far: it reports
// the outage from the consuming DELETE instead, which
// TestOIDCCallbackReportsAnUnreachableDatabaseAsAnOutage pins without a
// database. Reaching the *setting* leg therefore needs a database that answers
// the DELETE and then hands back a setting that cannot be read — the order a
// live key change or a poisoned row arrives in — so these tests mint a real
// flow row first. They are skipped when IGAME_TEST_DSN is unset; `make test-db
// DSN=...` runs them.

// startCallbackFlow records a flow row the way the login leg does and returns
// the state that names it.
func startCallbackFlow(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	state := "state-" + t.Name()
	hash := sha256.Sum256([]byte(state))
	_, err := pool.Exec(context.Background(),
		`INSERT INTO oidc_flows(state_hash,nonce,code_verifier,return_to,silent,expires_at) VALUES($1,$2,$3,$4,false,now()+interval '10 minutes')`,
		hash[:], "nonce-value", "verifier-value", "/")
	if err != nil {
		t.Fatalf("record flow: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM oidc_flows WHERE state_hash=$1`, hash[:])
	})
	return state
}

func flowRowCount(t *testing.T, pool *pgxpool.Pool, state string) int {
	t.Helper()
	hash := sha256.Sum256([]byte(state))
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM oidc_flows WHERE state_hash=$1`, hash[:]).Scan(&count); err != nil {
		t.Fatalf("count flows: %v", err)
	}
	return count
}

func TestOIDCCallbackReportsAnUnreadableSettingAsAnOutage(t *testing.T) {
	pool := migratedPool(t)
	sealed, opener := sealedWithAnotherKey(t)
	poisoned, err := json.Marshal(oidcSetting{Enabled: true, Issuer: "https://idp.example", ClientID: "igame", ClientSecret: sealed})
	if err != nil {
		t.Fatal(err)
	}
	s, logged := oidcSettingServer(t, string(poisoned))
	s.DB = pool
	s.Secrets = opener
	state := startCallbackFlow(t, pool)

	recorder := httptest.NewRecorder()
	s.Router().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/oidc/callback?code=auth-code&state="+url.QueryEscape(state), nil))

	code, message := errorEnvelope(t, recorder.Body.Bytes())
	if recorder.Code != http.StatusServiceUnavailable || code != "oidc_unavailable" {
		t.Fatalf("status %d code %q, want 503 oidc_unavailable", recorder.Code, code)
	}
	if strings.Contains(message, "not configured") {
		t.Fatalf("an unreadable setting was reported as an unconfigured one: %q", message)
	}
	if logged.Len() == 0 {
		t.Fatal("a callback outage was answered without logging the cause")
	}
	if strings.Contains(logged.String(), sealed) || strings.Contains(logged.String(), "s3cret-client-secret") {
		t.Fatal("the client secret escaped into the log")
	}
	// The state is single use and must stay single use: the row is consumed
	// before the setting is read, and reporting the outage must not put it back.
	if n := flowRowCount(t, pool, state); n != 0 {
		t.Fatalf("%d flow row(s) survived the callback, want the state consumed", n)
	}
}

func TestOIDCCallbackStillReportsAnUnconfiguredProviderAsDisabled(t *testing.T) {
	// The seeded setting of a fresh installation keeps its present answer, which
	// is a different status from the login leg's and is part of the published
	// contract.
	pool := migratedPool(t)
	s, logged := oidcSettingServer(t, `{"enabled":false,"issuer":"","client_id":"","client_secret":""}`)
	s.DB = pool
	state := startCallbackFlow(t, pool)

	recorder := httptest.NewRecorder()
	s.Router().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/oidc/callback?code=auth-code&state="+url.QueryEscape(state), nil))

	code, _ := errorEnvelope(t, recorder.Body.Bytes())
	if recorder.Code != http.StatusBadRequest || code != "oidc_disabled" {
		t.Fatalf("status %d code %q, want 400 oidc_disabled", recorder.Code, code)
	}
	if logged.Len() != 0 {
		t.Fatalf("an unconfigured provider logged a fault: %s", logged.String())
	}
	if n := flowRowCount(t, pool, state); n != 0 {
		t.Fatalf("%d flow row(s) survived the callback, want the state consumed", n)
	}
}

// A setting read that finds no row at all is still an unconfigured provider,
// not an outage — the one database error that keeps its old meaning.
func TestOIDCCallbackTreatsAMissingSettingRowAsDisabled(t *testing.T) {
	pool := migratedPool(t)
	s, _ := oidcSettingServer(t, "")
	s.DB = pool
	state := startCallbackFlow(t, pool)

	recorder := httptest.NewRecorder()
	s.Router().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/oidc/callback?code=auth-code&state="+url.QueryEscape(state), nil))

	code, _ := errorEnvelope(t, recorder.Body.Bytes())
	if recorder.Code != http.StatusBadRequest || code != "oidc_disabled" {
		t.Fatalf("status %d code %q, want 400 oidc_disabled", recorder.Code, code)
	}
}

// An expired or unknown state is consumed by nothing and still answers before
// the setting is ever read, so the outage above cannot mask a replayed state.
func TestOIDCCallbackAnswersAnUnknownStateBeforeReadingTheSetting(t *testing.T) {
	pool := migratedPool(t)
	sealed, opener := sealedWithAnotherKey(t)
	poisoned, err := json.Marshal(oidcSetting{Enabled: true, Issuer: "https://idp.example", ClientID: "igame", ClientSecret: sealed})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := oidcSettingServer(t, string(poisoned))
	s.DB = pool
	s.Secrets = opener

	recorder := httptest.NewRecorder()
	s.Router().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/oidc/callback?code=auth-code&state=never-minted", nil))

	code, _ := errorEnvelope(t, recorder.Body.Bytes())
	if recorder.Code != http.StatusBadRequest || code != "invalid_state" {
		t.Fatalf("status %d code %q, want 400 invalid_state", recorder.Code, code)
	}
}
