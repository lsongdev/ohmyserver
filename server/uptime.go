package server

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// MonitorType represents the type of uptime check
type MonitorType string

const (
	MonitorTypePing MonitorType = "ping"
	MonitorTypeTCP  MonitorType = "tcp"
	MonitorTypeHTTP MonitorType = "http"
)

// Monitor represents a remote target to monitor
type Monitor struct {
	ID             int64       `json:"id"`
	Name           string      `json:"name"`
	Type           MonitorType `json:"type"`
	Target         string      `json:"target"`
	Port           int         `json:"port"`
	Interval       int         `json:"interval"`        // seconds
	Timeout        int         `json:"timeout"`         // seconds
	ExpectedStatus int         `json:"expected_status"` // for HTTP checks
	Enabled        bool        `json:"enabled"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// MonitorCheck represents the result of a single check
type MonitorCheck struct {
	ID           int64     `json:"id"`
	MonitorID    int64     `json:"monitor_id"`
	Status       string    `json:"status"`        // up, down
	ResponseTime int64     `json:"response_time"` // ms
	StatusCode   int       `json:"status_code"`
	Message      string    `json:"message"`
	CheckedAt    time.Time `json:"checked_at"`
}

// MonitorWithStatus includes the latest check result
type MonitorWithStatus struct {
	Monitor
	LastCheck *MonitorCheck `json:"last_check"`
	UptimePct float64       `json:"uptime_pct"`
}

// MonitorCheckRequest is used to manually trigger a check
type MonitorCheckRequest struct {
	MonitorID int64 `json:"monitor_id"`
}

// initMonitorSchema creates monitor tables
func (s *Server) initMonitorSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS monitors (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT 'http',
		target TEXT NOT NULL,
		port INTEGER DEFAULT 0,
		interval INTEGER NOT NULL DEFAULT 60,
		timeout INTEGER NOT NULL DEFAULT 10,
		expected_status INTEGER DEFAULT 200,
		enabled INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_monitors_enabled ON monitors(enabled);

	CREATE TABLE IF NOT EXISTS monitor_checks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		monitor_id INTEGER NOT NULL,
		status TEXT NOT NULL,
		response_time INTEGER DEFAULT 0,
		status_code INTEGER DEFAULT 0,
		message TEXT DEFAULT '',
		checked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (monitor_id) REFERENCES monitors(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_monitor_checks_monitor_id ON monitor_checks(monitor_id);
	CREATE INDEX IF NOT EXISTS idx_monitor_checks_checked_at ON monitor_checks(checked_at);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateMonitor creates a new monitor
func (s *Server) CreateMonitor(m *Monitor) (*Monitor, error) {

	now := time.Now()
	query := `
	INSERT INTO monitors (name, type, target, port, interval, timeout, expected_status, enabled, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, m.Name, m.Type, m.Target, m.Port, m.Interval, m.Timeout, m.ExpectedStatus, boolToInt(m.Enabled), now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	m.ID = id
	m.CreatedAt = now
	m.UpdatedAt = now
	return m, nil
}

// GetAllMonitors retrieves all monitors
func (s *Server) GetAllMonitors() ([]Monitor, error) {

	query := `
	SELECT id, name, type, target, port, interval, timeout, expected_status, enabled, created_at, updated_at
	FROM monitors
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var monitors []Monitor
	for rows.Next() {
		var m Monitor
		var createdAt, updatedAt string
		var enabled int
		err := rows.Scan(&m.ID, &m.Name, &m.Type, &m.Target, &m.Port, &m.Interval, &m.Timeout, &m.ExpectedStatus, &enabled, &createdAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		m.Enabled = enabled == 1
		m.CreatedAt = parseTime(createdAt)
		m.UpdatedAt = parseTime(updatedAt)
		monitors = append(monitors, m)
	}

	return monitors, rows.Err()
}

// GetEnabledMonitors retrieves only enabled monitors
func (s *Server) GetEnabledMonitors() ([]Monitor, error) {

	query := `
	SELECT id, name, type, target, port, interval, timeout, expected_status, enabled, created_at, updated_at
	FROM monitors
	WHERE enabled = 1
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var monitors []Monitor
	for rows.Next() {
		var m Monitor
		var createdAt, updatedAt string
		var enabled int
		err := rows.Scan(&m.ID, &m.Name, &m.Type, &m.Target, &m.Port, &m.Interval, &m.Timeout, &m.ExpectedStatus, &enabled, &createdAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		m.Enabled = enabled == 1
		m.CreatedAt = parseTime(createdAt)
		m.UpdatedAt = parseTime(updatedAt)
		monitors = append(monitors, m)
	}

	return monitors, rows.Err()
}

// GetMonitorByID retrieves a monitor by ID
func (s *Server) GetMonitorByID(id int64) (*Monitor, error) {

	query := `
	SELECT id, name, type, target, port, interval, timeout, expected_status, enabled, created_at, updated_at
	FROM monitors
	WHERE id = ?
	`

	var m Monitor
	var createdAt, updatedAt string
	var enabled int
	err := s.db.QueryRow(query, id).Scan(&m.ID, &m.Name, &m.Type, &m.Target, &m.Port, &m.Interval, &m.Timeout, &m.ExpectedStatus, &enabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	m.Enabled = enabled == 1
	m.CreatedAt = parseTime(createdAt)
	m.UpdatedAt = parseTime(updatedAt)
	return &m, nil
}

// UpdateMonitor updates a monitor
func (s *Server) UpdateMonitor(m *Monitor) error {

	now := time.Now()
	query := `
	UPDATE monitors
	SET name = ?, type = ?, target = ?, port = ?, interval = ?, timeout = ?, expected_status = ?, enabled = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, m.Name, m.Type, m.Target, m.Port, m.Interval, m.Timeout, m.ExpectedStatus, boolToInt(m.Enabled), now, m.ID)
	return err
}

// DeleteMonitor deletes a monitor
func (s *Server) DeleteMonitor(id int64) error {

	query := `DELETE FROM monitors WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// ToggleMonitor enables/disables a monitor
func (s *Server) ToggleMonitor(id int64, enabled bool) error {

	now := time.Now()
	query := `UPDATE monitors SET enabled = ?, updated_at = ? WHERE id = ?`
	_, err := s.db.Exec(query, boolToInt(enabled), now, id)
	return err
}

// InsertMonitorCheck saves a check result
func (s *Server) InsertMonitorCheck(check *MonitorCheck) error {

	query := `
	INSERT INTO monitor_checks (monitor_id, status, response_time, status_code, message, checked_at)
	VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, check.MonitorID, check.Status, check.ResponseTime, check.StatusCode, check.Message, check.CheckedAt)
	return err
}

// GetLatestMonitorCheck gets the most recent check for a monitor
func (s *Server) GetLatestMonitorCheck(monitorID int64) (*MonitorCheck, error) {

	query := `
	SELECT id, monitor_id, status, response_time, status_code, message, checked_at
	FROM monitor_checks
	WHERE monitor_id = ?
	ORDER BY checked_at DESC
	LIMIT 1
	`

	var c MonitorCheck
	var checkedAt string
	err := s.db.QueryRow(query, monitorID).Scan(&c.ID, &c.MonitorID, &c.Status, &c.ResponseTime, &c.StatusCode, &c.Message, &checkedAt)
	if err != nil {
		return nil, err
	}
	c.CheckedAt = parseTime(checkedAt)
	return &c, nil
}

// GetMonitorChecks retrieves recent checks for a monitor
func (s *Server) GetMonitorChecks(monitorID int64, limit int) ([]MonitorCheck, error) {

	query := `
	SELECT id, monitor_id, status, response_time, status_code, message, checked_at
	FROM monitor_checks
	WHERE monitor_id = ?
	ORDER BY checked_at DESC
	LIMIT ?
	`

	rows, err := s.db.Query(query, monitorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []MonitorCheck
	for rows.Next() {
		var c MonitorCheck
		var checkedAt string
		err := rows.Scan(&c.ID, &c.MonitorID, &c.Status, &c.ResponseTime, &c.StatusCode, &c.Message, &checkedAt)
		if err != nil {
			return nil, err
		}
		c.CheckedAt = parseTime(checkedAt)
		checks = append(checks, c)
	}

	return checks, rows.Err()
}

// GetMonitorUptime calculates uptime percentage over the last N hours
func (s *Server) GetMonitorUptime(monitorID int64, hours int) (float64, error) {

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	query := `
	SELECT COUNT(*) as total, SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END) as up_count
	FROM monitor_checks
	WHERE monitor_id = ? AND checked_at >= ?
	`

	var total, up int64
	err := s.db.QueryRow(query, monitorID, since).Scan(&total, &up)
	if err != nil {
		return 0, err
	}
	if total == 0 {
		return 100, nil
	}
	return float64(up) / float64(total) * 100, nil
}

// CleanupOldMonitorChecks removes checks older than retention period
func (s *Server) CleanupOldMonitorChecks(days int) error {

	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	query := `DELETE FROM monitor_checks WHERE checked_at < ?`
	_, err := s.db.Exec(query, since)
	return err
}

// Check execution

// runPingCheck executes a ping check using system ping command
func runPingCheck(target string, timeoutSec int) (*MonitorCheck, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("ping", "-n", "1", "-w", strconv.Itoa(timeoutSec*1000), target)
	} else if runtime.GOOS == "darwin" {
		cmd = exec.Command("ping", "-c", "1", "-W", strconv.Itoa(timeoutSec*1000), target)
	} else {
		// Linux
		cmd = exec.Command("ping", "-c", "1", "-W", strconv.Itoa(timeoutSec), target)
	}

	start := time.Now()
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	check := &MonitorCheck{
		Status:       "down",
		ResponseTime: duration.Milliseconds(),
		CheckedAt:    time.Now(),
	}

	if err != nil {
		check.Message = fmt.Sprintf("ping failed: %v", err)
		return check, nil
	}

	outputStr := string(output)

	// Parse response time from ping output
	// macOS/Linux: time=12.3 ms
	// Windows: time=12ms or time=12.3ms
	re := regexp.MustCompile(`time[=<>]([0-9.]+)\s*ms`)
	matches := re.FindStringSubmatch(outputStr)
	if len(matches) > 1 {
		if rt, err := strconv.ParseFloat(matches[1], 64); err == nil {
			check.ResponseTime = int64(rt)
		}
	}

	// Check for packet loss indicators
	if strings.Contains(outputStr, "100.0% packet loss") ||
		strings.Contains(outputStr, "100% packet loss") ||
		strings.Contains(outputStr, "Request timed out") ||
		strings.Contains(outputStr, "0 packets received") {
		check.Message = "Host unreachable"
		return check, nil
	}

	// On macOS, if ping succeeds but host is unknown
	if strings.Contains(outputStr, "unknown host") || strings.Contains(outputStr, "Name or service not known") {
		check.Message = "Unknown host"
		return check, nil
	}

	check.Status = "up"
	check.Message = fmt.Sprintf("Ping %dms", check.ResponseTime)
	return check, nil
}

// runTCPCheck checks if a TCP port is open
func runTCPCheck(target string, port int, timeoutSec int) (*MonitorCheck, error) {
	address := net.JoinHostPort(target, strconv.Itoa(port))
	if port == 0 {
		address = target
	}

	start := time.Now()
	conn, err := net.DialTimeout("tcp", address, time.Duration(timeoutSec)*time.Second)
	duration := time.Since(start)

	check := &MonitorCheck{
		ResponseTime: duration.Milliseconds(),
		CheckedAt:    time.Now(),
	}

	if err != nil {
		check.Status = "down"
		check.Message = err.Error()
		return check, nil
	}
	defer conn.Close()

	check.Status = "up"
	check.Message = fmt.Sprintf("Connected in %dms", check.ResponseTime)
	return check, nil
}

// runHTTPCheck performs an HTTP/HTTPS request
func runHTTPCheck(target string, timeoutSec int, expectedStatus int) (*MonitorCheck, error) {
	// Ensure target has scheme
	checkURL := target
	if !strings.HasPrefix(checkURL, "http://") && !strings.HasPrefix(checkURL, "https://") {
		checkURL = "http://" + checkURL
	}

	client := &http.Client{
		Timeout: time.Duration(timeoutSec) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	start := time.Now()
	resp, err := client.Get(checkURL)
	duration := time.Since(start)

	check := &MonitorCheck{
		ResponseTime: duration.Milliseconds(),
		CheckedAt:    time.Now(),
	}

	if err != nil {
		check.Status = "down"
		if urlErr, ok := err.(*url.Error); ok && urlErr.Timeout() {
			check.Message = "Request timed out"
		} else {
			check.Message = err.Error()
		}
		return check, nil
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	check.StatusCode = resp.StatusCode

	if expectedStatus > 0 && resp.StatusCode != expectedStatus {
		check.Status = "down"
		check.Message = fmt.Sprintf("Unexpected status: %d (expected %d)", resp.StatusCode, expectedStatus)
		return check, nil
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		check.Status = "up"
		check.Message = fmt.Sprintf("HTTP %d in %dms", resp.StatusCode, check.ResponseTime)
	} else {
		check.Status = "down"
		check.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	return check, nil
}

// RunCheck executes a check for a monitor
func (s *Server) RunCheck(monitor *Monitor) (*MonitorCheck, error) {
	var check *MonitorCheck
	var err error

	switch monitor.Type {
	case MonitorTypePing:
		check, err = runPingCheck(monitor.Target, monitor.Timeout)
	case MonitorTypeTCP:
		check, err = runTCPCheck(monitor.Target, monitor.Port, monitor.Timeout)
	case MonitorTypeHTTP:
		check, err = runHTTPCheck(monitor.Target, monitor.Timeout, monitor.ExpectedStatus)
	default:
		return nil, fmt.Errorf("unknown monitor type: %s", monitor.Type)
	}

	if err != nil {
		return nil, err
	}

	check.MonitorID = monitor.ID
	return check, nil
}

// RunAllChecks runs checks for all enabled monitors
func (s *Server) RunAllChecks() {
	monitors, err := s.GetEnabledMonitors()
	if err != nil {
		return
	}

	for _, m := range monitors {
		check, err := s.RunCheck(&m)
		if err != nil {
			continue
		}
		_ = s.InsertMonitorCheck(check)
	}
}

// HTTP Handlers

// UptimeListView renders the uptime monitor list page
func (s *Server) UptimeListView(w http.ResponseWriter, r *http.Request) {
	monitors, err := s.GetAllMonitors()
	if err != nil {
		s.Error(w, err)
		return
	}

	var monitorsWithStatus []MonitorWithStatus
	for _, m := range monitors {
		mws := MonitorWithStatus{Monitor: m}
		lastCheck, _ := s.GetLatestMonitorCheck(m.ID)
		if lastCheck != nil {
			mws.LastCheck = lastCheck
		}
		uptime, _ := s.GetMonitorUptime(m.ID, 24)
		mws.UptimePct = uptime
		monitorsWithStatus = append(monitorsWithStatus, mws)
	}

	s.Render(w, "uptime/index", H{
		"monitors": monitorsWithStatus,
	})
}

// UptimeNewHandler handles creating a new monitor
func (s *Server) UptimeNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "uptime/new", H{})
		return
	}

	if err := ParseFormAll(r); err != nil {
		s.Error(w, err)
		return
	}

	interval, _ := strconv.Atoi(r.FormValue("interval"))
	if interval <= 0 {
		interval = 60
	}
	timeout, _ := strconv.Atoi(r.FormValue("timeout"))
	if timeout <= 0 {
		timeout = 10
	}
	port, _ := strconv.Atoi(r.FormValue("port"))
	expectedStatus, _ := strconv.Atoi(r.FormValue("expected_status"))
	if expectedStatus <= 0 {
		expectedStatus = 200
	}

	monitor := &Monitor{
		Name:           r.FormValue("name"),
		Type:           MonitorType(r.FormValue("type")),
		Target:         r.FormValue("target"),
		Port:           port,
		Interval:       interval,
		Timeout:        timeout,
		ExpectedStatus: expectedStatus,
		Enabled:        r.FormValue("enabled") == "on" || r.FormValue("enabled") == "1",
	}

	_, err := s.CreateMonitor(monitor)
	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/monitor/uptime", http.StatusSeeOther)
}

// UptimeDetailHandler handles viewing/editing a monitor
func (s *Server) UptimeDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	monitor, err := s.GetMonitorByID(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	if r.Method == http.MethodPost {
		if err := ParseFormAll(r); err != nil {
			s.Error(w, err)
			return
		}

		interval, _ := strconv.Atoi(r.FormValue("interval"))
		if interval <= 0 {
			interval = 60
		}
		timeout, _ := strconv.Atoi(r.FormValue("timeout"))
		if timeout <= 0 {
			timeout = 10
		}
		port, _ := strconv.Atoi(r.FormValue("port"))
		expectedStatus, _ := strconv.Atoi(r.FormValue("expected_status"))
		if expectedStatus <= 0 {
			expectedStatus = 200
		}

		monitor.Name = r.FormValue("name")
		monitor.Type = MonitorType(r.FormValue("type"))
		monitor.Target = r.FormValue("target")
		monitor.Port = port
		monitor.Interval = interval
		monitor.Timeout = timeout
		monitor.ExpectedStatus = expectedStatus
		monitor.Enabled = r.FormValue("enabled") == "on" || r.FormValue("enabled") == "1"

		if err := s.UpdateMonitor(monitor); err != nil {
			s.Error(w, err)
			return
		}

		http.Redirect(w, r, "/monitor/uptime", http.StatusSeeOther)
		return
	}

	checks, _ := s.GetMonitorChecks(id, 50)
	uptime24h, _ := s.GetMonitorUptime(id, 24)
	uptime7d, _ := s.GetMonitorUptime(id, 24*7)
	lastCheck, _ := s.GetLatestMonitorCheck(id)

	s.Render(w, "uptime/detail", H{
		"monitor":   monitor,
		"checks":    checks,
		"uptime24h": uptime24h,
		"uptime7d":  uptime7d,
		"lastCheck": lastCheck,
	})
}

// UptimeDeleteHandler deletes a monitor
func (s *Server) UptimeDeleteHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	if err := s.DeleteMonitor(id); err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/monitor/uptime", http.StatusSeeOther)
}

// UptimeToggleHandler enables/disables a monitor
func (s *Server) UptimeToggleHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	monitor, err := s.GetMonitorByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if err := s.ToggleMonitor(id, !monitor.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"enabled": !monitor.Enabled})
}

// UptimeCheckNowHandler triggers an immediate check
func (s *Server) UptimeCheckNowHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	monitor, err := s.GetMonitorByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	check, err := s.RunCheck(monitor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.InsertMonitorCheck(check)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(check)
}

// UptimeAPIListHandler returns all monitors as JSON
func (s *Server) UptimeAPIListHandler(w http.ResponseWriter, r *http.Request) {
	monitors, err := s.GetAllMonitors()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var result []MonitorWithStatus
	for _, m := range monitors {
		mws := MonitorWithStatus{Monitor: m}
		lastCheck, _ := s.GetLatestMonitorCheck(m.ID)
		if lastCheck != nil {
			mws.LastCheck = lastCheck
		}
		uptime, _ := s.GetMonitorUptime(m.ID, 24)
		mws.UptimePct = uptime
		result = append(result, mws)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// UptimeAPIChecksHandler returns check history for a monitor
func (s *Server) UptimeAPIChecksHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	checks, err := s.GetMonitorChecks(id, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(checks)
}

func init() {
	RegisterInitSchemaHook((*Server).initMonitorSchema)
}
