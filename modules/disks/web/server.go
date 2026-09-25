package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	disks "github.com/lsongdev/ohmyserver/modules/disks"
)

type Usage = disks.Usage
type Health = disks.Health
type Service = disks.Service

type Module struct{ service Service }

func New(service Service) *Module { return &Module{service: service} }
func (*Module) Name() string      { return "disks" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /usage", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.service.DiskUsage(r.Context())
		if err != nil {
			diskError(w, err)
			return
		}
		diskJSON(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("GET /devices", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.service.DiskDevices(r.Context())
		if err != nil {
			diskError(w, err)
			return
		}
		diskJSON(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		device := strings.TrimSpace(r.URL.Query().Get("device"))
		if device == "" {
			diskProblem(w, 400, "invalid_device", "device query parameter is required")
			return
		}
		item, err := m.service.DiskHealth(r.Context(), device)
		if err != nil {
			diskError(w, err)
			return
		}
		diskJSON(w, 200, map[string]any{"data": item})
	})
	return wrapModuleWeb(mux)
}
func (m *Module) StandaloneHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/disks/", http.StripPrefix("/v1/disks", m.Handler()))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFileFS(w, r, moduleWeb, "assets/index.html")
	})
	return mux
}
func diskJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func diskProblem(w http.ResponseWriter, status int, code, message string) {
	diskJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func diskError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, disks.ErrInvalidDevice) {
		status = http.StatusBadRequest
	}
	diskProblem(w, status, "disk_error", err.Error())
}
