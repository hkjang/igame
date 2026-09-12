// Package web serves the React SPA embedded in the igame binary.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

// dist is populated by the frontend build. The placeholder keeps local Go
// builds valid before Node assets are built.
//
//go:embed dist
var assets embed.FS

// IndexRewriter adjusts the SPA shell for one request before it is sent. The
// tracking snippet is inserted this way: the shell is small and served with
// no-store, so rewriting it per request costs nothing worth caching, and the
// per-request nonce it carries could not be cached anyway.
type IndexRewriter func(r *http.Request, index []byte) []byte

func Handler() http.Handler {
	return HandlerWith(nil)
}

// HandlerWith serves the SPA with rewrite applied to every copy of the shell.
// A nil rewrite serves the shell as built.
func HandlerWith(rewrite IndexRewriter) http.Handler {
	root, _ := fs.Sub(assets, "dist")
	return handlerFor(root, rewrite)
}

func handlerFor(root fs.FS, rewrite IndexRewriter) http.Handler {
	built, _ := fs.ReadFile(root, "index.html")
	shell := func(r *http.Request) []byte {
		if rewrite == nil {
			return built
		}
		return rewrite(r, built)
	}
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
			serveIndex(w, r, shell(r))
			return
		}
		// Only regular files are assets. A directory would make http.FileServer
		// answer /assets/ with an index of the whole bundle, and redirect /assets
		// to a trailing slash; both are handled by the rules below instead.
		if f, err := root.Open(clean); err == nil {
			info, statErr := f.Stat()
			_ = f.Close()
			if statErr == nil && !info.IsDir() {
				w.Header().Set("Cache-Control", cacheControlFor(clean))
				files.ServeHTTP(w, r)
				return
			}
		}
		// Missing assets must fail loudly; only extensionless browser routes are
		// eligible for the SPA history fallback.
		if path.Ext(clean) != "" {
			http.NotFound(w, r)
			return
		}
		serveIndex(w, r, shell(r))
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

// cacheControlFor lets browsers keep content-hashed bundle assets indefinitely.
// Everything else is revalidated so an upgraded image never serves stale files
// from a workstation that cannot reach an external cache buster.
func cacheControlFor(name string) string {
	if strings.HasPrefix(name, "assets/") && hashedAsset.MatchString(path.Base(name)) {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}

// Vite emits <name>-<hash>.<ext>; the hash changes whenever the content does.
var hashedAsset = regexp.MustCompile(`-[A-Za-z0-9_]{8,}\.[A-Za-z0-9]+$`)
