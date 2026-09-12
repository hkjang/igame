package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hkjang/igame/internal/tracking"
)

// trackingServer seeds the settings cache so the router runs without a
// database. A raw value of "" stands for a key that was never stored.
func trackingServer(t *testing.T, trackingRaw string) *Server {
	t.Helper()
	s := New(nil, nil, nil)
	for key, raw := range map[string]string{"tracking": trackingRaw, "service": ""} {
		entry := settingEntry{raw: []byte(raw), expires: time.Now().Add(time.Hour)}
		if raw == "" {
			entry = settingEntry{missing: true, expires: time.Now().Add(time.Hour)}
		}
		s.storeSetting(key, entry)
	}
	return s
}

func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// strictPolicy is the header every page carried before tracking existed. A
// deployment that never turned tracking on must keep sending exactly this.
const strictPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'self'; form-action 'self'"

var noncePattern = regexp.MustCompile(`'nonce-([A-Za-z0-9_-]+)'`)

const momentoSetting = `{"enabled":true,"provider":"momento","momento_url":"https://momento.corp.example","momento_site_id":"igame"}`

func TestTrackingOffLeavesEveryPageAsItWas(t *testing.T) {
	for name, raw := range map[string]string{"never stored": "", "stored off": `{"enabled":false,"provider":"momento","momento_url":"https://m.example","momento_site_id":"1"}`, "unreadable": `{"enabled":`} {
		t.Run(name, func(t *testing.T) {
			router := trackingServer(t, raw).Router()
			for _, path := range []string{"/", "/games/snake", "/admin/settings"} {
				w := get(t, router, path)
				if w.Code != 200 {
					t.Fatalf("%s returned %d", path, w.Code)
				}
				if got := w.Header().Get("Content-Security-Policy"); got != strictPolicy {
					t.Fatalf("%s policy changed with tracking off:\n%s", path, got)
				}
				if body := w.Body.String(); strings.Contains(body, "tracker.js") || strings.Contains(body, "nonce=") {
					t.Fatalf("%s carries a snippet with tracking off: %s", path, body)
				}
			}
			if w := get(t, router, "/momento/tracker.js"); w.Code != http.StatusNotFound {
				t.Fatalf("proxy answered %d with tracking off", w.Code)
			}
		})
	}
}

func TestTrackingOnPutsTheSameNonceInThePolicyAndTheSnippet(t *testing.T) {
	router := trackingServer(t, momentoSetting).Router()
	w := get(t, router, "/games/snake")
	policy := w.Header().Get("Content-Security-Policy")
	match := noncePattern.FindStringSubmatch(policy)
	if match == nil {
		t.Fatalf("policy carries no nonce: %s", policy)
	}
	nonce := match[1]
	body := w.Body.String()
	head := body[:strings.Index(body, "</head>")]
	if !strings.Contains(head, `<script nonce="`+nonce+`" async src="/momento/tracker.js"`) {
		t.Fatalf("head lacks the snippet with the policy's nonce %s:\n%s", nonce, body)
	}
	if !strings.Contains(body, `data-endpoint="/momento"`) || !strings.Contains(body, `data-site-id="igame"`) {
		t.Fatalf("snippet is not the proxied Momento loader: %s", body)
	}
	if strings.Contains(policy, "momento.corp.example") {
		t.Fatalf("proxied Momento put an external origin in the policy: %s", policy)
	}
	// style-src carried 'unsafe-inline' before tracking existed; script-src
	// must never gain it.
	if scriptSrc := strings.SplitN(policy, "; style-src", 2)[0]; strings.Contains(scriptSrc, "unsafe-inline") {
		t.Fatalf("script-src was loosened: %s", policy)
	}
	if !strings.HasSuffix(policy, "; report-uri "+cspReportPath) {
		t.Fatalf("policy does not ask for reports: %s", policy)
	}
	// Every page gets a nonce of its own.
	if again := noncePattern.FindStringSubmatch(get(t, router, "/").Header().Get("Content-Security-Policy")); again == nil || again[1] == nonce {
		t.Fatalf("nonce reused across requests: %v", again)
	}
}

