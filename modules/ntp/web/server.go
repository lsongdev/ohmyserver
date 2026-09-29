package web

import (
	"encoding/json"

	ntp "github.com/lsongdev/ohmyserver/modules/ntp"
	"net/http"
)

type Service = ntp.Service
type Module struct {
	repo    *Repository
	service Service
}

func New(repo *Repository, service Service) *Module { return &Module{repo: repo, service: service} }
func (*Module) Name() string                        { return "ntp" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		x, e := m.repo.Get(r.Context())
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"data": x})
	})
	mux.HandleFunc("PUT /settings", func(w http.ResponseWriter, r *http.Request) {
		var x Settings
		if json.NewDecoder(r.Body).Decode(&x) != nil {
			writeProblem(w, 400, "invalid_body", "invalid JSON settings object")
			return
		}
		if e := m.service.UpdateNTPConfiguration(r.Context(), x); e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"data": x})
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		status, err := m.service.NTPRuntimeStatus(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": status})
	})
	mux.HandleFunc("POST /action", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Action string `json:"action"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			writeProblem(w, 400, "invalid_body", "invalid JSON action")
			return
		}
		if e := m.service.NTPAction(r.Context(), input.Action); e != nil {
			writeError(w, e)
			return
		}
		status, err := m.service.NTPRuntimeStatus(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": status})
	})
	return wrapModuleWeb(mux)
}

func writeError(w http.ResponseWriter, e error) { writeProblem(w, 400, "ntp_error", e.Error()) }
func writeProblem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
