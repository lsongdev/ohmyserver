package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed assets
var moduleWeb embed.FS

func wrapModuleWeb(api http.Handler) http.Handler {
	assets, err := fs.Sub(moduleWeb, "assets")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "module web assets unavailable", http.StatusInternalServerError)
		})
	}
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