func TestTrackingPolicyAddsTheSnippetsOrigins(t *testing.T) {
	raw := `{"enabled":true,"provider":"custom","custom_snippet":"<script src=\"https://tracker.corp.example/t.js\"></script>","allowed_hosts":"https://pixel.corp.example","placement":"body"}`
	w := get(t, trackingServer(t, raw).Router(), "/")
	policy := w.Header().Get("Content-Security-Policy")
	for _, directive := range []string{"script-src 'self' 'nonce-", "connect-src 'self' https://tracker.corp.example https://pixel.corp.example", "img-src 'self' data: blob: https://tracker.corp.example https://pixel.corp.example"} {
		if !strings.Contains(policy, directive) {
			t.Fatalf("policy lacks %q: %s", directive, policy)
		}
	}
	if !strings.Contains(policy, "https://tracker.corp.example https://pixel.corp.example; style-src") {
		t.Fatalf("script-src lacks the snippet origins: %s", policy)
	}
	body := w.Body.String()
	bodyIndex, snippetIndex := strings.Index(body, "</body>"), strings.Index(body, "tracker.corp.example")
	if snippetIndex < 0 || snippetIndex > bodyIndex || snippetIndex < strings.Index(body, "</head>") {
		t.Fatalf("placement=body did not put the snippet at the end of the body:\n%s", body)
	}
}

func TestTrackingSkipsAdminScreensUnlessAsked(t *testing.T) {
	router := trackingServer(t, momentoSetting).Router()
	w := get(t, router, "/admin/settings")
	if got := w.Header().Get("Content-Security-Policy"); got != strictPolicy {
		t.Fatalf("admin screen policy widened without include_admin: %s", got)
	}
	if strings.Contains(w.Body.String(), "tracker.js") {
		t.Fatal("admin screen carries the snippet without include_admin")
	}
	including := trackingServer(t, strings.TrimSuffix(momentoSetting, "}")+`,"include_admin":true}`).Router()
	if w := get(t, including, "/admin/settings"); !strings.Contains(w.Body.String(), "tracker.js") {
		t.Fatal("include_admin did not track the admin screen")
	}
}

func TestNonPagePathsGetANarrowPolicyAndNoSnippet(t *testing.T) {
	router := trackingServer(t, momentoSetting).Router()
	for _, path := range []string{"/api/v1/version", "/api/v1/missing", "/healthz", "/mcp", "/momento/tracker.js"} {
		w := get(t, router, path)
		if got := w.Header().Get("Content-Security-Policy"); got != nonPagePolicy {
			t.Fatalf("%s policy = %q", path, got)
		}
		if strings.Contains(w.Body.String(), "tracker.js") && path != "/momento/tracker.js" {
			t.Fatalf("%s carries the snippet", path)
		}
	}
}

