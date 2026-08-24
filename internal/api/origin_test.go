package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// serviceSetting seeds the settings cache so these run without a database.
func serviceSetting(t *testing.T, raw string) *Server {
	t.Helper()
	s := &Server{}
	s.storeSetting("service", settingEntry{raw: []byte(raw), expires: time.Now().Add(time.Hour)})
	return s
}

func postFrom(origin, target string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, target, nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

func originVerdict(s *Server, r *http.Request) int {
	w := httptest.NewRecorder()
	s.csrfProtection(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(w, r)
	return w.Code
}

func TestOriginAcceptsBothTheConfiguredAndTheObservedAddress(t *testing.T) {
	// An intranet deployment is reached by its published name and, just as
	// often, by IP or short host name. Refusing the latter locked sign-in out
	// while the page itself still loaded.
	s := serviceSetting(t, `{"public_url":"https://igame.corp.local"}`)

	for _, origin := range []string{"https://igame.corp.local", "http://10.0.0.5:8080"} {
		r := postFrom(origin, "http://10.0.0.5:8080/api/v1/auth/login")
		if got := originVerdict(s, r); got != http.StatusNoContent {
			t.Fatalf("origin %s was refused with %d", origin, got)
		}
	}
}

func TestOriginStillRefusesAnotherSite(t *testing.T) {
	s := serviceSetting(t, `{"public_url":"https://igame.corp.local"}`)
	for _, origin := range []string{
		"https://evil.example",
		"http://igame.corp.local",            // same host, wrong scheme
		"https://igame.corp.local.evil.test", // suffix that merely looks related
		"https://igame.corp.local:8443",      // same host, different port
	} {
		r := postFrom(origin, "https://igame.corp.local/api/v1/auth/login")
		if got := originVerdict(s, r); got != http.StatusForbidden {
			t.Fatalf("origin %s was accepted with %d, want 403", origin, got)
		}
	}
}

func TestOriginFallsBackToTheRequestAddressWithoutAPublicURL(t *testing.T) {
	s := serviceSetting(t, `{"public_url":""}`)
	r := postFrom("http://127.0.0.1:8080", "http://127.0.0.1:8080/api/v1/auth/login")
	if got := originVerdict(s, r); got != http.StatusNoContent {
		t.Fatalf("self origin refused with %d", got)
	}
	r = postFrom("http://somewhere.else", "http://127.0.0.1:8080/api/v1/auth/login")
	if got := originVerdict(s, r); got != http.StatusForbidden {
		t.Fatalf("foreign origin accepted with %d", got)
	}
}

func TestOriginCheckSkipsRequestsABrowserDoesNotOriginate(t *testing.T) {
	s := serviceSetting(t, `{"public_url":"https://igame.corp.local"}`)

	// No Origin header: a script or CLI, which carries no ambient cookie.
	if got := originVerdict(s, postFrom("", "http://10.0.0.5:8080/api/v1/scores")); got != http.StatusNoContent {
		t.Fatalf("originless request refused with %d", got)
	}
	// Bearer credentials are not sent ambiently, so CSRF does not apply.
	r := postFrom("https://evil.example", "http://10.0.0.5:8080/api/v1/scores")
	r.Header.Set("Authorization", "Bearer igk_example")
	if got := originVerdict(s, r); got != http.StatusNoContent {
		t.Fatalf("bearer request refused with %d", got)
	}
	// Safe methods never change state.
	safe := httptest.NewRequest(http.MethodGet, "http://10.0.0.5:8080/api/v1/games", nil)
	safe.Header.Set("Origin", "https://evil.example")
	if got := originVerdict(s, safe); got != http.StatusNoContent {
		t.Fatalf("GET refused with %d", got)
	}
}

func TestObservedAddressHonoursATrustedProxy(t *testing.T) {
	s := serviceSetting(t, `{"public_url":"","trust_proxy":true}`)
	r := postFrom("https://igame.corp", "http://internal:8080/api/v1/auth/login")
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "igame.corp")
	if got := originVerdict(s, r); got != http.StatusNoContent {
		t.Fatalf("proxied origin refused with %d", got)
	}

	// The same headers must carry no weight when the proxy is not trusted.
	untrusted := serviceSetting(t, `{"public_url":"","trust_proxy":false}`)
	if got := originVerdict(untrusted, r); got != http.StatusForbidden {
		t.Fatalf("untrusted forwarded headers were honoured, got %d", got)
	}
}
