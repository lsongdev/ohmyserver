package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/lsongdev/openwrt-go/openwrt"
)

// OpenWrtServer represents an OpenWrt router
type OpenWrtServer struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	Password    string    `json:"-"`
	Protocol    string    `json:"protocol"` // http or https
	Status      string    `json:"status"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	client      *openwrt.Client
}

// OpenWrtSystemInfo represents system information from OpenWrt
type OpenWrtSystemInfo struct {
	Uptime        int
	LoadAvg       []int
	MemoryTotal   uint64
	MemoryFree    uint64
	MemoryAvail   uint64
	MemoryCached  uint64
	RootTotal     uint64
	RootUsed      uint64
	RootFree      uint64
	TmpTotal      uint64
	TmpUsed       uint64
	TmpFree       uint64
	SwapTotal     uint64
	SwapFree      uint64
	Hostname      string
	KernelVersion string
	Release       string
}

// OpenWrtInterface represents a network interface
type OpenWrtInterface struct {
	Interface  string
	Device     string
	L3Device   string
	Proto      string
	IPAddr     string
	Netmask    string
	IP6Addr    string
	Gateway    string
	Gateway6   string
	DNS        []string
	Up         bool
	Auto       bool
	Available  bool
	Uptime     int
	Metric     int
	PTPAddress string
}

// OpenWrtService represents a service
type OpenWrtService struct {
	Name      string
	Running   bool
	Enabled   bool
	Instances map[string]OpenWrtServiceInstance
	Triggers  []interface{}
}

// OpenWrtServiceInstance represents a running service instance
type OpenWrtServiceInstance struct {
	Running   bool
	PID       int
	Command   string
	Arguments []string
	Terminal  bool
	Exited    bool
}

// OpenWrtWiFi represents WiFi configuration
type OpenWrtWiFi struct {
	Device     string
	SSID       string
	Encryption string
	Mode       string
	Channel    int
	Disabled   bool
}

// OpenWrtDHCPLease represents a DHCP lease
type OpenWrtDHCPLease struct {
	IPAddress  string
	MACAddress string
	Hostname   string
	Expires    int
	IPv6       bool
	DUID       string
}

// OpenWrtPackage represents a package
type OpenWrtPackage struct {
	Name        string
	Version     string
	Description string
	Size        int
	Installed   bool
}

// OpenWrtFirewallRule represents a firewall rule
type OpenWrtFirewallRule struct {
	Name     string
	Src      string
	Dest     string
	DestPort string
	Proto    string
	Target   string
	Enabled  bool
}

// OpenWrtFirewallZone represents a firewall zone
type OpenWrtFirewallZone struct {
	Name    string
	Network []string
	Input   string
	Output  string
	Forward string
	Masq    bool
}

// OpenWrtPortForward represents a port forwarding rule
type OpenWrtPortForward struct {
	Name     string
	Src      string
	SrcDport string
	DestIP   string
	DestPort string
	Proto    string
	Enabled  bool
}

// initOpenWrtSchema creates the OpenWrt server table
func (s *Server) initOpenWrtSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS openwrt_servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		host TEXT NOT NULL,
		port INTEGER DEFAULT 80,
		username TEXT NOT NULL,
		password TEXT NOT NULL,
		protocol TEXT NOT NULL DEFAULT 'http',
		status TEXT NOT NULL DEFAULT 'active',
		description TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_openwrt_servers_name ON openwrt_servers(name);
	CREATE INDEX IF NOT EXISTS idx_openwrt_servers_host ON openwrt_servers(host);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateOpenWrtServer creates a new OpenWrt server
func (s *Server) CreateOpenWrtServer(name, host, username, password, protocol, description string, port int) (*OpenWrtServer, error) {

	now := time.Now()
	query := `
	INSERT INTO openwrt_servers (name, host, port, username, password, protocol, status, description, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, 'active', ?, ?, ?)
	`
	result, err := s.db.Exec(query, name, host, port, username, password, protocol, description, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &OpenWrtServer{
		ID:          id,
		Name:        name,
		Host:        host,
		Port:        port,
		Username:    username,
		Password:    password,
		Protocol:    protocol,
		Status:      "active",
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetAllOpenWrtServers retrieves all OpenWrt servers
func (s *Server) GetAllOpenWrtServers() ([]OpenWrtServer, error) {

	query := `
	SELECT id, name, host, port, username, protocol, status, description, created_at, updated_at
	FROM openwrt_servers
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []OpenWrtServer
	for rows.Next() {
		var server OpenWrtServer
		var createdAt, updatedAt string
		if err := rows.Scan(&server.ID, &server.Name, &server.Host, &server.Port, &server.Username, &server.Protocol, &server.Status, &server.Description, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		server.CreatedAt = parseTime(createdAt)
		server.UpdatedAt = parseTime(updatedAt)
		servers = append(servers, server)
	}

	return servers, rows.Err()
}

