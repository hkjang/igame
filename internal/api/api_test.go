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
