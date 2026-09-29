package web

import (
	"encoding/json"
	"errors"
	"fmt"
	sshmodule "github.com/lsongdev/ohmyserver/modules/ssh"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var ErrNotFound = sshmodule.ErrNotFound
var ErrInvalid = sshmodule.ErrInvalid
var ErrConnection = sshmodule.ErrConnection
var ErrDisabled = sshmodule.ErrDisabled

type Stats = sshmodule.Stats
type Metrics = sshmodule.Metrics
type Server = sshmodule.Server
type Input = sshmodule.Input
type Service = sshmodule.Service
type Module struct{ service Service }

func New(service Service) *Module { return &Module{service: service} }
func (*Module) Name() string      { return "ssh" }

func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /servers/test", func(w http.ResponseWriter, r *http.Request) {
		var in Input
		if !decode(w, r, &in) {
			return
		}
		if err := sshmodule.Validate(&in); err != nil {
			problem(w, err)
			return
		}
		if err := m.service.TestSSHProfile(r.Context(), in); err != nil {
			problem(w, err)
			return
		}
		write(w, 200, map[string]any{"data": map[string]string{"message": "SSH connection successful"}})
	})
	mux.HandleFunc("GET /servers", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.service.ListSSHProfiles(r.Context())
		if err != nil {
			problem(w, err)
			return
		}
		write(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("POST /servers", func(w http.ResponseWriter, r *http.Request) {
		var in Input
		if !decode(w, r, &in) {
			return
		}
		if err := sshmodule.Validate(&in); err != nil {
			problem(w, err)
			return
		}
		item, err := m.service.SaveSSHProfile(r.Context(), 0, in)
		if err != nil {
			problem(w, err)
			return
		}
		write(w, 201, map[string]any{"data": item})
	})
	mux.HandleFunc("GET /servers/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		item, err := m.service.GetSSHProfile(r.Context(), id)
		if err != nil {
			problem(w, err)
			return
		}
		write(w, 200, map[string]any{"data": item})
	})
	mux.HandleFunc("PUT /servers/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var in Input
		if !decode(w, r, &in) {
			return
		}
		item, err := m.service.SaveSSHProfile(r.Context(), id, in)
		if err != nil {
			problem(w, err)
			return
		}
		write(w, 200, map[string]any{"data": item})
	})
	mux.HandleFunc("DELETE /servers/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if err := m.service.DeleteSSHProfile(r.Context(), id); err != nil {
			problem(w, err)
			return
		}
		write(w, 200, map[string]any{"data": map[string]bool{"deleted": true}})
	})
	mux.HandleFunc("GET /servers/{id}/stats", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			problem(w, errors.New("streaming is not supported"))
			return
		}
		if _, err := m.service.GetSSHProfile(r.Context(), id); err != nil {
			problem(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			stats, err := m.service.SSHStats(r.Context(), id)
			if err != nil {
				_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", strings.ReplaceAll(err.Error(), "\n", " "))
			} else {
				data, _ := json.Marshal(stats)
				_, _ = fmt.Fprintf(w, "event: stats\ndata: %s\n\n", data)
			}
			flusher.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
			}
		}
	})
	mux.HandleFunc("GET /servers/{id}/metrics", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		metrics, err := m.service.SSHMetrics(r.Context(), id)
		if err != nil {
			problem(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		writeMetrics(w, metrics)
	})
	return wrapModuleWeb(mux)
}

func writeMetrics(w http.ResponseWriter, m Metrics) {
	label := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", " ").Replace(m.Host)
	items := []struct {
		name, help, kind string
		value            any
	}{
		{"node_cpu_usage_percent", "CPU usage percentage", "gauge", m.CPUUsage},
		{"node_cpu_cores", "Number of CPU cores", "gauge", m.CPUCores},
		{"node_memory_total_bytes", "Total memory bytes", "gauge", m.MemoryTotal},
		{"node_memory_used_bytes", "Used memory bytes", "gauge", m.MemoryUsed},
		{"node_memory_free_bytes", "Free memory bytes", "gauge", m.MemoryFree},
		{"node_memory_usage_percent", "Memory usage percentage", "gauge", m.MemoryUsage},
		{"node_disk_total_bytes", "Total disk bytes", "gauge", m.DiskTotal},
		{"node_disk_used_bytes", "Used disk bytes", "gauge", m.DiskUsed},
		{"node_disk_free_bytes", "Free disk bytes", "gauge", m.DiskFree},
		{"node_disk_usage_percent", "Disk usage percentage", "gauge", m.DiskUsage},
		{"node_network_receive_bytes_total", "Cumulative network bytes received", "counter", m.NetworkReceived},
		{"node_network_transmit_bytes_total", "Cumulative network bytes sent", "counter", m.NetworkSent},
		{"node_load1", "1m load average", "gauge", m.Load1},
		{"node_load5", "5m load average", "gauge", m.Load5},
		{"node_load15", "15m load average", "gauge", m.Load15},
		{"node_uptime_seconds", "Uptime in seconds", "counter", m.Uptime},
	}
	for _, item := range items {
		_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n%s{host=\"%s\"} %v\n", item.name, item.help, item.name, item.kind, item.name, label, item.value)
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		problem(w, ErrInvalid)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		problem(w, ErrInvalid)
		return false
	}
	return true
}
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		problem(w, ErrInvalid)
		return 0, false
	}
	return id, true
}
func problem(w http.ResponseWriter, err error) {
	status, code := 500, "internal_error"
	if errors.Is(err, sshmodule.ErrInvalid) {
		status, code = 400, "invalid_ssh_server"
	} else if errors.Is(err, sshmodule.ErrNotFound) {
		status, code = 404, "ssh_server_not_found"
	} else if errors.Is(err, sshmodule.ErrConnection) {
		status, code = 502, "ssh_connection_failed"
	} else if errors.Is(err, sshmodule.ErrDisabled) {
		status, code = 409, "ssh_server_disabled"
	}
	write(w, status, map[string]any{"error": map[string]string{"code": code, "message": err.Error()}})
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