// GetOpenWrtServerByID retrieves an OpenWrt server by ID
func (s *Server) GetOpenWrtServerByID(id int64) (*OpenWrtServer, error) {

	query := `
	SELECT id, name, host, port, username, password, protocol, status, description, created_at, updated_at
	FROM openwrt_servers
	WHERE id = ?
	`

	var server OpenWrtServer
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, id).Scan(&server.ID, &server.Name, &server.Host, &server.Port, &server.Username, &server.Password, &server.Protocol, &server.Status, &server.Description, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	server.CreatedAt = parseTime(createdAt)
	server.UpdatedAt = parseTime(updatedAt)

	return &server, nil
}

// UpdateOpenWrtServer updates an existing OpenWrt server
func (s *Server) UpdateOpenWrtServer(id int64, name, host, username, password, protocol, description string, port int) error {

	now := time.Now()
	query := `
	UPDATE openwrt_servers
	SET name = ?, host = ?, port = ?, username = ?, password = ?, protocol = ?, description = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, name, host, port, username, password, protocol, description, now, id)
	return err
}

// DeleteOpenWrtServer deletes an OpenWrt server by ID
func (s *Server) DeleteOpenWrtServer(id int64) error {

	query := `DELETE FROM openwrt_servers WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// Connect establishes connection to OpenWrt router
func (s *OpenWrtServer) Connect(ctx context.Context) error {
	baseURL := fmt.Sprintf("%s://%s", s.Protocol, s.Host)
	if s.Port != 80 && s.Port != 443 {
		baseURL = fmt.Sprintf("%s://%s:%d", s.Protocol, s.Host, s.Port)
	}

	client := openwrt.NewClient(
		baseURL,
		openwrt.WithCredentials(s.Username, s.Password),
		openwrt.WithTimeout(30*time.Second),
	)

	if err := client.Login(ctx); err != nil {
		return fmt.Errorf("failed to login to OpenWrt: %w", err)
	}

	s.client = client
	return nil
}

// Disconnect closes connection to OpenWrt router
func (s *OpenWrtServer) Disconnect(ctx context.Context) error {
	if s.client != nil {
		s.client.Logout(ctx)
		s.client = nil
	}
	return nil
}

// GetSystemInfo retrieves system information from OpenWrt
func (s *OpenWrtServer) GetSystemInfo(ctx context.Context) (*openwrt.SystemInfo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to OpenWrt")
	}
	sysInfo, err := s.client.GetSystemInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get system info: %w", err)
	}
	return sysInfo, nil
}

// GetInterfaces retrieves network interfaces
func (s *OpenWrtServer) GetInterfaces(ctx context.Context) ([]openwrt.NetworkInterface, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to OpenWrt")
	}

	networkSvc := openwrt.NewNetworkService(s.client)
	interfaces, err := networkSvc.ListInterfaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list interfaces: %w", err)
	}

	return interfaces, nil
}

// GetWiFiConfig retrieves WiFi configuration
func (s *OpenWrtServer) GetWiFiConfig(ctx context.Context) ([]OpenWrtWiFi, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to OpenWrt")
	}

	networkSvc := openwrt.NewNetworkService(s.client)
	wifiConfigs, err := networkSvc.GetWifiConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get WiFi config: %w", err)
	}

	result := make([]OpenWrtWiFi, 0, len(wifiConfigs))
	for _, wifi := range wifiConfigs {
		result = append(result, OpenWrtWiFi{
			Device:     wifi.Device,
			SSID:       wifi.SSID,
			Encryption: wifi.Encryption,
			Mode:       wifi.Mode,
			Channel:    wifi.Channel,
			Disabled:   wifi.Disabled,
		})
	}

	return result, nil
}

// GetDHCPLeases retrieves DHCP leases
func (s *OpenWrtServer) GetDHCPLeases(ctx context.Context) ([]openwrt.DHCPLease, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to OpenWrt")
	}

	networkSvc := openwrt.NewNetworkService(s.client)
	leases, err := networkSvc.GetDHCPLeases(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get DHCP leases: %w", err)
	}

	return leases, nil
}

