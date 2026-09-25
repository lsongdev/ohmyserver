package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	uptime "github.com/lsongdev/ohmyserver/modules/uptime"
)

type WithStatus = uptime.WithStatus

const (
	Ping = uptime.Ping
	TCP  = uptime.TCP
	HTTP = uptime.HTTP
)

type Module struct {
	repo    *Repository
	checker uptime.Checker
}

func New(repo *Repository, checker uptime.Checker) *Module {
	return &Module{repo: repo, checker: checker}
}
func (*Module) Name() string { return "uptime" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /monitors", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.repo.Monitors(r.Context(), false)
		if err != nil {
			problem(w, 500, err)
			return
		}
		result := make([]WithStatus, 0, len(items))
		for _, item := range items {
			last, _ := m.repo.LatestCheck(r.Context(), item.ID)
			uptime, _ := m.repo.Uptime(r.Context(), item.ID, 24)
			result = append(result, WithStatus{Monitor: item, LastCheck: last, UptimePct: uptime})
		}
		jsonResponse(w, 200, map[string]any{"data": result})
	})
	mux.HandleFunc("POST /monitors", func(w http.ResponseWriter, r *http.Request) {
		var item Monitor
		if decode(r, &item) != nil || !valid(item) {
			jsonError(w, 400, "invalid_monitor", "valid name, target and monitor type are required")
			return
		}
		if err := m.repo.SaveMonitor(r.Context(), &item); err != nil {
			problem(w, 500, err)
			return
		}
		jsonResponse(w, 201, map[string]any{"data": item})
	})
	mux.HandleFunc("GET /monitors/{id}/checks", func(w http.ResponseWriter, r *http.Request) {
		id, err := idFrom(r)
		if err != nil {
			jsonError(w, 400, "invalid_id", "monitor id must be positive")
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit < 1 || limit > 200 {
			limit = 50
		}
		items, err := m.repo.Checks(r.Context(), id, limit)
		if err != nil {
			problem(w, 500, err)
			return
		}
		jsonResponse(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("PUT /monitors/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := idFrom(r)
		if err != nil {
			jsonError(w, 400, "invalid_id", "monitor id must be positive")
			return
		}
		var item Monitor
		if decode(r, &item) != nil || !valid(item) {
			jsonError(w, 400, "invalid_monitor", "valid name, target and monitor type are required")
			return
		}
		item.ID = id
		if err := m.repo.SaveMonitor(r.Context(), &item); err != nil {
			problem(w, 500, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("DELETE /monitors/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := idFrom(r)
		if err != nil {
			jsonError(w, 400, "invalid_id", "monitor id must be positive")
			return
		}
		if err := m.repo.DeleteMonitor(r.Context(), id); err != nil {
			problem(w, 500, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /monitors/{id}/actions/{action}", func(w http.ResponseWriter, r *http.Request) {
		id, err := idFrom(r)
		if err != nil {
			jsonError(w, 400, "invalid_id", "monitor id must be positive")
			return
		}
		switch r.PathValue("action") {
		case "toggle":
			item, err := m.repo.Monitor(r.Context(), id)
			if err == nil {
				err = m.repo.SetEnabled(r.Context(), id, !item.Enabled)
			}
			if err != nil {
				problem(w, 500, err)
				return
			}
			w.WriteHeader(204)
		case "check":
			item, err := m.checker.CheckNow(r.Context(), id)
			if err != nil {
				problem(w, 422, err)
				return
			}
			jsonResponse(w, 200, map[string]any{"data": item})
		default:
			jsonError(w, 404, "unknown_action", "unknown monitor action")
		}
	})
	return wrapModuleWeb(mux)
}

// StandaloneHandler exposes the module UI and its API at their shell-compatible paths.
func (m *Module) StandaloneHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/uptime/", http.StripPrefix("/v1/uptime", m.Handler()))
	mux.HandleFunc("/", standalonePage)
	return mux
}
func valid(x Monitor) bool {
	return strings.TrimSpace(x.Name) != "" && strings.TrimSpace(x.Target) != "" && (x.Type == Ping || x.Type == TCP || x.Type == HTTP)
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
	jsonError(w, status, "uptime_error", err.Error())
}
