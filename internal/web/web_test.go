package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// builtDist stands in for the bundle a frontend build writes into dist, which
// the repository only carries as a bare index.html placeholder.
var builtDist = fstest.MapFS{
	"index.html":                       {Data: []byte("<!doctype html><div id=root></div>")},
	"favicon.ico":                      {Data: []byte("icon")},
	"assets/index-DbG3xk91.js":         {Data: []byte("console.log(1)")},
	"assets/chunks/vendor-a1b2c3d4.js": {Data: []byte("console.log(2)")},
}

func TestDirectoriesAreNotServedAsAssets(t *testing.T) {
	index := string(builtDist["index.html"].Data)
	for _, requestPath := range []string{"/assets/", "/assets", "/assets/chunks/", "/assets/chunks"} {
		request := httptest.NewRequest(http.MethodGet, requestPath, nil)
		response := httptest.NewRecorder()
		handlerFor(builtDist, nil).ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d, want 200", requestPath, response.Code)
		}
		if location := response.Header().Get("Location"); location != "" {
			t.Fatalf("%s redirected to %s", requestPath, location)
		}
		if body := response.Body.String(); body != index {
			t.Fatalf("%s did not serve the SPA index: %q", requestPath, body)
		}
		if strings.Contains(response.Body.String(), "DbG3xk91") {
			t.Fatalf("%s listed bundle filenames", requestPath)
		}
	}
}

func TestBundleFilesAreStillServed(t *testing.T) {
	tests := map[string]string{
		"/assets/index-DbG3xk91.js":         "public, max-age=31536000, immutable",
		"/assets/chunks/vendor-a1b2c3d4.js": "public, max-age=31536000, immutable",
		"/favicon.ico":                      "no-cache",
	}
	for requestPath, cacheControl := range tests {
		request := httptest.NewRequest(http.MethodGet, requestPath, nil)
		response := httptest.NewRecorder()
		handlerFor(builtDist, nil).ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d, want 200", requestPath, response.Code)
		}
		if got := response.Header().Get("Cache-Control"); got != cacheControl {
			t.Fatalf("%s Cache-Control = %q, want %q", requestPath, got, cacheControl)
		}
		want := string(builtDist[strings.TrimPrefix(requestPath, "/")].Data)
		if response.Body.String() != want {
			t.Fatalf("%s served %q, want %q", requestPath, response.Body.String(), want)
		}
	}
}

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

func TestIndexRewriterSeesEveryCopyOfTheShell(t *testing.T) {
	rewrite := func(r *http.Request, index []byte) []byte {
		return append(append([]byte{}, index...), []byte("<!--"+r.URL.Path+"-->")...)
	}
	for _, requestPath := range []string{"/", "/index.html", "/games/snake", "/assets/"} {
		request := httptest.NewRequest(http.MethodGet, requestPath, nil)
		response := httptest.NewRecorder()
		handlerFor(builtDist, rewrite).ServeHTTP(response, request)
		if !strings.HasSuffix(response.Body.String(), "<!--"+requestPath+"-->") {
			t.Fatalf("%s served an unrewritten shell: %q", requestPath, response.Body.String())
		}
	}
	// Bundle files are not the shell and must reach the browser byte for byte.
	request := httptest.NewRequest(http.MethodGet, "/assets/index-DbG3xk91.js", nil)
	response := httptest.NewRecorder()
	handlerFor(builtDist, rewrite).ServeHTTP(response, request)
	if response.Body.String() != "console.log(1)" {
		t.Fatalf("asset was rewritten: %q", response.Body.String())
	}
}
