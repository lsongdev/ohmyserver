package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	forward "github.com/lsongdev/ohmyserver/modules/forward"
)

type Service = forward.Service
type Module struct {
	repo    *Repository
	service Service
}

func New(repo *Repository, service Service) *Module { return &Module{repo: repo, service: service} }
func (*Module) Name() string                        { return "forward" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /listeners", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.repo.List(r.Context())
		if err != nil {
			problem(w, 500, err)
			return
		}
		jsonResponse(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("GET /listeners/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := idFrom(r)
		if err != nil {
			jsonError(w, 400, "invalid_id", "listener id must be positive")
			return
		}
		item, err := m.repo.Get(r.Context(), id)
		if err != nil {
			problem(w, 404, err)
			return
		}
		jsonResponse(w, 200, map[string]any{"data": item})
	})
	mux.HandleFunc("POST /listeners", func(w http.ResponseWriter, r *http.Request) {
		var item Listener
		if decode(r, &item) != nil {
			jsonError(w, 400, "invalid_listener", "invalid JSON listener")
			return
		}
		if item.ForwardHost == "" {
			item.ForwardHost = "127.0.0.1"
		}
		if !valid(item) {
			jsonError(w, 400, "invalid_listener", "name, tcp/udp protocol and valid ports are required")
			return
		}
		if err := m.service.CreateForwardListener(r.Context(), &item); err != nil {
			problem(w, 422, err)
			return
		}
		jsonResponse(w, 201, map[string]any{"data": item})
	})
	mux.HandleFunc("PUT /listeners/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := idFrom(r)
		if err != nil {
			jsonError(w, 400, "invalid_id", "listener id must be positive")
			return
		}
		var item Listener
		if decode(r, &item) != nil {
			jsonError(w, 400, "invalid_listener", "invalid JSON listener")
			return
		}
		if item.ForwardHost == "" {
			item.ForwardHost = "127.0.0.1"
		}
		if !valid(item) {
			jsonError(w, 400, "invalid_listener", "name, tcp/udp protocol and valid ports are required")
			return
		}
		item.ID = id
		if err := m.service.UpdateForwardListener(r.Context(), item); err != nil {
			problem(w, 422, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("DELETE /listeners/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := idFrom(r)
		if err != nil {
			jsonError(w, 400, "invalid_id", "listener id must be positive")
			return
		}
		if err := m.service.DeleteForwardListener(r.Context(), id); err != nil {
			problem(w, 422, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /listeners/{id}/actions/{action}", func(w http.ResponseWriter, r *http.Request) {
		id, err := idFrom(r)
		if err != nil {
			jsonError(w, 400, "invalid_id", "listener id must be positive")
			return
		}
		if err := m.service.ForwardListenerAction(r.Context(), id, r.PathValue("action")); err != nil {
			problem(w, 422, err)
			return
		}
		w.WriteHeader(204)
	})
	return wrapModuleWeb(mux)
}
func (m *Module) StandaloneHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/forward/", http.StripPrefix("/v1/forward", m.Handler()))
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
func valid(x Listener) bool {
	return strings.TrimSpace(x.Name) != "" && (x.Protocol == "tcp" || x.Protocol == "udp") && x.ListenPort > 0 && x.ListenPort < 65536 && x.ForwardPort > 0 && x.ForwardPort < 65536 && strings.TrimSpace(x.ForwardHost) != ""
}
func idFrom(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, http.ErrMissingFile
	}
	return id, nil
}
func decode(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func jsonError(w http.ResponseWriter, status int, code, message string) {
	jsonResponse(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func problem(w http.ResponseWriter, status int, err error) {
	jsonError(w, status, "forward_error", err.Error())
}
