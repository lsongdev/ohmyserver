package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	ipmi "github.com/bougou/go-ipmi"
)

// IPMIServer represents an IPMI/BMC server
type IPMIServer struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	BmcIP       string    `json:"bmc_ip"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	Password    string    `json:"-"`      // Never serialize
	Status      string    `json:"status"` // active, inactive
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	client      *ipmi.Client
}

// IPMIHost represents host/bmc information
type IPMIHost struct {
	DeviceID          uint8
	DeviceRevision    uint8
	FirmwareRevision  string
	ManufacturerID    string
	ProductID         uint16
	AuxFirmwareRev    string
	PowerState        string
	LastPowerEvent    string
	ChassisIntrusion  string
	FrontPanelLockout string
	DriveFault        string
	CoolingFault      string
}

// Connect establishes connection to IPMI device
func (s *IPMIServer) Connect(ctx context.Context) error {
	client, err := ipmi.NewClient(s.BmcIP, s.Port, s.Username, s.Password)
	if err != nil {
		return fmt.Errorf("failed to create IPMI client: %w", err)
	}

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to IPMI: %w", err)
	}

	s.client = client
	return nil
}

// Disconnect closes connection to IPMI device
func (s *IPMIServer) Disconnect(ctx context.Context) error {
	if s.client != nil {
		// go-ipmi doesn't have explicit disconnect, client will be garbage collected
		s.client = nil
	}
	return nil
}

// GetHostInfo retrieves host/BMC information
func (s *IPMIServer) GetHostInfo(ctx context.Context) (*IPMIHost, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to IPMI device")
	}

	host := &IPMIHost{}

	// Get device ID
	deviceID, err := s.client.GetDeviceID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get device ID: %w", err)
	}

	host.DeviceID = deviceID.DeviceID
	host.DeviceRevision = deviceID.DeviceRevision
	host.FirmwareRevision = deviceID.FirmwareVersionStr()
	host.ManufacturerID = fmt.Sprintf("%d", deviceID.ManufacturerID)
	host.ProductID = deviceID.ProductID
	if len(deviceID.AuxiliaryFirmwareRevision) == 4 {
		host.AuxFirmwareRev = fmt.Sprintf("%02x.%02x.%02x.%02x",
			deviceID.AuxiliaryFirmwareRevision[0],
			deviceID.AuxiliaryFirmwareRevision[1],
			deviceID.AuxiliaryFirmwareRevision[2],
			deviceID.AuxiliaryFirmwareRevision[3])
	}

	// Get chassis status
	chassisStatus, err := s.client.GetChassisStatus(ctx)
	if err != nil {
		// Non-critical error, continue with partial info
		log.Printf("Warning: failed to get chassis status: %v", err)
	} else {
		// Power state
		if chassisStatus.PowerIsOn {
			host.PowerState = "on"
		} else {
			host.PowerState = "off"
		}

		// Last power event
		if chassisStatus.LastPowerOnByCommand {
			host.LastPowerEvent = "power-on-by-command"
		} else if chassisStatus.LastPowerDownByPowerFault {
			host.LastPowerEvent = "power-fault"
		} else if chassisStatus.LastPowerDownByPowerInterlockActivated {
			host.LastPowerEvent = "interlock-activated"
		} else if chassisStatus.LastPowerDownByPowerOverload {
			host.LastPowerEvent = "overload"
		} else if chassisStatus.ACFailed {
			host.LastPowerEvent = "ac-failed"
		} else {
			host.LastPowerEvent = "unknown"
		}

		// Chassis intrusion
		if chassisStatus.ChassisIntrusionActive {
			host.ChassisIntrusion = "active"
		} else {
			host.ChassisIntrusion = "inactive"
		}

		// Front panel lockout
		if chassisStatus.FrontPanelLockoutActive {
			host.FrontPanelLockout = "active"
		} else {
			host.FrontPanelLockout = "inactive"
		}

		// Drive fault
		if chassisStatus.DriveFault {
			host.DriveFault = "detected"
		} else {
			host.DriveFault = "none"
		}

		// Cooling fault
		if chassisStatus.CollingFanFault {
			host.CoolingFault = "detected"
		} else {
			host.CoolingFault = "none"
		}
	}

	return host, nil
}

// GetFRUInfo retrieves FRU (Field Replaceable Unit) information
func (s *IPMIServer) GetFRUInfo(ctx context.Context) ([]*ipmi.FRU, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to IPMI device")
	}

	frus, err := s.client.GetFRUs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get FRU data: %w", err)
	}

	return frus, nil
}

// TestConnection tests the connection to IPMI device
func (s *IPMIServer) TestConnection(ctx context.Context) error {
	if s.client == nil {
		return fmt.Errorf("not connected")
	}

	_, err := s.client.GetDeviceID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get device info: %w", err)
	}

	return nil
}

// ControlPower controls the power state of the host
func (s *IPMIServer) ControlPower(ctx context.Context, action string) error {
	if s.client == nil {
		return fmt.Errorf("not connected to IPMI device")
	}

	var err error
	switch action {
	case "on":
		_, err = s.client.ChassisControl(ctx, ipmi.ChassisControlPowerUp)
	case "off":
		_, err = s.client.ChassisControl(ctx, ipmi.ChassisControlPowerDown)
	case "cycle":
		_, err = s.client.ChassisControl(ctx, ipmi.ChassisControlPowerCycle)
	case "reset":
		_, err = s.client.ChassisControl(ctx, ipmi.ChassisControlHardReset)
	default:
		return fmt.Errorf("unknown power action: %s", action)
	}

	return err
}

// initIPMISchema creates the IPMI server table
func (s *Server) initIPMISchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS ipmi_servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		bmc_ip TEXT NOT NULL,
		port INTEGER DEFAULT 623,
		username TEXT NOT NULL,
		password TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		description TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_ipmi_servers_name ON ipmi_servers(name);
	CREATE INDEX IF NOT EXISTS idx_ipmi_servers_bmc_ip ON ipmi_servers(bmc_ip);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateIPMIServer creates a new IPMI server
func (s *Server) CreateIPMIServer(name, bmcIP, username, password, description string, port int) (*IPMIServer, error) {

	now := time.Now()
	query := `
	INSERT INTO ipmi_servers (name, bmc_ip, port, username, password, status, description, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?)
	`
	result, err := s.db.Exec(query, name, bmcIP, port, username, password, description, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &IPMIServer{
		ID:          id,
		Name:        name,
		BmcIP:       bmcIP,
		Port:        port,
		Username:    username,
		Status:      "active",
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetAllIPMIServers retrieves all IPMI servers
func (s *Server) GetAllIPMIServers() ([]IPMIServer, error) {

	query := `
	SELECT id, name, bmc_ip, port, username, status, description, created_at, updated_at
	FROM ipmi_servers
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []IPMIServer
	for rows.Next() {
		var server IPMIServer
		var createdAt, updatedAt string
		if err := rows.Scan(&server.ID, &server.Name, &server.BmcIP, &server.Port, &server.Username, &server.Status, &server.Description, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		server.CreatedAt = parseTime(createdAt)
		server.UpdatedAt = parseTime(updatedAt)
		servers = append(servers, server)
	}

	return servers, rows.Err()
}

