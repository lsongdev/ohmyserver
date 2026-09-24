package web

import (
	"encoding/json"
	"net/http"
	"strings"

	system "github.com/lsongdev/ohmyserver/modules/system"
)

type Service = system.Service
type Info = system.Info
type Credit = system.Credit
type UpdateInfo = system.UpdateInfo
type Module struct{ service Service }

func New(service Service) *Module { return &Module{service: service} }
func (*Module) Name() string      { return "system" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /info", func(w http.ResponseWriter, r *http.Request) {
		info, err := m.service.SystemInfo(r.Context(), r.URL.Query().Get("check") == "1")
		if err != nil {
			systemProblem(w, err)
			return
		}
		systemJSON(w, http.StatusOK, map[string]any{"data": info})
	})
	mux.HandleFunc("POST /update", func(w http.ResponseWriter, r *http.Request) {
		if err := m.service.QueueSystemUpdate(r.Context()); err != nil {
			systemProblem(w, err)
			return
		}
		systemJSON(w, http.StatusAccepted, map[string]any{"data": map[string]string{"message": "update queued"}})
	})
	return wrapModuleWeb(mux)
}

// StandaloneHandler serves the same module API and assets without the panel shell.
func (m *Module) StandaloneHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/system/", http.StripPrefix("/v1/system", m.Handler()))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, moduleWeb, "assets/index.html")
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/system/") || r.URL.Path == "/" {
			mux.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}
func systemJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func systemProblem(w http.ResponseWriter, err error) {
	systemJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]string{"code": "system_error", "message": err.Error()}})
}
