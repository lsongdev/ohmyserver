package web

import "net/http"

// StandaloneHandler serves the module UI at / and its API/manifest/assets at
// /v1/acme, which is the API base consumed by the standalone bootstrap.
func (m *Module) StandaloneHandler() http.Handler {
	mux := http.NewServeMux()
	if challenge, ok := m.service.(interface {
		GetHTTP01Token(host, token string) (string, bool)
	}); ok {
		mux.Handle("GET /.well-known/acme-challenge/{token}", HTTP01Handler(challenge))
	}
	mux.Handle("/v1/acme/", http.StripPrefix("/v1/acme", m.Handler()))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, moduleWeb, "assets/index.html")
	})
	return mux
}
