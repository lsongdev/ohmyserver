package web

import (
	"fmt"
	"io"
	"net/http"
)

// HTTP01TokenSource provides the active key authorization for one host/token.
type HTTP01TokenSource interface {
	GetHTTP01Token(host, token string) (string, bool)
}

// HTTP01Handler serves active ACME HTTP-01 challenges. The module's public
// engine supplies token state while this web package owns HTTP semantics.
func HTTP01Handler(source HTTP01TokenSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		token := r.PathValue("token")
		value, ok := source.GetHTTP01Token(r.Host, token)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", fmt.Sprint(len(value)))
		if r.Method == http.MethodHead {
			return
		}
		_, _ = io.WriteString(w, value)
	})
}
