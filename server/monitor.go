package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/lsongdev/ohmyserver/sysinfo"
	"github.com/lsongdev/ohmyserver/telegraf"
)

// SystemStats represents comprehensive system metrics
type SystemStats struct {
	ID        int64     `json:"id"`
	Hostname  string    `json:"hostname"`
	Timestamp time.Time `json:"timestamp"`

	CPUUsage float64 `json:"cpu_usage"`
	CPUCores int     `json:"cpu_cores"`

	MemoryTotal uint64  `json:"memory_total"`
	MemoryUsed  uint64  `json:"memory_used"`
	MemoryFree  uint64  `json:"memory_free"`
	MemoryUsage float64 `json:"memory_usage"`

	DiskTotal uint64  `json:"disk_total"`
	DiskUsed  uint64  `json:"disk_used"`
	DiskFree  uint64  `json:"disk_free"`
	DiskUsage float64 `json:"disk_usage"`

	NetBytesSent uint64 `json:"net_bytes_sent"`
	NetBytesRecv uint64 `json:"net_bytes_recv"`

	Uptime uint64  `json:"uptime"`
	Load1  float64 `json:"load_1"`
	Load5  float64 `json:"load_5"`
	Load15 float64 `json:"load_15"`
}

// ---------- Stats CRUD ----------

