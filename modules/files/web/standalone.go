package web

import (
	"net/http"
)

// StandaloneHandler serves the module UI at / and mounts its API and assets
// under /v1/files, matching the standalone bootstrap's API base.
func (m *Module) StandaloneHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/files/", http.StripPrefix("/v1/files", m.Handler()))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, moduleWeb, "assets/index.html")
	})
	return mux
}
