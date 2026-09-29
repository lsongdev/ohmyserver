package web

import (
	"encoding/json"
	"net/http"

	dashboard "github.com/lsongdev/ohmyserver/modules/dashboard"
)

type Module struct{ service dashboard.Service }

func New(service dashboard.Service) *Module { return &Module{service: service} }
func (*Module) Name() string                { return "dashboard" }

func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /overview", func(w http.ResponseWriter, r *http.Request) {
		item, err := m.service.Overview(r.Context())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "dashboard_unavailable", err.Error())
			return
		}
		if item.Widgets == nil {
			item.Widgets = []dashboard.Widget{}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": item})
	})
	return wrapModuleWeb(mux)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
