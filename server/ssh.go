package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	terminalpkg "github.com/lsongdev/ohmyserver/terminal"
	"golang.org/x/crypto/ssh"
)

// SSHServer represents a remote SSH server
type SSHServer struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	AuthType    string    `json:"auth_type"` // "password" or "key"
	Password    string    `json:"-"`         // Never serialize
	PrivateKey  string    `json:"-"`         // Never serialize
	HostKey     string    `json:"host_key"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListSSH returns all SSH servers
func (s *Server) ListSSH() []*SSHServer {
	query := `
		SELECT id, name, host, port, username, auth_type, password, private_key, host_key, description, enabled, created_at, updated_at
		FROM ssh_servers
		ORDER BY name
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var servers []*SSHServer
	for rows.Next() {
		server := &SSHServer{}
		var enabled int
		err := rows.Scan(
			&server.ID,
			&server.Name,
			&server.Host,
			&server.Port,
			&server.Username,
			&server.AuthType,
			&server.Password,
			&server.PrivateKey,
			&server.HostKey,
			&server.Description,
			&enabled,
			&server.CreatedAt,
			&server.UpdatedAt,
		)
		if err != nil {
			continue
		}
		server.Enabled = enabled == 1
		servers = append(servers, server)
	}
	return servers
}

// GetSSH returns a single SSH server by ID
func (s *Server) GetSSH(id int64) (*SSHServer, bool) {
	query := `
		SELECT id, name, host, port, username, auth_type, password, private_key, host_key, description, enabled, created_at, updated_at
		FROM ssh_servers
		WHERE id = ?
	`
	server := &SSHServer{}
	var enabled int
	err := s.db.QueryRow(query, id).Scan(
		&server.ID,
		&server.Name,
		&server.Host,
		&server.Port,
		&server.Username,
		&server.AuthType,
		&server.Password,
		&server.PrivateKey,
		&server.HostKey,
		&server.Description,
		&enabled,
		&server.CreatedAt,
		&server.UpdatedAt,
	)
	if err != nil {
		return nil, false
	}
	server.Enabled = enabled == 1
	return server, true
}

// CreateSSH creates a new SSH server
func (s *Server) CreateSSH(server *SSHServer) (int64, error) {
	now := time.Now()
	enabled := 1
	if !server.Enabled {
		enabled = 0
	}

	query := `
		INSERT INTO ssh_servers (name, host, port, username, auth_type, password, private_key, host_key, description, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query,
		server.Name,
		server.Host,
		server.Port,
		server.Username,
		server.AuthType,
		server.Password,
		server.PrivateKey,
		server.HostKey,
		server.Description,
		enabled,
		now,
		now,
	)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	server.ID = id
	server.CreatedAt = now
	server.UpdatedAt = now
	return id, nil
}

// UpdateSSH updates an existing SSH server
func (s *Server) UpdateSSH(server *SSHServer) error {
	now := time.Now()
	enabled := 1
	if !server.Enabled {
		enabled = 0
	}

	query := `
		UPDATE ssh_servers
		SET name = ?, host = ?, port = ?, username = ?, auth_type = ?, password = ?, private_key = ?, host_key = ?, description = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := s.db.Exec(query,
		server.Name,
		server.Host,
		server.Port,
		server.Username,
		server.AuthType,
		server.Password,
		server.PrivateKey,
		server.HostKey,
		server.Description,
		enabled,
		now,
		server.ID,
	)
	server.UpdatedAt = now
	return err
}

