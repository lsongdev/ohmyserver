package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	transport "github.com/lsongdev/ohmyserver/pkg/terminal"
)

func TestOriginAllowed(t *testing.T) {
	tests := []struct {
		name, origin, host, forwardedHost string
		tls                               bool
		want                              bool
	}{
		{name: "missing origin", host: "127.0.0.1:9000", want: true},
		{name: "same origin", origin: "https://admin.example", host: "admin.example", tls: true, want: true},
		{name: "http same origin", origin: "http://admin.example", host: "admin.example", want: true},
		{name: "trusted proxy policy is explicit", origin: "https://admin.example", host: "127.0.0.1:9000", forwardedHost: "admin.example", want: false},
		{name: "forwarded host cannot authorize spoofed origin", origin: "https://attacker.example", host: "127.0.0.1:9000", forwardedHost: "attacker.example", want: false},
		{name: "scheme mismatch", origin: "http://admin.example", host: "admin.example", tls: true, want: false},
		{name: "path is not an origin", origin: "https://admin.example/path", host: "admin.example", tls: true, want: false},
		{name: "invalid origin", origin: "://invalid", host: "admin.example", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := "http"
			if tc.tls {
				scheme = "https"
			}
			r := httptest.NewRequest(http.MethodGet, scheme+"://"+tc.host+"/v1/terminal/local/ws", nil)
			r.Host = tc.host
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.forwardedHost != "" {
				r.Header.Set("X-Forwarded-Host", tc.forwardedHost)
			}
			if got := originAllowed(r); got != tc.want {
				t.Fatalf("originAllowed() = %v, want %v", got, tc.want)
			}
		})
	}
}

type terminalOpenerFunc func(context.Context, string, string, uint16, uint16) (transport.Session, error)

func (fn terminalOpenerFunc) OpenTerminal(ctx context.Context, kind, id string, cols, rows uint16) (transport.Session, error) {
	return fn(ctx, kind, id, cols, rows)
}

func TestWebsocketRouteRejectsSpoofedForwardedHostBeforeOpeningSession(t *testing.T) {
	var mu sync.Mutex
	opened := false
	handler := New(terminalOpenerFunc(func(context.Context, string, string, uint16, uint16) (transport.Session, error) {
		mu.Lock()
		opened = true
		mu.Unlock()
		return nil, nil
	})).Handler()
	server := httptest.NewServer(handler)
	defer server.Close()

	requestURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/local/ws"
	headers := http.Header{"Origin": []string{"https://attacker.example"}, "X-Forwarded-Host": []string{"attacker.example"}}
	conn, response, err := websocket.DefaultDialer.Dial(requestURL, headers)
	if err == nil {
		_ = conn.Close()
		t.Fatal("websocket handshake unexpectedly succeeded")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("response=%v, want HTTP %d (error=%v)", response, http.StatusForbidden, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if opened {
		t.Fatal("terminal opener was invoked for a rejected origin")
	}
}

func TestWebsocketRoutePassesSessionKindDimensionsAndOpenFailure(t *testing.T) {
	type openRequest struct {
		kind string
		id   string
		cols uint16
		rows uint16
	}
	opened := make(chan openRequest, 1)
	handler := New(terminalOpenerFunc(func(_ context.Context, kind, id string, cols, rows uint16) (transport.Session, error) {
		opened <- openRequest{kind: kind, id: id, cols: cols, rows: rows}
		return nil, errors.New("missing SSH adapter")
	})).Handler()
	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ssh/42/ws?cols=132&rows=43"
	origin := "http" + strings.TrimPrefix(server.URL, "http")
	conn, response, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Origin": []string{origin}})
	if err != nil {
		t.Fatalf("websocket handshake: %v (response=%v)", err, response)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var control struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(payload, &control); err != nil {
		t.Fatalf("decode control message %q: %v", payload, err)
	}
	if control.Type != "error" || control.Message != "missing SSH adapter" {
		t.Fatalf("control message = %+v", control)
	}
	select {
	case request := <-opened:
		if request != (openRequest{kind: "ssh", id: "42", cols: 132, rows: 43}) {
			t.Fatalf("opener request = %+v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("terminal opener was not called")
	}
}

func TestNewWithOriginPolicyAllowsExplicitProxyOrigin(t *testing.T) {
	customOrigin, err := url.Parse("https://console.example")
	if err != nil {
		t.Fatal(err)
	}
	module := NewWithOriginPolicy(terminalOpenerFunc(func(context.Context, string, string, uint16, uint16) (transport.Session, error) { return nil, nil }), func(r *http.Request) bool {
		origin, err := url.Parse(r.Header.Get("Origin"))
		return err == nil && origin.Scheme+"://"+origin.Host == customOrigin.Scheme+"://"+customOrigin.Host
	})
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:9000/v1/terminal/local/ws", nil)
	r.Header.Set("Origin", customOrigin.String())
	if !module.allowOrigin(r) {
		t.Fatal("explicit proxy origin policy rejected configured origin")
	}
}

func TestStandaloneHandlerServesUIManifestAndModuleAssets(t *testing.T) {
	handler := New(terminalOpenerFunc(func(context.Context, string, string, uint16, uint16) (transport.Session, error) {
		return nil, errors.New("not opened")
	})).StandaloneHandler()
	for path, contains := range map[string]string{
		"/":                                "api-base",
		"/v1/terminal/manifest.json":       "assets/module.js",
		"/v1/terminal/assets/index.js":     "lsong.org/scripts/react/index.js",
		"/v1/terminal/assets/module.js":    "createAPI",
		"/v1/terminal/assets/terminal.css": ".terminal-viewport",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), contains) {
			t.Errorf("GET %s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}
