package server

import (
	"bufio"
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	dnsconfig "github.com/lsongdev/dns-go/config"
	dnsfilter "github.com/lsongdev/dns-go/filter"
	dnszone "github.com/lsongdev/dns-go/zone"
)

const maxDNSSubscriptionSize = 32 << 20

type DNSSettings struct {
	Enabled      bool      `json:"enabled"`
	UDPAddr      string    `json:"udp_addr"`
	TCPAddr      string    `json:"tcp_addr"`
	DoTAddr      string    `json:"dot_addr"`
	DoHAddr      string    `json:"doh_addr"`
	CertFile     string    `json:"cert_file"`
	KeyFile      string    `json:"key_file"`
	CacheEnabled bool      `json:"cache_enabled"`
	CacheMinTTL  int       `json:"cache_min_ttl"`
	CacheMaxTTL  int       `json:"cache_max_ttl"`
	CacheEntries int       `json:"cache_entries"`
	Upstreams    string    `json:"upstreams"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type DNSZone struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type DNSLocalRecord struct {
	ID        int64     `json:"id"`
	ZoneID    int64     `json:"zone_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	TTL       int       `json:"ttl"`
	Value     string    `json:"value"`
	Priority  int       `json:"priority"`
	Weight    int       `json:"weight"`
	Port      int       `json:"port"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DNSSubscription struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Kind         string     `json:"kind"`
	URL          string     `json:"url"`
	Enabled      bool       `json:"enabled"`
	RefreshHours int        `json:"refresh_hours"`
	LocalPath    string     `json:"local_path"`
	LastUpdated  *time.Time `json:"last_updated"`
	LastError    string     `json:"last_error"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type DNSRule struct {
	ID        int64     `json:"id"`
	Rule      string    `json:"rule"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Server) initDNSSchema() error {
	if err := s.initDomainSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`
	CREATE TABLE IF NOT EXISTS dns_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		enabled INTEGER NOT NULL DEFAULT 0,
		udp_addr TEXT NOT NULL DEFAULT ':15353',
		tcp_addr TEXT NOT NULL DEFAULT ':15353',
		dot_addr TEXT NOT NULL DEFAULT '',
		doh_addr TEXT NOT NULL DEFAULT '',
		cert_file TEXT NOT NULL DEFAULT '',
		key_file TEXT NOT NULL DEFAULT '',
		cache_enabled INTEGER NOT NULL DEFAULT 1,
		cache_min_ttl INTEGER NOT NULL DEFAULT 60,
		cache_max_ttl INTEGER NOT NULL DEFAULT 86400,
		cache_entries INTEGER NOT NULL DEFAULT 10000,
		upstreams TEXT NOT NULL DEFAULT 'udp 1.1.1.1:53 5s
udp 8.8.8.8:53 5s',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	INSERT OR IGNORE INTO dns_settings (id) VALUES (1);

	CREATE TABLE IF NOT EXISTS dns_zones (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE COLLATE NOCASE,
		description TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS dns_server_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		zone_id INTEGER NOT NULL,
		name TEXT NOT NULL DEFAULT '@',
		type TEXT NOT NULL,
		ttl INTEGER NOT NULL DEFAULT 300,
		value TEXT NOT NULL,
		priority INTEGER NOT NULL DEFAULT 0,
		weight INTEGER NOT NULL DEFAULT 0,
		port INTEGER NOT NULL DEFAULT 0,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (zone_id) REFERENCES dns_zones(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_dns_server_records_zone ON dns_server_records(zone_id);

	CREATE TABLE IF NOT EXISTS dns_subscriptions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		kind TEXT NOT NULL CHECK (kind IN ('block', 'allow')),
		url TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		refresh_hours INTEGER NOT NULL DEFAULT 24,
		local_path TEXT NOT NULL DEFAULT '',
		last_updated DATETIME,
		last_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS dns_rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		rule TEXT NOT NULL UNIQUE,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS dns_migrations (
			name TEXT PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return err
	}
	result, err := s.db.Exec(`
		INSERT OR IGNORE INTO dns_migrations (name) VALUES ('default-port-15353')
	`)
	if err != nil {
		return err
	}
	applied, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if applied == 1 {
		_, err = s.db.Exec(`
			UPDATE dns_settings
			SET udp_addr = CASE WHEN udp_addr = ':5353' THEN ':15353' ELSE udp_addr END,
			    tcp_addr = CASE WHEN tcp_addr = ':5353' THEN ':15353' ELSE tcp_addr END
			WHERE id = 1
		`)
	}
	return err
}

func (s *Server) GetDNSSettings() (*DNSSettings, error) {
	var item DNSSettings
	var enabled, cacheEnabled int
	var updated string
	err := s.db.QueryRow(`
		SELECT enabled, udp_addr, tcp_addr, dot_addr, doh_addr, cert_file, key_file,
		       cache_enabled, cache_min_ttl,
		       cache_max_ttl, cache_entries, upstreams, updated_at
		FROM dns_settings WHERE id = 1
	`).Scan(&enabled, &item.UDPAddr, &item.TCPAddr, &item.DoTAddr, &item.DoHAddr,
		&item.CertFile, &item.KeyFile, &cacheEnabled, &item.CacheMinTTL,
		&item.CacheMaxTTL, &item.CacheEntries, &item.Upstreams, &updated)
	if err != nil {
		return nil, err
	}
	item.Enabled = enabled != 0
	item.CacheEnabled = cacheEnabled != 0
	item.UpdatedAt = parseTime(updated)
	return &item, nil
}

func (s *Server) UpdateDNSSettings(item DNSSettings) error {
	_, err := s.db.Exec(`
		UPDATE dns_settings
		SET enabled = ?, udp_addr = ?, tcp_addr = ?, dot_addr = ?, doh_addr = ?,
		    cert_file = ?, key_file = ?, cache_enabled = ?,
		    cache_min_ttl = ?, cache_max_ttl = ?, cache_entries = ?,
		    upstreams = ?, updated_at = ?
		WHERE id = 1
	`, item.Enabled, item.UDPAddr, item.TCPAddr, item.DoTAddr, item.DoHAddr,
		item.CertFile, item.KeyFile, item.CacheEnabled,
		item.CacheMinTTL, item.CacheMaxTTL, item.CacheEntries, item.Upstreams, time.Now())
	return err
}

func (s *Server) GetAllDNSZones() ([]DNSZone, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, enabled, created_at, updated_at
		FROM dns_zones ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DNSZone
	for rows.Next() {
		var item DNSZone
		var enabled int
		var created, updated string
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &enabled, &created, &updated); err != nil {
			return nil, err
		}
		item.Enabled = enabled != 0
		item.CreatedAt = parseTime(created)
		item.UpdatedAt = parseTime(updated)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Server) GetDNSZoneByID(id int64) (*DNSZone, error) {
	var item DNSZone
	var enabled int
	var created, updated string
	err := s.db.QueryRow(`
		SELECT id, name, description, enabled, created_at, updated_at
		FROM dns_zones WHERE id = ?
	`, id).Scan(&item.ID, &item.Name, &item.Description, &enabled, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.Enabled = enabled != 0
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return &item, nil
}

func (s *Server) CreateDNSZone(name, description string, enabled bool) (*DNSZone, error) {
	name = normalizeDNSName(name)
	if name == "" || !strings.Contains(name, ".") {
		return nil, fmt.Errorf("a valid DNS zone name is required")
	}
	now := time.Now()
	result, err := s.db.Exec(`
		INSERT INTO dns_zones (name, description, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, name, description, enabled, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &DNSZone{ID: id, Name: name, Description: description, Enabled: enabled, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Server) UpdateDNSZone(id int64, name, description string, enabled bool) error {
	name = normalizeDNSName(name)
	if name == "" || !strings.Contains(name, ".") {
		return fmt.Errorf("a valid DNS zone name is required")
	}
	_, err := s.db.Exec(`
		UPDATE dns_zones SET name = ?, description = ?, enabled = ?, updated_at = ? WHERE id = ?
	`, name, description, enabled, time.Now(), id)
	return err
}

func (s *Server) DeleteDNSZone(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM dns_server_records WHERE zone_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM dns_zones WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Server) GetDNSRecordsByZone(zoneID int64) ([]DNSLocalRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, zone_id, name, type, ttl, value, priority, weight, port,
		       enabled, created_at, updated_at
		FROM dns_server_records WHERE zone_id = ? ORDER BY name, type, id
	`, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DNSLocalRecord
	for rows.Next() {
		item, err := scanDNSRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

type dnsRecordScanner interface {
	Scan(dest ...any) error
}

func scanDNSRecord(row dnsRecordScanner) (*DNSLocalRecord, error) {
	var item DNSLocalRecord
	var enabled int
	var created, updated string
	err := row.Scan(&item.ID, &item.ZoneID, &item.Name, &item.Type, &item.TTL, &item.Value,
		&item.Priority, &item.Weight, &item.Port, &enabled, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.Enabled = enabled != 0
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return &item, nil
}

func (s *Server) GetDNSRecordByID(id int64) (*DNSLocalRecord, error) {
	return scanDNSRecord(s.db.QueryRow(`
		SELECT id, zone_id, name, type, ttl, value, priority, weight, port,
		       enabled, created_at, updated_at
		FROM dns_server_records WHERE id = ?
	`, id))
}

func (s *Server) saveDNSRecord(item *DNSLocalRecord) error {
	zone, err := s.GetDNSZoneByID(item.ZoneID)
	if err != nil {
		return err
	}
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" {
		item.Name = "@"
	}
	item.Type = strings.ToUpper(strings.TrimSpace(item.Type))
	item.Value = strings.TrimSpace(item.Value)
	if item.TTL <= 0 {
		item.TTL = 300
	}
	if err := validateDNSRecord(zone.Name, item); err != nil {
		return err
	}
	now := time.Now()
	if item.ID == 0 {
		result, err := s.db.Exec(`
			INSERT INTO dns_server_records
				(zone_id, name, type, ttl, value, priority, weight, port, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, item.ZoneID, item.Name, item.Type, item.TTL, item.Value, item.Priority,
			item.Weight, item.Port, item.Enabled, now, now)
		if err != nil {
			return err
		}
		item.ID, err = result.LastInsertId()
		return err
	}
	_, err = s.db.Exec(`
		UPDATE dns_server_records
		SET name = ?, type = ?, ttl = ?, value = ?, priority = ?, weight = ?,
		    port = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, item.Name, item.Type, item.TTL, item.Value, item.Priority, item.Weight,
		item.Port, item.Enabled, now, item.ID)
	return err
}

func (s *Server) DeleteDNSLocalRecord(id int64) error {
	_, err := s.db.Exec(`DELETE FROM dns_server_records WHERE id = ?`, id)
	return err
}

func validateDNSRecord(origin string, record *DNSLocalRecord) error {
	line := dnsRecordLine(*record)
	if _, err := dnszone.Parse([]byte("$ORIGIN " + origin + ".\n" + line + "\n")); err != nil {
		return fmt.Errorf("invalid %s record: %w", record.Type, err)
	}
	return nil
}

func dnsRecordLine(record DNSLocalRecord) string {
	value := record.Value
	switch record.Type {
	case "MX":
		value = fmt.Sprintf("%d %s", record.Priority, value)
	case "SRV":
		value = fmt.Sprintf("%d %d %d %s", record.Priority, record.Weight, record.Port, value)
	case "TXT":
		value = `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return fmt.Sprintf("%s %d IN %s %s", record.Name, record.TTL, record.Type, value)
}

func (s *Server) GetAllDNSSubscriptions() ([]DNSSubscription, error) {
	rows, err := s.db.Query(`
		SELECT id, name, kind, url, enabled, refresh_hours, local_path,
		       last_updated, last_error, created_at, updated_at
		FROM dns_subscriptions ORDER BY kind, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DNSSubscription
	for rows.Next() {
		var item DNSSubscription
		var enabled int
		var lastUpdated sql.NullString
		var created, updated string
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.URL, &enabled,
			&item.RefreshHours, &item.LocalPath, &lastUpdated, &item.LastError,
			&created, &updated); err != nil {
			return nil, err
		}
		item.Enabled = enabled != 0
		if lastUpdated.Valid {
			t := parseTime(lastUpdated.String)
			item.LastUpdated = &t
		}
		item.CreatedAt = parseTime(created)
		item.UpdatedAt = parseTime(updated)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Server) GetDNSSubscriptionByID(id int64) (*DNSSubscription, error) {
	var item DNSSubscription
	var enabled int
	var lastUpdated sql.NullString
	var created, updated string
	err := s.db.QueryRow(`
		SELECT id, name, kind, url, enabled, refresh_hours, local_path,
		       last_updated, last_error, created_at, updated_at
		FROM dns_subscriptions WHERE id = ?
	`, id).Scan(&item.ID, &item.Name, &item.Kind, &item.URL, &enabled,
		&item.RefreshHours, &item.LocalPath, &lastUpdated, &item.LastError,
		&created, &updated)
	if err != nil {
		return nil, err
	}
	item.Enabled = enabled != 0
	if lastUpdated.Valid {
		t := parseTime(lastUpdated.String)
		item.LastUpdated = &t
	}
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return &item, nil
}

func (s *Server) SaveDNSSubscription(item *DNSSubscription) error {
	if strings.TrimSpace(item.Name) == "" {
		return fmt.Errorf("subscription name is required")
	}
	if item.Kind != "block" && item.Kind != "allow" {
		return fmt.Errorf("subscription kind must be block or allow")
	}
	parsed, err := url.ParseRequestURI(item.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("subscription URL must use http or https")
	}
	if item.RefreshHours <= 0 {
		item.RefreshHours = 24
	}
	now := time.Now()
	if item.ID == 0 {
		result, err := s.db.Exec(`
			INSERT INTO dns_subscriptions
				(name, kind, url, enabled, refresh_hours, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, item.Name, item.Kind, item.URL, item.Enabled, item.RefreshHours, now, now)
		if err != nil {
			return err
		}
		item.ID, err = result.LastInsertId()
		return err
	}
	_, err = s.db.Exec(`
		UPDATE dns_subscriptions
		SET name = ?, kind = ?, url = ?, enabled = ?, refresh_hours = ?, updated_at = ?
		WHERE id = ?
	`, item.Name, item.Kind, item.URL, item.Enabled, item.RefreshHours, now, item.ID)
	return err
}

func (s *Server) DeleteDNSSubscription(id int64) error {
	item, err := s.GetDNSSubscriptionByID(id)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM dns_subscriptions WHERE id = ?`, id); err != nil {
		return err
	}
	return removeDNSListFile(item.LocalPath)
}

func (s *Server) refreshDNSSubscription(ctx context.Context, item *DNSSubscription) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "OhMyServer DNS/1.0")
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.setDNSSubscriptionError(item.ID, err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err = fmt.Errorf("subscription returned %s", resp.Status)
		s.setDNSSubscriptionError(item.ID, err)
		return err
	}
	if err := os.MkdirAll(s.dnsListDirectory(), 0755); err != nil {
		return err
	}
	path := filepath.Join(s.dnsListDirectory(), fmt.Sprintf("%d.txt", item.ID))
	temp, err := os.CreateTemp(s.dnsListDirectory(), fmt.Sprintf("%d-*.tmp", item.ID))
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	reader := io.LimitReader(resp.Body, maxDNSSubscriptionSize+1)
	n, copyErr := io.Copy(temp, reader)
	closeErr := temp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n > maxDNSSubscriptionSize {
		return fmt.Errorf("subscription exceeds the 32 MiB limit")
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	now := time.Now()
	_, err = s.db.Exec(`
		UPDATE dns_subscriptions
		SET local_path = ?, last_updated = ?, last_error = '', updated_at = ?
		WHERE id = ?
	`, path, now, now, item.ID)
	return err
}

func (s *Server) setDNSSubscriptionError(id int64, refreshErr error) {
	_, _ = s.db.Exec(`
		UPDATE dns_subscriptions SET last_error = ?, updated_at = ? WHERE id = ?
	`, refreshErr.Error(), time.Now(), id)
}

func (s *Server) GetAllDNSRules() ([]DNSRule, error) {
	rows, err := s.db.Query(`
		SELECT id, rule, enabled, created_at, updated_at
		FROM dns_rules ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DNSRule
	for rows.Next() {
		var item DNSRule
		var enabled int
		var created, updated string
		if err := rows.Scan(&item.ID, &item.Rule, &enabled, &created, &updated); err != nil {
			return nil, err
		}
		item.Enabled = enabled != 0
		item.CreatedAt = parseTime(created)
		item.UpdatedAt = parseTime(updated)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Server) buildDNSConfig(ctx context.Context, refresh bool) (*dnsconfig.Config, bool, error) {
	settings, err := s.GetDNSSettings()
	if err != nil {
		return nil, false, err
	}
	cfg := &dnsconfig.Config{
		Cache: dnsconfig.CacheSpec{
			Enabled:     settings.CacheEnabled,
			MinTTL:      dnsconfig.Duration(time.Duration(settings.CacheMinTTL) * time.Second),
			MaxTTL:      dnsconfig.Duration(time.Duration(settings.CacheMaxTTL) * time.Second),
			NegativeTTL: dnsconfig.Duration(60 * time.Second),
			MaxEntries:  settings.CacheEntries,
		},
		Proxy: dnsconfig.ProxySpec{Strategy: "failover"},
	}
	if settings.Enabled {
		if strings.TrimSpace(settings.UDPAddr) != "" {
			cfg.Listens = append(cfg.Listens, dnsconfig.ListenSpec{Type: "udp", Addr: settings.UDPAddr})
		}
		if strings.TrimSpace(settings.TCPAddr) != "" {
			cfg.Listens = append(cfg.Listens, dnsconfig.ListenSpec{Type: "tcp", Addr: settings.TCPAddr})
		}
		if strings.TrimSpace(settings.DoTAddr) != "" {
			cfg.Listens = append(cfg.Listens, dnsconfig.ListenSpec{
				Type: "dot", Addr: settings.DoTAddr, CertFile: settings.CertFile, KeyFile: settings.KeyFile,
			})
		}
		if strings.TrimSpace(settings.DoHAddr) != "" {
			cfg.Listens = append(cfg.Listens, dnsconfig.ListenSpec{Type: "doh", Addr: settings.DoHAddr})
		}
	}
	upstreams, err := parseDNSUpstreams(settings.Upstreams)
	if err != nil {
		return nil, false, err
	}
	cfg.Proxy.Upstreams = upstreams

	zones, err := s.GetAllDNSZones()
	if err != nil {
		return nil, false, err
	}
	for _, zone := range zones {
		if !zone.Enabled {
			continue
		}
		records, err := s.GetDNSRecordsByZone(zone.ID)
		if err != nil {
			return nil, false, err
		}
		spec := dnsconfig.DomainSpec{Domain: zone.Name}
		for _, record := range records {
			if record.Enabled {
				spec.Records = append(spec.Records, dnsRecordLine(record))
			}
		}
		cfg.Domains = append(cfg.Domains, spec)
	}

	subscriptions, err := s.GetAllDNSSubscriptions()
	if err != nil {
		return nil, false, err
	}
	for i := range subscriptions {
		item := &subscriptions[i]
		if !item.Enabled {
			continue
		}
		due := item.LastUpdated == nil || time.Since(*item.LastUpdated) >= time.Duration(item.RefreshHours)*time.Hour
		if refresh && settings.Enabled && (due || item.LocalPath == "") {
			if err := s.refreshDNSSubscription(ctx, item); err != nil {
				// Keep serving a previously downloaded copy when refresh fails.
				if item.LocalPath == "" {
					continue
				}
			} else {
				current, getErr := s.GetDNSSubscriptionByID(item.ID)
				if getErr == nil {
					item = current
				}
			}
		}
		if item.LocalPath == "" {
			continue
		}
		spec := dnsconfig.ListSpec{
			Name:    item.Name,
			URL:     item.URL,
			File:    item.LocalPath,
			Enabled: true,
			Refresh: dnsconfig.Duration(time.Duration(item.RefreshHours) * time.Hour),
		}
		if item.Kind == "allow" {
			cfg.Filters.Allowlists = append(cfg.Filters.Allowlists, spec)
		} else {
			cfg.Filters.Blocklists = append(cfg.Filters.Blocklists, spec)
		}
	}
	rules, err := s.GetAllDNSRules()
	if err != nil {
		return nil, false, err
	}
	for _, rule := range rules {
		if rule.Enabled {
			cfg.Filters.Rules = append(cfg.Filters.Rules, rule.Rule)
		}
	}

	if settings.Enabled {
		if err := cfg.Validate(); err != nil {
			return nil, false, err
		}
	}
	return cfg, settings.Enabled, nil
}

func parseDNSUpstreams(value string) ([]dnsconfig.UpstreamSpec, error) {
	var upstreams []dnsconfig.UpstreamSpec
	for lineNo, line := range splitDNSLines(value) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("upstream line %d must be: type address [timeout] [method]", lineNo+1)
		}
		upstream := dnsconfig.UpstreamSpec{
			Type:    strings.ToLower(fields[0]),
			Addr:    fields[1],
			Timeout: dnsconfig.Duration(5 * time.Second),
		}
		if len(fields) > 2 {
			timeout, err := time.ParseDuration(fields[2])
			if err != nil {
				return nil, fmt.Errorf("upstream line %d timeout: %w", lineNo+1, err)
			}
			upstream.Timeout = dnsconfig.Duration(timeout)
		}
		if len(fields) > 3 {
			upstream.Method = strings.ToLower(fields[3])
		} else if upstream.Type == "doh" {
			upstream.Method = "post"
		}
		upstreams = append(upstreams, upstream)
	}
	return upstreams, nil
}

func splitDNSLines(value string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(value))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines
}

func normalizeDNSName(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
}

func parseDNSBool(r *http.Request, key string) bool {
	value := strings.ToLower(r.FormValue(key))
	return value == "1" || value == "true" || value == "on" || value == "yes"
}

func parseDNSInt(r *http.Request, key string, fallback int) (int, error) {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return fallback, nil
	}
	result, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	return result, nil
}

func dnsID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (s *Server) DNSView(w http.ResponseWriter, r *http.Request) {
	settings, err := s.GetDNSSettings()
	if err != nil {
		s.Error(w, err)
		return
	}
	zones, err := s.GetAllDNSZones()
	if err != nil {
		s.Error(w, err)
		return
	}
	subscriptions, err := s.GetAllDNSSubscriptions()
	if err != nil {
		s.Error(w, err)
		return
	}
	recordCount := 0
	for _, zone := range zones {
		records, getErr := s.GetDNSRecordsByZone(zone.ID)
		if getErr != nil {
			s.Error(w, getErr)
			return
		}
		recordCount += len(records)
	}
	status := DNSRuntimeStatus{}
	if s.dnsRuntime != nil {
		status = s.dnsRuntime.Status()
	}
	s.Render(w, "dns/index", H{
		"settings":          settings,
		"status":            status,
		"zoneCount":         len(zones),
		"recordCount":       recordCount,
		"subscriptionCount": len(subscriptions),
	})
}

func (s *Server) DNSSettingsHandler(w http.ResponseWriter, r *http.Request) {
	settings, err := s.GetDNSSettings()
	if err != nil {
		s.Error(w, err)
		return
	}
	if r.Method == http.MethodGet {
		s.Render(w, "dns/settings", H{"settings": settings})
		return
	}
	previous := *settings
	if err := r.ParseForm(); err != nil {
		s.Error(w, err)
		return
	}
	settings.Enabled = parseDNSBool(r, "enabled")
	settings.CacheEnabled = parseDNSBool(r, "cache_enabled")
	settings.UDPAddr = strings.TrimSpace(r.FormValue("udp_addr"))
	settings.TCPAddr = strings.TrimSpace(r.FormValue("tcp_addr"))
	settings.DoTAddr = strings.TrimSpace(r.FormValue("dot_addr"))
	settings.DoHAddr = strings.TrimSpace(r.FormValue("doh_addr"))
	settings.CertFile = strings.TrimSpace(r.FormValue("cert_file"))
	settings.KeyFile = strings.TrimSpace(r.FormValue("key_file"))
	settings.Upstreams = strings.TrimSpace(r.FormValue("upstreams"))
	if settings.CacheMinTTL, err = parseDNSInt(r, "cache_min_ttl", 60); err != nil {
		s.Error(w, err)
		return
	}
	if settings.CacheMaxTTL, err = parseDNSInt(r, "cache_max_ttl", 86400); err != nil {
		s.Error(w, err)
		return
	}
	if settings.CacheEntries, err = parseDNSInt(r, "cache_entries", 10000); err != nil {
		s.Error(w, err)
		return
	}
	if settings.Enabled && settings.UDPAddr == "" && settings.TCPAddr == "" && settings.DoTAddr == "" && settings.DoHAddr == "" {
		s.Error(w, fmt.Errorf("at least one DNS listen address is required"))
		return
	}
	if settings.CacheMinTTL < 0 || settings.CacheMaxTTL <= 0 || settings.CacheMinTTL > settings.CacheMaxTTL {
		s.Error(w, fmt.Errorf("cache TTL values are invalid"))
		return
	}
	if settings.CacheEntries <= 0 {
		s.Error(w, fmt.Errorf("cache entries must be greater than zero"))
		return
	}
	if settings.DoTAddr != "" {
		if _, err := tls.LoadX509KeyPair(settings.CertFile, settings.KeyFile); err != nil {
			s.Error(w, fmt.Errorf("invalid DNS-over-TLS certificate: %w", err))
			return
		}
	}
	upstreams, err := parseDNSUpstreams(settings.Upstreams)
	if err != nil {
		s.Error(w, err)
		return
	}
	candidate := &dnsconfig.Config{Proxy: dnsconfig.ProxySpec{Strategy: "failover", Upstreams: upstreams}}
	if settings.Enabled {
		if settings.UDPAddr != "" {
			candidate.Listens = append(candidate.Listens, dnsconfig.ListenSpec{Type: "udp", Addr: settings.UDPAddr})
		}
		if settings.TCPAddr != "" {
			candidate.Listens = append(candidate.Listens, dnsconfig.ListenSpec{Type: "tcp", Addr: settings.TCPAddr})
		}
		if settings.DoTAddr != "" {
			candidate.Listens = append(candidate.Listens, dnsconfig.ListenSpec{
				Type: "dot", Addr: settings.DoTAddr, CertFile: settings.CertFile, KeyFile: settings.KeyFile,
			})
		}
		if settings.DoHAddr != "" {
			candidate.Listens = append(candidate.Listens, dnsconfig.ListenSpec{Type: "doh", Addr: settings.DoHAddr})
		}
		if err := candidate.Validate(); err != nil {
			s.Error(w, err)
			return
		}
	}
	if err := s.UpdateDNSSettings(*settings); err != nil {
		s.Error(w, err)
		return
	}
	if err := s.setServiceDesired("dns", settings.Enabled); err != nil {
		_ = s.UpdateDNSSettings(previous)
		s.Error(w, err)
		return
	}
	if settings.Enabled {
		conflicts, conflictErr := s.configuredServiceConflicts("dns")
		if conflictErr != nil {
			_ = s.UpdateDNSSettings(previous)
			_ = s.setServiceDesired("dns", previous.Enabled)
			s.Error(w, conflictErr)
			return
		}
		if len(conflicts) > 0 {
			_ = s.UpdateDNSSettings(previous)
			_ = s.setServiceDesired("dns", previous.Enabled)
			s.Error(w, fmt.Errorf("listener conflict: %s is also used by %s",
				conflicts[0].Endpoint, strings.Join(conflicts[0].Services, ", ")))
			return
		}
	}
	if err := s.refreshDNSAndReload(); err != nil {
		if rollbackErr := s.UpdateDNSSettings(previous); rollbackErr == nil {
			_ = s.setServiceDesired("dns", previous.Enabled)
			_ = s.reloadDNS()
		}
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, "/dns", http.StatusSeeOther)
}

func (s *Server) DNSActionHandler(w http.ResponseWriter, r *http.Request) {
	s.serviceAction(w, r, "dns", "/dns")
}

func (s *Server) DNSZonesView(w http.ResponseWriter, r *http.Request) {
	items, err := s.GetAllDNSZones()
	if err != nil {
		s.Error(w, err)
		return
	}
	type row struct {
		DNSZone
		RecordCount int
	}
	rows := make([]row, 0, len(items))
	for _, item := range items {
		records, err := s.GetDNSRecordsByZone(item.ID)
		if err != nil {
			s.Error(w, err)
			return
		}
		rows = append(rows, row{DNSZone: item, RecordCount: len(records)})
	}
	s.Render(w, "dns/zones", H{"zones": rows})
}

func (s *Server) DNSZoneNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "dns/zone_form", H{})
		return
	}
	if err := r.ParseForm(); err != nil {
		s.Error(w, err)
		return
	}
	zone, err := s.CreateDNSZone(r.FormValue("name"), r.FormValue("description"), parseDNSBool(r, "enabled"))
	if err != nil {
		s.Render(w, "dns/zone_form", H{"error": err.Error(), "name": r.FormValue("name"), "description": r.FormValue("description")})
		return
	}
	if err := s.reloadDNS(); err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/dns/zones/%d", zone.ID), http.StatusSeeOther)
}

func (s *Server) DNSZoneDetailHandler(w http.ResponseWriter, r *http.Request) {
	id, err := dnsID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	zone, err := s.GetDNSZoneByID(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	records, err := s.GetDNSRecordsByZone(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "dns/zone_detail", H{"zone": zone, "records": records})
}

func (s *Server) DNSZoneEditHandler(w http.ResponseWriter, r *http.Request) {
	id, err := dnsID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	zone, err := s.GetDNSZoneByID(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	if r.Method == http.MethodGet {
		s.Render(w, "dns/zone_form", H{"zone": zone})
		return
	}
	if err := r.ParseForm(); err != nil {
		s.Error(w, err)
		return
	}
	if err := s.UpdateDNSZone(id, r.FormValue("name"), r.FormValue("description"), parseDNSBool(r, "enabled")); err != nil {
		s.Error(w, err)
		return
	}
	if err := s.reloadDNS(); err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/dns/zones/%d", id), http.StatusSeeOther)
}

func (s *Server) DNSZoneDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id, err := dnsID(r)
	if err == nil {
		err = s.DeleteDNSZone(id)
	}
	if err == nil {
		err = s.reloadDNS()
	}
	if err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, "/dns/zones", http.StatusSeeOther)
}

func dnsRecordFromForm(r *http.Request, id, zoneID int64) (*DNSLocalRecord, error) {
	ttl, err := parseDNSInt(r, "ttl", 300)
	if err != nil {
		return nil, err
	}
	priority, err := parseDNSInt(r, "priority", 0)
	if err != nil {
		return nil, err
	}
	weight, err := parseDNSInt(r, "weight", 0)
	if err != nil {
		return nil, err
	}
	port, err := parseDNSInt(r, "port", 0)
	if err != nil {
		return nil, err
	}
	return &DNSLocalRecord{
		ID: id, ZoneID: zoneID, Name: r.FormValue("name"), Type: r.FormValue("type"),
		TTL: ttl, Value: r.FormValue("value"), Priority: priority, Weight: weight,
		Port: port, Enabled: parseDNSBool(r, "enabled"),
	}, nil
}

func (s *Server) DNSRecordNewHandler(w http.ResponseWriter, r *http.Request) {
	zoneID, err := dnsID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	zone, err := s.GetDNSZoneByID(zoneID)
	if err != nil {
		s.Error(w, err)
		return
	}
	if r.Method == http.MethodGet {
		s.Render(w, "dns/record_form", H{"zone": zone})
		return
	}
	if err := r.ParseForm(); err != nil {
		s.Error(w, err)
		return
	}
	record, err := dnsRecordFromForm(r, 0, zoneID)
	if err == nil {
		err = s.saveDNSRecord(record)
	}
	if err != nil {
		s.Error(w, err)
		return
	}
	if err := s.reloadDNS(); err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/dns/zones/%d", zoneID), http.StatusSeeOther)
}

func (s *Server) DNSRecordEditHandler(w http.ResponseWriter, r *http.Request) {
	id, err := dnsID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	current, err := s.GetDNSRecordByID(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	zone, err := s.GetDNSZoneByID(current.ZoneID)
	if err != nil {
		s.Error(w, err)
		return
	}
	if r.Method == http.MethodGet {
		s.Render(w, "dns/record_form", H{"zone": zone, "record": current})
		return
	}
	if err := r.ParseForm(); err != nil {
		s.Error(w, err)
		return
	}
	record, err := dnsRecordFromForm(r, id, current.ZoneID)
	if err == nil {
		err = s.saveDNSRecord(record)
	}
	if err != nil {
		s.Error(w, err)
		return
	}
	if err := s.reloadDNS(); err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/dns/zones/%d", current.ZoneID), http.StatusSeeOther)
}

func (s *Server) DNSRecordDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id, err := dnsID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	record, err := s.GetDNSRecordByID(id)
	if err == nil {
		err = s.DeleteDNSLocalRecord(id)
	}
	if err == nil {
		err = s.reloadDNS()
	}
	if err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/dns/zones/%d", record.ZoneID), http.StatusSeeOther)
}

func (s *Server) DNSFiltersView(w http.ResponseWriter, r *http.Request) {
	subscriptions, err := s.GetAllDNSSubscriptions()
	if err != nil {
		s.Error(w, err)
		return
	}
	rules, err := s.GetAllDNSRules()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "dns/filters", H{"subscriptions": subscriptions, "rules": rules})
}

func (s *Server) DNSSubscriptionNewHandler(w http.ResponseWriter, r *http.Request) {
	s.dnsSubscriptionFormHandler(w, r, nil)
}

func (s *Server) DNSSubscriptionEditHandler(w http.ResponseWriter, r *http.Request) {
	id, err := dnsID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	item, err := s.GetDNSSubscriptionByID(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	s.dnsSubscriptionFormHandler(w, r, item)
}

func (s *Server) dnsSubscriptionFormHandler(w http.ResponseWriter, r *http.Request, item *DNSSubscription) {
	if r.Method == http.MethodGet {
		s.Render(w, "dns/subscription_form", H{"subscription": item})
		return
	}
	if err := r.ParseForm(); err != nil {
		s.Error(w, err)
		return
	}
	if item == nil {
		item = &DNSSubscription{}
	}
	item.Name = strings.TrimSpace(r.FormValue("name"))
	item.Kind = r.FormValue("kind")
	item.URL = strings.TrimSpace(r.FormValue("url"))
	item.Enabled = parseDNSBool(r, "enabled")
	refreshHours, err := parseDNSInt(r, "refresh_hours", 24)
	if err != nil {
		s.Error(w, err)
		return
	}
	item.RefreshHours = refreshHours
	if err := s.SaveDNSSubscription(item); err != nil {
		s.Error(w, err)
		return
	}
	if item.Enabled {
		ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
		err = s.refreshDNSSubscription(ctx, item)
		cancel()
		if err != nil {
			s.Error(w, err)
			return
		}
	}
	if err := s.reloadDNS(); err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, "/dns/filters", http.StatusSeeOther)
}

func (s *Server) DNSSubscriptionActionHandler(w http.ResponseWriter, r *http.Request) {
	id, err := dnsID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	item, err := s.GetDNSSubscriptionByID(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	switch r.FormValue("action") {
	case "toggle":
		item.Enabled = !item.Enabled
		err = s.SaveDNSSubscription(item)
	case "refresh":
		ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
		err = s.refreshDNSSubscription(ctx, item)
		cancel()
	case "delete":
		err = s.DeleteDNSSubscription(id)
	default:
		err = fmt.Errorf("unknown subscription action")
	}
	if err == nil {
		err = s.reloadDNS()
	}
	if err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, "/dns/filters", http.StatusSeeOther)
}

func (s *Server) DNSRuleNewHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.Error(w, err)
		return
	}
	rule := strings.TrimSpace(r.FormValue("rule"))
	if rule == "" {
		s.Error(w, fmt.Errorf("filter rule is required"))
		return
	}
	filter := dnsfilter.New()
	if err := filter.AddRule(rule); err != nil {
		s.Error(w, err)
		return
	}
	if _, err := s.db.Exec(`
		INSERT INTO dns_rules (rule, enabled, created_at, updated_at) VALUES (?, 1, ?, ?)
	`, rule, time.Now(), time.Now()); err != nil {
		s.Error(w, err)
		return
	}
	if err := s.reloadDNS(); err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, "/dns/filters", http.StatusSeeOther)
}

func (s *Server) DNSRuleActionHandler(w http.ResponseWriter, r *http.Request) {
	id, err := dnsID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	switch r.FormValue("action") {
	case "toggle":
		_, err = s.db.Exec(`UPDATE dns_rules SET enabled = NOT enabled, updated_at = ? WHERE id = ?`, time.Now(), id)
	case "delete":
		_, err = s.db.Exec(`DELETE FROM dns_rules WHERE id = ?`, id)
	default:
		err = fmt.Errorf("unknown rule action")
	}
	if err == nil {
		err = s.reloadDNS()
	}
	if err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, "/dns/filters", http.StatusSeeOther)
}

func init() {
	RegisterInitSchemaHook((*Server).initDNSSchema)
	RegisterInitHook(func(s *Server) error {
		s.dnsRuntime = &DNSRuntime{}
		s.dnsRuntime.StartRefreshLoop(s)
		if s.serviceDesiredEnabled("dns", false) {
			if err := s.validateServiceStart("dns"); err != nil {
				s.setServiceError("dns", err)
				return err
			}
		}
		err := s.refreshDNSAndReload()
		s.setServiceError("dns", err)
		return err
	})
	RegisterCloseHook(func(s *Server) error {
		if s.dnsRuntime == nil {
			return nil
		}
		return s.dnsRuntime.Close()
	})
}
