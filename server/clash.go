package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/lsongdev/clashapi-go/clashapi"
)

// ClashServer represents a Clash server configuration
type ClashServer struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	APIURL      string    `json:"api_url"`
	Secret      string    `json:"-"`      // Never serialize
	Status      string    `json:"status"` // active, inactive
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// initClashSchema creates the Clash server table
func (s *Server) initClashSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS clash_servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		api_url TEXT NOT NULL,
		secret TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		description TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_clash_servers_name ON clash_servers(name);
	`
	_, err := s.db.Exec(query)
	return err
}

// CreateClashServer creates a new Clash server
func (s *Server) CreateClashServer(name, apiURL, secret, description string) (*ClashServer, error) {

	now := time.Now()
	query := `
	INSERT INTO clash_servers (name, api_url, secret, status, description, created_at, updated_at)
	VALUES (?, ?, ?, 'active', ?, ?, ?)
	`
	result, err := s.db.Exec(query, name, apiURL, secret, description, now, now)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &ClashServer{
		ID:          id,
		Name:        name,
		APIURL:      apiURL,
		Secret:      secret,
		Status:      "active",
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetAllClashServers retrieves all Clash servers (without secret for list view)
func (s *Server) GetAllClashServers() ([]ClashServer, error) {

	query := `
	SELECT id, name, api_url, status, description, created_at, updated_at
	FROM clash_servers
	ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []ClashServer
	for rows.Next() {
		var server ClashServer
		var createdAt, updatedAt string
		if err := rows.Scan(&server.ID, &server.Name, &server.APIURL, &server.Status, &server.Description, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		server.CreatedAt = parseTime(createdAt)
		server.UpdatedAt = parseTime(updatedAt)
		servers = append(servers, server)
	}

	return servers, rows.Err()
}

// GetClashServerByID retrieves a Clash server by ID (with secret)
func (s *Server) GetClashServerByID(id int64) (*ClashServer, error) {

	query := `
	SELECT id, name, api_url, secret, status, description, created_at, updated_at
	FROM clash_servers
	WHERE id = ?
	`

	var server ClashServer
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, id).Scan(&server.ID, &server.Name, &server.APIURL, &server.Secret, &server.Status, &server.Description, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	server.CreatedAt = parseTime(createdAt)
	server.UpdatedAt = parseTime(updatedAt)

	return &server, nil
}

// UpdateClashServer updates an existing Clash server
func (s *Server) UpdateClashServer(id int64, name, apiURL, secret, description string) error {

	now := time.Now()
	query := `
	UPDATE clash_servers
	SET name = ?, api_url = ?, secret = ?, description = ?, updated_at = ?
	WHERE id = ?
	`
	_, err := s.db.Exec(query, name, apiURL, secret, description, now, id)
	return err
}