// DeleteSSH deletes an SSH server
func (s *Server) DeleteSSH(id int64) error {
	query := `DELETE FROM ssh_servers WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// SSHClient wraps an SSH client connection
type SSHClient struct {
	client          *ssh.Client
	session         *ssh.Session
	ObservedHostKey string
}

// ConnectSSH connects to an SSH server
func ConnectSSH(server *SSHServer) (*SSHClient, error) {
	var observedHostKey string
	config := &ssh.ClientConfig{
		User:    server.Username,
		Auth:    []ssh.AuthMethod{},
		Timeout: 10 * time.Second,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			observedHostKey = ssh.FingerprintSHA256(key)
			if server.HostKey != "" && server.HostKey != observedHostKey {
				return fmt.Errorf("SSH host key changed: expected %s, got %s", server.HostKey, observedHostKey)
			}
			return nil
		},
	}

	switch server.AuthType {
	case "password":
		config.Auth = append(config.Auth, ssh.Password(server.Password))
	case "key":
		signer, err := ssh.ParsePrivateKey([]byte(server.PrivateKey))
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		config.Auth = append(config.Auth, ssh.PublicKeys(signer))
	default:
		return nil, fmt.Errorf("unsupported SSH authentication type %q", server.AuthType)
	}

	addr := fmt.Sprintf("%s:%d", server.Host, server.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return &SSHClient{client: client, ObservedHostKey: observedHostKey}, nil
}

// connectSSH pins the host key after the first successful connection (TOFU)
// and rejects any later key change in ConnectSSH.
func (s *Server) connectSSH(server *SSHServer) (*SSHClient, error) {
	client, err := ConnectSSH(server)
	if err != nil {
		return nil, err
	}
	if server.ID != 0 && server.HostKey == "" && client.ObservedHostKey != "" {
		if _, err := s.db.Exec(
			`UPDATE ssh_servers SET host_key = ?, updated_at = ? WHERE id = ?`,
			client.ObservedHostKey, time.Now(), server.ID,
		); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("pin SSH host key: %w", err)
		}
		server.HostKey = client.ObservedHostKey
	}
	return client, nil
}

// NewSession creates a new SSH session
func (c *SSHClient) NewSession() error {
	session, err := c.client.NewSession()
	if err != nil {
		return err
	}
	c.session = session
	return nil
}

// StartShell starts an interactive shell
func (c *SSHClient) StartShell(stdin io.Reader, stdout, stderr io.Writer) error {
	if c.session == nil {
		if err := c.NewSession(); err != nil {
			return err
		}
	}

	c.session.Stdin = stdin
	c.session.Stdout = stdout
	c.session.Stderr = stderr

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := c.session.RequestPty("xterm", 80, 24, modes); err != nil {
		return err
	}

	if err := c.session.Shell(); err != nil {
		return err
	}

	return nil
}

// Close closes the SSH connection
func (c *SSHClient) Close() error {
	if c.session != nil {
		c.session.Close()
	}
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// SSHListView handles the SSH servers list view
func (s *Server) SSHListView(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.SSHCreate(w, r)
		return
	}

	servers := s.ListSSH()
	s.Render(w, "ssh/index", H{
		"servers": servers,
	})
}

// SSHNewView handles the new SSH server form
func (s *Server) SSHNewView(w http.ResponseWriter, r *http.Request) {
	s.Render(w, "ssh/new", H{
		"server": nil,
	})
}

// SSHEditView handles the edit SSH server form
func (s *Server) SSHEditView(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.SSHUpdate(w, r)
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	server, ok := s.GetSSH(id)
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	s.Render(w, "ssh/new", H{
		"server": server,
	})
}

// SSHDetailView handles the SSH server detail view with terminal
func (s *Server) SSHDetailView(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		s.SSHDelete(w, r)
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	server, ok := s.GetSSH(id)
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	if r.Method == http.MethodPost {
		s.SSHConnectHandler(w, r, server)
		return
	}

	needConnect := server.Password == "" && server.PrivateKey == ""
	if needConnect {
		s.Render(w, "ssh/new", H{
			"server":      server,
			"connectOnly": true,
		})
		return
	}

	s.Render(w, "ssh/detail", H{
		"server": server,
	})
}

func (s *Server) SSHConnectHandler(w http.ResponseWriter, r *http.Request, server *SSHServer) {
	password := r.FormValue("password")
	privateKey := r.FormValue("private_key")

	if password != "" {
		server.Password = password
		server.AuthType = "password"
	}
	if privateKey != "" {
		server.PrivateKey = privateKey
		server.AuthType = "key"
	}
	if server.Password == "" && server.PrivateKey == "" {
		s.Render(w, "ssh/new", H{
			"server":       server,
			"connectOnly":  true,
			"errorMessage": "credentials must be saved before opening a terminal",
		})
		return
	}
	if err := s.UpdateSSH(server); err != nil {
		s.Render(w, "ssh/new", H{
			"server":       server,
			"connectOnly":  true,
			"errorMessage": err.Error(),
		})
		return
	}
	http.Redirect(w, r, "/ssh/servers/"+strconv.FormatInt(server.ID, 10)+"/terminal", http.StatusSeeOther)
}

// SSHCreate handles creating a new SSH server
func (s *Server) SSHCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	server := &SSHServer{
		Name:        r.FormValue("name"),
		Host:        r.FormValue("host"),
		Port:        22,
		Username:    r.FormValue("username"),
		AuthType:    r.FormValue("auth_type"),
		Password:    r.FormValue("password"),
		PrivateKey:  r.FormValue("private_key"),
		Description: r.FormValue("description"),
	}

	// Parse port
	if port := r.FormValue("port"); port != "" {
		fmt.Sscanf(port, "%d", &server.Port)
	}

	if server.Name == "" || server.Host == "" || server.Username == "" {
		s.Render(w, "ssh/new", H{
			"server":       server,
			"errorMessage": "name, host, and username are required",
		})
		return
	}

	if server.AuthType == "password" && server.Password == "" {
		s.Render(w, "ssh/new", H{
			"server":       server,
			"errorMessage": "password is required for password authentication",
		})
		return
	}

	if server.AuthType == "key" && server.PrivateKey == "" {
		s.Render(w, "ssh/new", H{
			"server":       server,
			"errorMessage": "private key is required for key authentication",
		})
		return
	}

	_, err := s.CreateSSH(server)
	if err != nil {
		s.Render(w, "ssh/new", H{
			"server":       server,
			"errorMessage": err.Error(),
		})
		return
	}
	http.Redirect(w, r, "/ssh/servers", http.StatusSeeOther)
}

// SSHUpdate handles updating an SSH server
func (s *Server) SSHUpdate(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	server, ok := s.GetSSH(id)
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	oldHost, oldPort := server.Host, server.Port
	server.Name = r.FormValue("name")
	server.Host = r.FormValue("host")
	server.Username = r.FormValue("username")
	server.AuthType = r.FormValue("auth_type")
	server.Description = r.FormValue("description")

	// Parse port
	if port := r.FormValue("port"); port != "" {
		fmt.Sscanf(port, "%d", &server.Port)
	}
	if server.Host != oldHost || server.Port != oldPort {
		server.HostKey = ""
	}

	// Update password only if provided
	if password := r.FormValue("password"); password != "" {
		server.Password = password
	}

	// Update private key only if provided
	if privateKey := r.FormValue("private_key"); privateKey != "" {
		server.PrivateKey = privateKey
	}

	if server.Name == "" || server.Host == "" || server.Username == "" {
		s.Render(w, "ssh/new", H{
			"server":       server,
			"errorMessage": "name, host, and username are required",
		})
		return
	}

	if err := s.UpdateSSH(server); err != nil {
		s.Render(w, "ssh/new", H{
			"server":       server,
			"errorMessage": err.Error(),
		})
		return
	}
	http.Redirect(w, r, "/ssh/servers", http.StatusSeeOther)
}

// SSHDelete handles deleting an SSH server
func (s *Server) SSHDelete(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := s.DeleteSSH(id); err != nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, "/ssh/servers", http.StatusSeeOther)
}

// SSHTerminalView renders the shared terminal UI for a saved SSH profile.
func (s *Server) SSHTerminalView(w http.ResponseWriter, r *http.Request) {
	server, ok := s.sshServerFromRequest(w, r)
	if !ok {
		return
	}
	s.renderTerminal(
		w,
		server.Name,
		"/ssh/servers/"+strconv.FormatInt(server.ID, 10)+"/terminal/ws",
		server.Host,
	)
}

// SSHTerminal handles the WebSocket endpoint for a saved SSH profile.
func (s *Server) SSHTerminal(w http.ResponseWriter, r *http.Request) {
	server, ok := s.sshServerFromRequest(w, r)
	if !ok {
		return
	}
	s.serveTerminal(w, r, terminalpkg.BackendFunc(func(ctx context.Context, cols, rows uint16) (terminalpkg.Session, error) {
		return s.OpenSSHTerminal(ctx, server, cols, rows)
	}))
}

func (s *Server) sshServerFromRequest(w http.ResponseWriter, r *http.Request) (*SSHServer, bool) {
	idStr := r.PathValue("id")
	if idStr == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return nil, false
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return nil, false
	}

	server, ok := s.GetSSH(id)
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return nil, false
	}
	if !server.Enabled {
		http.Error(w, "server is disabled", http.StatusConflict)
		return nil, false
	}
	return server, true
}

// OpenSSHTerminal adapts an SSH PTY to the generic terminal session contract.
func (s *Server) OpenSSHTerminal(_ context.Context, server *SSHServer, cols, rows uint16) (terminalpkg.Session, error) {
	client, err := s.connectSSH(server)
	if err != nil {
		return nil, err
	}
	session, err := client.client.NewSession()
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("create SSH session: %w", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		return nil, fmt.Errorf("create SSH stdin: %w", err)
	}
	output, outputWriter := io.Pipe()
	session.Stdout = outputWriter
	session.Stderr = outputWriter

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", int(rows), int(cols), modes); err != nil {
		_ = output.Close()
		_ = outputWriter.Close()
		_ = session.Close()
		_ = client.Close()
		return nil, fmt.Errorf("request SSH PTY: %w", err)
	}
	if err := session.Shell(); err != nil {
		_ = output.Close()
		_ = outputWriter.Close()
		_ = session.Close()
		_ = client.Close()
		return nil, fmt.Errorf("start SSH shell: %w", err)
	}

	return &sshTerminalSession{
		client: client, session: session, stdin: stdin,
		output: output, outputWriter: outputWriter,
	}, nil
}

type sshTerminalSession struct {
	client       *SSHClient
	session      *ssh.Session
	stdin        io.WriteCloser
	output       *io.PipeReader
	outputWriter *io.PipeWriter
	closeOnce    sync.Once
}

func (s *sshTerminalSession) Input() io.WriteCloser { return s.stdin }
func (s *sshTerminalSession) Output() io.Reader     { return s.output }

func (s *sshTerminalSession) Resize(cols, rows uint16) error {
	return s.session.WindowChange(int(rows), int(cols))
}

func (s *sshTerminalSession) Wait() error {
	err := s.session.Wait()
	_ = s.outputWriter.CloseWithError(err)
	return err
}

func (s *sshTerminalSession) Close() error {
	var err error
	s.closeOnce.Do(func() {
		_ = s.stdin.Close()
		_ = s.output.Close()
		_ = s.outputWriter.Close()
		_ = s.session.Close()
		err = s.client.Close()
	})
	return err
}

// SSHTestConnection tests the SSH connection
func (s *Server) SSHTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	server := &SSHServer{
		Host:       r.FormValue("host"),
		Port:       22,
		Username:   r.FormValue("username"),
		AuthType:   r.FormValue("auth_type"),
		Password:   r.FormValue("password"),
		PrivateKey: r.FormValue("private_key"),
	}

	if port := r.FormValue("port"); port != "" {
		fmt.Sscanf(port, "%d", &server.Port)
	}

	sshClient, err := s.connectSSH(server)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	defer sshClient.Close()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(H{
		"success": true,
		"message": "Connection successful",
	})
}

// SSHStatsResponse represents the combined status and stats response
type SSHStatsResponse struct {
	Connected     bool      `json:"connected"`
	ConnectedAt   time.Time `json:"connected_at,omitempty"`
	ClientCount   int       `json:"client_count"`
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryPercent float64   `json:"memory_percent"`
	MemoryUsage   uint64    `json:"memory_usage"`
	MemoryTotal   uint64    `json:"memory_total"`
	NetworkRX     uint64    `json:"network_rx"`
	NetworkTX     uint64    `json:"network_tx"`
	LoadAverage   []float64 `json:"load_average"`
}

// SSHStatsHandler handles SSE connection for SSH server status and stats
func (s *Server) SSHStatsHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	server, ok := s.GetSSH(id)
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Create ticker for periodic updates
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Send updates periodically
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			response := s.collectSSHStats(id, server)
			data, _ := json.Marshal(response)
			fmt.Fprintf(w, "event: stats\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// collectSSHStats collects server stats
func (s *Server) collectSSHStats(serverID int64, server *SSHServer) *SSHStatsResponse {
	response := &SSHStatsResponse{}

	// Collect server stats
	client, err := s.connectSSH(server)
	if err != nil {
		return response
	}
	defer client.Close()

	session, err := client.client.NewSession()
	if err != nil {
		return response
	}
	defer session.Close()

	// Collect CPU, Memory, and Network stats using a single command
	cmd := `
cpu=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | cut -d'%' -f1 2>/dev/null || echo "0")
if [ -z "$cpu" ] || [ "$cpu" = "0" ]; then
  cpu=$(vmstat 1 2 | tail -1 | awk '{print 100 - $15}' 2>/dev/null || echo "0")
