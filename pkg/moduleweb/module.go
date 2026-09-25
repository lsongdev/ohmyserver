// Package moduleweb provides the small HTTP adapter modules use to serve
// their own embedded browser assets and discovery manifest beside their API.
package moduleweb

import (
	"io/fs"
	"net/http"
	"strings"
)

// Wrap adds module-owned static assets and manifest handling to an API handler.
// Assets are rooted at the module's embedded web directory; files are exposed
// under /assets and manifest.json is exposed at the module root.
func Wrap(api http.Handler, assets fs.FS) http.Handler {
	static := http.StripPrefix("/assets/", http.FileServer(http.FS(assets)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFileFS(w, r, assets, "manifest.json")
			return
		}
		if r.URL.Path == "/assets" || strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "no-cache")
			static.ServeHTTP(w, r)
			return
		}
		api.ServeHTTP(w, r)
	})
}
