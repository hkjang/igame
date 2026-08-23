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

func TestCacheControlOnlyPinsHashedBundleAssets(t *testing.T) {
	immutable := "public, max-age=31536000, immutable"
	tests := map[string]string{
		"assets/index-DbG3xk91.js":  immutable,
		"assets/theme-a1b2c3d4.css": immutable,
		"assets/logo.svg":           "no-cache",
		"assets/index.js":           "no-cache",
		"favicon.ico":               "no-cache",
		"licenses/phaser":           "no-cache",
	}
	for name, want := range tests {
		if got := cacheControlFor(name); got != want {
			t.Fatalf("cacheControlFor(%q)=%q, want %q", name, got, want)
		}
	}
}