fi

mem=$(free -b | awk 'NR==2{printf "%.1f %d %d", $3/$2*100, $3, $2}' 2>/dev/null || echo "0 0 0")
mem_percent=$(echo $mem | awk '{print $1}')
mem_used=$(echo $mem | awk '{print $2}')
mem_total=$(echo $mem | awk '{print $3}')

net=$(cat /proc/net/dev 2>/dev/null | grep -E "^\s*(eth0|ens|enp|wlan|wl)" | head -1 | awk '{print $2, $10}' || echo "0 0")
net_rx=$(echo $net | awk '{print $1}')
net_tx=$(echo $net | awk '{print $2}')

load=$(cat /proc/loadavg 2>/dev/null | awk '{print $1, $2, $3}' || echo "0 0 0")

echo "{\"cpu\":$cpu,\"mem_percent\":$mem_percent,\"mem_used\":$mem_used,\"mem_total\":$mem_total,\"net_rx\":$net_rx,\"net_tx\":$net_tx,\"load\":\"$load\"}"
`

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return response
	}

	// Parse output - extract JSON part
	outputStr := strings.TrimSpace(string(output))
	startIdx := strings.Index(outputStr, "{")
	endIdx := strings.LastIndex(outputStr, "}")
	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		outputStr = outputStr[startIdx : endIdx+1]
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(outputStr), &result); err != nil {
		return response
	}

	// Parse CPU
	if cpu, ok := result["cpu"].(float64); ok {
		response.CPUPercent = cpu
	}

	// Parse Memory
	if memPercent, ok := result["mem_percent"].(float64); ok {
		response.MemoryPercent = memPercent
	}
	if memUsed, ok := result["mem_used"].(float64); ok {
		response.MemoryUsage = uint64(memUsed)
	}
	if memTotal, ok := result["mem_total"].(float64); ok {
		response.MemoryTotal = uint64(memTotal)
	}

	// Parse Network
	if netRX, ok := result["net_rx"].(float64); ok {
		response.NetworkRX = uint64(netRX)
	}
	if netTX, ok := result["net_tx"].(float64); ok {
		response.NetworkTX = uint64(netTX)
	}

	// Parse Load Average
	if loadStr, ok := result["load"].(string); ok {
		parts := strings.Fields(loadStr)
		for _, part := range parts {
			if val, err := strconv.ParseFloat(part, 64); err == nil {
				response.LoadAverage = append(response.LoadAverage, val)
			}
		}
	}

	return response
}

// SSHMetricsHandler exposes SSH server metrics in Prometheus/OpenMetrics format
func (s *Server) SSHMetricsHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		http.Error(w, "id is required\n", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id\n", http.StatusBadRequest)
		return
	}

	server, ok := s.GetSSH(id)
	if !ok {
		http.Error(w, "server not found\n", http.StatusNotFound)
		return
	}

	stats, err := s.collectSSHMetrics(server)
	if err != nil {
		log.Printf("SSH metrics error: %v", err)
		http.Error(w, fmt.Sprintf("collect error: %v\n", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP node_cpu_seconds CPU usage percentage\n")
	fmt.Fprintf(w, "# TYPE node_cpu_seconds gauge\n")
	fmt.Fprintf(w, "node_cpu_seconds{host=%q} %.1f\n", server.Name, stats.CPUUsage)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_memory_total_bytes Total memory in bytes\n")
	fmt.Fprintf(w, "# TYPE node_memory_total_bytes gauge\n")
	fmt.Fprintf(w, "node_memory_total_bytes{host=%q} %d\n", server.Name, stats.MemoryTotal)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_memory_used_bytes Used memory in bytes\n")
	fmt.Fprintf(w, "# TYPE node_memory_used_bytes gauge\n")
	fmt.Fprintf(w, "node_memory_used_bytes{host=%q} %d\n", server.Name, stats.MemoryUsed)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_memory_free_bytes Free memory in bytes\n")
	fmt.Fprintf(w, "# TYPE node_memory_free_bytes gauge\n")
	fmt.Fprintf(w, "node_memory_free_bytes{host=%q} %d\n", server.Name, stats.MemoryFree)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_memory_usage_percent Memory usage percentage\n")
	fmt.Fprintf(w, "# TYPE node_memory_usage_percent gauge\n")
	fmt.Fprintf(w, "node_memory_usage_percent{host=%q} %.1f\n", server.Name, stats.MemoryUsage)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_disk_total_bytes Total disk in bytes\n")
	fmt.Fprintf(w, "# TYPE node_disk_total_bytes gauge\n")
	fmt.Fprintf(w, "node_disk_total_bytes{host=%q} %d\n", server.Name, stats.DiskTotal)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_disk_used_bytes Used disk in bytes\n")
	fmt.Fprintf(w, "# TYPE node_disk_used_bytes gauge\n")
	fmt.Fprintf(w, "node_disk_used_bytes{host=%q} %d\n", server.Name, stats.DiskUsed)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_disk_free_bytes Free disk in bytes\n")
	fmt.Fprintf(w, "# TYPE node_disk_free_bytes gauge\n")
	fmt.Fprintf(w, "node_disk_free_bytes{host=%q} %d\n", server.Name, stats.DiskFree)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_disk_usage_percent Disk usage percentage\n")
	fmt.Fprintf(w, "# TYPE node_disk_usage_percent gauge\n")
	fmt.Fprintf(w, "node_disk_usage_percent{host=%q} %.1f\n", server.Name, stats.DiskUsage)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_network_receive_bytes_total Cumulative network bytes received\n")
	fmt.Fprintf(w, "# TYPE node_network_receive_bytes_total counter\n")
	fmt.Fprintf(w, "node_network_receive_bytes_total{host=%q} %d\n", server.Name, stats.NetBytesRecv)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_network_transmit_bytes_total Cumulative network bytes sent\n")
	fmt.Fprintf(w, "# TYPE node_network_transmit_bytes_total counter\n")
	fmt.Fprintf(w, "node_network_transmit_bytes_total{host=%q} %d\n", server.Name, stats.NetBytesSent)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_load1 1m load average\n")
	fmt.Fprintf(w, "# TYPE node_load1 gauge\n")
	fmt.Fprintf(w, "node_load1{host=%q} %.2f\n", server.Name, stats.Load1)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_load5 5m load average\n")
	fmt.Fprintf(w, "# TYPE node_load5 gauge\n")
	fmt.Fprintf(w, "node_load5{host=%q} %.2f\n", server.Name, stats.Load5)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_load15 15m load average\n")
	fmt.Fprintf(w, "# TYPE node_load15 gauge\n")
	fmt.Fprintf(w, "node_load15{host=%q} %.2f\n", server.Name, stats.Load15)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_uptime_seconds Uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE node_uptime_seconds counter\n")
	fmt.Fprintf(w, "node_uptime_seconds{host=%q} %d\n", server.Name, stats.Uptime)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "# HELP node_cpu_cores Number of CPU cores\n")
	fmt.Fprintf(w, "# TYPE node_cpu_cores gauge\n")
	fmt.Fprintf(w, "node_cpu_cores{host=%q} %d\n", server.Name, stats.CPUCores)
}

// collectSSHMetrics connects via SSH and collects metrics, returns SystemStats without storing
func (s *Server) collectSSHMetrics(server *SSHServer) (*SystemStats, error) {
	client, err := s.connectSSH(server)
	if err != nil {
		return nil, fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	session, err := client.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	cmd := `
