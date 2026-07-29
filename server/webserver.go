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
	TrustedProxies         string
	PolicyIDs              []int64
	PolicyNames            []string
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
	PolicyIDs       []int64
	PolicyNames     []string
	Routes          []WebRoute
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type WebRoute struct {
	ID                           int64
	HostID                       int64
	HostName                     string
	Name                         string
	Enabled                      bool
	Method                       string
	Path                         string
	HandlerType                  string
	UpstreamURL                  string
	Root                         string
	StripPrefix                  string
	ResponseStatus               int
	ResponseBody                 string
	RedirectLocation             string
	RedirectStatus               int
	WorkerScript                 string
	RequestTimeoutSeconds        int
	ResponseHeaderTimeoutSeconds int
	MaxConnsPerHost              int
	PolicyIDs                    []int64
	PolicyNames                  []string
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
}

type WebPolicy struct {
	ID                     int64
	Name                   string
	MaxBodyBytes           int64
	RequestTimeoutSeconds  int
	RateRequestsPerSecond  float64
	RateBurst              int
	RateKey                string
	RateMaxKeys            int
	RateIdleTimeoutSeconds int
	ConcurrencyMax         int
	CreatedAt              time.Time
	UpdatedAt              time.Time
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
		trusted_proxies TEXT NOT NULL DEFAULT '',
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
		request_timeout_seconds INTEGER NOT NULL DEFAULT 0,
		response_header_timeout_seconds INTEGER NOT NULL DEFAULT 0,
		max_conns_per_host INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (host_id) REFERENCES web_hosts(id) ON DELETE CASCADE,
		UNIQUE (host_id, name)
	);
	CREATE TABLE IF NOT EXISTS web_policies (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		max_body_bytes INTEGER NOT NULL DEFAULT 0,
		request_timeout_seconds INTEGER NOT NULL DEFAULT 0,
		rate_requests_per_second REAL NOT NULL DEFAULT 0,
		rate_burst INTEGER NOT NULL DEFAULT 0,
		rate_key TEXT NOT NULL DEFAULT 'client_ip',
		rate_max_keys INTEGER NOT NULL DEFAULT 10000,
		rate_idle_timeout_seconds INTEGER NOT NULL DEFAULT 600,
		concurrency_max INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS web_listener_policies (
		listener_id INTEGER NOT NULL,
		policy_id INTEGER NOT NULL,
		position INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (listener_id, policy_id),
		FOREIGN KEY (listener_id) REFERENCES web_listeners(id) ON DELETE CASCADE,
		FOREIGN KEY (policy_id) REFERENCES web_policies(id) ON DELETE RESTRICT
	);
	CREATE TABLE IF NOT EXISTS web_host_policies (
		host_id INTEGER NOT NULL,
		policy_id INTEGER NOT NULL,
		position INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (host_id, policy_id),
		FOREIGN KEY (host_id) REFERENCES web_hosts(id) ON DELETE CASCADE,
		FOREIGN KEY (policy_id) REFERENCES web_policies(id) ON DELETE RESTRICT
	);
	CREATE TABLE IF NOT EXISTS web_route_policies (
		route_id INTEGER NOT NULL,
		policy_id INTEGER NOT NULL,
		position INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (route_id, policy_id),
		FOREIGN KEY (route_id) REFERENCES web_routes(id) ON DELETE CASCADE,
		FOREIGN KEY (policy_id) REFERENCES web_policies(id) ON DELETE RESTRICT
	);
	CREATE INDEX IF NOT EXISTS idx_web_listeners_enabled ON web_listeners(enabled);
	CREATE INDEX IF NOT EXISTS idx_web_hosts_enabled ON web_hosts(enabled);
	CREATE INDEX IF NOT EXISTS idx_web_routes_host ON web_routes(host_id);
	`
	if _, err := s.db.Exec(query); err != nil {
		return err
	}
	for _, column := range []struct {
		table      string
		name       string
		definition string
	}{
		{"web_listeners", "trusted_proxies", "TEXT NOT NULL DEFAULT ''"},
		{"web_routes", "request_timeout_seconds", "INTEGER NOT NULL DEFAULT 0"},
		{"web_routes", "response_header_timeout_seconds", "INTEGER NOT NULL DEFAULT 0"},
		{"web_routes", "max_conns_per_host", "INTEGER NOT NULL DEFAULT 0"},
	} {
		if err := s.ensureWebServerColumn(column.table, column.name, column.definition); err != nil {
			return err
		}
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

func (s *Server) ensureWebServerColumn(table, name, definition string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var columnName, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if columnName == name {
			return rows.Close()
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	_, err = s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + name + " " + definition)
	return err
}

func (s *Server) GetAllWebListeners() ([]WebListener, error) {
	rows, err := s.db.Query(`
		SELECT l.id, l.name, l.address, l.protocol, l.enabled, l.min_tls_version,
		       l.default_certificate_id, COALESCE(c.domains, ''), l.trusted_proxies,
		       l.created_at, l.updated_at
		FROM web_listeners l
		LEFT JOIN certificates c ON c.id = l.default_certificate_id
		ORDER BY l.created_at ASC
	`)
	if err != nil {
		return nil, err
	}

	var listeners []WebListener
	for rows.Next() {
		listener, err := scanWebListener(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		listeners = append(listeners, *listener)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range listeners {
		if err := s.loadWebPolicyRelations("listener", listeners[i].ID, &listeners[i].PolicyIDs, &listeners[i].PolicyNames); err != nil {
			return nil, err
		}
	}
	return listeners, nil
}

func (s *Server) GetWebListener(id int64) (*WebListener, error) {
	row := s.db.QueryRow(`
		SELECT l.id, l.name, l.address, l.protocol, l.enabled, l.min_tls_version,
		       l.default_certificate_id, COALESCE(c.domains, ''), l.trusted_proxies,
		       l.created_at, l.updated_at
		FROM web_listeners l
		LEFT JOIN certificates c ON c.id = l.default_certificate_id
		WHERE l.id = ?
	`, id)
	listener, err := scanWebListener(row)
	if err != nil {
		return nil, err
	}
	if err := s.loadWebPolicyRelations("listener", listener.ID, &listener.PolicyIDs, &listener.PolicyNames); err != nil {
		return nil, err
	}
	return listener, nil
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
		&listener.DefaultCertificateName, &listener.TrustedProxies, &createdAt, &updatedAt,
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
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`
		INSERT INTO web_listeners
			(name, address, protocol, enabled, min_tls_version, default_certificate_id,
			 trusted_proxies, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, listener.Name, listener.Address, listener.Protocol, listener.Enabled,
		listener.MinTLSVersion, nullableInt64(listener.DefaultCertificateID),
		listener.TrustedProxies, time.Now(), time.Now())
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := replaceWebPolicyRelations(tx, "listener", id, listener.PolicyIDs); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Server) UpdateWebListener(listener *WebListener) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		UPDATE web_listeners
		SET name = ?, address = ?, protocol = ?, enabled = ?,
		    min_tls_version = ?, default_certificate_id = ?, trusted_proxies = ?, updated_at = ?
		WHERE id = ?
	`, listener.Name, listener.Address, listener.Protocol, listener.Enabled,
		listener.MinTLSVersion, nullableInt64(listener.DefaultCertificateID),
		listener.TrustedProxies, time.Now(), listener.ID); err != nil {
		return err
	}
	if err := replaceWebPolicyRelations(tx, "listener", listener.ID, listener.PolicyIDs); err != nil {
		return err
	}
	return tx.Commit()
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
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM web_listener_policies WHERE listener_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM web_listeners WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
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
	if err := s.loadWebPolicyRelations("host", host.ID, &host.PolicyIDs, &host.PolicyNames); err != nil {
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
	if err := replaceWebPolicyRelations(tx, "host", id, host.PolicyIDs); err != nil {
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
	if err := replaceWebPolicyRelations(tx, "host", host.ID, host.PolicyIDs); err != nil {
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
		`DELETE FROM web_route_policies WHERE route_id IN (SELECT id FROM web_routes WHERE host_id = ?)`,
		`DELETE FROM web_routes WHERE host_id = ?`,
		`DELETE FROM web_host_policies WHERE host_id = ?`,
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
		       r.request_timeout_seconds, r.response_header_timeout_seconds,
		       r.max_conns_per_host,
		       r.created_at, r.updated_at
		FROM web_routes r
		JOIN web_hosts h ON h.id = r.host_id
		WHERE r.host_id = ?
		ORDER BY r.created_at ASC
	`, hostID)
	if err != nil {
		return nil, err
	}
	var routes []WebRoute
	for rows.Next() {
		route, err := scanWebRoute(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		routes = append(routes, *route)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range routes {
		if err := s.loadWebPolicyRelations("route", routes[i].ID, &routes[i].PolicyIDs, &routes[i].PolicyNames); err != nil {
			return nil, err
		}
	}
	return routes, nil
}

func (s *Server) GetWebRoute(id int64) (*WebRoute, error) {
	row := s.db.QueryRow(`
		SELECT r.id, r.host_id, h.name, r.name, r.enabled, r.method, r.path,
		       r.handler_type, r.upstream_url, r.root, r.strip_prefix,
		       r.response_status, r.response_body, r.redirect_location,
		       r.redirect_status, r.worker_script,
		       r.request_timeout_seconds, r.response_header_timeout_seconds,
		       r.max_conns_per_host,
		       r.created_at, r.updated_at
		FROM web_routes r
		JOIN web_hosts h ON h.id = r.host_id
		WHERE r.id = ?
	`, id)
	route, err := scanWebRoute(row)
	if err != nil {
		return nil, err
	}
	if err := s.loadWebPolicyRelations("route", route.ID, &route.PolicyIDs, &route.PolicyNames); err != nil {
		return nil, err
	}
	return route, nil
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
		&route.RequestTimeoutSeconds, &route.ResponseHeaderTimeoutSeconds,
		&route.MaxConnsPerHost,
		&createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	route.CreatedAt = parseTime(createdAt)
	route.UpdatedAt = parseTime(updatedAt)
	return &route, nil
}

func (s *Server) CreateWebRoute(route *WebRoute) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now()
	result, err := tx.Exec(`
		INSERT INTO web_routes
			(host_id, name, enabled, method, path, handler_type, upstream_url,
			 root, strip_prefix, response_status, response_body,
			 redirect_location, redirect_status, worker_script,
			 request_timeout_seconds, response_header_timeout_seconds,
			 max_conns_per_host, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, route.HostID, route.Name, route.Enabled, route.Method, route.Path,
		route.HandlerType, route.UpstreamURL, route.Root, route.StripPrefix,
		route.ResponseStatus, route.ResponseBody, route.RedirectLocation,
		route.RedirectStatus, route.WorkerScript, route.RequestTimeoutSeconds,
		route.ResponseHeaderTimeoutSeconds, route.MaxConnsPerHost, now, now)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := replaceWebPolicyRelations(tx, "route", id, route.PolicyIDs); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Server) UpdateWebRoute(route *WebRoute) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		UPDATE web_routes
		SET name = ?, enabled = ?, method = ?, path = ?, handler_type = ?,
		    upstream_url = ?, root = ?, strip_prefix = ?, response_status = ?,
		    response_body = ?, redirect_location = ?, redirect_status = ?,
		    worker_script = ?, request_timeout_seconds = ?,
		    response_header_timeout_seconds = ?, max_conns_per_host = ?, updated_at = ?
		WHERE id = ?
	`, route.Name, route.Enabled, route.Method, route.Path, route.HandlerType,
		route.UpstreamURL, route.Root, route.StripPrefix, route.ResponseStatus,
		route.ResponseBody, route.RedirectLocation, route.RedirectStatus,
		route.WorkerScript, route.RequestTimeoutSeconds,
		route.ResponseHeaderTimeoutSeconds, route.MaxConnsPerHost,
		time.Now(), route.ID); err != nil {
		return err
	}
	if err := replaceWebPolicyRelations(tx, "route", route.ID, route.PolicyIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Server) DeleteWebRoute(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM web_route_policies WHERE route_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM web_routes WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func webPolicyRelation(scope string) (table, idColumn string, ok bool) {
	switch scope {
	case "listener":
		return "web_listener_policies", "listener_id", true
	case "host":
		return "web_host_policies", "host_id", true
	case "route":
		return "web_route_policies", "route_id", true
	default:
		return "", "", false
	}
}

func (s *Server) loadWebPolicyRelations(scope string, id int64, ids *[]int64, names *[]string) error {
	table, idColumn, ok := webPolicyRelation(scope)
	if !ok {
		return fmt.Errorf("invalid web policy scope %q", scope)
	}
	rows, err := s.db.Query(`
		SELECT p.id, p.name
		FROM `+table+` b
		JOIN web_policies p ON p.id = b.policy_id
		WHERE b.`+idColumn+` = ?
		ORDER BY b.position ASC, p.name ASC
	`, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var policyID int64
		var policyName string
		if err := rows.Scan(&policyID, &policyName); err != nil {
			return err
		}
		*ids = append(*ids, policyID)
		*names = append(*names, policyName)
	}
	return rows.Err()
}

func replaceWebPolicyRelations(tx *sql.Tx, scope string, id int64, policyIDs []int64) error {
	table, idColumn, ok := webPolicyRelation(scope)
	if !ok {
		return fmt.Errorf("invalid web policy scope %q", scope)
	}
	if _, err := tx.Exec(`DELETE FROM `+table+` WHERE `+idColumn+` = ?`, id); err != nil {
		return err
	}
	for position, policyID := range policyIDs {
		if _, err := tx.Exec(
			`INSERT INTO `+table+` (`+idColumn+`, policy_id, position) VALUES (?, ?, ?)`,
			id, policyID, position,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) GetAllWebPolicies() ([]WebPolicy, error) {
	rows, err := s.db.Query(`
		SELECT id, name, max_body_bytes, request_timeout_seconds,
		       rate_requests_per_second, rate_burst, rate_key, rate_max_keys,
		       rate_idle_timeout_seconds, concurrency_max, created_at, updated_at
		FROM web_policies
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []WebPolicy
	for rows.Next() {
		policy, err := scanWebPolicy(rows)
		if err != nil {
			return nil, err
		}
		policies = append(policies, *policy)
	}
	return policies, rows.Err()
}

func (s *Server) GetWebPolicy(id int64) (*WebPolicy, error) {
	return scanWebPolicy(s.db.QueryRow(`
		SELECT id, name, max_body_bytes, request_timeout_seconds,
		       rate_requests_per_second, rate_burst, rate_key, rate_max_keys,
		       rate_idle_timeout_seconds, concurrency_max, created_at, updated_at
		FROM web_policies
		WHERE id = ?
	`, id))
}

func scanWebPolicy(row rowScanner) (*WebPolicy, error) {
	var policy WebPolicy
	var createdAt, updatedAt string
	if err := row.Scan(
		&policy.ID, &policy.Name, &policy.MaxBodyBytes, &policy.RequestTimeoutSeconds,
		&policy.RateRequestsPerSecond, &policy.RateBurst, &policy.RateKey,
		&policy.RateMaxKeys, &policy.RateIdleTimeoutSeconds, &policy.ConcurrencyMax,
		&createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	policy.CreatedAt = parseTime(createdAt)
	policy.UpdatedAt = parseTime(updatedAt)
	return &policy, nil
}

func (s *Server) CreateWebPolicy(policy *WebPolicy) (int64, error) {
	result, err := s.db.Exec(`
		INSERT INTO web_policies
			(name, max_body_bytes, request_timeout_seconds, rate_requests_per_second,
			 rate_burst, rate_key, rate_max_keys, rate_idle_timeout_seconds,
			 concurrency_max, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, policy.Name, policy.MaxBodyBytes, policy.RequestTimeoutSeconds,
		policy.RateRequestsPerSecond, policy.RateBurst, policy.RateKey,
		policy.RateMaxKeys, policy.RateIdleTimeoutSeconds, policy.ConcurrencyMax,
		time.Now(), time.Now())
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Server) UpdateWebPolicy(policy *WebPolicy) error {
	_, err := s.db.Exec(`
		UPDATE web_policies
		SET name = ?, max_body_bytes = ?, request_timeout_seconds = ?,
		    rate_requests_per_second = ?, rate_burst = ?, rate_key = ?,
		    rate_max_keys = ?, rate_idle_timeout_seconds = ?,
		    concurrency_max = ?, updated_at = ?
		WHERE id = ?
	`, policy.Name, policy.MaxBodyBytes, policy.RequestTimeoutSeconds,
		policy.RateRequestsPerSecond, policy.RateBurst, policy.RateKey,
		policy.RateMaxKeys, policy.RateIdleTimeoutSeconds, policy.ConcurrencyMax,
		time.Now(), policy.ID)
	return err
}

func (s *Server) DeleteWebPolicy(id int64) error {
	var references int
	if err := s.db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM web_listener_policies WHERE policy_id = ?) +
			(SELECT COUNT(*) FROM web_host_policies WHERE policy_id = ?) +
			(SELECT COUNT(*) FROM web_route_policies WHERE policy_id = ?)
	`, id, id, id).Scan(&references); err != nil {
		return err
	}
	if references > 0 {
		return fmt.Errorf("policy is used by %d listener, host, or route binding(s)", references)
	}
	_, err := s.db.Exec(`DELETE FROM web_policies WHERE id = ?`, id)
	return err
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
	policies, err := s.GetAllWebPolicies()
	if err != nil {
		return yabber.Config{}, err
	}
	config := yabber.Config{
		Version: yabber.ConfigVersion,
		Server: yabber.ServerConfig{
			ShutdownTimeout:   10 * time.Second,
			ReadTimeout:       30 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    1 << 20,
		},
	}
	for _, policy := range policies {
		item := yabber.Policy{Name: policy.Name}
		if policy.MaxBodyBytes > 0 || policy.RequestTimeoutSeconds > 0 {
			item.Request = &yabber.RequestPolicy{
				MaxBodyBytes: policy.MaxBodyBytes,
				Timeout:      time.Duration(policy.RequestTimeoutSeconds) * time.Second,
			}
		}
		if policy.RateRequestsPerSecond > 0 {
			item.RateLimit = &yabber.RateLimitPolicy{
				RequestsPerSecond: policy.RateRequestsPerSecond,
				Burst:             policy.RateBurst,
				Key:               policy.RateKey,
				MaxKeys:           policy.RateMaxKeys,
				IdleTimeout:       time.Duration(policy.RateIdleTimeoutSeconds) * time.Second,
			}
		}
		if policy.ConcurrencyMax > 0 {
			item.Concurrency = &yabber.ConcurrencyPolicy{Max: policy.ConcurrencyMax}
		}
		config.Policies = append(config.Policies, item)
	}
	certificateNames := make(map[string]struct{})
	for _, listener := range listeners {
		enabled := listener.Enabled
		item := yabber.Listener{
			Name:           listener.Name,
			Address:        listener.Address,
			Protocol:       yabber.Protocol(listener.Protocol),
			Enabled:        &enabled,
			TrustedProxies: splitLines(listener.TrustedProxies),
			Policies:       listener.PolicyNames,
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
			Policies:  host.PolicyNames,
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
				Policies: route.PolicyNames,
			}
			switch route.HandlerType {
			case "reverse_proxy":
				yabberRoute.Handle.ReverseProxy = &yabber.ReverseProxy{
					URL:                   route.UpstreamURL,
					StripPrefix:           route.StripPrefix,
					RequestTimeout:        time.Duration(route.RequestTimeoutSeconds) * time.Second,
					ResponseHeaderTimeout: time.Duration(route.ResponseHeaderTimeoutSeconds) * time.Second,
					MaxConnsPerHost:       route.MaxConnsPerHost,
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

func splitLines(value string) []string {
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
			yabber.WithAccessLog(log.Writer()),
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
	running := false
	var metrics yabber.MetricsSnapshot
	s.webServerMu.Lock()
	if s.webServer != nil {
		running = s.webServer.Running()
		metrics = s.webServer.Metrics()
	}
	s.webServerMu.Unlock()
	s.Render(w, "webservers/index", H{
		"running": running,
		"metrics": metrics,
	})
}

func (s *Server) WebListenersView(w http.ResponseWriter, r *http.Request) {
	listeners, err := s.GetAllWebListeners()
	if err != nil {
		s.Error(w, err)
		return
	}
	addresses := map[string]string{}
	var statuses map[string]yabber.ListenerStatus
	s.webServerMu.Lock()
	if s.webServer != nil {
		addresses = s.webServer.Addresses()
		statuses = s.webServer.ListenerStatuses()
	}
	s.webServerMu.Unlock()
	s.Render(w, "webservers/listeners", H{
		"listeners": listeners,
		"addresses": addresses,
		"statuses":  statuses,
	})
}

func (s *Server) WebPoliciesView(w http.ResponseWriter, r *http.Request) {
	policies, err := s.GetAllWebPolicies()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "webservers/policies", H{"policies": policies})
}

func (s *Server) WebHostsView(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.GetAllWebHosts()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "webservers/hosts", H{"hosts": hosts})
}

func (s *Server) WebServerMetricsHandler(w http.ResponseWriter, r *http.Request) {
	s.webServerMu.Lock()
	instance := s.webServer
	s.webServerMu.Unlock()
	if instance == nil {
		http.Error(w, "web server is stopped", http.StatusServiceUnavailable)
		return
	}
	instance.MetricsHandler().ServeHTTP(w, r)
}

func (s *Server) WebListenerNewHandler(w http.ResponseWriter, r *http.Request) {
	certificates, err := s.activeCertificates()
	if err != nil {
		s.Error(w, err)
		return
	}
	policies, err := s.GetAllWebPolicies()
	if err != nil {
		s.Error(w, err)
		return
	}
	if r.Method == http.MethodGet {
		s.Render(w, "webservers/listener", H{"certificates": certificates, "policies": policies, "isEdit": false})
		return
	}
	listener, err := parseWebListenerForm(r)
	if err != nil {
		s.Render(w, "webservers/listener", H{"error": err.Error(), "listener": listener, "certificates": certificates, "policies": policies, "isEdit": false})
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
		s.Render(w, "webservers/listener", H{"error": err.Error(), "listener": listener, "certificates": certificates, "policies": policies, "isEdit": false})
		return
	}
	http.Redirect(w, r, "/web/listeners", http.StatusSeeOther)
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
	policies, err := s.GetAllWebPolicies()
	if err != nil {
		s.Error(w, err)
		return
	}
	method := requestMethod(r)
	switch method {
	case http.MethodGet:
		s.Render(w, "webservers/listener", H{"listener": listener, "certificates": certificates, "policies": policies, "isEdit": true})
	case http.MethodPut:
		updated, err := parseWebListenerForm(r)
		if err != nil {
			updated.ID = id
			s.Render(w, "webservers/listener", H{"error": err.Error(), "listener": updated, "certificates": certificates, "policies": policies, "isEdit": true})
			return
		}
		updated.ID = id
		if err = s.UpdateWebListener(updated); err == nil {
			err = s.applyYabber()
			if err != nil {
				_ = s.UpdateWebListener(listener)
			}
		}
		if err != nil {
			s.Render(w, "webservers/listener", H{"error": err.Error(), "listener": updated, "certificates": certificates, "policies": policies, "isEdit": true})
			return
		}
		http.Redirect(w, r, "/web/listeners", http.StatusSeeOther)
	case http.MethodDelete:
		if err = s.DeleteWebListener(id); err == nil {
			err = s.applyYabber()
		}
		if err != nil {
			s.Error(w, err)
			return
		}
		http.Redirect(w, r, "/web/listeners", http.StatusSeeOther)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func parseWebListenerForm(r *http.Request) (*WebListener, error) {
	if err := r.ParseForm(); err != nil {
		return &WebListener{}, err
	}
	listener := &WebListener{
		Name:           strings.TrimSpace(r.FormValue("name")),
		Address:        strings.TrimSpace(r.FormValue("address")),
		Protocol:       r.FormValue("protocol"),
		Enabled:        r.FormValue("enabled") == "on",
		MinTLSVersion:  r.FormValue("min_tls_version"),
		TrustedProxies: strings.TrimSpace(r.FormValue("trusted_proxies")),
	}
	var err error
	listener.PolicyIDs, err = parseWebPolicyIDs(r)
	if err != nil {
		return listener, err
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
	var metrics []yabber.RouteMetrics
	s.webServerMu.Lock()
	if s.webServer != nil {
		for _, routeMetrics := range s.webServer.Metrics().Routes {
			if routeMetrics.Host == host.Name {
				metrics = append(metrics, routeMetrics)
			}
		}
	}
	s.webServerMu.Unlock()
	s.Render(w, "webservers/host", H{"host": host, "metrics": metrics})
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
	policies, err := s.GetAllWebPolicies()
	if err != nil {
		s.Error(w, err)
		return
	}
	if r.Method == http.MethodGet {
		s.Render(w, "webservers/host_form", H{"host": current, "listeners": listeners, "certificates": certificates, "policies": policies, "isEdit": current != nil})
		return
	}
	method := requestMethod(r)
	if method == http.MethodDelete && current != nil {
		if err = s.DeleteWebHost(current.ID); err == nil {
			err = s.applyYabber()
		}
		if err != nil {
			s.Error(w, err)
			return
		}
		http.Redirect(w, r, "/web/hosts", http.StatusSeeOther)
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
		s.Render(w, "webservers/host_form", H{"error": err.Error(), "host": host, "listeners": listeners, "certificates": certificates, "policies": policies, "isEdit": current != nil})
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
		s.Render(w, "webservers/host_form", H{"error": err.Error(), "host": host, "listeners": listeners, "certificates": certificates, "policies": policies, "isEdit": current != nil})
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
	var err error
	host.PolicyIDs, err = parseWebPolicyIDs(r)
	if err != nil {
		return host, err
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
	policies, err := s.GetAllWebPolicies()
	if err != nil {
		s.Error(w, err)
		return
	}
	if r.Method == http.MethodGet {
		s.Render(w, "webservers/route", H{"host": host, "route": current, "policies": policies, "isEdit": current != nil})
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
		s.Render(w, "webservers/route", H{"error": err.Error(), "host": host, "route": route, "policies": policies, "isEdit": current != nil})
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
		s.Render(w, "webservers/route", H{"error": err.Error(), "host": host, "route": route, "policies": policies, "isEdit": current != nil})
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
	var err error
	route.PolicyIDs, err = parseWebPolicyIDs(r)
	if err != nil {
		return route, err
	}
	route.ResponseStatus, _ = strconv.Atoi(r.FormValue("response_status"))
	route.RedirectStatus, _ = strconv.Atoi(r.FormValue("redirect_status"))
	proxyLimits := []struct {
		value  string
		target *int
		name   string
	}{
		{r.FormValue("request_timeout_seconds"), &route.RequestTimeoutSeconds, "request timeout"},
		{r.FormValue("response_header_timeout_seconds"), &route.ResponseHeaderTimeoutSeconds, "response header timeout"},
		{r.FormValue("max_conns_per_host"), &route.MaxConnsPerHost, "maximum upstream connections"},
	}
	for _, limit := range proxyLimits {
		value, parseErr := parseOptionalInt(limit.value)
		if parseErr != nil {
			return route, fmt.Errorf("%s must be a non-negative integer", limit.name)
		}
		*limit.target = value
	}
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

func parseWebPolicyIDs(r *http.Request) ([]int64, error) {
	var ids []int64
	for _, value := range r.Form["policy_ids"] {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return ids, errors.New("invalid policy")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *Server) WebPolicyNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "webservers/policy", H{"isEdit": false})
		return
	}
	policy, err := parseWebPolicyForm(r)
	if err != nil {
		s.Render(w, "webservers/policy", H{"error": err.Error(), "policy": policy, "isEdit": false})
		return
	}
	id, err := s.CreateWebPolicy(policy)
	if err == nil {
		policy.ID = id
		err = s.applyYabber()
		if err != nil {
			_ = s.DeleteWebPolicy(id)
		}
	}
	if err != nil {
		s.Render(w, "webservers/policy", H{"error": err.Error(), "policy": policy, "isEdit": false})
		return
	}
	http.Redirect(w, r, "/web/policies", http.StatusSeeOther)
}

func (s *Server) WebPolicyHandler(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	current, err := s.GetWebPolicy(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	switch requestMethod(r) {
	case http.MethodGet:
		s.Render(w, "webservers/policy", H{"policy": current, "isEdit": true})
	case http.MethodPut:
		policy, parseErr := parseWebPolicyForm(r)
		policy.ID = id
		if parseErr != nil {
			s.Render(w, "webservers/policy", H{"error": parseErr.Error(), "policy": policy, "isEdit": true})
			return
		}
		if err = s.UpdateWebPolicy(policy); err == nil {
			err = s.applyYabber()
			if err != nil {
				_ = s.UpdateWebPolicy(current)
			}
		}
		if err != nil {
			s.Render(w, "webservers/policy", H{"error": err.Error(), "policy": policy, "isEdit": true})
			return
		}
		http.Redirect(w, r, "/web/policies", http.StatusSeeOther)
	case http.MethodDelete:
		if err = s.DeleteWebPolicy(id); err == nil {
			err = s.applyYabber()
		}
		if err != nil {
			s.Error(w, err)
			return
		}
		http.Redirect(w, r, "/web/policies", http.StatusSeeOther)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func parseWebPolicyForm(r *http.Request) (*WebPolicy, error) {
	if err := r.ParseForm(); err != nil {
		return &WebPolicy{}, err
	}
	policy := &WebPolicy{
		Name:    strings.TrimSpace(r.FormValue("name")),
		RateKey: r.FormValue("rate_key"),
	}
	if policy.RateKey == "" {
		policy.RateKey = "client_ip"
	}
	var err error
	if policy.MaxBodyBytes, err = parseOptionalInt64(r.FormValue("max_body_bytes")); err != nil {
		return policy, errors.New("maximum body bytes must be a non-negative integer")
	}
	intFields := []struct {
		value  string
		target *int
		name   string
	}{
		{r.FormValue("request_timeout_seconds"), &policy.RequestTimeoutSeconds, "request timeout"},
		{r.FormValue("rate_burst"), &policy.RateBurst, "rate burst"},
		{r.FormValue("rate_max_keys"), &policy.RateMaxKeys, "rate max keys"},
		{r.FormValue("rate_idle_timeout_seconds"), &policy.RateIdleTimeoutSeconds, "rate idle timeout"},
		{r.FormValue("concurrency_max"), &policy.ConcurrencyMax, "concurrency max"},
	}
	for _, field := range intFields {
		value, parseErr := parseOptionalInt(field.value)
		if parseErr != nil {
			return policy, fmt.Errorf("%s must be a non-negative integer", field.name)
		}
		*field.target = value
	}
	if value := strings.TrimSpace(r.FormValue("rate_requests_per_second")); value != "" {
		policy.RateRequestsPerSecond, err = strconv.ParseFloat(value, 64)
		if err != nil || policy.RateRequestsPerSecond < 0 {
			return policy, errors.New("requests per second must be a non-negative number")
		}
	}
	if policy.Name == "" {
		return policy, errors.New("name is required")
	}
	if policy.RateKey != "client_ip" && policy.RateKey != "global" {
		return policy, errors.New("rate key must be client_ip or global")
	}
	if policy.RateRequestsPerSecond > 0 {
		if policy.RateBurst == 0 {
			return policy, errors.New("rate burst is required when rate limiting is enabled")
		}
		if policy.RateMaxKeys == 0 {
			policy.RateMaxKeys = 10000
		}
		if policy.RateIdleTimeoutSeconds == 0 {
			policy.RateIdleTimeoutSeconds = 600
		}
	} else {
		policy.RateBurst = 0
		policy.RateMaxKeys = 0
		policy.RateIdleTimeoutSeconds = 0
	}
	if policy.MaxBodyBytes == 0 && policy.RequestTimeoutSeconds == 0 &&
		policy.RateRequestsPerSecond == 0 && policy.ConcurrencyMax == 0 {
		return policy, errors.New("configure at least one policy limit")
	}
	return policy, nil
}

func parseOptionalInt(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	result, err := strconv.Atoi(value)
	if err != nil || result < 0 {
		return 0, errors.New("invalid non-negative integer")
	}
	return result, nil
}

func parseOptionalInt64(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	result, err := strconv.ParseInt(value, 10, 64)
	if err != nil || result < 0 {
		return 0, errors.New("invalid non-negative integer")
	}
	return result, nil
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
