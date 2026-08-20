// Package web serves the React SPA embedded in the igame binary.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// dist is populated by the frontend build. The placeholder keeps local Go
// builds valid before Node assets are built.
//
//go:embed dist
var assets embed.FS

func Handler() http.Handler {
	root, _ := fs.Sub(assets, "dist")
	index, _ := fs.ReadFile(root, "index.html")
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/mcp" || strings.HasPrefix(r.URL.Path, "/mcp/") || strings.HasPrefix(r.URL.Path, "/.well-known/") {
			http.NotFound(w, r)
			return
		}
		if clean == "." || clean == "index.html" {
			serveIndex(w, r, index)
			return
		}
		if f, err := root.Open(clean); err == nil {
			_ = f.Close()
			files.ServeHTTP(w, r)
			return
		}
		// Missing assets must fail loudly; only extensionless browser routes are
		// eligible for the SPA history fallback.
		if path.Ext(clean) != "" {
			http.NotFound(w, r)
			return
		}
		serveIndex(w, r, index)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(index)
	}
}