// GetIPMIServerByID retrieves an IPMI server by ID
func (s *Server) GetIPMIServerByID(id int64) (*IPMIServer, error) {

	query := `
	SELECT id, name, bmc_ip, port, username, status, description, created_at, updated_at
	FROM ipmi_servers
	WHERE id = ?
	`

	var server IPMIServer
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, id).Scan(&server.ID, &server.Name, &server.BmcIP, &server.Port, &server.Username, &server.Status, &server.Description, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	server.CreatedAt = parseTime(createdAt)
	server.UpdatedAt = parseTime(updatedAt)

	return &server, nil
}

// UpdateIPMIServer updates an existing IPMI server
func (s *Server) UpdateIPMIServer(id int64, name, bmcIP, username, password, description string, port int) error {

	now := time.Now()
	query := `
	UPDATE ipmi_servers
	SET name = ?, bmc_ip = ?, port = ?, username = ?, password = ?, description = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, name, bmcIP, port, username, password, description, now, id)
	return err
}

// DeleteIPMIServer deletes an IPMI server by ID
func (s *Server) DeleteIPMIServer(id int64) error {

	query := `DELETE FROM ipmi_servers WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// IPMIView renders the IPMI server list page
func (s *Server) IPMIView(w http.ResponseWriter, r *http.Request) {
	servers, err := s.GetAllIPMIServers()
	if err != nil {
		log.Printf("Error fetching IPMI servers: %v", err)
		s.Error(w, err)
		return
	}

	s.Render(w, "ipmi/index", H{
		"servers": servers,
	})
}

// IPMINewHandler handles creating a new IPMI server
func (s *Server) IPMINewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "ipmi/new", H{})
		return
	}

	// POST: Create new IPMI server
	name := r.FormValue("name")
	bmcIP := r.FormValue("bmc_ip")
	username := r.FormValue("username")
	password := r.FormValue("password")
	description := r.FormValue("description")
	port := 623

	if portStr := r.FormValue("port"); portStr != "" {
		var err error
		port, err = strconv.Atoi(portStr)
		if err != nil {
			port = 623
		}
	}

	if name == "" || bmcIP == "" || username == "" || password == "" {
		s.Render(w, "ipmi/new", H{
			"error":       "All fields are required",
			"name":        name,
			"bmcIP":       bmcIP,
			"username":    username,
			"description": description,
			"port":        port,
		})
		return
	}

	_, err := s.CreateIPMIServer(name, bmcIP, username, password, description, port)
	if err != nil {
		s.Render(w, "ipmi/new", H{
			"error":       "Failed to create IPMI server: " + err.Error(),
			"name":        name,
			"bmcIP":       bmcIP,
			"username":    username,
			"description": description,
			"port":        port,
		})
		return
	}

	http.Redirect(w, r, "/ipmi", http.StatusSeeOther)
}

