package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSPAFallbackDoesNotRedirect(t *testing.T) {
	for _, path := range []string{"/", "/games/snake", "/admin/settings", "/index.html"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d", path, response.Code)
		}
		if response.Header().Get("Location") != "" {
			t.Fatalf("%s redirected to %s", path, response.Header().Get("Location"))
		}
		if response.Body.Len() == 0 {
			t.Fatalf("%s returned empty body", path)
		}
	}
}

func TestSPAFallbackDoesNotMaskMissingAPIsOrAssets(t *testing.T) {
	for _, requestPath := range []string{"/api/v1/missing", "/mcp/missing", "/.well-known/missing", "/assets/missing.js", "/missing.css"} {
		request := httptest.NewRequest(http.MethodGet, requestPath, nil)
		response := httptest.NewRecorder()
		Handler().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s returned %d, want 404", requestPath, response.Code)
		}
	}
}
