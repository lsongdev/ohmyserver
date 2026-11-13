package web

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"github.com/lsongdev/ohmyserver/panel"
)

// Standalone serves the same UI and API without a panel host.
func (m *Module) Standalone(password string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", m.Handler()))
	mux.Handle("/lib/", http.StripPrefix("/lib", panel.Static(panel.FS)))
	mux.Handle("/", panel.Static(m.Assets()))
	handler := http.NewCrossOriginProtection().Handler(mux)
	if password == "" {
		return handler
	}
	expected := sha256.Sum256([]byte(password))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, p, ok := r.BasicAuth()
		sum := sha256.Sum256([]byte(p))
		if !ok || subtle.ConstantTimeCompare(sum[:], expected[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Xray", charset="UTF-8"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	})
}
