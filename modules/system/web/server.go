package web

import (
	"encoding/json"
	"net/http"

	system "github.com/lsongdev/ohmyserver/modules/system"
)

type Service = system.Service
type Info = system.Info
type Credit = system.Credit
type Module struct{ service Service }

func New(service Service) *Module { return &Module{service: service} }
func (*Module) Name() string      { return "system" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /info", func(w http.ResponseWriter, r *http.Request) {
		info, err := m.service.SystemInfo(r.Context())
		if err != nil {
			systemProblem(w, err)
			return
		}
		systemJSON(w, http.StatusOK, map[string]any{"data": info})
	})
	return wrapModuleWeb(mux)
}

func systemJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func systemProblem(w http.ResponseWriter, err error) {
	systemJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]string{"code": "system_error", "message": err.Error()}})
}
