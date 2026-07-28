package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTerminalOriginAllowed(t *testing.T) {
	tests := []struct {
		name          string
		origin        string
		host          string
		forwardedHost string
		want          bool
	}{
		{name: "missing origin", host: "127.0.0.1:9000", want: true},
		{name: "direct same origin", origin: "https://admin.sczh.cc", host: "admin.sczh.cc", want: true},
		{
			name:          "reverse proxy origin",
			origin:        "https://admin.sczh.cc",
			host:          "127.0.0.1:9000",
			forwardedHost: "admin.sczh.cc",
			want:          true,
		},
		{
			name:          "first forwarded host",
			origin:        "https://admin.sczh.cc",
			host:          "127.0.0.1:9000",
			forwardedHost: "admin.sczh.cc, proxy.internal",
			want:          true,
		},
		{
			name:          "foreign origin",
			origin:        "https://attacker.example",
			host:          "127.0.0.1:9000",
			forwardedHost: "admin.sczh.cc",
			want:          false,
		},
		{name: "invalid origin", origin: "://invalid", host: "admin.sczh.cc", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://"+test.host+"/terminal/local/ws", nil)
			request.Host = test.host
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.forwardedHost != "" {
				request.Header.Set("X-Forwarded-Host", test.forwardedHost)
			}
			if got := terminalOriginAllowed(request); got != test.want {
				t.Fatalf("terminalOriginAllowed() = %v, want %v", got, test.want)
			}
		})
	}
}
