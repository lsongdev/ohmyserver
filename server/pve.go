package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lsongdev/ohmyserver/pve"
)

// PVEServer represents a Proxmox VE server connection
type PVEServer struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"-"`
	Insecure  bool   `json:"insecure"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	client    *pve.Client
	clientMu  sync.RWMutex
}

func (s *Server) initPVESchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS pve_servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		host TEXT NOT NULL,
		port INTEGER NOT NULL DEFAULT 8006,
		username TEXT NOT NULL,
		password TEXT NOT NULL,
		insecure BOOLEAN NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_pve_servers_name ON pve_servers(name);
	`
	_, err := s.db.Exec(query)
	return err
}

func (p *PVEServer) Connect() error {
	p.clientMu.Lock()
	defer p.clientMu.Unlock()

	baseURL := fmt.Sprintf("https://%s:%d", p.Host, p.Port)
	p.client = pve.NewClient(baseURL, p.Username, p.Password, p.Insecure)
	return p.client.Login()
}

func (p *PVEServer) Disconnect() error {
	p.clientMu.Lock()
	defer p.clientMu.Unlock()
	p.client = nil
	return nil
}

func (p *PVEServer) GetNodes() ([]pve.NodeInfo, error) {
	p.clientMu.RLock()
	client := p.client
	p.clientMu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected")
	}
	return client.GetNodes()
}

func (p *PVEServer) GetVMs(node string) ([]pve.VMInfo, error) {
	p.clientMu.RLock()
	client := p.client
	p.clientMu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("not connected")
	}
	return client.GetVMs(node)
}

func (p *PVEServer) TestConnection() error {
	if err := p.Connect(); err != nil {
		return err
	}
	defer p.Disconnect()
	_, err := p.GetNodes()
	return err
}

func (s *Server) CreatePVEServer(name, host, username, password string, port int, insecure bool) (*PVEServer, error) {
	now := time.Now()
	query := `
	INSERT INTO pve_servers (name, host, port, username, password, insecure, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, name, host, port, username, password, insecure, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &PVEServer{
		ID:        id,
		Name:      name,
		Host:      host,
		Port:      port,
		Username:  username,
		Password:  password,
		Insecure:  insecure,
		CreatedAt: now.Format("2006-01-02 15:04:05"),
		UpdatedAt: now.Format("2006-01-02 15:04:05"),
	}, nil
}

func (s *Server) GetPVEServer(id int64) (*PVEServer, error) {
	query := `
	SELECT id, name, host, port, username, password, insecure, created_at, updated_at
	FROM pve_servers
	WHERE id = ?
	`

	var server PVEServer
	err := s.db.QueryRow(query, id).Scan(
		&server.ID, &server.Name, &server.Host, &server.Port,
		&server.Username, &server.Password, &server.Insecure,
		&server.CreatedAt, &server.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &server, nil
}

func (s *Server) GetAllPVEServers() ([]PVEServer, error) {
	query := `
	SELECT id, name, host, port, username, password, insecure, created_at, updated_at
	FROM pve_servers
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []PVEServer
	for rows.Next() {
		var server PVEServer
		err := rows.Scan(
			&server.ID, &server.Name, &server.Host, &server.Port,
			&server.Username, &server.Password, &server.Insecure,
			&server.CreatedAt, &server.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}

	return servers, rows.Err()
}

func (s *Server) UpdatePVEServer(id int64, name, host, username, password string, port int, insecure bool) error {
	now := time.Now()
	query := `
	UPDATE pve_servers
	SET name = ?, host = ?, port = ?, username = ?, password = ?, insecure = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, name, host, port, username, password, insecure, now, id)
	return err
}

