package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lsongdev/idrac-go/idrac"
)

// IDRACServer represents a Dell iDRAC server
type IDRACServer struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	Password    string    `json:"-"`       // Never serialize
	Version     string    `json:"version"` // iDRAC version (7, 8, 9, etc.)
	Status      string    `json:"status"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	client      *idrac.Client
}

// IDRACSystemInfo represents system information from iDRAC
type IDRACSystemInfo struct {
	Model            string
	AssetTag         string
	SerialNumber     string
	HostName         string
	PowerState       string
	BiosVersion      string
	MemoryGiB        float64
	ProcessorCount   int
	ProcessorModel   string
	ProcessorThreads int
	Manufacturer     string
	Name             string
}

// IDRACSensor represents a hardware sensor
type IDRACSensor struct {
	Name         string
	Reading      float64
	ReadingUnits string
	State        string
	Health       string
}

// initIDRACSchema creates the iDRAC server table
func (s *Server) initIDRACSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS idrac_servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		host TEXT NOT NULL,
		port INTEGER DEFAULT 443,
		username TEXT NOT NULL,
		password TEXT NOT NULL,
		version TEXT NOT NULL DEFAULT '9',
		status TEXT NOT NULL DEFAULT 'active',
		description TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_idrac_servers_name ON idrac_servers(name);
	CREATE INDEX IF NOT EXISTS idx_idrac_servers_host ON idrac_servers(host);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateIDRACServer creates a new iDRAC server
func (s *Server) CreateIDRACServer(name, host, username, password, version, description string, port int) (*IDRACServer, error) {

	now := time.Now()
	query := `
	INSERT INTO idrac_servers (name, host, port, username, password, version, status, description, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, 'active', ?, ?, ?)
	`
	result, err := s.db.Exec(query, name, host, port, username, password, version, description, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &IDRACServer{
		ID:          id,
		Name:        name,
		Host:        host,
		Port:        port,
		Username:    username,
		Version:     version,
		Status:      "active",
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetAllIDRACServers retrieves all iDRAC servers
func (s *Server) GetAllIDRACServers() ([]IDRACServer, error) {

	query := `
	SELECT id, name, host, port, username, version, status, description, created_at, updated_at
	FROM idrac_servers
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []IDRACServer
	for rows.Next() {
		var server IDRACServer
		var createdAt, updatedAt string
		if err := rows.Scan(&server.ID, &server.Name, &server.Host, &server.Port, &server.Username, &server.Version, &server.Status, &server.Description, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		server.CreatedAt = parseTime(createdAt)
		server.UpdatedAt = parseTime(updatedAt)
		servers = append(servers, server)
	}

	return servers, rows.Err()
}

// GetIDRACServerByID retrieves an iDRAC server by ID
func (s *Server) GetIDRACServerByID(id int64) (*IDRACServer, error) {

	query := `
	SELECT id, name, host, port, username, password, version, status, description, created_at, updated_at
	FROM idrac_servers
	WHERE id = ?
	`

	var server IDRACServer
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, id).Scan(&server.ID, &server.Name, &server.Host, &server.Port, &server.Username, &server.Password, &server.Version, &server.Status, &server.Description, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	server.CreatedAt = parseTime(createdAt)
	server.UpdatedAt = parseTime(updatedAt)

	return &server, nil
}

// UpdateIDRACServer updates an existing iDRAC server
func (s *Server) UpdateIDRACServer(id int64, name, host, username, password, version, description string, port int) error {

	now := time.Now()
	query := `
	UPDATE idrac_servers
	SET name = ?, host = ?, port = ?, username = ?, password = ?, version = ?, description = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, name, host, port, username, password, version, description, now, id)
	return err
}

// DeleteIDRACServer deletes an iDRAC server by ID
func (s *Server) DeleteIDRACServer(id int64) error {

	query := `DELETE FROM idrac_servers WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// Connect establishes connection to iDRAC
func (s *IDRACServer) Connect(ctx context.Context) error {
	client, err := idrac.NewClient(&idrac.Config{
		Host:     s.Host,
		Username: s.Username,
		Password: s.Password,
	})
	if err != nil {
		return fmt.Errorf("failed to create iDRAC client: %w", err)
	}

	s.client = client
	return nil
}

// Disconnect closes connection to iDRAC
func (s *IDRACServer) Disconnect(ctx context.Context) error {
	s.client = nil
	return nil
}

// GetSystemInfo retrieves system information from iDRAC
func (s *IDRACServer) GetSystemInfo(ctx context.Context) (*IDRACSystemInfo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to iDRAC")
	}

	sysInfo, err := s.client.GetSystemInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get system info: %w", err)
	}

	return &IDRACSystemInfo{
		Model:            sysInfo.Model,
		AssetTag:         sysInfo.AssetTag,
		SerialNumber:     sysInfo.SerialNumber,
		HostName:         sysInfo.HostName,
		PowerState:       sysInfo.PowerState,
		BiosVersion:      sysInfo.BiosVersion,
		MemoryGiB:        sysInfo.MemoryGiB,
		ProcessorCount:   sysInfo.ProcessorSummary.Count,
		ProcessorModel:   sysInfo.ProcessorSummary.Model,
		ProcessorThreads: sysInfo.ProcessorSummary.LogicalCount,
		Manufacturer:     sysInfo.Manufacturer,
		Name:             sysInfo.Name,
	}, nil
}