func (s *Server) InsertStats(stats *SystemStats) error {
	_, err := s.db.Exec(`
	INSERT INTO system_stats
		(hostname, timestamp, cpu_usage, cpu_cores,
		 memory_total, memory_used, memory_free, memory_usage,
		 disk_total, disk_used, disk_free, disk_usage,
		 net_bytes_sent, net_bytes_recv,
		 uptime, load_1, load_5, load_15)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		stats.Hostname, stats.Timestamp,
		stats.CPUUsage, stats.CPUCores,
		stats.MemoryTotal, stats.MemoryUsed, stats.MemoryFree, stats.MemoryUsage,
		stats.DiskTotal, stats.DiskUsed, stats.DiskFree, stats.DiskUsage,
		stats.NetBytesSent, stats.NetBytesRecv,
		stats.Uptime, stats.Load1, stats.Load5, stats.Load15,
	)
	return err
}

func (s *Server) GetRecentStats(duration time.Duration) ([]SystemStats, error) {
	since := time.Now().Add(-duration)
	rows, err := s.db.Query(`
	SELECT id, COALESCE(hostname,''), timestamp,
		cpu_usage, COALESCE(cpu_cores,0),
		memory_total, memory_used, memory_free, memory_usage,
		COALESCE(disk_total,0), COALESCE(disk_used,0), COALESCE(disk_free,0), COALESCE(disk_usage,0),
		COALESCE(net_bytes_sent,0), COALESCE(net_bytes_recv,0),
		COALESCE(uptime,0), COALESCE(load_1,0), COALESCE(load_5,0), COALESCE(load_15,0)
	FROM system_stats
	WHERE timestamp >= ?
	ORDER BY timestamp ASC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStats(rows)
}

func (s *Server) GetRecentStatsByHost(hostname string, duration time.Duration) ([]SystemStats, error) {
	since := time.Now().Add(-duration)
	rows, err := s.db.Query(`
	SELECT id, COALESCE(hostname,''), timestamp,
		cpu_usage, COALESCE(cpu_cores,0),
		memory_total, memory_used, memory_free, memory_usage,
		COALESCE(disk_total,0), COALESCE(disk_used,0), COALESCE(disk_free,0), COALESCE(disk_usage,0),
		COALESCE(net_bytes_sent,0), COALESCE(net_bytes_recv,0),
		COALESCE(uptime,0), COALESCE(load_1,0), COALESCE(load_5,0), COALESCE(load_15,0)
	FROM system_stats
	WHERE hostname = ? AND timestamp >= ?
	ORDER BY timestamp ASC`, hostname, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStats(rows)
}

func (s *Server) GetLatestStatsByHost(hostname string) (*SystemStats, error) {
	var sStat SystemStats
	var ts string
	err := s.db.QueryRow(`
	SELECT id, COALESCE(hostname,''), timestamp,
		cpu_usage, COALESCE(cpu_cores,0),
		memory_total, memory_used, memory_free, memory_usage,
		COALESCE(disk_total,0), COALESCE(disk_used,0), COALESCE(disk_free,0), COALESCE(disk_usage,0),
		COALESCE(net_bytes_sent,0), COALESCE(net_bytes_recv,0),
		COALESCE(uptime,0), COALESCE(load_1,0), COALESCE(load_5,0), COALESCE(load_15,0)
	FROM system_stats
	WHERE hostname = ?
	ORDER BY timestamp DESC
	LIMIT 1`, hostname).Scan(
		&sStat.ID, &sStat.Hostname, &ts,
		&sStat.CPUUsage, &sStat.CPUCores,
		&sStat.MemoryTotal, &sStat.MemoryUsed, &sStat.MemoryFree, &sStat.MemoryUsage,
		&sStat.DiskTotal, &sStat.DiskUsed, &sStat.DiskFree, &sStat.DiskUsage,
		&sStat.NetBytesSent, &sStat.NetBytesRecv,
		&sStat.Uptime, &sStat.Load1, &sStat.Load5, &sStat.Load15,
	)
	if err != nil {
		return nil, err
	}
	sStat.Timestamp = parseTime(ts)
	return &sStat, nil
}

func (s *Server) GetLatestStatsForAllHosts() ([]SystemStats, error) {
	rows, err := s.db.Query(`
	SELECT ss.id, COALESCE(ss.hostname,''), ss.timestamp,
		ss.cpu_usage, COALESCE(ss.cpu_cores,0),
		ss.memory_total, ss.memory_used, ss.memory_free, ss.memory_usage,
		COALESCE(ss.disk_total,0), COALESCE(ss.disk_used,0), COALESCE(ss.disk_free,0), COALESCE(ss.disk_usage,0),
		COALESCE(ss.net_bytes_sent,0), COALESCE(ss.net_bytes_recv,0),
		COALESCE(ss.uptime,0), COALESCE(ss.load_1,0), COALESCE(ss.load_5,0), COALESCE(ss.load_15,0)
	FROM system_stats ss
	JOIN (
		SELECT hostname, MAX(timestamp) as max_ts
		FROM system_stats
		WHERE hostname != ''
		GROUP BY hostname
	) latest ON ss.hostname = latest.hostname AND ss.timestamp = latest.max_ts
	ORDER BY ss.hostname ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStats(rows)
}

func (s *Server) GetDistinctHostnames() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT hostname FROM system_stats WHERE hostname != '' ORDER BY hostname ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hosts []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

func scanStats(rows *sql.Rows) ([]SystemStats, error) {
	var stats []SystemStats
	for rows.Next() {
		var s SystemStats
		var ts string
		err := rows.Scan(
			&s.ID, &s.Hostname, &ts,
			&s.CPUUsage, &s.CPUCores,
			&s.MemoryTotal, &s.MemoryUsed, &s.MemoryFree, &s.MemoryUsage,
			&s.DiskTotal, &s.DiskUsed, &s.DiskFree, &s.DiskUsage,
			&s.NetBytesSent, &s.NetBytesRecv,
			&s.Uptime, &s.Load1, &s.Load5, &s.Load15,
		)
		if err != nil {
			return nil, err
		}
		s.Timestamp = parseTime(ts)
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// ---------- Local Collection ----------

func (s *Server) CollectStats() (*SystemStats, error) {
	m, err := sysinfo.Collect()
	if err != nil {
		return nil, err
	}
	return mapMetrics(m), nil
}

func mapMetrics(m *sysinfo.Metrics) *SystemStats {
	return &SystemStats{
		Hostname:     m.Hostname,
		Timestamp:    time.Now(),
		CPUUsage:     m.CPUUsage,
		CPUCores:     m.CPUCores,
		MemoryTotal:  m.MemoryTotal,
		MemoryUsed:   m.MemoryUsed,
		MemoryFree:   m.MemoryFree,
		MemoryUsage:  m.MemoryUsage,
		DiskTotal:    m.DiskTotal,
		DiskUsed:     m.DiskUsed,
		DiskFree:     m.DiskFree,
		DiskUsage:    m.DiskUsage,
		NetBytesSent: m.NetBytesSent,
		NetBytesRecv: m.NetBytesRecv,
		Uptime:       m.Uptime,
		Load1:        m.Load1,
		Load5:        m.Load5,
		Load15:       m.Load15,
	}
}

func (s *Server) collectOnce() {
	// Local agent collection
	stats, err := s.CollectStats()
	if err != nil {
		log.Printf("collect stats error: %v", err)
	} else if err := s.InsertStats(stats); err != nil {
		log.Printf("insert stats error: %v", err)
	}

	// SSH server collection
	for _, sshServer := range s.ListSSH() {
		_, err := s.collectAndStoreSSHStats(sshServer)
		if err != nil {
			log.Printf("ssh collect %s: %v", sshServer.Name, err)
		}
	}
}

// ---------- Built-in Agent Loop ----------

func (s *Server) startAgentCollection(ctx context.Context) {
	interval := s.config.CollectionInterval
	if interval <= 0 {
		interval = 3
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	s.collectOnce()

	for {
		select {
		case <-ticker.C:
			s.collectOnce()
		case <-ctx.Done():
			return
		}
	}
}

// ---------- Response types ----------

type StatsResponse struct {
	Current *SystemStats  `json:"current"`
	History []SystemStats `json:"history"`
}

type HostStats struct {
	Hostname string       `json:"hostname"`
	Stats    *SystemStats `json:"stats"`
}

type AllHostsStatsResponse struct {
	Hosts []HostStats `json:"hosts"`
}

// ---------- HTTP Handlers ----------

func (s *Server) MonitorView(w http.ResponseWriter, r *http.Request) {
	allStats, err := s.GetLatestStatsForAllHosts()
	if err != nil {
		allStats = []SystemStats{}
	}

	var hostStats []HostStats
	for _, st := range allStats {
		stat := st
		hostStats = append(hostStats, HostStats{Hostname: st.Hostname, Stats: &stat})
	}
	if hostStats == nil {
		hostStats = []HostStats{}
	}

	s.Render(w, "monitor", H{"hosts": hostStats})
}

func (s *Server) StatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	duration := time.Hour
	if d := r.URL.Query().Get("duration"); d != "" {
		if parsed, err := time.ParseDuration(d); err == nil {
			duration = parsed
		}
	}

	hostname := r.URL.Query().Get("hostname")
	history := []SystemStats{}
	var err error

	if hostname != "" {
		history, err = s.GetRecentStatsByHost(hostname, duration)
		if err != nil {
			history = []SystemStats{}
		}
	} else {
		history, err = s.GetRecentStats(duration)
		if err != nil {
			history = []SystemStats{}
		}
	}

	json.NewEncoder(w).Encode(StatsResponse{History: history})
}

func (s *Server) CurrentStatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	stats, err := s.CollectStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) AllHostsStatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	allStats, err := s.GetLatestStatsForAllHosts()
	if err != nil {
		json.NewEncoder(w).Encode(AllHostsStatsResponse{Hosts: []HostStats{}})
		return
	}

	var result []HostStats
	for _, st := range allStats {
		stat := st
		result = append(result, HostStats{Hostname: st.Hostname, Stats: &stat})
	}
	if result == nil {
		result = []HostStats{}
	}

	json.NewEncoder(w).Encode(AllHostsStatsResponse{Hosts: result})
}

// ---------- Telegraf ingestion handler ----------

func (s *Server) TelegrafHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	batch, err := telegraf.ParseMetrics(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	stats := batchToStats(batch)
	if stats == nil {
		http.Error(w, "hostname not found in telegraf metrics", http.StatusBadRequest)
		return
	}

	if err := s.InsertStats(stats); err != nil {
		log.Printf("telegraf insert error: %v", err)
		http.Error(w, "failed to store stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func batchToStats(batch *telegraf.Batch) *SystemStats {
	hostname := ""
	stats := &SystemStats{Timestamp: time.Now()}

	for _, m := range batch.Metrics {
		if h := m.Tags["host"]; h != "" {
			hostname = h
		}
		switch m.Name {
		case "cpu":
			if m.Tags["cpu"] != "" && m.Tags["cpu"] != "cpu-total" {
				continue
			}
			if v, ok := telegraf.ToFloat64(m.Fields["usage_active"]); ok {
				stats.CPUUsage = v
			} else if v, ok := telegraf.ToFloat64(m.Fields["usage_idle"]); ok {
				stats.CPUUsage = 100 - v
			}
		case "mem":
			if v, ok := telegraf.ToFloat64(m.Fields["used_percent"]); ok {
				stats.MemoryUsage = v
			}
			if v, ok := telegraf.ToUint64(m.Fields["total"]); ok {
				stats.MemoryTotal = v
			}
			if v, ok := telegraf.ToUint64(m.Fields["used"]); ok {
				stats.MemoryUsed = v
			}
			if v, ok := telegraf.ToUint64(m.Fields["free"]); ok {
				stats.MemoryFree = v
			}
		case "disk":
			if v, ok := telegraf.ToFloat64(m.Fields["used_percent"]); ok {
				stats.DiskUsage = v
			}
			if v, ok := telegraf.ToUint64(m.Fields["total"]); ok {
				stats.DiskTotal = v
			}
			if v, ok := telegraf.ToUint64(m.Fields["used"]); ok {
				stats.DiskUsed = v
			}
			if v, ok := telegraf.ToUint64(m.Fields["free"]); ok {
				stats.DiskFree = v
			}
		case "net":
			if m.Tags["interface"] == "" {
				continue
			}
			if v, ok := telegraf.ToUint64(m.Fields["bytes_sent"]); ok {
				stats.NetBytesSent = v
			}
			if v, ok := telegraf.ToUint64(m.Fields["bytes_recv"]); ok {
				stats.NetBytesRecv = v
			}
		case "system":
			if v, ok := telegraf.ToUint64(m.Fields["uptime"]); ok {
				stats.Uptime = v
			}
			if v, ok := telegraf.ToFloat64(m.Fields["load1"]); ok {
				stats.Load1 = v
			}
			if v, ok := telegraf.ToFloat64(m.Fields["load5"]); ok {
				stats.Load5 = v
			}
			if v, ok := telegraf.ToFloat64(m.Fields["load15"]); ok {
				stats.Load15 = v
			}
		}
	}

	if hostname == "" {
		return nil
	}
	stats.Hostname = hostname
	return stats
}

// ---------- History API ----------

func (s *Server) HostHistoryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	hostname := r.URL.Query().Get("hostname")
	if hostname == "" {
		http.Error(w, "hostname required", http.StatusBadRequest)
		return
	}

	duration := time.Hour
	if d := r.URL.Query().Get("duration"); d != "" {
		if parsed, err := time.ParseDuration(d); err == nil {
			duration = parsed
		}
	}

	history, err := s.GetRecentStatsByHost(hostname, duration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	current, _ := s.GetLatestStatsByHost(hostname)

	json.NewEncoder(w).Encode(StatsResponse{
		Current: current,
		History: history,
	})
}

// ---------- Detail page ----------

func (s *Server) MonitorDetailView(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	if hostname == "" {
		http.Error(w, "hostname required", http.StatusBadRequest)
		return
	}

	stats, err := s.GetLatestStatsByHost(hostname)
	if err != nil {
		stats = nil
	}

	s.Render(w, "monitor/detail", H{
		"hostname": hostname,
		"stats":    stats,
	})
}

const statsRetentionDays = 7

func (s *Server) startStatsCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			deleted, err := s.db.Exec(`DELETE FROM system_stats WHERE timestamp < ?`,
				time.Now().Add(-time.Duration(statsRetentionDays)*24*time.Hour))
			if err != nil {
				log.Printf("stats cleanup error: %v", err)
			} else if n, _ := deleted.RowsAffected(); n > 0 {
				log.Printf("cleaned up %d old stats records", n)
				s.db.Exec(`VACUUM`)
			}
		case <-ctx.Done():
			return
		}
	}
}

func init() {
	RegisterInitHook(func(s *Server) error {
		go s.startAgentCollection(context.Background())
		go s.startStatsCleanup(context.Background())
		return nil
	})
}
