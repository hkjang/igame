package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

// A colleague reporting "저장이 안 됩니다" can only be helped if the failure they
// saw can be found in the log. The identifier the log carries is returned on
// every response so there is something to quote.
func TestEveryResponseCarriesTheIdentifierTheLogUses(t *testing.T) {
	server := &Server{}
	handler := middleware.RequestID(server.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if got := recorder.Header().Get("X-Request-Id"); got == "" {
		t.Fatal("no X-Request-Id on the response; a reported failure cannot be found in the log")
	}
}

// A refusal is the audit entry an auditor most wants to find, and there was
// none: an identified account could probe user management, the audit log and
// the OIDC configuration and leave no row and no log line.
func TestAccessDeniedDetailSaysWhoTriedWhatAndWhatWasNeeded(t *testing.T) {
	detail := accessDeniedDetail("PATCH", "/api/v1/admin/users/abc", "operator", "session", []string{"admin"})
	for key, want := range map[string]any{
		"method":    "PATCH",
		"path":      "/api/v1/admin/users/abc",
		"role":      "operator",
		"auth_type": "session",
	} {
		if detail[key] != want {
			t.Fatalf("detail[%q] is %v, want %v", key, detail[key], want)
		}
	}
	roles, ok := detail["required_roles"].([]string)
	if !ok || len(roles) != 1 || roles[0] != "admin" {
		t.Fatalf("required_roles is %v, want [admin]", detail["required_roles"])
	}
}

// Where login sends the reader afterwards is chosen by whoever wrote the link
// they followed. A "//host/path" value has no scheme and a path that starts
// with one slash, so it passed the old check and left the portal: the reader
// finished a real SSO login and landed on someone else's site, which is exactly
// the shape a phishing link wants.
func TestLoginOnlyReturnsToAScreenOnThisService(t *testing.T) {
	for _, value := range []string{
		"//evil.example/portal",
		"///evil.example",
		"https://evil.example/portal",
		"http://user@evil.example/",
		"javascript:alert(1)",
		"mailto:someone@example.com",
	} {
		if got := safeReturnTo(value); got != "/" {
			t.Fatalf("safeReturnTo(%q) is %q; login would leave the portal", value, got)
		}
	}
	for value, want := range map[string]string{
		"":                           "/",
		"/":                          "/",
		"/games/realmguard":          "/games/realmguard",
		"/rankings?season=2#top":     "/rankings?season=2#top",
		"/notices?q=%EA%B3%B5%EC%A7": "/notices?q=%EA%B3%B5%EC%A7",
	} {
		if got := safeReturnTo(value); got != want {
			t.Fatalf("safeReturnTo(%q) is %q, want %q", value, got, want)
		}
	}
}

// The console search boxes are substring filters, but the term used to be
// pasted straight between two wildcards. A single "%" or "_" then matched every
// row, and an operator looking for "50%" or "user_id" got the whole table back
// while the filter chip still said it was applied.
func TestSearchPatternMatchesTheTermAndNotEveryRow(t *testing.T) {
	for term, want := range map[string]string{
		"":         "",
		"admin":    "%admin%",
		"50%":      `%50\%%`,
		"%":        `%\%%`,
		"_":        `%\_%`,
		"user_id":  `%user\_id%`,
		`C:\Users`: `%C:\\Users%`,
		`100\%`:    `%100\\\%%`,
		"공지":       "%공지%",
	} {
		if got := searchPattern(term); got != want {
			t.Fatalf("searchPattern(%q) is %q, want %q", term, got, want)
		}
	}
}
