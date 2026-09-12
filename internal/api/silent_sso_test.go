package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// seedSettings fills the settings cache so handlers that only read settings
// run without a database. A raw value of "" stands for a key never stored.
func seedSettings(t *testing.T, raw map[string]string) *Server {
	t.Helper()
	s := &Server{Now: time.Now}
	for key, value := range raw {
		entry := settingEntry{raw: []byte(value), expires: time.Now().Add(time.Hour)}
		if value == "" {
			entry = settingEntry{missing: true, expires: time.Now().Add(time.Hour)}
		}
		s.storeSetting(key, entry)
	}
	return s
}

// The silent attempt is what sends a signed-out visitor bouncing back to the
// login screen, so where it can happen must be the administrator's decision.
// Anybody can append ?prompt=none to a link; only auto_login makes it count.
func TestSilentLoginNeedsBothTheParameterAndTheSetting(t *testing.T) {
	cases := []struct {
		name      string
		autoLogin bool
		query     string
		want      bool
	}{
		{"parameter without the setting is an ordinary login", false, "?prompt=none", false},
		{"setting without the parameter is an ordinary login", true, "", false},
		{"another prompt value is not silent", true, "?prompt=login", false},
		{"both together are silent", true, "?prompt=none&return_to=%2Fgames", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/login"+tc.query, nil)
			if got := silentLoginRequested(r, oidcSetting{Enabled: true, AutoLogin: tc.autoLogin}); got != tc.want {
				t.Fatalf("silentLoginRequested = %v, want %v", got, tc.want)
			}
		})
	}
}

// The marker in the address is the guard that survives a cleared
// sessionStorage, and the deep link must survive the detour — but only a
// path on this service may ride along.
func TestSilentRefusalLandsOnTheLoginScreenWithTheMarker(t *testing.T) {
	for returnTo, want := range map[string]string{
		"":                           "/login?sso=none",
		"/":                          "/login?sso=none",
		"/games/snake?tab=2":         "/login?sso=none&return_to=%2Fgames%2Fsnake%3Ftab%3D2",
		"//evil.example/portal":      "/login?sso=none",
		"https://evil.example/games": "/login?sso=none",
	} {
		if got := silentRefusalPath(returnTo); got != want {
			t.Fatalf("silentRefusalPath(%q) = %q, want %q", returnTo, got, want)
		}
	}
}

// A refusal without a state names no flow, so it cannot have been silent: the
// answer is the ordinary provider error, and no database is consulted.
func TestCallbackRefusalWithoutStateIsAnOrdinaryError(t *testing.T) {
	s := &Server{}
	recorder := httptest.NewRecorder()
	s.oidcCallback(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?error=login_required", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", recorder.Code)
	}
}

// The portal decides whether to try a silent sign-in from the public config,
// so the flag must be published only when the server would honour it: never
// for a provider that is switched off, and never by default.
func TestPublicConfigPublishesAutoLoginOnlyWithAnEnabledProvider(t *testing.T) {
	for oidc, want := range map[string]bool{
		"":                                    false,
		`{"enabled":true}`:                    false,
		`{"enabled":false,"auto_login":true}`: false,
		`{"enabled":true,"auto_login":true}`:  true,
	} {
		s := seedSettings(t, map[string]string{"oidc": oidc, "service": "", "ai": "", "approval": ""})
		recorder := httptest.NewRecorder()
		s.publicConfig(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/public/config", nil))
		var body struct {
			AutoLogin bool `json:"oidc_auto_login"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.AutoLogin != want {
			t.Fatalf("oidc setting %q published oidc_auto_login=%v, want %v", oidc, body.AutoLogin, want)
		}
	}
}
