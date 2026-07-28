package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lsongdev/yabber"
)

type WebListener struct {
	ID                     int64
	Name                   string
	Address                string
	Protocol               string
	Enabled                bool
	MinTLSVersion          string
	DefaultCertificateID   *int64
	DefaultCertificateName string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type WebHost struct {
	ID              int64
	Name            string
	Hostnames       string
	Enabled         bool
	CertificateID   *int64
	CertificateName string
	ForceHTTPS      bool
	ListenerIDs     []int64
	ListenerNames   []string
	Routes          []WebRoute
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type WebRoute struct {
	ID               int64
	HostID           int64
	HostName         string
	Name             string
	Enabled          bool
	Method           string
	Path             string
	HandlerType      string
	UpstreamURL      string
	Root             string
	StripPrefix      string
	ResponseStatus   int
	ResponseBody     string
	RedirectLocation string
	RedirectStatus   int
	WorkerScript     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (s *Server) initWebServerSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS web_listeners (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		address TEXT NOT NULL,
		protocol TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		min_tls_version TEXT NOT NULL DEFAULT 'TLS1.2',
		default_certificate_id INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (default_certificate_id) REFERENCES certificates(id)
	);
	CREATE TABLE IF NOT EXISTS web_hosts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		hostnames TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		certificate_id INTEGER,
		force_https INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (certificate_id) REFERENCES certificates(id)
	);
	CREATE TABLE IF NOT EXISTS web_host_listeners (
		host_id INTEGER NOT NULL,
		listener_id INTEGER NOT NULL,
		PRIMARY KEY (host_id, listener_id),
		FOREIGN KEY (host_id) REFERENCES web_hosts(id) ON DELETE CASCADE,
		FOREIGN KEY (listener_id) REFERENCES web_listeners(id) ON DELETE CASCADE
	);
	CREATE TABLE IF NOT EXISTS web_routes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		host_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		method TEXT NOT NULL DEFAULT '',
		path TEXT NOT NULL DEFAULT '/',
		handler_type TEXT NOT NULL,
		upstream_url TEXT NOT NULL DEFAULT '',
		root TEXT NOT NULL DEFAULT '',
		strip_prefix TEXT NOT NULL DEFAULT '',
		response_status INTEGER NOT NULL DEFAULT 0,
		response_body TEXT NOT NULL DEFAULT '',
		redirect_location TEXT NOT NULL DEFAULT '',
		redirect_status INTEGER NOT NULL DEFAULT 0,
		worker_script TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (host_id) REFERENCES web_hosts(id) ON DELETE CASCADE,
		UNIQUE (host_id, name)
	);
	CREATE INDEX IF NOT EXISTS idx_web_listeners_enabled ON web_listeners(enabled);
	CREATE INDEX IF NOT EXISTS idx_web_hosts_enabled ON web_hosts(enabled);
	CREATE INDEX IF NOT EXISTS idx_web_routes_host ON web_routes(host_id);
	`
	if _, err := s.db.Exec(query); err != nil {
		return err
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM web_listeners`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		_, err := s.db.Exec(`
			INSERT INTO web_listeners (name, address, protocol, enabled)
			VALUES ('http', ?, 'http', 1)
		`, ":8081")
		return err
	}
	return nil
}

