package web

import (
	"encoding/json"
	monitor "github.com/lsongdev/ohmyserver/modules/monitor"
	"net/http"
	"time"
)

type Collector = monitor.Collector
type Stats = monitor.Stats
type Module struct {
	repo      *Repository
	collector Collector
}

func New(repo *Repository, collector Collector) *Module {
	return &Module{repo: repo, collector: collector}
}
func (*Module) Name() string { return "monitor" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hosts", m.hosts)
	mux.HandleFunc("GET /hosts/{hostname}", m.host)
	mux.HandleFunc("GET /hosts/{hostname}/history", m.history)
	mux.HandleFunc("GET /current", m.current)
	mux.Handle("POST /telegraf", TelegrafHandler(m.repo))
	return wrapModuleWeb(mux)
}

func (m *Module) hosts(w http.ResponseWriter, r *http.Request) {
	v, e := m.repo.LatestForAllHosts(r.Context())
	if e != nil {
		fail(w, 500, "monitor_read_failed", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) host(w http.ResponseWriter, r *http.Request) {
	v, e := m.repo.LatestForHost(r.Context(), r.PathValue("hostname"))
	if e != nil {
		fail(w, 404, "host_not_found", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) history(w http.ResponseWriter, r *http.Request) {
	duration := time.Hour
	if raw := r.URL.Query().Get("duration"); raw != "" {
		parsed, e := time.ParseDuration(raw)
		if e != nil || parsed <= 0 || parsed > 30*24*time.Hour {
			fail(w, 400, "invalid_duration", "duration must be a positive duration of 30 days or less")
			return
		}
		duration = parsed
	}
	v, step, e := m.repo.RecentSampled(r.Context(), r.PathValue("hostname"), duration)
	if e != nil {
		fail(w, 500, "monitor_read_failed", e.Error())
		return
	}
	current, _ := m.repo.LatestForHost(r.Context(), r.PathValue("hostname"))
	ok(w, 200, map[string]any{"current": current, "history": v, "sample_interval_seconds": step})
}
func (m *Module) current(w http.ResponseWriter, r *http.Request) {
	v, e := m.collector.CollectMonitorStats(r.Context())
	if e != nil {
		fail(w, 503, "collector_unavailable", e.Error())
		return
	}
	ok(w, 200, v)
}
func ok(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}
func fail(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