// GetPowerState retrieves the current power state
func (s *IDRACServer) GetPowerState(ctx context.Context) (string, error) {
	if s.client == nil {
		return "", fmt.Errorf("not connected to iDRAC")
	}

	return s.client.GetPowerState()
}

// GetHardwareStatus retrieves hardware health information
func (s *IDRACServer) GetHardwareStatus(ctx context.Context) ([]IDRACSensor, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to iDRAC")
	}

	sensors, err := s.client.GetHardwareStatus()
	if err != nil {
		return nil, fmt.Errorf("failed to get hardware status: %w", err)
	}

	result := make([]IDRACSensor, 0, len(sensors))
	for _, sensor := range sensors {
		result = append(result, IDRACSensor{
			Name:         sensor.Name,
			Reading:      sensor.Reading,
			ReadingUnits: sensor.ReadingUnits,
			State:        sensor.State,
			Health:       sensor.Health,
		})
	}

	return result, nil
}

// ControlPower changes the power state of the system
func (s *IDRACServer) ControlPower(ctx context.Context, action idrac.PowerState) error {
	if s.client == nil {
		return fmt.Errorf("not connected to iDRAC")
	}

	return s.client.SetPowerState(action)
}

// TestConnection tests the connection to iDRAC
func (s *IDRACServer) TestConnection(ctx context.Context) error {
	if s.client == nil {
		return fmt.Errorf("not connected")
	}

	_, err := s.client.GetSystemInfo()
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}

	return nil
}

// IDRACListView renders the iDRAC server list page
func (s *Server) IDRACListView(w http.ResponseWriter, r *http.Request) {
	servers, err := s.GetAllIDRACServers()
	if err != nil {
		log.Printf("Error fetching iDRAC servers: %v", err)
		s.Error(w, err)
		return
	}

	s.Render(w, "idrac/index", H{
		"servers": servers,
	})
}

// IDRACNewHandler handles creating a new iDRAC server
func (s *Server) IDRACNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "idrac/new", H{})
		return
	}

	// POST: Create new iDRAC server
	name := r.FormValue("name")
	host := r.FormValue("host")
	username := r.FormValue("username")
	password := r.FormValue("password")
	version := r.FormValue("version")
	description := r.FormValue("description")
	port := 443

	if portStr := r.FormValue("port"); portStr != "" {
		var err error
		port, err = strconv.Atoi(portStr)
		if err != nil {
			port = 443
		}
	}

	if name == "" || host == "" || username == "" || password == "" {
		s.Render(w, "idrac/new", H{
			"error":       "Name, host, username, and password are required",
			"name":        name,
			"host":        host,
			"username":    username,
			"version":     version,
			"description": description,
			"port":        port,
		})
		return
	}

	_, err := s.CreateIDRACServer(name, host, username, password, version, description, port)
	if err != nil {
		s.Render(w, "idrac/new", H{
			"error":       "Failed to create iDRAC server: " + err.Error(),
			"name":        name,
			"host":        host,
			"username":    username,
			"version":     version,
			"description": description,
			"port":        port,
		})
		return
	}

	http.Redirect(w, r, "/idrac", http.StatusSeeOther)
}

// IDRACEditHandler handles editing an iDRAC server
func (s *Server) IDRACEditHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetIDRACServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "iDRAC server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	if r.Method == http.MethodGet {
		s.Render(w, "idrac/new", H{
			"server": server,
		})
		return
	}

	// POST: Update iDRAC server
	name := r.FormValue("name")
	host := r.FormValue("host")
	username := r.FormValue("username")
	password := r.FormValue("password")
	version := r.FormValue("version")
	description := r.FormValue("description")
	port := 443

	if portStr := r.FormValue("port"); portStr != "" {
		var err error
		port, err = strconv.Atoi(portStr)
		if err != nil {
			port = 443
		}
	}

	// If password is empty, keep the old one
	if password == "" {
		password = server.Password
	}

	err = s.UpdateIDRACServer(id, name, host, username, password, version, description, port)
	if err != nil {
		s.Render(w, "idrac/new", H{
			"error":  "Failed to update iDRAC server: " + err.Error(),
			"server": server,
		})
		return
	}

	http.Redirect(w, r, "/idrac", http.StatusSeeOther)
}

