package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	transport "github.com/lsongdev/ohmyserver/pkg/terminal"
)

type Service interface {
	OpenTerminal(context.Context, string, string, uint16, uint16) (transport.Session, error)
}

type Module struct {
	service     Service
	allowOrigin func(*http.Request) bool
}

func New(service Service) *Module { return NewWithOriginPolicy(service, originAllowed) }

// NewWithOriginPolicy allows a host application to explicitly extend the
// default same-origin policy for a known reverse-proxy deployment.
func NewWithOriginPolicy(service Service, allowOrigin func(*http.Request) bool) *Module {
	if allowOrigin == nil {
		allowOrigin = originAllowed
	}
	return &Module{service: service, allowOrigin: allowOrigin}
}
func (*Module) Name() string { return "terminal" }

func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /local/ws", m.serve("local", ""))
	mux.HandleFunc("GET /ssh/{id}/ws", func(w http.ResponseWriter, r *http.Request) { m.serve("ssh", r.PathValue("id"))(w, r) })
	mux.HandleFunc("GET /docker/{id}/ws", func(w http.ResponseWriter, r *http.Request) { m.serve("docker", r.PathValue("id"))(w, r) })
	return wrapModuleWeb(mux)
}

func (m *Module) serve(kind, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{CheckOrigin: m.allowOrigin}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.SetReadLimit(1 << 20)
		cols, rows := dimension(r.URL.Query().Get("cols"), 80), dimension(r.URL.Query().Get("rows"), 24)
		session, err := m.service.OpenTerminal(r.Context(), kind, id, cols, rows)
		if err != nil {
			_ = conn.WriteMessage(websocket.TextMessage, controlError(err.Error()))
			_ = conn.Close()
			return
		}
		_ = transport.Bridge(r.Context(), conn, session)
	}
}

func dimension(value string, fallback uint16) uint16 {
	n, err := strconv.ParseUint(value, 10, 16)
	if err != nil || n == 0 {
		return fallback
	}
	return uint16(n)
}

func controlError(message string) []byte {
	data, _ := json.Marshal(map[string]string{"type": "error", "message": message})
	return data
}

func originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Host == "" {
		return false
	}
	expectedScheme := "http"
	if r.TLS != nil {
		expectedScheme = "https"
	}
	return strings.EqualFold(u.Scheme, expectedScheme) && strings.EqualFold(u.Host, r.Host)
}
