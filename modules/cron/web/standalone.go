package web

import (
	"crypto/subtle"
	"net/http"

	"github.com/lsongdev/ohmyserver/panel"
)

// Standalone serves the module on its own: the UI at / and the API at
// /api/. A non-empty password requires HTTP basic authentication (any user
// name), which browsers also send on EventSource and WebSocket requests.
func (c *Cron) Standalone(password string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", c.Handler()))
	mux.Handle("/", panel.Static(c.Assets()))
	mux.Handle("/lib/", http.StripPrefix("/lib", panel.Static(panel.FS)))
	var h http.Handler = http.NewCrossOriginProtection().Handler(mux)
	if password != "" {
		h = basicAuth(h, password)
	}
	return h
}

func basicAuth(next http.Handler, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="cron", charset="UTF-8"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
