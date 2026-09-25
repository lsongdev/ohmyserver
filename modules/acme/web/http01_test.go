package web

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type tokenSource map[string]string

func (s tokenSource) GetHTTP01Token(host, token string) (string, bool) {
	if normalized, _, err := net.SplitHostPort(host); err == nil {
		host = normalized
	}
	value, ok := s[strings.ToLower(strings.TrimSuffix(host, "."))+"\x00"+token]
	return value, ok
}

func TestHTTP01HandlerServesOnlyActiveHostToken(t *testing.T) {
	handler := HTTP01Handler(tokenSource{"example.test\x00token-1": "token-1.thumbprint"})
	mux := http.NewServeMux()
	mux.Handle("GET /.well-known/acme-challenge/{token}", handler)
	for _, test := range []struct {
		method, host string
		status       int
		body         string
	}{
		{http.MethodGet, "example.test", http.StatusOK, "token-1.thumbprint"},
		{http.MethodGet, "example.test:80", http.StatusOK, "token-1.thumbprint"},
		{http.MethodHead, "example.test", http.StatusOK, ""},
		{http.MethodGet, "other.test", http.StatusNotFound, "404 page not found\n"},
	} {
		req := httptest.NewRequest(test.method, "http://"+test.host+"/.well-known/acme-challenge/token-1", nil)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, req)
		if response.Code != test.status || response.Body.String() != test.body {
			t.Errorf("%s host=%s: status=%d body=%q", test.method, test.host, response.Code, response.Body.String())
		}
	}
}