// IDRACDetailHandler handles viewing/deleting an iDRAC server
func (s *Server) IDRACDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetIDRACServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "iDRAC server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	// Check for method override
	method := r.Method
	if r.Method == http.MethodPost {
		overrideMethod := r.FormValue("_method")
		if overrideMethod != "" {
			method = overrideMethod
		}
	}

	switch method {
	case http.MethodGet:
		// Connect and fetch system information on-demand
		ctx := context.Background()
		var systemInfo *IDRACSystemInfo
		var powerState string
		var sensors []IDRACSensor
		// var connectionError string

		err = server.Connect(ctx)
		if err != nil {
			s.Error(w, err)
			return
		}

		systemInfo, _ = server.GetSystemInfo(ctx)
		powerState, _ = server.GetPowerState(ctx)
		sensors, _ = server.GetHardwareStatus(ctx)
		defer server.Disconnect(ctx)
		// Build sensor data for template
		// Group sensors by type (temperature, fan, voltage, etc.)
		cpuTempSensors := make([]IDRACSensor, 0)
		otherTempSensors := make([]IDRACSensor, 0)
		fanSensors := make([]IDRACSensor, 0)
		otherSensors := make([]IDRACSensor, 0)

		for _, sensor := range sensors {
			// Group by sensor type based on name or units
			name := sensor.Name
			if sensor.ReadingUnits == "Celsius" {
				if containsAny(name, []string{"CPU", "Proc"}) {
					cpuTempSensors = append(cpuTempSensors, sensor)
				} else {
					otherTempSensors = append(otherTempSensors, sensor)
				}
			} else if sensor.ReadingUnits == "RPM" || containsAny(name, []string{"Fan", "Tach"}) {
				fanSensors = append(fanSensors, sensor)
			} else {
				otherSensors = append(otherSensors, sensor)
			}
		}

		s.Render(w, "idrac/detail", H{
			"server":           server,
			"system":           systemInfo,
			"powerState":       powerState,
			"cpuTempSensors":   cpuTempSensors,
			"otherTempSensors": otherTempSensors,
			"fanSensors":       fanSensors,
			"otherSensors":     otherSensors,
		})
	case http.MethodPut:
		// Update iDRAC server
		name := r.FormValue("name")
		host := r.FormValue("host")
		username := r.FormValue("username")
		password := r.FormValue("password")
		version := r.FormValue("version")
		description := r.FormValue("description")
		port := 443

		if portStr := r.FormValue("port"); portStr != "" {
			var err error
			port, err = strconv.Atoi(portStr)
			if err != nil {
				port = 443
			}
		}

		// If password is empty, keep the old one
		if password == "" {
			password = server.Password
		}

		err = s.UpdateIDRACServer(id, name, host, username, password, version, description, port)
		if err != nil {
			s.Render(w, "idrac/detail", H{
				"error":  "Failed to update iDRAC server: " + err.Error(),
				"server": server,
			})
			return
		}

		http.Redirect(w, r, "/idrac", http.StatusSeeOther)

	case http.MethodDelete:
		err = s.DeleteIDRACServer(id)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Failed to delete iDRAC server"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("iDRAC server deleted successfully"))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// IDRACPowerControlHandler handles power control operations
func (s *Server) IDRACPowerControlHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetIDRACServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "iDRAC server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	action := r.FormValue("action")
	if action == "" {
		s.Error(w, &Error{Message: "Power action is required"})
		return
	}

	var powerAction idrac.PowerState
	switch action {
	case "on":
		powerAction = idrac.PowerOn
	case "off":
		powerAction = idrac.PowerOff
	case "graceful":
		powerAction = idrac.PowerGracefulShutdown
	case "restart":
		powerAction = idrac.PowerRestart
	case "pushbutton":
		powerAction = idrac.PowerPushPowerButton
	default:
		s.Error(w, &Error{Message: "Unknown power action: " + action})
		return
	}

	ctx := context.Background()
	if err := server.Connect(ctx); err != nil {
		s.Error(w, &Error{Message: "Failed to connect to iDRAC: " + err.Error()})
		return
	}
	defer server.Disconnect(ctx)

	if err := server.ControlPower(ctx, powerAction); err != nil {
		s.Error(w, &Error{Message: "Failed to control power: " + err.Error()})
		return
	}

	// Redirect back to detail page
	http.Redirect(w, r, fmt.Sprintf("/idrac/%d", id), http.StatusSeeOther)
}

// containsAny checks if a string contains any of the substrings
func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func init() {
	RegisterInitSchemaHook((*Server).initIDRACSchema)
}
