package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed assets
var moduleAssets embed.FS

func assetsFS() fs.FS {
	assets, err := fs.Sub(moduleAssets, "assets")
	if err != nil {
		return nil
	}
	return assets
}

func wrapModuleWeb(api http.Handler) http.Handler {
	assets := assetsFS()
	if assets == nil {
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