// IPMIEditHandler handles editing an IPMI server
func (s *Server) IPMIEditHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetIPMIServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "IPMI server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	if r.Method == http.MethodGet {
		s.Render(w, "ipmi/new", H{
			"server": server,
		})
		return
	}

	// POST: Update IPMI server
	name := r.FormValue("name")
	bmcIP := r.FormValue("bmc_ip")
	username := r.FormValue("username")
	password := r.FormValue("password")
	description := r.FormValue("description")
	port := 623

	if portStr := r.FormValue("port"); portStr != "" {
		var err error
		port, err = strconv.Atoi(portStr)
		if err != nil {
			port = 623
		}
	}

	// If password is empty, keep the old one
	if password == "" {
		password = server.Password
	}

	err = s.UpdateIPMIServer(id, name, bmcIP, username, password, description, port)
	if err != nil {
		s.Render(w, "ipmi/new", H{
			"error":  "Failed to update IPMI server: " + err.Error(),
			"server": server,
		})
		return
	}

	http.Redirect(w, r, "/ipmi", http.StatusSeeOther)
}

// IPMIDetailHandler handles viewing/deleting an IPMI server
func (s *Server) IPMIDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetIPMIServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "IPMI server not found", http.StatusNotFound)
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
		// Connect and fetch host information on-demand
		ctx := context.Background()
		var hostInfo *IPMIHost
		var fruInfo []*ipmi.FRU
		var connectionError string

		if err := server.Connect(ctx); err == nil {
			hostInfo, _ = server.GetHostInfo(ctx)
			fruInfo, _ = server.GetFRUInfo(ctx)
			defer server.Disconnect(ctx)
		} else {
			connectionError = err.Error()
		}

		// Build host data for template
		hostData := H{}
		if hostInfo != nil {
			hostData = H{
				"DeviceID":          hostInfo.DeviceID,
				"DeviceRevision":    hostInfo.DeviceRevision,
				"FirmwareRevision":  hostInfo.FirmwareRevision,
				"ManufacturerID":    hostInfo.ManufacturerID,
				"ProductID":         hostInfo.ProductID,
				"AuxFirmwareRev":    hostInfo.AuxFirmwareRev,
				"PowerState":        hostInfo.PowerState,
				"LastPowerEvent":    hostInfo.LastPowerEvent,
				"ChassisIntrusion":  hostInfo.ChassisIntrusion,
				"FrontPanelLockout": hostInfo.FrontPanelLockout,
				"DriveFault":        hostInfo.DriveFault,
				"CoolingFault":      hostInfo.CoolingFault,
			}
		}

		// Build FRU data for template
		fruList := make([]H, 0, len(fruInfo))
		for _, fru := range fruInfo {
			fruEntry := H{
				"ID": fru.DeviceID(),
			}

			if fru.BoardInfoArea != nil {
				fruEntry["BoardManufacturer"] = string(fru.BoardInfoArea.Manufacturer)
				fruEntry["BoardProductName"] = string(fru.BoardInfoArea.ProductName)
				fruEntry["BoardSerial"] = string(fru.BoardInfoArea.SerialNumber)
				fruEntry["BoardPartNumber"] = string(fru.BoardInfoArea.PartNumber)
			}

			if fru.ProductInfoArea != nil {
				fruEntry["ProductManufacturer"] = string(fru.ProductInfoArea.Manufacturer)
				fruEntry["ProductName"] = string(fru.ProductInfoArea.Name)
				fruEntry["ProductSerial"] = string(fru.ProductInfoArea.SerialNumber)
				fruEntry["ProductPartNumber"] = string(fru.ProductInfoArea.PartModel)
				fruEntry["ProductVersion"] = string(fru.ProductInfoArea.Version)
			}

			if fru.ChassisInfoArea != nil {
				fruEntry["ChassisPartNumber"] = string(fru.ChassisInfoArea.PartNumber)
				fruEntry["ChassisSerial"] = string(fru.ChassisInfoArea.SerialNumber)
			}

			fruList = append(fruList, fruEntry)
		}

		s.Render(w, "ipmi/detail", H{
			"server":          server,
			"host":            hostData,
			"frus":            fruList,
			"connectionError": connectionError,
		})
	case http.MethodPut:
		// Update IPMI server
		name := r.FormValue("name")
		bmcIP := r.FormValue("bmc_ip")
		username := r.FormValue("username")
		password := r.FormValue("password")
		description := r.FormValue("description")
		port := 623

		if portStr := r.FormValue("port"); portStr != "" {
			var err error
			port, err = strconv.Atoi(portStr)
			if err != nil {
				port = 623
			}
		}

		// If password is empty, keep the old one
		if password == "" {
			password = server.Password
		}

		err = s.UpdateIPMIServer(id, name, bmcIP, username, password, description, port)
		if err != nil {
			s.Render(w, "ipmi/detail", H{
				"error":  "Failed to update IPMI server: " + err.Error(),
				"server": server,
			})
			return
		}

		http.Redirect(w, r, "/ipmi", http.StatusSeeOther)

	case http.MethodDelete:
		err = s.DeleteIPMIServer(id)
		if err != nil {
			s.Error(w, err)
			return
		}
		http.Redirect(w, r, "/ipmi", http.StatusSeeOther)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// IPMIPowerControlHandler handles power control operations
