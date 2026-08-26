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