// GetServices retrieves service status
func (s *OpenWrtServer) GetServices(ctx context.Context) ([]OpenWrtService, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to OpenWrt")
	}

	serviceNames := []string{"network", "firewall", "dnsmasq", "dropbear", "uhttpd", "odhcpd"}
	result := make([]OpenWrtService, 0, len(serviceNames))

	for _, name := range serviceNames {
		svc, err := s.client.GetServiceStatus(ctx, name)
		if err != nil {
			continue
		}

		instances := make(map[string]OpenWrtServiceInstance)
		for instID, inst := range svc.Instances {
			instances[instID] = OpenWrtServiceInstance{
				Running:   inst.Running,
				PID:       inst.PID,
				Command:   inst.Command,
				Arguments: inst.Arguments,
				Terminal:  inst.Terminal,
				Exited:    inst.Exited,
			}
		}

		result = append(result, OpenWrtService{
			Name:      name,
			Running:   svc.Running,
			Enabled:   svc.Enabled,
			Instances: instances,
			Triggers:  svc.Triggers,
		})
	}

	return result, nil
}

// GetPackages retrieves package list
func (s *OpenWrtServer) GetPackages(ctx context.Context) ([]OpenWrtPackage, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to OpenWrt")
	}

	packages, err := s.client.ListPackages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list packages: %w", err)
	}

	result := make([]OpenWrtPackage, 0, len(packages))
	for _, pkg := range packages {
		result = append(result, OpenWrtPackage{
			Name:        pkg.Name,
			Version:     pkg.Version,
			Description: pkg.Description,
			Size:        pkg.Size,
			Installed:   pkg.Installed,
		})
	}

	return result, nil
}

// GetFirewallRules retrieves firewall rules
func (s *OpenWrtServer) GetFirewallRules(ctx context.Context) ([]OpenWrtFirewallRule, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to OpenWrt")
	}

	firewallSvc := openwrt.NewFirewallService(s.client)
	rules, err := firewallSvc.ListRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list firewall rules: %w", err)
	}

	result := make([]OpenWrtFirewallRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, OpenWrtFirewallRule{
			Name:     rule.Name,
			Src:      rule.Src,
			Dest:     rule.Dest,
			DestPort: rule.DestPort,
			Proto:    rule.Proto,
			Target:   rule.Target,
			Enabled:  rule.Enabled,
		})
	}

	return result, nil
}

// GetFirewallZones retrieves firewall zones
func (s *OpenWrtServer) GetFirewallZones(ctx context.Context) ([]OpenWrtFirewallZone, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to OpenWrt")
	}

	firewallSvc := openwrt.NewFirewallService(s.client)
	zones, err := firewallSvc.ListZones(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list firewall zones: %w", err)
	}

	result := make([]OpenWrtFirewallZone, 0, len(zones))
	for _, zone := range zones {
		result = append(result, OpenWrtFirewallZone{
			Name:    zone.Name,
			Network: zone.Network,
			Input:   zone.Input,
			Output:  zone.Output,
			Forward: zone.Forward,
			Masq:    zone.Masq,
		})
	}

	return result, nil
}

// GetPortForwards retrieves port forwarding rules
func (s *OpenWrtServer) GetPortForwards(ctx context.Context) ([]OpenWrtPortForward, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to OpenWrt")
	}

	firewallSvc := openwrt.NewFirewallService(s.client)
	forwards, err := firewallSvc.ListPortForwards(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list port forwards: %w", err)
	}

	result := make([]OpenWrtPortForward, 0, len(forwards))
	for _, pf := range forwards {
		result = append(result, OpenWrtPortForward{
			Name:     pf.Name,
			Src:      pf.Src,
			SrcDport: pf.SrcDport,
			DestIP:   pf.DestIP,
			DestPort: pf.DestPort,
			Proto:    pf.Proto,
			Enabled:  pf.Enabled,
		})
	}

	return result, nil
}

// Reboot reboots the OpenWrt router
func (s *OpenWrtServer) Reboot(ctx context.Context) error {
	if s.client == nil {
		return fmt.Errorf("not connected to OpenWrt")
	}

	return s.client.Reboot(ctx)
}

// RestartService restarts a service
func (s *OpenWrtServer) RestartService(ctx context.Context, name string) error {
	if s.client == nil {
		return fmt.Errorf("not connected to OpenWrt")
	}

	return s.client.RestartService(ctx, name)
}

// TestConnection tests the connection to OpenWrt
func (s *OpenWrtServer) TestConnection(ctx context.Context) error {
	if s.client == nil {
		return fmt.Errorf("not connected")
	}

	_, err := s.client.GetSystemInfo(ctx)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}

	return nil
}