cpu=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | cut -d'%' -f1 2>/dev/null || echo "")
if [ -z "$cpu" ] || [ "$cpu" = "" ]; then
  cpu=$(vmstat 1 2 | tail -1 | awk '{print 100 - $15}' 2>/dev/null || echo "0")
fi

mem=$(free -b | awk 'NR==2{printf "%.0f %.0f %.0f", $2, $3, $2-$3}' 2>/dev/null || echo "0 0 0")
mem_total=$(echo $mem | awk '{print $1}')
mem_used=$(echo $mem | awk '{print $2}')
mem_free=$(echo $mem | awk '{print $3}')

disk=$(df -B1 / 2>/dev/null | awk 'NR==2{printf "%.0f %.0f %.0f", $2, $3, $4}' || echo "0 0 0")
disk_total=$(echo $disk | awk '{print $1}')
disk_used=$(echo $disk | awk '{print $2}')
disk_free=$(echo $disk | awk '{print $3}')

net=$(cat /proc/net/dev 2>/dev/null | grep -E "^\s*(eth0|ens|enp|wlan|wl)" | head -1 | awk '{print $2, $10}' || echo "0 0")
net_rx=$(echo $net | awk '{print $1}')
net_tx=$(echo $net | awk '{print $2}')

load=$(cat /proc/loadavg 2>/dev/null | awk '{print $1, $2, $3}' || echo "0 0 0")