// DeleteClashServer deletes a Clash server by ID
func (s *Server) DeleteClashServer(id int64) error {

	query := `DELETE FROM clash_servers WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

// GetClashClient creates a Clash API client from server config
func (cs *ClashServer) GetClashClient() *clashapi.Client {
	return clashapi.NewClient(cs.APIURL, cs.Secret)
}

// ======= HTTP Handlers =======

// ClashIndexView renders the Clash server list page
func (s *Server) ClashIndexView(w http.ResponseWriter, r *http.Request) {
	servers, err := s.GetAllClashServers()
	if err != nil {
		log.Printf("Error fetching Clash servers: %v", err)
		s.Error(w, err)
		return
	}

	s.Render(w, "clash/index", H{
		"servers": servers,
	})
}

// ClashNewHandler handles creating a new Clash server
func (s *Server) ClashNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "clash/new", H{})
		return
	}

	// POST: Create new Clash server
	name := r.FormValue("name")
	apiURL := r.FormValue("api_url")
	secret := r.FormValue("secret")
	description := r.FormValue("description")

	if name == "" || apiURL == "" {
		s.Render(w, "clash/new", H{
			"error":       "Name and API URL are required",
			"name":        name,
			"apiURL":      apiURL,
			"secret":      secret,
			"description": description,
		})
		return
	}

	_, err := s.CreateClashServer(name, apiURL, secret, description)
	if err != nil {
		s.Render(w, "clash/new", H{
			"error":       "Failed to create Clash server: " + err.Error(),
			"name":        name,
			"apiURL":      apiURL,
			"secret":      secret,
			"description": description,
		})
		return
	}

	http.Redirect(w, r, "/clash", http.StatusSeeOther)
}

// ClashEditHandler handles editing a Clash server
func (s *Server) ClashEditHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetClashServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Clash server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	if r.Method == http.MethodGet {
		s.Render(w, "clash/new", H{
			"server": server,
		})
		return
	}

	// POST: Update Clash server
	name := r.FormValue("name")
	apiURL := r.FormValue("api_url")
	secret := r.FormValue("secret")
	description := r.FormValue("description")

	if name == "" || apiURL == "" {
		s.Render(w, "clash/new", H{
			"error":  "Name and API URL are required",
			"server": server,
		})
		return
	}

	// If secret is empty, keep the old one
	if secret == "" {
		secret = server.Secret
	}

	err = s.UpdateClashServer(id, name, apiURL, secret, description)
	if err != nil {
		s.Render(w, "clash/new", H{
			"error":  "Failed to update Clash server: " + err.Error(),
			"server": server,
		})
		return
	}

	http.Redirect(w, r, "/clash", http.StatusSeeOther)
}

// ClashDetailHandler handles viewing/deleting a Clash server
func (s *Server) ClashDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetClashServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Clash server not found", http.StatusNotFound)
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
		// Connect and fetch dashboard data on-demand
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := server.GetClashClient()
		var connectionError string
		var config *clashapi.Config
		var proxyCount, ruleCount, connCount int
		var uploadTotal, downloadTotal int64

		config, err = client.GetConfig(ctx)
		if err != nil {
			connectionError = err.Error()
		} else {
			proxiesResp, proxiesErr := client.GetProxies(ctx)
			if proxiesErr == nil && proxiesResp != nil {
				proxyCount = len(proxiesResp.Proxies)
			}

			rules, rulesErr := client.GetRules(ctx)
			if rulesErr == nil {
				ruleCount = len(rules)
			}

			conns, connsErr := client.GetConnections(ctx)
			if connsErr == nil {
				connCount = len(conns.Connections)
				uploadTotal = conns.UploadTotal
				downloadTotal = conns.DownloadTotal
			}
		}

		s.Render(w, "clash/detail", H{
			"server":          server,
			"connectionError": connectionError,
			"config":          config,
			"proxyCount":      proxyCount,
			"ruleCount":       ruleCount,
			"connectionCount": connCount,
			"uploadTotal":     uploadTotal,
			"downloadTotal":   downloadTotal,
		})

	case http.MethodPut:
		// Update Clash server
		name := r.FormValue("name")
		apiURL := r.FormValue("api_url")
		secret := r.FormValue("secret")
		description := r.FormValue("description")

		if secret == "" {
			secret = server.Secret
		}

		err = s.UpdateClashServer(id, name, apiURL, secret, description)
		if err != nil {
			s.Render(w, "clash/detail", H{
				"error":  "Failed to update Clash server: " + err.Error(),
				"server": server,
			})
			return
		}

		http.Redirect(w, r, "/clash", http.StatusSeeOther)

	case http.MethodDelete:
		err = s.DeleteClashServer(id)
		if err != nil {
			s.Error(w, err)
			return
		}
		http.Redirect(w, r, "/clash", http.StatusSeeOther)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ClashTestConnectionHandler tests the connection to a Clash server
func (s *Server) ClashTestConnectionHandler(w http.ResponseWriter, r *http.Request) {
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

	server, err := s.GetClashServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Clash server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := server.GetClashClient()
	if err := client.TestConnection(ctx); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Connection failed: " + err.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Connection successful"))
}

// ClashProxiesView renders the proxies page
func (s *Server) ClashProxiesView(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetClashServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Clash server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := server.GetClashClient()
	proxiesResp, err := client.GetProxies(ctx)
	if err != nil {
		s.Error(w, err)
		return
	}

	proxyProviders, _ := client.GetProxyProviders(ctx)

	// Filter proxies: only include those with len(All) > 0, mapping All members to full proxy objects
	groupProxies := make(map[string]*clashapi.Proxy)
	for name, proxy := range proxiesResp.Proxies {
		if len(proxy.All) > 0 {
			members := make(map[string]*clashapi.Proxy)
			for _, memberName := range proxy.All {
				if member, ok := proxiesResp.Proxies[memberName]; ok {
					members[memberName] = member
				}
			}
			groupProxy := *proxy
			groupProxy.All = nil
			groupProxy.Proxies = make([]clashapi.Proxy, 0, len(members))
			for _, m := range members {
				groupProxy.Proxies = append(groupProxy.Proxies, *m)
			}
			groupProxies[name] = &groupProxy
		}
	}

	s.Render(w, "clash/proxies", H{
		"server":         server,
		"proxies":        groupProxies,
		"proxyProviders": proxyProviders,
	})
}

// ClashRulesView renders the rules page
func (s *Server) ClashRulesView(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetClashServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Clash server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := server.GetClashClient()
	rules, err := client.GetRules(ctx)
	if err != nil {
		s.Render(w, "clash/rules", H{
			"server":          server,
			"connectionError": err.Error(),
		})
		return
	}

	ruleProviders, _ := client.GetRuleProviders(ctx)

	s.Render(w, "clash/rules", H{
		"server":        server,
		"rules":         rules,
		"ruleProviders": ruleProviders,
	})
}

// ClashConnectionsView renders the connections page
func (s *Server) ClashConnectionsView(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	server, err := s.GetClashServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Clash server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	s.Render(w, "clash/connections", H{
		"server": server,
	})
}

// ClashStreamSSE handles SSE connection for real-time clash data.
// Connects to Clash /connections WebSocket for connections + totals.
// Also connects to /traffic WebSocket for real-time speed.
// When /connections data arrives, sends both traffic and connections events.
func (s *Server) ClashStreamSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	server, err := s.GetClashServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Clash server not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sse, err := NewSSE(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := server.GetClashClient()

	// Connect to /traffic WebSocket for speed
	speedCh, speedErr := client.GetTrafficStream(ctx)
	if speedErr != nil {
		log.Printf("Traffic WebSocket not available: %v", speedErr)
		speedCh = nil
	}

	// Connect to /connections WebSocket for connections + totals
	connsCh, connsErr := client.GetConnectionsStream(ctx)
	if connsErr != nil {
		http.Error(w, fmt.Sprintf("Connections WebSocket not available: %v", connsErr), http.StatusInternalServerError)
		return
	}

	sendTraffic := func(traffic clashapi.Traffic) {
		trafficJSON, _ := json.Marshal(map[string]interface{}{
			"uploadSpeed":   traffic.Up,
			"downloadSpeed": traffic.Down,
		})
		sse.Send("traffic", string(trafficJSON))
	}

	sendConnections := func(conns clashapi.ConnectionsResponse) {
		data := map[string]interface{}{
			"connections":   conns.Connections,
			"uploadTotal":   conns.UploadTotal,
			"downloadTotal": conns.DownloadTotal,
			"connCount":     len(conns.Connections),
		}
		connsJSON, _ := json.Marshal(data)
		sse.Send("connections", string(connsJSON))
	}

	for {
		select {
		case <-r.Context().Done():
			return

		case conns, ok := <-connsCh:
			if !ok {
				log.Println("Connections WebSocket closed")
				return
			}
			sendConnections(conns)

		case traffic, ok := <-speedCh:
			if !ok {
				speedCh = nil
				continue
			}
			sendTraffic(traffic)
		}
	}
}

// ClashSettingsSaveHandler handles saving settings
func (s *Server) ClashConfigHandler(w http.ResponseWriter, r *http.Request) {
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

	server, err := s.GetClashServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Clash server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := server.GetClashClient()

	patch := map[string]interface{}{}

	mode := r.FormValue("mode")
	if mode != "" {
		patch["mode"] = mode
	}

	if mixedPort := r.FormValue("mixedPort"); mixedPort != "" {
		if mp, err := strconv.Atoi(mixedPort); err == nil {
			patch["mixed-port"] = mp
		}
	}

	if port := r.FormValue("port"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			patch["port"] = p
		}
	}

	if socksPort := r.FormValue("socksPort"); socksPort != "" {
		if sp, err := strconv.Atoi(socksPort); err == nil {
			patch["socks-port"] = sp
		}
	}

	if redirPort := r.FormValue("redirPort"); redirPort != "" {
		if rp, err := strconv.Atoi(redirPort); err == nil {
			patch["redir-port"] = rp
		}
	}

	if logLevel := r.FormValue("logLevel"); logLevel != "" {
		patch["log-level"] = logLevel
	}

	patch["allow-lan"] = r.FormValue("allowLan") == "true"
	patch["ipv6"] = r.FormValue("ipv6") == "true"

	if len(patch) == 0 {
		http.Redirect(w, r, fmt.Sprintf("/clash/%d/settings", id), http.StatusSeeOther)
		return
	}

	err = client.UpdateConfig(ctx, patch)
	if err != nil {
		// Re-fetch config to show form again with error
		config, _ := client.GetConfig(ctx)
		s.Render(w, "clash/settings", H{
			"server":       server,
			"config":       config,
			"errorMessage": "Failed to save settings: " + err.Error(),
		})
		return
	}

	// Re-fetch config to show success
	config, _ := client.GetConfig(ctx)
	s.Render(w, "clash/settings", H{
		"server":         server,
		"config":         config,
		"successMessage": "Settings saved successfully.",
	})
}

// ClashProxyActionHandler handles proxy actions (switch, delay test, mode change)
func (s *Server) ClashProxyActionHandler(w http.ResponseWriter, r *http.Request) {
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

	server, err := s.GetClashServerByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Clash server not found", http.StatusNotFound)
			return
		}
		s.Error(w, err)
		return
	}

	action := r.FormValue("action")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := server.GetClashClient()

	switch action {
	case "select":
		selector := r.FormValue("selector")
		proxyName := r.FormValue("proxy_name")
		if selector == "" || proxyName == "" {
			s.Error(w, &Error{Message: "selector and proxy_name are required"})
			return
		}
		if err := client.SelectProxy(ctx, selector, proxyName); err != nil {
			s.Error(w, &Error{Message: "Failed to switch proxy: " + err.Error()})
			return
		}

	case "delay":
		proxyName := r.FormValue("proxy_name")
		if proxyName == "" {
			s.Error(w, &Error{Message: "proxy_name is required"})
			return
		}
		delay, err := client.GetProxyDelay(ctx, proxyName, "", 5000)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Delay test failed: " + err.Error()))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"delay": %d}`, delay)))
		return

	case "mode":
		mode := r.FormValue("mode")
		if mode == "" {
			s.Error(w, &Error{Message: "mode is required"})
			return
		}
		if err := client.SetMode(ctx, mode); err != nil {
			s.Error(w, &Error{Message: "Failed to set mode: " + err.Error()})
			return
		}
	}

	// Redirect back to detail page
	http.Redirect(w, r, fmt.Sprintf("/clash/%d", id), http.StatusSeeOther)
}

func init() {
	RegisterInitSchemaHook((*Server).initClashSchema)
}