// OpenWrtListView renders the OpenWrt server list page
func (s *Server) OpenWrtListView(w http.ResponseWriter, r *http.Request) {
	servers, err := s.GetAllOpenWrtServers()
	if err != nil {
		log.Printf("Error fetching OpenWrt servers: %v", err)
		s.Error(w, err)
		return
	}

	s.Render(w, "openwrt/index", H{
		"servers": servers,
	})
}

// OpenWrtNewHandler handles creating a new OpenWrt server
func (s *Server) OpenWrtNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "openwrt/new", H{})
		return
	}

	name := r.FormValue("name")
	host := r.FormValue("host")
	username := r.FormValue("username")
	password := r.FormValue("password")
	protocol := r.FormValue("protocol")
	description := r.FormValue("description")
	port := 80

	if portStr := r.FormValue("port"); portStr != "" {
		var err error
		port, err = strconv.Atoi(portStr)
		if err != nil {
			port = 80
		}
	}

	if name == "" || host == "" || username == "" || password == "" {
		s.Render(w, "openwrt/new", H{
			"error":       "Name, host, username, and password are required",
			"name":        name,
			"host":        host,
			"username":    username,
			"protocol":    protocol,
			"description": description,
			"port":        port,
		})
		return
	}

	_, err := s.CreateOpenWrtServer(name, host, username, password, protocol, description, port)
	if err != nil {
		s.Render(w, "openwrt/new", H{
			"error":       "Failed to create OpenWrt server: " + err.Error(),
			"name":        name,
			"host":        host,
			"username":    username,
			"protocol":    protocol,
			"description": description,
			"port":        port,
		})
		return
	}

	http.Redirect(w, r, "/openwrt", http.StatusSeeOther)
}

// OpenWrtEditHandler handles editing an OpenWrt server
func (s *Server) OpenWrtEditHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetOpenWrtServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "OpenWrt server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	if r.Method == http.MethodGet {
		s.Render(w, "openwrt/new", H{
			"server": server,
		})
		return
	}

	name := r.FormValue("name")
	host := r.FormValue("host")
	username := r.FormValue("username")
	password := r.FormValue("password")
	protocol := r.FormValue("protocol")
	description := r.FormValue("description")
	port := 80

	if portStr := r.FormValue("port"); portStr != "" {
		var err error
		port, err = strconv.Atoi(portStr)
		if err != nil {
			port = 80
		}
	}

	if password == "" {
		password = server.Password
	}

	err = s.UpdateOpenWrtServer(id, name, host, username, password, protocol, description, port)
	if err != nil {
		s.Render(w, "openwrt/new", H{
			"error":  "Failed to update OpenWrt server: " + err.Error(),
			"server": server,
		})
		return
	}

	http.Redirect(w, r, "/openwrt", http.StatusSeeOther)
}

// OpenWrtDetailHandler handles viewing/deleting an OpenWrt server
func (s *Server) OpenWrtDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetOpenWrtServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "OpenWrt server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	if r.Method == http.MethodDelete {
		err = s.DeleteOpenWrtServer(id)
		return
	}

	if r.Method == http.MethodGet {
		ctx := context.Background()
		var systemInfo *openwrt.SystemInfo
		if err := server.Connect(ctx); err == nil {
			systemInfo, _ = server.GetSystemInfo(ctx)
		}
		s.Render(w, "openwrt/overview", H{
			"server": server,
			"system": systemInfo,
		})
		return
	}

	name := r.FormValue("name")
	host := r.FormValue("host")
	username := r.FormValue("username")
	password := r.FormValue("password")
	protocol := r.FormValue("protocol")
	description := r.FormValue("description")
	port := 80

	if portStr := r.FormValue("port"); portStr != "" {
		var err error
		port, err = strconv.Atoi(portStr)
		if err != nil {
			port = 80
		}
	}

	if password == "" {
		password = server.Password
	}

	err = s.UpdateOpenWrtServer(id, name, host, username, password, protocol, description, port)
	if err != nil {
		s.Render(w, "openwrt/overview", H{
			"error":  "Failed to update OpenWrt server: " + err.Error(),
			"server": server,
		})
		return
	}

	http.Redirect(w, r, "/openwrt", http.StatusSeeOther)
}

