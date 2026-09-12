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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// fakeIssuer serves the discovery document the login leg needs and nothing
// else: these tests stop at the redirect to the provider, and the callback
// they exercise is the refusal, which never reaches the token endpoint.
func fakeIssuer(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 srv.URL,
			"authorization_endpoint": srv.URL + "/auth",
			"token_endpoint":         srv.URL + "/token",
			"jwks_uri":               srv.URL + "/keys",
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// silentSSOServer wires a migrated database to an OIDC setting that points at
// the fake issuer. The setting is seeded into the cache, so no secret needs to
// be sealed and nothing is written to system_settings.
func silentSSOServer(t *testing.T, pool *pgxpool.Pool, issuer string, autoLogin bool) *Server {
	t.Helper()
	setting, _ := json.Marshal(oidcSetting{Enabled: true, Issuer: issuer, ClientID: "igame", AutoLogin: autoLogin})
	s := &Server{DB: pool, HTTP: http.DefaultClient, Now: time.Now}
	s.storeSetting("oidc", settingEntry{raw: setting, expires: time.Now().Add(time.Hour)})
	s.storeSetting("service", settingEntry{missing: true, expires: time.Now().Add(time.Hour)})
	return s
}

// startLogin runs the login leg and returns the provider URL it redirected to
// and the state it minted.
func startLogin(t *testing.T, s *Server, query string) (*url.URL, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	s.oidcLogin(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/login"+query, nil))
	if recorder.Code != http.StatusFound {
		t.Fatalf("login leg answered %d: %s", recorder.Code, recorder.Body.String())
	}
	location, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return location, location.Query().Get("state")
}

func flowIsSilent(t *testing.T, pool *pgxpool.Pool, state string) (silent, found bool) {
	t.Helper()
	hash := sha256.Sum256([]byte(state))
	err := pool.QueryRow(context.Background(), `SELECT silent FROM oidc_flows WHERE state_hash=$1`, hash[:]).Scan(&silent)
	if err != nil {
		return false, false
	}
	return silent, true
}

func TestSilentLoginIsDowngradedWhenAutoLoginIsOff(t *testing.T) {
	// The default installation must not change: ?prompt=none on the address is
	// ignored, the provider gets an ordinary request and the flow is recorded
	// as ordinary, so a later login_required is the error it always was.
	pool := migratedPool(t)
	s := silentSSOServer(t, pool, fakeIssuer(t).URL, false)

	location, state := startLogin(t, s, "?prompt=none&return_to=%2Fgames")
	t.Cleanup(func() {
		hash := sha256.Sum256([]byte(state))
		_, _ = pool.Exec(context.Background(), `DELETE FROM oidc_flows WHERE state_hash=$1`, hash[:])
	})
	if got := location.Query().Get("prompt"); got != "" {
		t.Fatalf("prompt=%q reached the provider with auto_login off", got)
	}
	if silent, found := flowIsSilent(t, pool, state); !found || silent {
		t.Fatalf("flow silent=%v found=%v, want an ordinary flow", silent, found)
	}

	recorder := httptest.NewRecorder()
	s.oidcCallback(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?error=login_required&state="+url.QueryEscape(state), nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("refusal of an ordinary flow answered %d, want 401", recorder.Code)
	}
}

func TestSilentLoginAsksForPromptNoneAndLandsRefusalsOnTheLoginScreen(t *testing.T) {
	pool := migratedPool(t)
	s := silentSSOServer(t, pool, fakeIssuer(t).URL, true)

	location, state := startLogin(t, s, "?prompt=none&return_to=%2Fgames%2Fsnake")
	t.Cleanup(func() {
		hash := sha256.Sum256([]byte(state))
		_, _ = pool.Exec(context.Background(), `DELETE FROM oidc_flows WHERE state_hash=$1`, hash[:])
	})
	if got := location.Query().Get("prompt"); got != "none" {
		t.Fatalf("prompt=%q reached the provider, want none", got)
	}
	if silent, found := flowIsSilent(t, pool, state); !found || !silent {
		t.Fatalf("flow silent=%v found=%v, want a silent flow", silent, found)
	}

	// No provider session: the ordinary answer for a signed-out visitor. The
	// browser must land on the login screen carrying the do-not-retry marker
	// and the deep link it was headed for.
	recorder := httptest.NewRecorder()
	s.oidcCallback(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?error=login_required&state="+url.QueryEscape(state), nil))
	if recorder.Code != http.StatusFound {
		t.Fatalf("silent refusal answered %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Location"); got != "/login?sso=none&return_to=%2Fgames%2Fsnake" {
		t.Fatalf("silent refusal redirected to %q", got)
	}
	if _, found := flowIsSilent(t, pool, state); found {
		t.Fatal("the refused flow was left behind and could be replayed")
	}

	// Replaying the refusal names a flow that no longer exists, so it cannot
	// steer anyone into the silent landing.
	recorder = httptest.NewRecorder()
	s.oidcCallback(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?error=login_required&state="+url.QueryEscape(state), nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("replayed refusal answered %d, want 401", recorder.Code)
	}
}

func TestOrdinaryLoginStaysOrdinaryWithAutoLoginOn(t *testing.T) {
	// auto_login only changes what happens when the portal asks for a silent
	// attempt. The "사내 SSO로 계속" button still shows the provider's screen.
	pool := migratedPool(t)
	s := silentSSOServer(t, pool, fakeIssuer(t).URL, true)

	location, state := startLogin(t, s, "")
	t.Cleanup(func() {
		hash := sha256.Sum256([]byte(state))
		_, _ = pool.Exec(context.Background(), `DELETE FROM oidc_flows WHERE state_hash=$1`, hash[:])
	})
	if strings.Contains(location.RawQuery, "prompt=") {
		t.Fatalf("an ordinary login carried a prompt: %s", location.RawQuery)
	}
	if silent, found := flowIsSilent(t, pool, state); !found || silent {
		t.Fatalf("flow silent=%v found=%v, want an ordinary flow", silent, found)
	}
}