func (s *Server) IPMIPowerControlHandler(w http.ResponseWriter, r *http.Request) {
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

	server, err := s.GetIPMIServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "IPMI server not found", http.StatusNotFound)
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

	ctx := context.Background()
	if err := server.Connect(ctx); err != nil {
		s.Error(w, &Error{Message: "Failed to connect to IPMI: " + err.Error()})
		return
	}
	defer server.Disconnect(ctx)

	if err := server.ControlPower(ctx, action); err != nil {
		s.Error(w, &Error{Message: "Failed to control power: " + err.Error()})
		return
	}

	// Redirect back to detail page
	http.Redirect(w, r, fmt.Sprintf("/ipmi/%d", id), http.StatusSeeOther)
}

// IPMITestConnectionHandler tests the connection to an IPMI server
func (s *Server) IPMITestConnectionHandler(w http.ResponseWriter, r *http.Request) {
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

	server, err := s.GetIPMIServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "IPMI server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	ctx := context.Background()
	if err := server.Connect(ctx); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Connection failed: " + err.Error()))
		return
	}
	defer server.Disconnect(ctx)

	if err := server.TestConnection(ctx); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Connection test failed: " + err.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Connection successful"))
}

func init() {
	RegisterInitSchemaHook((*Server).initIPMISchema)
}