func TestCSPReportsAreRecordedAndShownToAdmins(t *testing.T) {
	s := trackingServer(t, momentoSetting)
	router := s.Router()
	body := `{"csp-report":{"blocked-uri":"https://pixel.corp.example/p.gif","effective-directive":"img-src","document-uri":"https://igame.corp.example/games/snake"}}`
	for i := 0; i < 3; i++ {
		r := httptest.NewRequest(http.MethodPost, cspReportPath, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/csp-report")
		// Browsers decide the Origin of a report themselves; a foreign or absent
		// one must not turn the report into a CSRF rejection.
		r.Header.Set("Origin", "null")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Fatalf("report answered %d: %s", w.Code, w.Body.String())
		}
	}
	broken := httptest.NewRecorder()
	router.ServeHTTP(broken, httptest.NewRequest(http.MethodPost, cspReportPath, strings.NewReader("not json")))
	if broken.Code != http.StatusNoContent {
		t.Fatalf("malformed report answered %d", broken.Code)
	}
	items := s.violations.List(s.trackingConfig(httptest.NewRequest(http.MethodGet, "/", nil).Context()))
	if len(items) != 1 || items[0].Origin != "https://pixel.corp.example" || items[0].Directive != "img-src" || items[0].Count != 3 || items[0].Allowed {
		t.Fatalf("violations = %+v", items)
	}
	// The listing is an admin call; the report endpoint itself needs nobody.
	if w := get(t, router, "/api/v1/admin/tracking/violations"); w.Code != http.StatusUnauthorized {
		t.Fatalf("violation list without a session answered %d", w.Code)
	}
	// An administrator sees the list, and an operator does not: the 404 of an
	// unregistered route and the 401 of a missing session look alike from the
	// outside, so the handler is reached with a principal in hand.
	asRole := func(method, path, role string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r = r.WithContext(context.WithValue(r.Context(), principalKey, Principal{UserID: uuid.New(), Role: role, AuthType: "session"}))
		w := httptest.NewRecorder()
		s.listTrackingViolations(w, r)
		return w
	}
	w := asRole(http.MethodGet, "/api/v1/admin/tracking/violations", "admin")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"origin":"https://pixel.corp.example"`) {
		t.Fatalf("admin listing = %d %s", w.Code, w.Body.String())
	}
	registered := map[string]bool{}
	_ = chi.Walk(router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		registered[method+" "+route] = true
		return nil
	})
	for _, want := range []string{"GET /api/v1/admin/tracking/violations", "DELETE /api/v1/admin/tracking/violations", "POST /api/v1/admin/tracking/allow", "POST " + cspReportPath, "GET /momento/*", "POST /momento/*"} {
		if !registered[want] {
			t.Fatalf("%s is not registered", want)
		}
	}
}

func TestMomentoProxyForwardsWithoutCredentials(t *testing.T) {
	var seen *http.Request
	var seenBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Clone(r.Context())
		b := make([]byte, 1024)
		n, _ := r.Body.Read(b)
		seenBody = string(b[:n])
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte("tracker()"))
	}))
	defer upstream.Close()
	raw := `{"enabled":true,"provider":"momento","momento_url":"` + upstream.URL + `/collector/","momento_site_id":"igame"}`
	router := trackingServer(t, raw).Router()

	r := httptest.NewRequest(http.MethodPost, "/momento/collect/v1/events?x=1", strings.NewReader(`{"event":"page_view"}`))
	r.Header.Set("Cookie", sessionCookie+"=secret")
	r.Header.Set("Authorization", "Bearer igk_secret")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 200 || w.Body.String() != "tracker()" {
		t.Fatalf("proxy answered %d %q", w.Code, w.Body.String())
	}
	if seen == nil {
		t.Fatal("upstream was not called")
	}
	if seen.URL.Path != "/collector/collect/v1/events" || seen.URL.RawQuery != "x=1" {
		t.Fatalf("upstream saw %s?%s", seen.URL.Path, seen.URL.RawQuery)
	}
	if seen.Header.Get("Cookie") != "" || seen.Header.Get("Authorization") != "" {
		t.Fatalf("credentials were forwarded: cookie=%q authorization=%q", seen.Header.Get("Cookie"), seen.Header.Get("Authorization"))
	}
	if seenBody != `{"event":"page_view"}` {
		t.Fatalf("body was not forwarded: %q", seenBody)
	}
	if got := w.Header().Get("Content-Security-Policy"); got != nonPagePolicy {
		t.Fatalf("proxied response policy = %q", got)
	}

	// The proxy is only open for the proxied Momento setup.
	direct := trackingServer(t, `{"enabled":true,"provider":"momento","momento_url":"`+upstream.URL+`","momento_site_id":"igame","momento_proxy":false}`).Router()
	if w := get(t, direct, "/momento/tracker.js"); w.Code != http.StatusNotFound {
		t.Fatalf("direct Momento left the proxy open: %d", w.Code)
	}
	ga := trackingServer(t, `{"enabled":true,"provider":"ga4","measurement_id":"G-1"}`).Router()
	if w := get(t, ga, "/momento/tracker.js"); w.Code != http.StatusNotFound {
		t.Fatalf("GA4 left the proxy open: %d", w.Code)
	}
}

func TestTrackingSettingIsValidatedOnSave(t *testing.T) {
	if problem := validateSetting("tracking", []byte(momentoSetting)); problem != "" {
		t.Fatalf("valid setting refused: %s", problem)
	}
	if problem := validateSetting("tracking", []byte(`{"enabled":false}`)); problem != "" {
		t.Fatalf("empty setting refused: %s", problem)
	}
	big := `{"enabled":true,"provider":"custom","custom_snippet":"<script>` + strings.Repeat("x", tracking.MaxSnippetBytes) + `</script>"}`
	if problem := validateSetting("tracking", []byte(big)); !strings.Contains(problem, "8192") {
		t.Fatalf("oversized snippet accepted: %q", problem)
	}
	if problem := validateSetting("tracking", []byte(`{"enabled":true,"provider":"momento"}`)); problem == "" {
		t.Fatal("momento without an address accepted")
	}
	if problem := validateSetting("tracking", []byte(`{"enabled":true,"provider":"none","allowed_hosts":"https://a.example; script-src *"}`)); problem == "" {
		t.Fatal("allow list entry that would end a directive accepted")
	}
	if !editableSettings["tracking"] {
		t.Fatal("tracking is not an editable setting")
	}
}