uptime=$(cat /proc/uptime 2>/dev/null | awk '{print $1}' || echo "0")

cores=$(nproc 2>/dev/null || echo "1")

echo "{\"cpu\":$cpu,\"mem_total\":$mem_total,\"mem_used\":$mem_used,\"mem_free\":$mem_free,\"disk_total\":$disk_total,\"disk_used\":$disk_used,\"disk_free\":$disk_free,\"net_rx\":$net_rx,\"net_tx\":$net_tx,\"load1\":$(echo $load | awk '{print $1}'),\"load5\":$(echo $load | awk '{print $2}'),\"load15\":$(echo $load | awk '{print $3}'),\"uptime\":$uptime,\"cores\":$cores}"
`

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return nil, fmt.Errorf("ssh exec: %w", err)
	}

	outputStr := strings.TrimSpace(string(output))
	startIdx := strings.Index(outputStr, "{")
	endIdx := strings.LastIndex(outputStr, "}")
	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		return nil, fmt.Errorf("no JSON in output: %s", outputStr)
	}
	outputStr = outputStr[startIdx : endIdx+1]

	var result map[string]float64
	if err := json.Unmarshal([]byte(outputStr), &result); err != nil {
		return nil, fmt.Errorf("json parse: %w", err)
	}

	now := time.Now()
	memTotal := uint64(result["mem_total"])
	memUsed := uint64(result["mem_used"])
	memFree := uint64(result["mem_free"])
	diskTotal := uint64(result["disk_total"])
	diskUsed := uint64(result["disk_used"])
	diskFree := uint64(result["disk_free"])

	stats := &SystemStats{
		Hostname:     server.Name,
		Timestamp:    now,
		CPUUsage:     result["cpu"],
		CPUCores:     int(result["cores"]),
		MemoryTotal:  memTotal,
		MemoryUsed:   memUsed,
		MemoryFree:   memFree,
		DiskTotal:    diskTotal,
		DiskUsed:     diskUsed,
		DiskFree:     diskFree,
		NetBytesSent: uint64(result["net_tx"]),
		NetBytesRecv: uint64(result["net_rx"]),
		Uptime:       uint64(result["uptime"]),
		Load1:        result["load1"],
		Load5:        result["load5"],
		Load15:       result["load15"],
	}

	if memTotal > 0 {
		stats.MemoryUsage = float64(memUsed) / float64(memTotal) * 100
	}
	if diskTotal > 0 {
		stats.DiskUsage = float64(diskUsed) / float64(diskTotal) * 100
	}

	return stats, nil
}

// collectAndStoreSSHStats connects via SSH, collects metrics, and inserts into system_stats
func (s *Server) collectAndStoreSSHStats(server *SSHServer) (*SystemStats, error) {
	stats, err := s.collectSSHMetrics(server)
	if err != nil {
		return nil, err
	}
	if err := s.InsertStats(stats); err != nil {
		return nil, fmt.Errorf("insert stats: %w", err)
	}
	return stats, nil
}
