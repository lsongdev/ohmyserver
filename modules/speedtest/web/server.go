package web

import (
	"context"
	"encoding/json"
	speedtest "github.com/lsongdev/ohmyserver/modules/speedtest"
	"net/http"
	"strconv"
)

type Result = speedtest.Result
type ServerInfo = speedtest.ServerInfo
type Progress = speedtest.Progress
type RunRequest = speedtest.RunRequest
type Service interface {
	ListResults(context.Context) ([]speedtest.Result, error)
	DeleteResult(context.Context, int64) error
	ListServers(context.Context, string) ([]speedtest.ServerInfo, error)
	RunSpeedtest(context.Context, http.ResponseWriter, *http.Request)
}
type Module struct{ service Service }

func New(service Service) *Module { return &Module{service: service} }
func (*Module) Name() string      { return "speedtest" }

func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /results", m.listResults)
	mux.HandleFunc("DELETE /results/{id}", m.deleteResult)
	mux.HandleFunc("GET /servers", m.listServers)
	mux.HandleFunc("POST /run", func(w http.ResponseWriter, r *http.Request) {
		m.service.RunSpeedtest(r.Context(), w, r)
	})
	return wrapModuleWeb(mux)
}

func (m *Module) listResults(w http.ResponseWriter, r *http.Request) {
	items, err := m.service.ListResults(r.Context())
	if err != nil {
		writeError(w, 503, "results_unavailable", err.Error())
		return
	}
	if items == nil {
		items = []Result{}
	}
	writeJSON(w, 200, map[string]any{"data": items})
}
func (m *Module) deleteResult(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, 400, "invalid_id", "invalid result id")
		return
	}
	if err = m.service.DeleteResult(r.Context(), id); err != nil {
		writeError(w, 404, "result_not_found", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (m *Module) listServers(w http.ResponseWriter, r *http.Request) {
	testType := r.URL.Query().Get("type")
	if testType != "speedtest" && testType != "librespeed" {
		writeError(w, 400, "invalid_type", "type must be speedtest or librespeed")
		return
	}
	items, err := m.service.ListServers(r.Context(), testType)
	if err != nil {
		writeError(w, 502, "servers_unavailable", err.Error())
		return
	}
	if items == nil {
		items = []ServerInfo{}
	}
	writeJSON(w, 200, map[string]any{"data": items})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