// OpenWrtRestartServiceHandler handles restarting services
func (s *Server) OpenWrtRestartServiceHandler(w http.ResponseWriter, r *http.Request) {
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

	server, err := s.GetOpenWrtServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "OpenWrt server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	serviceName := r.FormValue("service")
	if serviceName == "" {
		s.Error(w, &Error{Message: "Service name is required"})
		return
	}

	ctx := context.Background()
	if err := server.Connect(ctx); err != nil {
		s.Error(w, &Error{Message: "Failed to connect to OpenWrt: " + err.Error()})
		return
	}
	defer server.Disconnect(ctx)

	if err := server.RestartService(ctx, serviceName); err != nil {
		s.Error(w, &Error{Message: "Failed to restart service: " + err.Error()})
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/openwrt/%d", id), http.StatusSeeOther)
}

// OpenWrtTestConnectionHandler tests the connection to an OpenWrt server
func (s *Server) OpenWrtTestConnectionHandler(w http.ResponseWriter, r *http.Request) {
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

	server, err := s.GetOpenWrtServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "OpenWrt server not found", http.StatusNotFound)
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

// OpenWrtRebootHandler handles rebooting an OpenWrt router
func (s *Server) OpenWrtRebootHandler(w http.ResponseWriter, r *http.Request) {
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

	server, err := s.GetOpenWrtServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "OpenWrt server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	ctx := context.Background()
	if err := server.Connect(ctx); err != nil {
		s.Error(w, &Error{Message: "Failed to connect to OpenWrt: " + err.Error()})
		return
	}
	defer server.Disconnect(ctx)

	if err := server.Reboot(ctx); err != nil {
		s.Error(w, &Error{Message: "Failed to reboot OpenWrt: " + err.Error()})
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/openwrt/%d", id), http.StatusSeeOther)
}

// OpenWrtNetworkHandler handles the network sub-page
func (s *Server) OpenWrtNetworkHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetOpenWrtServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "OpenWrt server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	ctx := context.Background()
	var interfaces []openwrt.NetworkInterface
	var wifiConfigs []OpenWrtWiFi
	var connectionError string

	if err := server.Connect(ctx); err == nil {
		interfaces, _ = server.GetInterfaces(ctx)
		wifiConfigs, _ = server.GetWiFiConfig(ctx)
		defer server.Disconnect(ctx)
	} else {
		connectionError = err.Error()
	}

	s.Render(w, "openwrt/network", H{
		"server":          server,
		"interfaces":      interfaces,
		"wifi":            wifiConfigs,
		"connectionError": connectionError,
	})
}

// OpenWrtDHCPHandler handles the DHCP sub-page
func (s *Server) OpenWrtDHCPHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetOpenWrtServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "OpenWrt server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	ctx := context.Background()
	var dhcpLeases []openwrt.DHCPLease
	var connectionError string

	if err := server.Connect(ctx); err == nil {
		dhcpLeases, _ = server.GetDHCPLeases(ctx)
		defer server.Disconnect(ctx)
	} else {
		connectionError = err.Error()
	}

	s.Render(w, "openwrt/dhcp", H{
		"server":          server,
		"leases":          dhcpLeases,
		"connectionError": connectionError,
	})
}

// OpenWrtPackagesHandler handles the packages sub-page
func (s *Server) OpenWrtPackagesHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetOpenWrtServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "OpenWrt server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	ctx := context.Background()
	var packages []OpenWrtPackage
	var connectionError string

	if err := server.Connect(ctx); err == nil {
		packages, _ = server.GetPackages(ctx)
		defer server.Disconnect(ctx)
	} else {
		connectionError = err.Error()
	}

	s.Render(w, "openwrt/packages", H{
		"server":          server,
		"packages":        packages,
		"connectionError": connectionError,
	})
}

// OpenWrtFirewallHandler handles the firewall sub-page
func (s *Server) OpenWrtFirewallHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetOpenWrtServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "OpenWrt server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	ctx := context.Background()
	var firewallRules []OpenWrtFirewallRule
	var firewallZones []OpenWrtFirewallZone
	var portForwards []OpenWrtPortForward
	var connectionError string

	if err := server.Connect(ctx); err == nil {
		firewallRules, _ = server.GetFirewallRules(ctx)
		firewallZones, _ = server.GetFirewallZones(ctx)
		portForwards, _ = server.GetPortForwards(ctx)
		defer server.Disconnect(ctx)
	} else {
		connectionError = err.Error()
	}

	s.Render(w, "openwrt/firewall", H{
		"server":          server,
		"firewallRules":   firewallRules,
		"firewallZones":   firewallZones,
		"portForwards":    portForwards,
		"connectionError": connectionError,
	})
}

func init() {
	RegisterInitSchemaHook((*Server).initOpenWrtSchema)
}
