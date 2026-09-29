// Package moduleweb provides the small HTTP adapter modules use to serve
// their own embedded browser assets and discovery manifest beside their API.
package moduleweb

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed browser/paths.js browser/transport.js browser/router.js browser/definition.js browser/view.js browser/standalone.js browser/standalone.css
var browser embed.FS

// BrowserHandler serves the shared browser runtime relative to its mount.
// Both the standalone host and an embedding host use these exact assets.
func BrowserHandler() http.Handler {
	files, _ := fs.Sub(browser, "browser")
	return http.FileServer(http.FS(files))
}

// Wrap creates the complete, mount-independent module handler. It serves the
// standalone document at /, the manifest at /manifest.json, module assets at
// /assets/, and the shared runtime at the reserved /assets/runtime/ path.
// The supplied mux owns relative API routes. Neither mode wraps business
// responses: only the mux's own unmatched-route errors become JSON.
func Wrap(api *http.ServeMux, assets fs.FS) http.Handler {
	static := http.StripPrefix("/assets/", http.FileServer(http.FS(assets)))
	runtime := http.StripPrefix("/assets/runtime/", BrowserHandler())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/manifest.json" || strings.HasPrefix(r.URL.Path, "/assets/") || r.URL.Path == "/assets" {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		switch {
		case r.URL.Path == "/":
			http.ServeFileFS(w, r, assets, "index.html")
			return
		case r.URL.Path == "/manifest.json":
			http.ServeFileFS(w, r, assets, "manifest.json")
			return
		case strings.HasPrefix(r.URL.Path, "/assets/runtime/"):
			runtime.ServeHTTP(w, r)
			return
		case r.URL.Path == "/assets":
			http.Redirect(w, r, "assets/", http.StatusPermanentRedirect)
			return
		case strings.HasPrefix(r.URL.Path, "/assets/"):
			static.ServeHTTP(w, r)
			return
		}
		w.Header().Del("Cache-Control")
		if fallback, pattern := api.Handler(r); pattern == "" {
			fallback.ServeHTTP(&routeErrorWriter{ResponseWriter: w}, r)
			return
		}
		api.ServeHTTP(w, r)
	})
}

// Only used for ServeMux-generated responses, never for a business handler.
type routeErrorWriter struct {
	http.ResponseWriter
	blocked bool
}

func (w *routeErrorWriter) WriteHeader(status int) {
	if status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.blocked = true
	code, message := "api_not_found", "API endpoint not found"
	if status == http.StatusMethodNotAllowed {
		code, message = "method_not_allowed", "Method not allowed"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(status)
	_ = json.NewEncoder(w.ResponseWriter).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func (w *routeErrorWriter) Write(body []byte) (int, error) {
	if w.blocked {
		return len(body), nil
	}
	return w.ResponseWriter.Write(body)
}
