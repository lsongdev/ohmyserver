package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type Service interface {
	UpdateDHCPConfiguration(context.Context, Settings) error
	SaveReservation(context.Context, int64, StaticLease) error
	DeleteReservation(context.Context, int64) error
	DeleteDHCPLease(context.Context, int64) error
	DHCPRuntimeStatus(context.Context) (any, error)
	DHCPAction(context.Context, string) error
}

type Module struct {
	repo    *Repository
	service Service
}

func New(repo *Repository, service Service) *Module { return &Module{repo: repo, service: service} }
func (*Module) Name() string                        { return "dhcp" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		item, err := m.repo.Settings(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": item})
	})
	mux.HandleFunc("PUT /settings", func(w http.ResponseWriter, r *http.Request) {
		var item Settings
		if json.NewDecoder(r.Body).Decode(&item) != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_body", "invalid JSON settings object")
			return
		}
		if err := m.service.UpdateDHCPConfiguration(r.Context(), item); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": item})
	})
	mux.HandleFunc("GET /leases", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.repo.Leases(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items})
	})
	mux.HandleFunc("GET /static-leases", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.repo.StaticLeases(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items})
	})
	mux.HandleFunc("POST /static-leases", func(w http.ResponseWriter, r *http.Request) {
		var item StaticLease
		if json.NewDecoder(r.Body).Decode(&item) != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_body", "invalid JSON reservation object")
			return
		}
		if err := m.service.SaveReservation(r.Context(), 0, item); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("PUT /static-leases/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_id", err.Error())
			return
		}
		var item StaticLease
		if json.NewDecoder(r.Body).Decode(&item) != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_body", "invalid JSON reservation object")
			return
		}
		if err := m.service.SaveReservation(r.Context(), id, item); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /static-leases/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_id", err.Error())
			return
		}
		if err := m.service.DeleteReservation(r.Context(), id); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /leases/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_id", err.Error())
			return
		}
		if err := m.service.DeleteDHCPLease(r.Context(), id); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		status, err := m.service.DHCPRuntimeStatus(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": status})
	})
	mux.HandleFunc("POST /action", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Action string `json:"action"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_body", "invalid JSON action")
			return
		}
		if err := m.service.DHCPAction(r.Context(), input.Action); err != nil {
			writeError(w, err)
			return
		}
		status, err := m.service.DHCPRuntimeStatus(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": status})
	})
	return wrapModuleWeb(mux)
}

func (m *Module) StandaloneHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/dhcp/", http.StripPrefix("/v1/dhcp", m.Handler()))
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
		if r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/v1/dhcp/") {
			mux.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}
func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errInvalidID
	}
	return id, nil
}

var errInvalidID = strconv.ErrSyntax

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "dhcp_error", "message": err.Error()}})
}
func writeProblem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
