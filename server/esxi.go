package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
)

// ESXiServer represents a VMware ESXi server
type ESXiServer struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Username  string `json:"username"`
	Password  string `json:"-"` // Do not expose in JSON
	Port      int    `json:"port"`
	Insecure  bool   `json:"insecure"` // Skip TLS verification
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	client    *govmomi.Client
	clientMu  sync.RWMutex
}

// ESXiVM represents a VMware virtual machine
type ESXiVM struct {
	Name       string `json:"name"`
	GuestOS    string `json:"guest_os"`
	PowerState string `json:"power_state"`
	CPU        int32  `json:"cpu"`
	Memory     int64  `json:"memory"` // in MB
	IPAddress  string `json:"ip_address"`
	Host       string `json:"host"`
	Path       string `json:"path"`
}

// ESXiHost represents a VMware ESXi host
type ESXiHost struct {
	Name        string  `json:"name"`
	ProductName string  `json:"product_name"`
	Version     string  `json:"version"`
	CPU         int32   `json:"cpu"` // Total CPUs
	CPUUsage    float64 `json:"cpu_usage"`
	Memory      int64   `json:"memory"` // Total memory in MB
	MemoryUsage int64   `json:"memory_usage"`
}

// Connect establishes connection to ESXi server
func (e *ESXiServer) Connect(ctx context.Context) error {
	e.clientMu.Lock()
	defer e.clientMu.Unlock()

	u := &url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s:%d", e.Host, e.Port),
		Path:   "/sdk",
	}
	u.User = url.UserPassword(e.Username, e.Password)

	client, err := govmomi.NewClient(ctx, u, e.Insecure)
	if err != nil {
		return err
	}

	e.client = client
	return nil
}

// Disconnect closes connection to ESXi server
func (e *ESXiServer) Disconnect(ctx context.Context) error {
	e.clientMu.Lock()
	defer e.clientMu.Unlock()

	if e.client != nil {
		return e.client.Logout(ctx)
	}
	return nil
}

// GetClient returns the govmomi client
func (e *ESXiServer) GetClient() *govmomi.Client {
	e.clientMu.RLock()
	defer e.clientMu.RUnlock()
	return e.client
}

// GetVimClient returns the vim25 client
func (e *ESXiServer) GetVimClient() *vim25.Client {
	e.clientMu.RLock()
	defer e.clientMu.RUnlock()
	if e.client == nil {
		return nil
	}
	return e.client.Client
}

// GetVMs retrieves all virtual machines from ESXi server
func (e *ESXiServer) GetVMs(ctx context.Context) ([]ESXiVM, error) {
	vimClient := e.GetVimClient()
	if vimClient == nil {
		return nil, fmt.Errorf("not connected to ESXi server")
	}

	m := view.NewManager(vimClient)
	v, err := m.CreateContainerView(ctx, vimClient.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
	if err != nil {
		return nil, err
	}
	defer v.Destroy(ctx)

	var vms []mo.VirtualMachine
	err = v.Retrieve(ctx, []string{"VirtualMachine"}, []string{"summary", "runtime", "config", "guest"}, &vms)
	if err != nil {
		return nil, err
	}

	result := make([]ESXiVM, 0, len(vms))
	for _, vm := range vms {
		ipAddress := ""
		if vm.Guest.IpAddress != "" {
			ipAddress = vm.Guest.IpAddress
		}

		result = append(result, ESXiVM{
			Name:       vm.Summary.Config.Name,
			GuestOS:    vm.Summary.Config.GuestFullName,
			PowerState: string(vm.Summary.Runtime.PowerState),
			CPU:        vm.Summary.Config.NumCpu,
			Memory:     int64(vm.Summary.Config.MemorySizeMB),
			IPAddress:  ipAddress,
			Host:       vm.Summary.Runtime.Host.Reference().Value,
			Path:       vm.Summary.Config.VmPathName,
		})
	}

	return result, nil
}

// GetHosts retrieves all ESXi hosts
func (e *ESXiServer) GetHosts(ctx context.Context) ([]ESXiHost, error) {
	vimClient := e.GetVimClient()
	if vimClient == nil {
		return nil, fmt.Errorf("not connected to ESXi server")
	}

	m := view.NewManager(vimClient)
	v, err := m.CreateContainerView(ctx, vimClient.ServiceContent.RootFolder, []string{"HostSystem"}, true)
	if err != nil {
		return nil, err
	}
	defer v.Destroy(ctx)

	var hosts []mo.HostSystem
	err = v.Retrieve(ctx, []string{"HostSystem"}, []string{"summary", "hardware", "runtime"}, &hosts)
	if err != nil {
		return nil, err
	}

	result := make([]ESXiHost, 0, len(hosts))
	for _, host := range hosts {
		result = append(result, ESXiHost{
			Name:        host.Summary.Config.Name,
			ProductName: host.Summary.Config.Product.Name,
			Version:     host.Summary.Config.Product.Version,
			CPU:         int32(host.Summary.Hardware.NumCpuThreads),
			CPUUsage:    float64(host.Summary.QuickStats.OverallCpuUsage),
			Memory:      host.Summary.Hardware.MemorySize / 1024 / 1024, // Convert to MB
			MemoryUsage: int64(host.Summary.QuickStats.OverallMemoryUsage),
		})
	}

	return result, nil
}

// TestConnection tests the connection to ESXi server
func (e *ESXiServer) TestConnection(ctx context.Context) error {
	// Connection is already verified if we have a valid client
	// Just check if we can access the service content
	vimClient := e.GetVimClient()
	if vimClient == nil {
		return fmt.Errorf("not connected")
	}

	// If we have service content, connection is working
	if vimClient.ServiceContent.RootFolder.Value == "" {
		return fmt.Errorf("unable to retrieve service content")
	}
	return nil
}

// ESXiServersView handles the ESXi servers list page
func (s *Server) ESXiServersView(w http.ResponseWriter, r *http.Request) {
	servers, err := s.GetAllESXiServers()
	if err != nil {
		s.Error(w, err)
		return
	}

	s.Render(w, "esxi/index", H{
		"servers": servers,
	})
}

// ESXiServerNewHandler handles both displaying the new server form and creating a new server
// Also handles edit mode when .server is passed in the template data
func (s *Server) ESXiServerNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "esxi/new", H{})
		return
	}

	err := r.ParseForm()
	if err != nil {
		s.Error(w, err)
		return
	}

	name := r.FormValue("name")
	host := r.FormValue("host")
	username := r.FormValue("username")
	password := r.FormValue("password")
	portStr := r.FormValue("port")
	insecure := r.FormValue("insecure") == "on"

	if name == "" || host == "" || username == "" {
		s.Error(w, &Error{Message: "Name, host, username are required"})
		return
	}

	port := 443
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			s.Error(w, &Error{Message: "Invalid port"})
			return
		}
		port = p
	}

	// If password is empty and we're editing, keep existing password
	if idStr := r.FormValue("id"); idStr != "" && password == "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err == nil {
			existing, err := s.GetESXiServer(id)
			if err == nil {
				password = existing.Password
			}
		}
	}

	if password == "" {
		s.Error(w, &Error{Message: "Password is required"})
		return
	}

	_, err = s.CreateESXiServer(name, host, username, password, port, insecure)
	if err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, "/esxi", http.StatusSeeOther)
}

