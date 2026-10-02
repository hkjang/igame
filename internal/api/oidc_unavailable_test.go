package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hkjang/igame/internal/secretbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

// "the SSO is not configured" and "the SSO configuration cannot be read right
// now" are different answers to different questions. The first is the state a
// fresh installation is in; the second is a site-wide login outage, either a
// database that cannot be reached or an installation key that no longer opens
// the stored client secret. Reporting the outage as the fresh install told the
// visitor to ask the administrator about a setting that was already there, and
// told the administrator nothing at all — the answer was written without the
// cause.

// oidcSettingServer seeds the settings cache the way the running server fills
// it, so the handlers below read their configuration through the real cache and
// the real setting decoder. The returned buffer receives everything the server
// logs.
func oidcSettingServer(t *testing.T, raw string) (*Server, *bytes.Buffer) {
	t.Helper()
	var logged bytes.Buffer
	s := &Server{Now: time.Now, HTTP: http.DefaultClient, Log: slog.New(slog.NewTextHandler(&logged, nil))}
	entry := settingEntry{raw: []byte(raw), expires: time.Now().Add(time.Hour)}
	if raw == "" {
		entry = settingEntry{missing: true, expires: time.Now().Add(time.Hour)}
	}
	s.storeSetting("oidc", entry)
	// The security-header middleware consults the service setting on every
	// request. Caching it as absent keeps these tests off the database without
	// taking the middleware out of the chain.
	s.storeSetting("service", settingEntry{missing: true, expires: time.Now().Add(time.Hour)})
	return s, &logged
}

// sealedWithAnotherKey returns a client secret sealed with one installation key
// together with a box holding a different one, which is what an installation
// looks like after IGAME_SECRET_KEY was replaced: the stored ciphertext is
// intact and no key in the process can open it. Both boxes are real.
func sealedWithAnotherKey(t *testing.T) (sealed string, opener *secretbox.Box) {
	t.Helper()
	writer, err := secretbox.New(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	opener, err = secretbox.New(bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err = writer.Seal("s3cret-client-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opener.Open(sealed); err == nil {
		t.Fatal("the second key opened a secret sealed with the first; the keys are not distinct")
	}
	return sealed, opener
}

// unreachablePool points at a port nothing listens on, so every query fails the
// way an unreachable database does.
func unreachablePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://igame:igame@127.0.0.1:1/igame")
	if err != nil {
		t.Fatalf("build pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func errorEnvelope(t *testing.T, body []byte) (code, message string) {
	t.Helper()
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode error envelope %q: %v", body, err)
	}
	return envelope.Error.Code, envelope.Error.Message
}

func TestOIDCLoginReportsAnUnreadableSettingAsAnOutage(t *testing.T) {
	sealed, opener := sealedWithAnotherKey(t)
	poisoned, err := json.Marshal(oidcSetting{Enabled: true, Issuer: "https://idp.example", ClientID: "igame", ClientSecret: sealed})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		raw  string
		// brokenDB leaves the setting out of the cache so the handler has to
		// reach the database for it.
		brokenDB bool
	}{
		{name: "the installation key no longer opens the stored client secret", raw: string(poisoned)},
		{name: "the stored setting cannot be decoded", raw: `{"enabled":true,"issuer":`},
		{name: "the database cannot be reached", brokenDB: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, logged := oidcSettingServer(t, tc.raw)
			s.Secrets = opener
			if tc.brokenDB {
				s.invalidateSetting("oidc")
				s.DB = unreachablePool(t)
			}

			recorder := httptest.NewRecorder()
			s.Router().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/login", nil))

			code, message := errorEnvelope(t, recorder.Body.Bytes())
			if recorder.Code != http.StatusServiceUnavailable || code != "oidc_unavailable" {
				t.Fatalf("status %d code %q, want 503 oidc_unavailable", recorder.Code, code)
			}
			if strings.Contains(message, "not configured") {
				t.Fatalf("an unreadable setting was reported as an unconfigured one: %q", message)
			}
			if logged.Len() == 0 {
				t.Fatal("a login outage was answered without logging the cause")
			}
			if secret := "s3cret-client-secret"; strings.Contains(logged.String(), secret) || strings.Contains(recorder.Body.String(), secret) {
				t.Fatal("the client secret escaped into the log or the response")
			}
			if strings.Contains(logged.String(), sealed) || strings.Contains(recorder.Body.String(), sealed) {
				t.Fatal("the sealed client secret escaped into the log or the response")
			}
		})
	}
}

// The callback consumes the state row before it reads the setting, so an
// unreachable database surfaces there first. That is still the same site-wide
// outage, and answering it as an invalid state sent the operator looking at the
// state table and wrote nothing down.
func TestOIDCCallbackReportsAnUnreachableDatabaseAsAnOutage(t *testing.T) {
	s, logged := oidcSettingServer(t, `{"enabled":true,"issuer":"https://idp.example","client_id":"igame"}`)
	s.DB = unreachablePool(t)

	recorder := httptest.NewRecorder()
	s.Router().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/oidc/callback?code=auth-code&state=minted-state", nil))

	code, _ := errorEnvelope(t, recorder.Body.Bytes())
	if recorder.Code != http.StatusServiceUnavailable || code != "oidc_unavailable" {
		t.Fatalf("status %d code %q, want 503 oidc_unavailable", recorder.Code, code)
	}
	if logged.Len() == 0 {
		t.Fatal("a callback outage was answered without logging the cause")
	}
}

func TestOIDCLoginStillReportsAnUnconfiguredProviderAsDisabled(t *testing.T) {
	// These are the answers the portal's own wording depends on, and the first
	// of them is what a freshly migrated installation serves: migrations seed
	// the oidc key with enabled=false. None of them may become an outage.
	cases := []struct {
		name string
		raw  string
	}{
		{"the seeded setting of a fresh installation", `{"enabled":false,"issuer":"","client_id":"","client_secret":""}`},
		{"no row was ever stored", ""},
		{"enabled without an issuer", `{"enabled":true,"issuer":"","client_id":"igame"}`},
		{"enabled without a client id", `{"enabled":true,"issuer":"https://idp.example","client_id":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, logged := oidcSettingServer(t, tc.raw)

			recorder := httptest.NewRecorder()
			s.Router().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/login", nil))

			code, _ := errorEnvelope(t, recorder.Body.Bytes())
			if recorder.Code != http.StatusNotFound || code != "oidc_disabled" {
				t.Fatalf("status %d code %q, want 404 oidc_disabled", recorder.Code, code)
			}
			if logged.Len() != 0 {
				t.Fatalf("an unconfigured provider logged a fault: %s", logged.String())
			}
		})
	}
}
