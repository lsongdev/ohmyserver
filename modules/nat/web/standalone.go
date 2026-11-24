package web

import (
	"crypto/subtle"
	"net/http"

	"github.com/lsongdev/ohmyserver/panel"
)

// Standalone serves the same module with the shared browser runtime and
// optional HTTP Basic authentication, independently of the OhMyServer shell.
func (n *NAT) Standalone(password string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", n.Handler()))
	mux.Handle("/lib/", http.StripPrefix("/lib", panel.Static(panel.FS)))
	mux.Handle("/", panel.Static(n.Assets()))
	var h http.Handler = http.NewCrossOriginProtection().Handler(mux)
	if password != "" {
		next := h
		h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, p, ok := r.BasicAuth()
			if !ok || subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="nat", charset="UTF-8"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	return h
}