func (s *Server) GetAllWebListeners() ([]WebListener, error) {
	rows, err := s.db.Query(`
		SELECT l.id, l.name, l.address, l.protocol, l.enabled, l.min_tls_version,
		       l.default_certificate_id, COALESCE(c.domains, ''),
		       l.created_at, l.updated_at
		FROM web_listeners l
		LEFT JOIN certificates c ON c.id = l.default_certificate_id
		ORDER BY l.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var listeners []WebListener
	for rows.Next() {
		listener, err := scanWebListener(rows)
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, *listener)
	}
	return listeners, rows.Err()
}

func (s *Server) GetWebListener(id int64) (*WebListener, error) {
	row := s.db.QueryRow(`
		SELECT l.id, l.name, l.address, l.protocol, l.enabled, l.min_tls_version,
		       l.default_certificate_id, COALESCE(c.domains, ''),
		       l.created_at, l.updated_at
		FROM web_listeners l
		LEFT JOIN certificates c ON c.id = l.default_certificate_id
		WHERE l.id = ?
	`, id)
	return scanWebListener(row)
}

type rowScanner interface {
	Scan(...any) error
}

func scanWebListener(row rowScanner) (*WebListener, error) {
	var listener WebListener
	var certificateID sql.NullInt64
	var createdAt, updatedAt string
	if err := row.Scan(
		&listener.ID, &listener.Name, &listener.Address, &listener.Protocol,
		&listener.Enabled, &listener.MinTLSVersion, &certificateID,
		&listener.DefaultCertificateName, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	if certificateID.Valid {
		listener.DefaultCertificateID = &certificateID.Int64
	}
	listener.CreatedAt = parseTime(createdAt)
	listener.UpdatedAt = parseTime(updatedAt)
	return &listener, nil
}

func (s *Server) CreateWebListener(listener *WebListener) (int64, error) {
	result, err := s.db.Exec(`
		INSERT INTO web_listeners
			(name, address, protocol, enabled, min_tls_version, default_certificate_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, listener.Name, listener.Address, listener.Protocol, listener.Enabled,
		listener.MinTLSVersion, nullableInt64(listener.DefaultCertificateID), time.Now(), time.Now())
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Server) UpdateWebListener(listener *WebListener) error {
	_, err := s.db.Exec(`
		UPDATE web_listeners
		SET name = ?, address = ?, protocol = ?, enabled = ?,
		    min_tls_version = ?, default_certificate_id = ?, updated_at = ?
		WHERE id = ?
	`, listener.Name, listener.Address, listener.Protocol, listener.Enabled,
		listener.MinTLSVersion, nullableInt64(listener.DefaultCertificateID), time.Now(), listener.ID)
	return err
}

func (s *Server) DeleteWebListener(id int64) error {
	var references int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM web_host_listeners WHERE listener_id = ?
	`, id).Scan(&references); err != nil {
		return err
	}
	if references > 0 {
		return fmt.Errorf("listener is used by %d virtual host(s)", references)
	}
	_, err := s.db.Exec(`DELETE FROM web_listeners WHERE id = ?`, id)
	return err
}

func (s *Server) GetAllWebHosts() ([]WebHost, error) {
	rows, err := s.db.Query(`
		SELECT h.id, h.name, h.hostnames, h.enabled, h.certificate_id,
		       COALESCE(c.domains, ''), h.force_https, h.created_at, h.updated_at
		FROM web_hosts h
		LEFT JOIN certificates c ON c.id = h.certificate_id
		ORDER BY h.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	var hosts []WebHost
	for rows.Next() {
		host, err := scanWebHost(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		hosts = append(hosts, *host)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range hosts {
		if err := s.loadWebHostRelations(&hosts[i]); err != nil {
			return nil, err
		}
	}
	return hosts, nil
}

func (s *Server) GetWebHost(id int64) (*WebHost, error) {
	row := s.db.QueryRow(`
		SELECT h.id, h.name, h.hostnames, h.enabled, h.certificate_id,
		       COALESCE(c.domains, ''), h.force_https, h.created_at, h.updated_at
		FROM web_hosts h
		LEFT JOIN certificates c ON c.id = h.certificate_id
		WHERE h.id = ?
	`, id)
	host, err := scanWebHost(row)
	if err != nil {
		return nil, err
	}
	if err := s.loadWebHostRelations(host); err != nil {
		return nil, err
	}
	return host, nil
}

func scanWebHost(row rowScanner) (*WebHost, error) {
	var host WebHost
	var certificateID sql.NullInt64
	var createdAt, updatedAt string
	if err := row.Scan(
		&host.ID, &host.Name, &host.Hostnames, &host.Enabled, &certificateID,
		&host.CertificateName, &host.ForceHTTPS, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	if certificateID.Valid {
		host.CertificateID = &certificateID.Int64
	}
	host.CreatedAt = parseTime(createdAt)
	host.UpdatedAt = parseTime(updatedAt)
	return &host, nil
}

func (s *Server) loadWebHostRelations(host *WebHost) error {
	rows, err := s.db.Query(`
		SELECT l.id, l.name
		FROM web_host_listeners hl
		JOIN web_listeners l ON l.id = hl.listener_id
		WHERE hl.host_id = ?
		ORDER BY l.created_at ASC
	`, host.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return err
		}
		host.ListenerIDs = append(host.ListenerIDs, id)
		host.ListenerNames = append(host.ListenerNames, name)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	routes, err := s.GetWebRoutesByHost(host.ID)
	if err != nil {
		return err
	}
	host.Routes = routes
	return nil
}

func (s *Server) CreateWebHost(host *WebHost) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now()
	result, err := tx.Exec(`
		INSERT INTO web_hosts
			(name, hostnames, enabled, certificate_id, force_https, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, host.Name, host.Hostnames, host.Enabled, nullableInt64(host.CertificateID), host.ForceHTTPS, now, now)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := replaceHostListeners(tx, id, host.ListenerIDs); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Server) UpdateWebHost(host *WebHost) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		UPDATE web_hosts
		SET name = ?, hostnames = ?, enabled = ?, certificate_id = ?,
		    force_https = ?, updated_at = ?
		WHERE id = ?
	`, host.Name, host.Hostnames, host.Enabled, nullableInt64(host.CertificateID),
		host.ForceHTTPS, time.Now(), host.ID); err != nil {
		return err
	}
	if err := replaceHostListeners(tx, host.ID, host.ListenerIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func replaceHostListeners(tx *sql.Tx, hostID int64, listenerIDs []int64) error {
	if _, err := tx.Exec(`DELETE FROM web_host_listeners WHERE host_id = ?`, hostID); err != nil {
		return err
	}
	for _, listenerID := range listenerIDs {
		if _, err := tx.Exec(`
			INSERT INTO web_host_listeners (host_id, listener_id) VALUES (?, ?)
		`, hostID, listenerID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) DeleteWebHost(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`DELETE FROM web_routes WHERE host_id = ?`,
		`DELETE FROM web_host_listeners WHERE host_id = ?`,
		`DELETE FROM web_hosts WHERE id = ?`,
	} {
		if _, err := tx.Exec(query, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Server) GetWebRoutesByHost(hostID int64) ([]WebRoute, error) {
	rows, err := s.db.Query(`
		SELECT r.id, r.host_id, h.name, r.name, r.enabled, r.method, r.path,
		       r.handler_type, r.upstream_url, r.root, r.strip_prefix,
		       r.response_status, r.response_body, r.redirect_location,
		       r.redirect_status, r.worker_script,
		       r.created_at, r.updated_at
		FROM web_routes r
		JOIN web_hosts h ON h.id = r.host_id
		WHERE r.host_id = ?
		ORDER BY r.created_at ASC
	`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var routes []WebRoute
	for rows.Next() {
		route, err := scanWebRoute(rows)
		if err != nil {
			return nil, err
		}
		routes = append(routes, *route)
	}
	return routes, rows.Err()
}

func (s *Server) GetWebRoute(id int64) (*WebRoute, error) {
	row := s.db.QueryRow(`
		SELECT r.id, r.host_id, h.name, r.name, r.enabled, r.method, r.path,
		       r.handler_type, r.upstream_url, r.root, r.strip_prefix,
		       r.response_status, r.response_body, r.redirect_location,
		       r.redirect_status, r.worker_script,
		       r.created_at, r.updated_at
		FROM web_routes r
		JOIN web_hosts h ON h.id = r.host_id
		WHERE r.id = ?
	`, id)
	return scanWebRoute(row)
}

func scanWebRoute(row rowScanner) (*WebRoute, error) {
	var route WebRoute
	var createdAt, updatedAt string
	if err := row.Scan(
		&route.ID, &route.HostID, &route.HostName, &route.Name, &route.Enabled,
		&route.Method, &route.Path, &route.HandlerType, &route.UpstreamURL,
		&route.Root, &route.StripPrefix, &route.ResponseStatus,
		&route.ResponseBody, &route.RedirectLocation, &route.RedirectStatus,
		&route.WorkerScript,
		&createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	route.CreatedAt = parseTime(createdAt)
	route.UpdatedAt = parseTime(updatedAt)
	return &route, nil
}

func (s *Server) CreateWebRoute(route *WebRoute) (int64, error) {
	now := time.Now()
	result, err := s.db.Exec(`
		INSERT INTO web_routes
			(host_id, name, enabled, method, path, handler_type, upstream_url,
			 root, strip_prefix, response_status, response_body,
			 redirect_location, redirect_status, worker_script, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, route.HostID, route.Name, route.Enabled, route.Method, route.Path,
		route.HandlerType, route.UpstreamURL, route.Root, route.StripPrefix,
		route.ResponseStatus, route.ResponseBody, route.RedirectLocation,
		route.RedirectStatus, route.WorkerScript, now, now)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Server) UpdateWebRoute(route *WebRoute) error {
	_, err := s.db.Exec(`
		UPDATE web_routes
		SET name = ?, enabled = ?, method = ?, path = ?, handler_type = ?,
		    upstream_url = ?, root = ?, strip_prefix = ?, response_status = ?,
		    response_body = ?, redirect_location = ?, redirect_status = ?,
		    worker_script = ?, updated_at = ?
		WHERE id = ?
	`, route.Name, route.Enabled, route.Method, route.Path, route.HandlerType,
		route.UpstreamURL, route.Root, route.StripPrefix, route.ResponseStatus,
		route.ResponseBody, route.RedirectLocation, route.RedirectStatus,
		route.WorkerScript, time.Now(), route.ID)
	return err
}

func (s *Server) DeleteWebRoute(id int64) error {
	_, err := s.db.Exec(`DELETE FROM web_routes WHERE id = ?`, id)
	return err
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

type databaseCertificateLoader struct {
	server *Server
}

func (l databaseCertificateLoader) LoadCertificate(definition yabber.Certificate) (*tls.Certificate, error) {
	idText := strings.TrimPrefix(definition.Name, "certificate:")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid certificate reference %q", definition.Name)
	}
	certificate, err := l.server.GetCertificate(id)
	if err != nil {
		return nil, err
	}
	if certificate.Status != "active" || certificate.CertPem == "" {
		return nil, fmt.Errorf("certificate %d is not active", id)
	}
	loaded, err := tls.X509KeyPair([]byte(certificate.CertPem), []byte(certificate.PrivateKeyPem))
	if err != nil {
		return nil, err
	}
	return &loaded, nil
}

func certificateReference(id *int64) string {
	if id == nil {
		return ""
	}
	return fmt.Sprintf("certificate:%d", *id)
}

func (s *Server) buildYabberConfig() (yabber.Config, error) {
	listeners, err := s.GetAllWebListeners()
	if err != nil {
		return yabber.Config{}, err
	}
	hosts, err := s.GetAllWebHosts()
	if err != nil {
		return yabber.Config{}, err
	}
	config := yabber.Config{
		Version: yabber.ConfigVersion,
		Server: yabber.ServerConfig{
			ShutdownTimeout:   10 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
	}
	certificateNames := make(map[string]struct{})
	for _, listener := range listeners {
		enabled := listener.Enabled
		item := yabber.Listener{
			Name:     listener.Name,
			Address:  listener.Address,
			Protocol: yabber.Protocol(listener.Protocol),
			Enabled:  &enabled,
		}
		if listener.Protocol == string(yabber.ProtocolHTTPS) {
			item.TLS = &yabber.TLSConfig{
				MinVersion:         listener.MinTLSVersion,
				DefaultCertificate: certificateReference(listener.DefaultCertificateID),
			}
			if item.TLS.DefaultCertificate != "" && listener.Enabled {
				certificateNames[item.TLS.DefaultCertificate] = struct{}{}
			}
		}
		config.Listeners = append(config.Listeners, item)
	}

	for _, host := range hosts {
		enabled := host.Enabled
		item := yabber.VirtualHost{
			Name:      host.Name,
			Hostnames: splitHostnames(host.Hostnames),
			Listeners: host.ListenerNames,
			Enabled:   &enabled,
			HTTP:      &yabber.HostHTTP{RedirectToHTTPS: host.ForceHTTPS},
		}
		if host.CertificateID != nil {
			name := certificateReference(host.CertificateID)
			item.TLS = &yabber.HostTLS{Certificate: name}
			if host.Enabled {
				certificateNames[name] = struct{}{}
			}
		}
		for _, route := range host.Routes {
			routeEnabled := route.Enabled
			yabberRoute := yabber.Route{
				Name:    route.Name,
				Enabled: &routeEnabled,
				Match: yabber.Match{
					Method: route.Method,
					Path:   route.Path,
				},
			}
			switch route.HandlerType {
			case "reverse_proxy":
				yabberRoute.Handle.ReverseProxy = &yabber.ReverseProxy{
					URL: route.UpstreamURL, StripPrefix: route.StripPrefix,
				}
			case "file_server":
				yabberRoute.Handle.FileServer = &yabber.FileServer{
					Root: route.Root, StripPrefix: route.StripPrefix,
				}
			case "static_response":
				yabberRoute.Handle.StaticResponse = &yabber.StaticResponse{
					Status: route.ResponseStatus, Body: route.ResponseBody,
				}
			case "redirect":
				yabberRoute.Handle.Redirect = &yabber.Redirect{
					Location: route.RedirectLocation, Status: route.RedirectStatus,
				}
			case "worker":
				yabberRoute.Handle.Worker = &yabber.WorkerHandler{Script: route.WorkerScript}
			default:
				return yabber.Config{}, fmt.Errorf("route %q has invalid handler type %q", route.Name, route.HandlerType)
			}
			item.Routes = append(item.Routes, yabberRoute)
		}
		config.Hosts = append(config.Hosts, item)
	}
	for name := range certificateNames {
		config.Certificates = append(config.Certificates, yabber.Certificate{Name: name})
	}
	return config, nil
}

func splitHostnames(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
}

func (s *Server) applyYabber() error {
	if s.serviceDesiredEnabled("web", true) {
		if err := s.validateServiceStart("web"); err != nil {
			return err
		}
	}
	s.webServerMu.Lock()
	defer s.webServerMu.Unlock()

	config, err := s.buildYabberConfig()
	if err != nil {
		return err
	}
	if s.webServer == nil {
		instance, err := yabber.New(
			config,
			yabber.WithCertificateLoader(databaseCertificateLoader{server: s}),
			yabber.WithHTTP01Store(s),
		)
		if err != nil {
			return err
		}
		if err := instance.Start(); err != nil {
			return err
		}
		s.webServer = instance
		return nil
	}
	if err := s.webServer.Apply(config); err != nil {
		if !errors.Is(err, yabber.ErrRestartRequired) {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.webServer.Restart(ctx, config)
	}
	return nil
}

func (s *Server) stopWebServer() error {
	s.webServerMu.Lock()
	defer s.webServerMu.Unlock()
	if s.webServer == nil {
		return nil
	}
	err := s.webServer.Close()
	s.webServer = nil
	return err
}

func (s *Server) activeCertificates() ([]Certificate, error) {
	certificates, err := s.GetAllCertificates()
	if err != nil {
		return nil, err
	}
	active := certificates[:0]
	for _, certificate := range certificates {
		if certificate.Status == "active" && certificate.CertPem != "" {
			active = append(active, certificate)
		}
	}
	return active, nil
}

func (s *Server) WebServerView(w http.ResponseWriter, r *http.Request) {
	listeners, err := s.GetAllWebListeners()
	if err != nil {
		s.Error(w, err)
		return
	}
	hosts, err := s.GetAllWebHosts()
	if err != nil {
		s.Error(w, err)
		return
	}
	addresses := map[string]string{}
	running := false
	s.webServerMu.Lock()
	if s.webServer != nil {
		addresses = s.webServer.Addresses()
		running = s.webServer.Running()
	}
	s.webServerMu.Unlock()
	s.Render(w, "webservers/index", H{
		"listeners": listeners,
		"hosts":     hosts,
		"addresses": addresses,
		"running":   running,
	})
}

func (s *Server) WebListenerNewHandler(w http.ResponseWriter, r *http.Request) {
	certificates, err := s.activeCertificates()
	if err != nil {
		s.Error(w, err)
		return
	}
	if r.Method == http.MethodGet {
		s.Render(w, "webservers/listener", H{"certificates": certificates, "isEdit": false})
		return
	}
	listener, err := parseWebListenerForm(r)
	if err != nil {
		s.Render(w, "webservers/listener", H{"error": err.Error(), "listener": listener, "certificates": certificates, "isEdit": false})
		return
	}
	id, err := s.CreateWebListener(listener)
	if err == nil {
		listener.ID = id
		err = s.applyYabber()
		if err != nil {
			_ = s.DeleteWebListener(id)
		}
	}
	if err != nil {
		s.Render(w, "webservers/listener", H{"error": err.Error(), "listener": listener, "certificates": certificates, "isEdit": false})
		return
	}
	http.Redirect(w, r, "/web", http.StatusSeeOther)
}

func (s *Server) WebListenerHandler(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	listener, err := s.GetWebListener(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	certificates, err := s.activeCertificates()
	if err != nil {
		s.Error(w, err)
		return
	}
	method := requestMethod(r)
	switch method {
	case http.MethodGet:
		s.Render(w, "webservers/listener", H{"listener": listener, "certificates": certificates, "isEdit": true})
	case http.MethodPut:
		updated, err := parseWebListenerForm(r)
		if err != nil {
			updated.ID = id
			s.Render(w, "webservers/listener", H{"error": err.Error(), "listener": updated, "certificates": certificates, "isEdit": true})
			return
		}
		updated.ID = id
		if err := s.UpdateWebListener(updated); err == nil {
			err = s.applyYabber()
			if err != nil {
				_ = s.UpdateWebListener(listener)
			}
		}
		if err != nil {
			s.Render(w, "webservers/listener", H{"error": err.Error(), "listener": updated, "certificates": certificates, "isEdit": true})
			return
		}
		http.Redirect(w, r, "/web", http.StatusSeeOther)
	case http.MethodDelete:
		if err := s.DeleteWebListener(id); err == nil {
			err = s.applyYabber()
		}
		if err != nil {
			s.Error(w, err)
			return
		}
		http.Redirect(w, r, "/web", http.StatusSeeOther)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func parseWebListenerForm(r *http.Request) (*WebListener, error) {
	if err := r.ParseForm(); err != nil {
		return &WebListener{}, err
	}
	listener := &WebListener{
		Name:          strings.TrimSpace(r.FormValue("name")),
		Address:       strings.TrimSpace(r.FormValue("address")),
		Protocol:      r.FormValue("protocol"),
		Enabled:       r.FormValue("enabled") == "on",
		MinTLSVersion: r.FormValue("min_tls_version"),
	}
	if listener.MinTLSVersion == "" {
		listener.MinTLSVersion = "TLS1.2"
	}
	if id := r.FormValue("default_certificate_id"); id != "" {
		value, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return listener, errors.New("invalid default certificate")
		}
		listener.DefaultCertificateID = &value
	}
	if listener.Name == "" || listener.Address == "" {
		return listener, errors.New("name and address are required")
	}
	if listener.Protocol != "http" && listener.Protocol != "https" {
		return listener, errors.New("protocol must be http or https")
	}
	return listener, nil
}

func (s *Server) WebHostNewHandler(w http.ResponseWriter, r *http.Request) {
	s.webHostForm(w, r, nil)
}

func (s *Server) WebHostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := pathID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	host, err := s.GetWebHost(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "webservers/host", H{"host": host})
}

func (s *Server) WebHostEditHandler(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	host, err := s.GetWebHost(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	s.webHostForm(w, r, host)
}

func (s *Server) WebHostRoutesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := pathID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	host, err := s.GetWebHost(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "webservers/routes", H{"host": host})
}

func (s *Server) webHostForm(w http.ResponseWriter, r *http.Request, current *WebHost) {
	listeners, err := s.GetAllWebListeners()
	if err != nil {
		s.Error(w, err)
		return
	}
	certificates, err := s.activeCertificates()
	if err != nil {
		s.Error(w, err)
		return
	}
	if r.Method == http.MethodGet {
		s.Render(w, "webservers/host_form", H{"host": current, "listeners": listeners, "certificates": certificates, "isEdit": current != nil})
		return
	}
	method := requestMethod(r)
	if method == http.MethodDelete && current != nil {
		if err := s.DeleteWebHost(current.ID); err == nil {
			err = s.applyYabber()
		}
		if err != nil {
			s.Error(w, err)
			return
		}
		http.Redirect(w, r, "/web", http.StatusSeeOther)
		return
	}
	host, err := parseWebHostForm(r)
	if current != nil {
		host.ID = current.ID
		host.Routes = current.Routes
	}
	if err == nil && host.CertificateID != nil {
		certificate, certificateErr := s.GetCertificate(*host.CertificateID)
		if certificateErr != nil {
			err = errors.New("selected certificate does not exist")
		} else if certificate.Status != "active" || !certificateCoversHostnames(certificate, splitHostnames(host.Hostnames)) {
			err = errors.New("selected certificate does not cover every host name")
		}
	}
	if err != nil {
		s.Render(w, "webservers/host_form", H{"error": err.Error(), "host": host, "listeners": listeners, "certificates": certificates, "isEdit": current != nil})
		return
	}
	if current == nil {
		host.ID, err = s.CreateWebHost(host)
	} else {
		err = s.UpdateWebHost(host)
	}
	if err == nil {
		err = s.applyYabber()
		if err != nil {
			if current == nil {
				_ = s.DeleteWebHost(host.ID)
			} else {
				_ = s.UpdateWebHost(current)
			}
		}
	}
	if err != nil {
		s.Render(w, "webservers/host_form", H{"error": err.Error(), "host": host, "listeners": listeners, "certificates": certificates, "isEdit": current != nil})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/web/hosts/%d", host.ID), http.StatusSeeOther)
}

func parseWebHostForm(r *http.Request) (*WebHost, error) {
	if err := r.ParseForm(); err != nil {
		return &WebHost{}, err
	}
	host := &WebHost{
		Name:       strings.TrimSpace(r.FormValue("name")),
		Hostnames:  strings.TrimSpace(r.FormValue("hostnames")),
		Enabled:    r.FormValue("enabled") == "on",
		ForceHTTPS: r.FormValue("force_https") == "on",
	}
	for _, value := range r.Form["listener_ids"] {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return host, errors.New("invalid listener")
		}
		host.ListenerIDs = append(host.ListenerIDs, id)
	}
	if value := r.FormValue("certificate_id"); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return host, errors.New("invalid certificate")
		}
		host.CertificateID = &id
	}
	if host.Name == "" || len(splitHostnames(host.Hostnames)) == 0 {
		return host, errors.New("name and at least one hostname are required")
	}
	if len(host.ListenerIDs) == 0 {
		return host, errors.New("at least one listener is required")
	}
	return host, nil
}

func certificateCoversHostnames(certificate *Certificate, hostnames []string) bool {
	if certificate == nil || certificate.CertPem == "" {
		return false
	}
	block, _ := pem.Decode([]byte(certificate.CertPem))
	if block == nil {
		return false
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	for _, hostname := range hostnames {
		if err := leaf.VerifyHostname(strings.TrimSuffix(strings.ToLower(hostname), ".")); err != nil {
			return false
		}
	}
	return true
}

func (s *Server) WebRouteNewHandler(w http.ResponseWriter, r *http.Request) {
	hostID, err := pathID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	host, err := s.GetWebHost(hostID)
	if err != nil {
		s.Error(w, err)
		return
	}
	s.webRouteForm(w, r, host, nil)
}

func (s *Server) WebRouteHandler(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	route, err := s.GetWebRoute(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	host, err := s.GetWebHost(route.HostID)
	if err != nil {
		s.Error(w, err)
		return
	}
	s.webRouteForm(w, r, host, route)
}

func (s *Server) webRouteForm(w http.ResponseWriter, r *http.Request, host *WebHost, current *WebRoute) {
	if r.Method == http.MethodGet {
		s.Render(w, "webservers/route", H{"host": host, "route": current, "isEdit": current != nil})
		return
	}
	method := requestMethod(r)
	if method == http.MethodDelete && current != nil {
		err := s.DeleteWebRoute(current.ID)
		if err == nil {
			err = s.applyYabber()
		}
		if err != nil {
			s.Error(w, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/web/hosts/%d/routes", host.ID), http.StatusSeeOther)
		return
	}
	route, err := parseWebRouteForm(r)
	route.HostID = host.ID
	if current != nil {
		route.ID = current.ID
	}
	if err != nil {
		s.Render(w, "webservers/route", H{"error": err.Error(), "host": host, "route": route, "isEdit": current != nil})
		return
	}
	if current == nil {
		route.ID, err = s.CreateWebRoute(route)
	} else {
		err = s.UpdateWebRoute(route)
	}
	if err == nil {
		err = s.applyYabber()
		if err != nil {
			if current == nil {
				_ = s.DeleteWebRoute(route.ID)
			} else {
				_ = s.UpdateWebRoute(current)
			}
		}
	}
	if err != nil {
		s.Render(w, "webservers/route", H{"error": err.Error(), "host": host, "route": route, "isEdit": current != nil})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/web/hosts/%d/routes", host.ID), http.StatusSeeOther)
}

func parseWebRouteForm(r *http.Request) (*WebRoute, error) {
	if err := r.ParseForm(); err != nil {
		return &WebRoute{}, err
	}
	route := &WebRoute{
		Name:             strings.TrimSpace(r.FormValue("name")),
		Enabled:          r.FormValue("enabled") == "on",
		Method:           strings.ToUpper(strings.TrimSpace(r.FormValue("method"))),
		Path:             strings.TrimSpace(r.FormValue("path")),
		HandlerType:      r.FormValue("handler_type"),
		UpstreamURL:      strings.TrimSpace(r.FormValue("upstream_url")),
		Root:             strings.TrimSpace(r.FormValue("root")),
		StripPrefix:      strings.TrimSpace(r.FormValue("strip_prefix")),
		ResponseBody:     r.FormValue("response_body"),
		RedirectLocation: strings.TrimSpace(r.FormValue("redirect_location")),
		WorkerScript:     r.FormValue("worker_script"),
	}
	route.ResponseStatus, _ = strconv.Atoi(r.FormValue("response_status"))
	route.RedirectStatus, _ = strconv.Atoi(r.FormValue("redirect_status"))
	if route.Path == "" {
		route.Path = "/"
	}
	if route.Name == "" {
		return route, errors.New("name is required")
	}
	switch route.HandlerType {
	case "reverse_proxy":
		if route.UpstreamURL == "" {
			return route, errors.New("upstream URL is required")
		}
		route.Root = ""
		route.ResponseBody = ""
		route.ResponseStatus = 0
		route.RedirectLocation = ""
		route.RedirectStatus = 0
	case "file_server":
		if route.Root == "" {
			return route, errors.New("root is required")
		}
		route.UpstreamURL = ""
		route.ResponseBody = ""
		route.ResponseStatus = 0
		route.RedirectLocation = ""
		route.RedirectStatus = 0
	case "static_response":
		route.UpstreamURL = ""
		route.Root = ""
		route.StripPrefix = ""
		route.RedirectLocation = ""
		route.RedirectStatus = 0
	case "redirect":
		if route.RedirectLocation == "" {
			return route, errors.New("redirect location is required")
		}
		route.UpstreamURL = ""
		route.Root = ""
		route.StripPrefix = ""
		route.ResponseBody = ""
		route.ResponseStatus = 0
	case "worker":
		if strings.TrimSpace(route.WorkerScript) == "" {
			return route, errors.New("worker script is required")
		}
		route.UpstreamURL = ""
		route.Root = ""
		route.StripPrefix = ""
		route.ResponseBody = ""
		route.ResponseStatus = 0
		route.RedirectLocation = ""
		route.RedirectStatus = 0
	default:
		return route, errors.New("invalid handler type")
	}
	return route, nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, errors.New("invalid ID")
	}
	return id, nil
}

func requestMethod(r *http.Request) string {
	if r.Method == http.MethodPost {
		if method := r.FormValue("_method"); method != "" {
			return strings.ToUpper(method)
		}
	}
	return r.Method
}

// SeedDefaultWebWorker idempotently provisions the minimal public Web Server
// configuration used by install.sh. It only manages resources named
// "http", "default", and "worker", leaving all other user configuration intact.
func (s *Server) SeedDefaultWebWorker() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()
	if _, err := tx.Exec(`
		INSERT INTO web_listeners
			(name, address, protocol, enabled, min_tls_version, created_at, updated_at)
		VALUES ('http', ':80', 'http', 1, 'TLS1.2', ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			address = excluded.address,
			protocol = excluded.protocol,
			enabled = 1,
			updated_at = excluded.updated_at
	`, now, now); err != nil {
		return err
	}

	var listenerID int64
	if err := tx.QueryRow(`SELECT id FROM web_listeners WHERE name = 'http'`).Scan(&listenerID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO web_hosts
			(name, hostnames, enabled, force_https, created_at, updated_at)
		VALUES ('default', '*', 1, 0, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			hostnames = excluded.hostnames,
			enabled = 1,
			force_https = 0,
			updated_at = excluded.updated_at
	`, now, now); err != nil {
		return err
	}

	var hostID int64
	if err := tx.QueryRow(`SELECT id FROM web_hosts WHERE name = 'default'`).Scan(&hostID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO web_host_listeners (host_id, listener_id)
		VALUES (?, ?)
		ON CONFLICT(host_id, listener_id) DO NOTHING
	`, hostID, listenerID); err != nil {
		return err
	}

	const script = `export default {
  fetch(request, env) {
    return new Response("OhMyServer Web Server", {
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    });
  }
}`
	if _, err := tx.Exec(`
		INSERT INTO web_routes
			(host_id, name, enabled, method, path, handler_type, worker_script,
			 created_at, updated_at)
		VALUES (?, 'worker', 1, '', '/', 'worker', ?, ?, ?)
		ON CONFLICT(host_id, name) DO UPDATE SET
			enabled = 1,
			method = '',
			path = '/',
			handler_type = 'worker',
			worker_script = excluded.worker_script,
			updated_at = excluded.updated_at
	`, hostID, script, now, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO service_preferences (service_id, enabled, updated_at)
		VALUES ('web', 1, ?)
		ON CONFLICT(service_id) DO UPDATE SET enabled = 1, updated_at = excluded.updated_at
	`, now); err != nil {
		return err
	}
	return tx.Commit()
}

func init() {
	RegisterInitSchemaHook((*Server).initWebServerSchema)
	RegisterMigrateHook(func(s *Server) error {
		_, err := s.db.Exec(`ALTER TABLE web_routes ADD COLUMN worker_script TEXT NOT NULL DEFAULT ''`)
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
		// Preserve scripts created by the former standalone /workers feature.
		_, _ = s.db.Exec(`
			UPDATE web_routes
			SET worker_script = COALESCE(
				(SELECT script FROM workers WHERE workers.id = web_routes.worker_id),
				worker_script
			)
			WHERE handler_type = 'worker' AND worker_script = ''
		`)
		return nil
	})
	RegisterInitHook(func(s *Server) error {
		if !s.serviceDesiredEnabled("web", true) {
			return nil
		}
		if err := s.validateServiceStart("web"); err != nil {
			s.setServiceError("web", err)
			return nil
		}
		if err := s.applyYabber(); err != nil {
			log.Printf("Yabber failed to start: %v", err)
			s.setServiceError("web", err)
			return nil
		}
		s.setServiceError("web", nil)
		return nil
	})
	RegisterCloseHook(func(s *Server) error {
		return s.stopWebServer()
	})
}
