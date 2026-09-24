package web

import (
	"net/http"
)

// StandaloneHandler serves the module UI at / and mounts its module handler
// under /v1/clash, matching the API base used by the standalone bootstrap.
func (m *Module) StandaloneHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/clash/", http.StripPrefix("/v1/clash", m.Handler()))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, moduleWeb, "assets/index.html")
	})
	return mux
}