// ESXiServerEditHandler handles both displaying the edit form and updating a server
func (s *Server) ESXiServerEditHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	if r.Method == http.MethodGet {
		server, err := s.GetESXiServer(id)
		if err != nil {
			s.Error(w, err)
			return
		}

		s.Render(w, "esxi/new", H{
			"server": server,
		})
		return
	}

	err = r.ParseForm()
	if err != nil {
		s.Error(w, err)
		return
	}

	name := r.FormValue("name")
	host := r.FormValue("host")
	username := r.FormValue("username")
	password := r.FormValue("password")
	portStr := r.FormValue("port")
	insecure := r.FormValue("insecure") == "on"

	if name == "" || host == "" || username == "" {
		s.Error(w, &Error{Message: "Name, host, username are required"})
		return
	}

	port := 443
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			s.Error(w, &Error{Message: "Invalid port"})
			return
		}
		port = p
	}

	// If password is empty, keep existing password
	if password == "" {
		existing, err := s.GetESXiServer(id)
		if err == nil {
			password = existing.Password
		}
	}

	if password == "" {
		s.Error(w, &Error{Message: "Password is required"})
		return
	}

	err = s.UpdateESXiServer(id, name, host, username, password, port, insecure)
	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/esxi", http.StatusSeeOther)
}

// ESXiServerDetailHandler handles ESXi server detail page
func (s *Server) ESXiServerDetailHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		s.ESXiServerDeleteHandler(w, r) // Delegate to delete handler for DELETE method
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetESXiServer(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	// Connect and fetch data on-demand
	ctx := context.Background()
	err = server.Connect(ctx)
	if err != nil {
		s.Error(w, err)
		return
	}

	vms, _ := server.GetVMs(ctx)
	hosts, _ := server.GetHosts(ctx)
	defer server.Disconnect(ctx)
	s.Render(w, "esxi/detail", H{
		"server": server,
		"vms":    vms,
		"hosts":  hosts,
	})
}

// ESXiServerDeleteHandler handles deleting an ESXi server
func (s *Server) ESXiServerDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	err = s.DeleteESXiServer(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/esxi", http.StatusSeeOther)
}

// ESXiServerTestConnection handles testing ESXi server connection
func (s *Server) ESXiServerTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	host := r.FormValue("host")
	portStr := r.FormValue("port")
	username := r.FormValue("username")
	password := r.FormValue("password")
	insecure := r.FormValue("insecure") == "on"

	port := 443
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			http.Error(w, "Invalid port", http.StatusBadRequest)
			return
		}
		port = p
	}

	testServer := &ESXiServer{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		Insecure: insecure,
	}

	ctx := context.Background()
	if err := testServer.Connect(ctx); err != nil {
		http.Error(w, "Connection failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer testServer.Disconnect(ctx)

	if err := testServer.TestConnection(ctx); err != nil {
		http.Error(w, "Connection test failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Connection successful"))
}