func (s *Server) DeletePVEServer(id int64) error {
	query := `DELETE FROM pve_servers WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

func (s *Server) PVEListView(w http.ResponseWriter, r *http.Request) {
	servers, err := s.GetAllPVEServers()
	if err != nil {
		s.Error(w, err)
		return
	}

	s.Render(w, "pve/index", H{
		"servers": servers,
	})
}

func (s *Server) PVENewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "pve/new", H{})
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
		s.Error(w, &Error{Message: "Name, host, and username are required"})
		return
	}

	port := 8006
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			s.Error(w, &Error{Message: "Invalid port"})
			return
		}
		port = p
	}

	if idStr := r.FormValue("id"); idStr != "" && password == "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err == nil {
			existing, err := s.GetPVEServer(id)
			if err == nil {
				password = existing.Password
			}
		}
	}

	if password == "" {
		s.Error(w, &Error{Message: "Password is required"})
		return
	}

	_, err = s.CreatePVEServer(name, host, username, password, port, insecure)
	if err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, "/pve", http.StatusSeeOther)
}

func (s *Server) PVEEditHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	if r.Method == http.MethodGet {
		server, err := s.GetPVEServer(id)
		if err != nil {
			s.Error(w, err)
			return
		}

		s.Render(w, "pve/new", H{
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
		s.Error(w, &Error{Message: "Name, host, and username are required"})
		return
	}

	port := 8006
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			s.Error(w, &Error{Message: "Invalid port"})
			return
		}
		port = p
	}

	if password == "" {
		existing, err := s.GetPVEServer(id)
		if err == nil {
			password = existing.Password
		}
	}

	if password == "" {
		s.Error(w, &Error{Message: "Password is required"})
		return
	}

	err = s.UpdatePVEServer(id, name, host, username, password, port, insecure)
	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/pve", http.StatusSeeOther)
}

func (s *Server) PVEDetailHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		s.PVEDeleteHandler(w, r)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetPVEServer(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	if err := server.Connect(); err != nil {
		s.Error(w, err)
		return
	}
	defer server.Disconnect()

	nodes, err := server.GetNodes()
	if err != nil {
		s.Error(w, err)
		return
	}

	type nodeWithVMs struct {
		pve.NodeInfo
		VMs []pve.VMInfo
	}

	nodeResults := make([]nodeWithVMs, 0, len(nodes))
	for _, node := range nodes {
		vms, _ := server.GetVMs(node.Node)
		nodeResults = append(nodeResults, nodeWithVMs{
			NodeInfo: node,
			VMs:      vms,
		})
	}

	s.Render(w, "pve/detail", H{
		"server": server,
		"nodes":  nodeResults,
	})
}

func (s *Server) PVEDeleteHandler(w http.ResponseWriter, r *http.Request) {
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

	err = s.DeletePVEServer(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/pve", http.StatusSeeOther)
}

func (p *PVEServer) GetNextVMID() (int, error) {
	p.clientMu.RLock()
	client := p.client
	p.clientMu.RUnlock()
	if client == nil {
		return 0, fmt.Errorf("not connected")
	}
	return client.GetNextVMID()
}

func (p *PVEServer) GetTemplates(node string) ([]pve.VMInfo, error) {
	p.clientMu.RLock()
	client := p.client
	p.clientMu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("not connected")
	}
	return client.GetTemplates(node)
}

func (p *PVEServer) CloneVM(node string, vmid int, params pve.CloneVMParams) (string, error) {
	p.clientMu.RLock()
	client := p.client
	p.clientMu.RUnlock()
	if client == nil {
		return "", fmt.Errorf("not connected")
	}
	return client.CloneVM(node, vmid, params)
}

func (p *PVEServer) GetVMConfig(node string, vmid int) (map[string]interface{}, error) {
	p.clientMu.RLock()
	client := p.client
	p.clientMu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("not connected")
	}
	return client.GetVMConfig(node, vmid)
}

func (p *PVEServer) UpdateVMConfig(node string, vmid int, params map[string]string) error {
	p.clientMu.RLock()
	client := p.client
	p.clientMu.RUnlock()
	if client == nil {
		return fmt.Errorf("not connected")
	}
	return client.UpdateVMConfig(node, vmid, params)
}

func (p *PVEServer) ResizeDiskVM(node string, vmid int, disk string, addGB int) error {
	p.clientMu.RLock()
	client := p.client
	p.clientMu.RUnlock()
	if client == nil {
		return fmt.Errorf("not connected")
	}
	return client.ResizeDiskVM(node, vmid, disk, addGB)
}

func (p *PVEServer) StartVM(node string, vmid int) error {
	p.clientMu.RLock()
	client := p.client
	p.clientMu.RUnlock()
	if client == nil {
		return fmt.Errorf("not connected")
	}
	return client.StartVM(node, vmid)
}

func (s *Server) PVECreateInstanceHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetPVEServer(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	if err := server.Connect(); err != nil {
		s.Error(w, err)
		return
	}
	defer server.Disconnect()

	if r.Method == http.MethodGet {
		nodes, err := server.GetNodes()
		if err != nil {
			s.Error(w, err)
			return
		}

		var allTemplates []pve.VMInfo
		for _, node := range nodes {
			templates, err := server.GetTemplates(node.Node)
			if err == nil {
				allTemplates = append(allTemplates, templates...)
			}
		}

		s.Render(w, "pve/create", H{
			"server":    server,
			"nodes":     nodes,
			"templates": allTemplates,
		})
		return
	}

	err = r.ParseForm()
	if err != nil {
		s.Error(w, err)
		return
	}

	node := r.FormValue("node")
	templateVmidStr := r.FormValue("template")
	name := r.FormValue("name")
	coresStr := r.FormValue("cores")
	socketsStr := r.FormValue("sockets")
	memoryStr := r.FormValue("memory")
	diskStr := r.FormValue("disk")
	start := r.FormValue("start") == "on"

	if node == "" || templateVmidStr == "" || name == "" {
		s.Error(w, &Error{Message: "Node, template, and name are required"})
		return
	}

	templateVmid, err := strconv.Atoi(templateVmidStr)
	if err != nil {
		s.Error(w, &Error{Message: "Invalid template ID"})
		return
	}

	cores := 2
	if coresStr != "" {
		c, err := strconv.Atoi(coresStr)
		if err == nil && c > 0 {
			cores = c
		}
	}

	sockets := 1
	if socketsStr != "" {
		so, err := strconv.Atoi(socketsStr)
		if err == nil && so > 0 {
			sockets = so
		}
	}

	memory := 2048
	if memoryStr != "" {
		m, err := strconv.Atoi(memoryStr)
		if err == nil && m > 0 {
			memory = m
		}
	}

	diskSize := 0
	if diskStr != "" {
		d, err := strconv.Atoi(diskStr)
		if err == nil && d > 0 {
			diskSize = d
		}
	}

	newID, err := server.GetNextVMID()
	if err != nil {
		s.Error(w, err)
		return
	}

	cloneParams := pve.CloneVMParams{
		NewID: newID,
		Name:  name,
		Full:  1,
	}

	if _, err := server.CloneVM(node, templateVmid, cloneParams); err != nil {
		s.Error(w, err)
		return
	}

	configParams := map[string]string{
		"cores":   fmt.Sprintf("%d", cores),
		"sockets": fmt.Sprintf("%d", sockets),
		"memory":  fmt.Sprintf("%d", memory),
	}
	if err := server.UpdateVMConfig(node, newID, configParams); err != nil {
		s.Error(w, err)
		return
	}

	if diskSize > 0 {
		vmConfig, err := server.GetVMConfig(node, newID)
		if err == nil {
			diskKeys := []string{"scsi0", "scsi1", "virtio0", "virtio1", "sata0", "sata1"}
			for _, key := range diskKeys {
				val, ok := vmConfig[key]
				if !ok {
					continue
				}
				str, ok := val.(string)
				if !ok {
					continue
				}
				parts := strings.Split(str, ":")
				if len(parts) < 2 {
					continue
				}
				sizeStr := parts[len(parts)-1]
				sizeStr = strings.TrimSuffix(sizeStr, "G")
				currentSize, err := strconv.Atoi(sizeStr)
				if err != nil || diskSize <= currentSize {
					break
				}
				addSize := diskSize - currentSize
				server.ResizeDiskVM(node, newID, key, addSize)
				break
			}
		}
	}

	if start {
		server.StartVM(node, newID)
	}

	http.Redirect(w, r, fmt.Sprintf("/pve/%d", server.ID), http.StatusSeeOther)
}

func (s *Server) PVETestConnectionHandler(w http.ResponseWriter, r *http.Request) {
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

	port := 8006
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			http.Error(w, "Invalid port", http.StatusBadRequest)
			return
		}
		port = p
	}

	testServer := &PVEServer{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		Insecure: insecure,
	}

	if err := testServer.TestConnection(); err != nil {
		http.Error(w, "Connection failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Connection successful"))
}

func init() {
	RegisterInitSchemaHook((*Server).initPVESchema)
}
