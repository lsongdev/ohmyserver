// Package dns owns DNS-provider connection persistence and API boundaries.
package web

import (
	"context"
	"database/sql"
	dnsmodule "github.com/lsongdev/ohmyserver/modules/dns"
	"time"
)

type Provider = dnsmodule.Provider
type ProviderSummary = dnsmodule.ProviderSummary
type Settings = dnsmodule.Settings
type Zone = dnsmodule.Zone
type LocalRecord = dnsmodule.LocalRecord
type Subscription = dnsmodule.Subscription
type Rule = dnsmodule.Rule
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

// InitializeModule creates every DNS-owned table from one feature boundary.
func (r *Repository) InitializeModule(ctx context.Context) error {
	initializers := []func(context.Context) error{
		r.Initialize,
		r.InitializeSettings,
		r.InitializeLocal,
		r.InitializeFilters,
	}
	for _, initialize := range initializers {
		if err := initialize(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) Initialize(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS dns_provider_connections(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,type TEXT NOT NULL,config TEXT NOT NULL,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);CREATE INDEX IF NOT EXISTS idx_dns_provider_connections_name ON dns_provider_connections(name);`); err != nil {
		return err
	}
	return nil
}

// ImportProviders inserts provider records supplied by an application-level
// migration adapter. The repository never reads another module's or legacy
// core tables itself.
func (r *Repository) ImportProviders(ctx context.Context, providers []dnsmodule.Provider) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, provider := range providers {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO dns_provider_connections(id,name,type,config) VALUES(?,?,?,?)`, provider.ID, provider.Name, provider.Type, provider.Config); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (r *Repository) InitializeSettings(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS dns_settings(id INTEGER PRIMARY KEY CHECK(id=1),enabled INTEGER NOT NULL DEFAULT 0,udp_addr TEXT NOT NULL DEFAULT ':15353',tcp_addr TEXT NOT NULL DEFAULT ':15353',dot_addr TEXT NOT NULL DEFAULT '',doh_addr TEXT NOT NULL DEFAULT '',cert_file TEXT NOT NULL DEFAULT '',key_file TEXT NOT NULL DEFAULT '',cache_enabled INTEGER NOT NULL DEFAULT 1,cache_min_ttl INTEGER NOT NULL DEFAULT 60,cache_max_ttl INTEGER NOT NULL DEFAULT 86400,cache_entries INTEGER NOT NULL DEFAULT 10000,upstreams TEXT NOT NULL DEFAULT 'udp 1.1.1.1:53 5s
udp 8.8.8.8:53 5s',updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);INSERT OR IGNORE INTO dns_settings(id) VALUES(1);CREATE TABLE IF NOT EXISTS dns_migrations(name TEXT PRIMARY KEY,applied_at DATETIME DEFAULT CURRENT_TIMESTAMP);`); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO dns_migrations(name) VALUES('default-port-15353')`)
	if err != nil {
		return err
	}
	applied, err := result.RowsAffected()
	if err != nil || applied != 1 {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE dns_settings SET udp_addr=CASE WHEN udp_addr=':5353' THEN ':15353' ELSE udp_addr END,tcp_addr=CASE WHEN tcp_addr=':5353' THEN ':15353' ELSE tcp_addr END WHERE id=1`)
	return err
}
func (r *Repository) InitializeLocal(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS dns_zones(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE COLLATE NOCASE,description TEXT NOT NULL DEFAULT '',enabled INTEGER NOT NULL DEFAULT 1,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);CREATE TABLE IF NOT EXISTS dns_server_records(id INTEGER PRIMARY KEY AUTOINCREMENT,zone_id INTEGER NOT NULL,name TEXT NOT NULL DEFAULT '@',type TEXT NOT NULL,ttl INTEGER NOT NULL DEFAULT 300,value TEXT NOT NULL,priority INTEGER NOT NULL DEFAULT 0,weight INTEGER NOT NULL DEFAULT 0,port INTEGER NOT NULL DEFAULT 0,enabled INTEGER NOT NULL DEFAULT 1,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,FOREIGN KEY(zone_id) REFERENCES dns_zones(id) ON DELETE CASCADE);CREATE INDEX IF NOT EXISTS idx_dns_server_records_zone ON dns_server_records(zone_id);`)
	return err
}
func (r *Repository) InitializeFilters(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS dns_subscriptions(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,kind TEXT NOT NULL CHECK(kind IN ('block','allow')),url TEXT NOT NULL,enabled INTEGER NOT NULL DEFAULT 1,refresh_hours INTEGER NOT NULL DEFAULT 24,local_path TEXT NOT NULL DEFAULT '',last_updated DATETIME,last_error TEXT NOT NULL DEFAULT '',created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);CREATE TABLE IF NOT EXISTS dns_rules(id INTEGER PRIMARY KEY AUTOINCREMENT,rule TEXT NOT NULL UNIQUE,enabled INTEGER NOT NULL DEFAULT 1,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);`)
	return err
}
func (r *Repository) Subscriptions(ctx context.Context) ([]dnsmodule.Subscription, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,kind,url,enabled,refresh_hours,local_path,last_updated,last_error,created_at,updated_at FROM dns_subscriptions ORDER BY kind,name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []dnsmodule.Subscription{}
	for rows.Next() {
		item, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}
func scanSubscription(row interface{ Scan(...any) error }) (*dnsmodule.Subscription, error) {
	var x dnsmodule.Subscription
	var enabled int
	var last, created, updated sql.NullString
	err := row.Scan(&x.ID, &x.Name, &x.Kind, &x.URL, &enabled, &x.RefreshHours, &x.LocalPath, &last, &x.LastError, &created, &updated)
	if err != nil {
		return nil, err
	}
	x.Enabled = enabled != 0
	if last.Valid {
		t := parseTime(last.String)
		x.LastUpdated = &t
	}
	if created.Valid {
		x.CreatedAt = parseTime(created.String)
	}
	if updated.Valid {
		x.UpdatedAt = parseTime(updated.String)
	}
	return &x, nil
}
func (r *Repository) Subscription(ctx context.Context, id int64) (*dnsmodule.Subscription, error) {
	return scanSubscription(r.db.QueryRowContext(ctx, `SELECT id,name,kind,url,enabled,refresh_hours,local_path,last_updated,last_error,created_at,updated_at FROM dns_subscriptions WHERE id=?`, id))
}
func (r *Repository) SaveSubscription(ctx context.Context, x *dnsmodule.Subscription) error {
	now := time.Now()
	if x.ID == 0 {
		result, err := r.db.ExecContext(ctx, `INSERT INTO dns_subscriptions(name,kind,url,enabled,refresh_hours,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, x.Name, x.Kind, x.URL, x.Enabled, x.RefreshHours, now, now)
		if err != nil {
			return err
		}
		x.ID, err = result.LastInsertId()
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE dns_subscriptions SET name=?,kind=?,url=?,enabled=?,refresh_hours=?,updated_at=? WHERE id=?`, x.Name, x.Kind, x.URL, x.Enabled, x.RefreshHours, now, x.ID)
	return err
}
func (r *Repository) DeleteSubscription(ctx context.Context, id int64) (string, error) {
	var path string
	if err := r.db.QueryRowContext(ctx, `SELECT local_path FROM dns_subscriptions WHERE id=?`, id).Scan(&path); err != nil {
		return "", err
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM dns_subscriptions WHERE id=?`, id)
	return path, err
}
func (r *Repository) SetSubscriptionRefresh(ctx context.Context, id int64, path string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE dns_subscriptions SET local_path=?,last_updated=?,last_error='',updated_at=? WHERE id=?`, path, at, at, id)
	return err
}
func (r *Repository) SetSubscriptionError(ctx context.Context, id int64, message string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE dns_subscriptions SET last_error=?,updated_at=? WHERE id=?`, message, time.Now(), id)
	return err
}
func (r *Repository) Rules(ctx context.Context) ([]dnsmodule.Rule, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,rule,enabled,created_at,updated_at FROM dns_rules ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []dnsmodule.Rule{}
	for rows.Next() {
		var x dnsmodule.Rule
		var enabled int
		var c, u string
		if err := rows.Scan(&x.ID, &x.Rule, &enabled, &c, &u); err != nil {
			return nil, err
		}
		x.Enabled = enabled != 0
		x.CreatedAt, x.UpdatedAt = parseTime(c), parseTime(u)
		items = append(items, x)
	}
	return items, rows.Err()
}
func (r *Repository) CreateRule(ctx context.Context, value string) (*dnsmodule.Rule, error) {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `INSERT INTO dns_rules(rule,enabled,created_at,updated_at) VALUES(?,1,?,?)`, value, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &dnsmodule.Rule{ID: id, Rule: value, Enabled: true, CreatedAt: now, UpdatedAt: now}, nil
}
func (r *Repository) ToggleRule(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE dns_rules SET enabled=NOT enabled,updated_at=? WHERE id=?`, time.Now(), id)
	return err
}
func (r *Repository) DeleteRule(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM dns_rules WHERE id=?`, id)
	return err
}
func (r *Repository) Zones(ctx context.Context) ([]dnsmodule.Zone, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,description,enabled,created_at,updated_at FROM dns_zones ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []dnsmodule.Zone{}
	for rows.Next() {
		var z dnsmodule.Zone
		var enabled int
		var c, u string
		if err := rows.Scan(&z.ID, &z.Name, &z.Description, &enabled, &c, &u); err != nil {
			return nil, err
		}
		z.Enabled = enabled != 0
		z.CreatedAt, z.UpdatedAt = parseTime(c), parseTime(u)
		items = append(items, z)
	}
	return items, rows.Err()
}
func (r *Repository) Zone(ctx context.Context, id int64) (*dnsmodule.Zone, error) {
	var z dnsmodule.Zone
	var enabled int
	var c, u string
	err := r.db.QueryRowContext(ctx, `SELECT id,name,description,enabled,created_at,updated_at FROM dns_zones WHERE id=?`, id).Scan(&z.ID, &z.Name, &z.Description, &enabled, &c, &u)
	if err != nil {
		return nil, err
	}
	z.Enabled = enabled != 0
	z.CreatedAt, z.UpdatedAt = parseTime(c), parseTime(u)
	return &z, nil
}
func (r *Repository) CreateZone(ctx context.Context, name, description string, enabled bool) (*dnsmodule.Zone, error) {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `INSERT INTO dns_zones(name,description,enabled,created_at,updated_at) VALUES(?,?,?,?,?)`, name, description, enabled, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &dnsmodule.Zone{ID: id, Name: name, Description: description, Enabled: enabled, CreatedAt: now, UpdatedAt: now}, nil
}
func (r *Repository) UpdateZone(ctx context.Context, id int64, name, description string, enabled bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE dns_zones SET name=?,description=?,enabled=?,updated_at=? WHERE id=?`, name, description, enabled, time.Now(), id)
	return err
}
func (r *Repository) DeleteZone(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM dns_server_records WHERE zone_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM dns_zones WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
func scanLocalRecord(row interface{ Scan(...any) error }) (*dnsmodule.LocalRecord, error) {
	var x dnsmodule.LocalRecord
	var enabled int
	var c, u string
	err := row.Scan(&x.ID, &x.ZoneID, &x.Name, &x.Type, &x.TTL, &x.Value, &x.Priority, &x.Weight, &x.Port, &enabled, &c, &u)
	if err != nil {
		return nil, err
	}
	x.Enabled = enabled != 0
	x.CreatedAt, x.UpdatedAt = parseTime(c), parseTime(u)
	return &x, nil
}
func (r *Repository) RecordsByZone(ctx context.Context, zoneID int64) ([]dnsmodule.LocalRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,zone_id,name,type,ttl,value,priority,weight,port,enabled,created_at,updated_at FROM dns_server_records WHERE zone_id=? ORDER BY name,type,id`, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []dnsmodule.LocalRecord{}
	for rows.Next() {
		x, err := scanLocalRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *x)
	}
	return items, rows.Err()
}
func (r *Repository) Record(ctx context.Context, id int64) (*dnsmodule.LocalRecord, error) {
	return scanLocalRecord(r.db.QueryRowContext(ctx, `SELECT id,zone_id,name,type,ttl,value,priority,weight,port,enabled,created_at,updated_at FROM dns_server_records WHERE id=?`, id))
}
func (r *Repository) SaveRecord(ctx context.Context, x *dnsmodule.LocalRecord) error {
	now := time.Now()
	if x.ID == 0 {
		result, err := r.db.ExecContext(ctx, `INSERT INTO dns_server_records(zone_id,name,type,ttl,value,priority,weight,port,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, x.ZoneID, x.Name, x.Type, x.TTL, x.Value, x.Priority, x.Weight, x.Port, x.Enabled, now, now)
		if err != nil {
			return err
		}
		x.ID, err = result.LastInsertId()
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE dns_server_records SET name=?,type=?,ttl=?,value=?,priority=?,weight=?,port=?,enabled=?,updated_at=? WHERE id=?`, x.Name, x.Type, x.TTL, x.Value, x.Priority, x.Weight, x.Port, x.Enabled, now, x.ID)
	return err
}
func (r *Repository) DeleteRecord(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM dns_server_records WHERE id=?`, id)
	return err
}
func (r *Repository) GetSettings(ctx context.Context) (*dnsmodule.Settings, error) {
	var x dnsmodule.Settings
	var enabled, cache int
	var updated string
	err := r.db.QueryRowContext(ctx, `SELECT enabled,udp_addr,tcp_addr,dot_addr,doh_addr,cert_file,key_file,cache_enabled,cache_min_ttl,cache_max_ttl,cache_entries,upstreams,updated_at FROM dns_settings WHERE id=1`).Scan(&enabled, &x.UDPAddr, &x.TCPAddr, &x.DoTAddr, &x.DoHAddr, &x.CertFile, &x.KeyFile, &cache, &x.CacheMinTTL, &x.CacheMaxTTL, &x.CacheEntries, &x.Upstreams, &updated)
	if err != nil {
		return nil, err
	}
	x.Enabled = enabled != 0
	x.CacheEnabled = cache != 0
	x.UpdatedAt = parseTime(updated)
	return &x, nil
}
func (r *Repository) UpdateSettings(ctx context.Context, x dnsmodule.Settings) error {
	_, err := r.db.ExecContext(ctx, `UPDATE dns_settings SET enabled=?,udp_addr=?,tcp_addr=?,dot_addr=?,doh_addr=?,cert_file=?,key_file=?,cache_enabled=?,cache_min_ttl=?,cache_max_ttl=?,cache_entries=?,upstreams=?,updated_at=? WHERE id=1`, x.Enabled, x.UDPAddr, x.TCPAddr, x.DoTAddr, x.DoHAddr, x.CertFile, x.KeyFile, x.CacheEnabled, x.CacheMinTTL, x.CacheMaxTTL, x.CacheEntries, x.Upstreams, time.Now())
	return err
}
func (r *Repository) Create(ctx context.Context, name, kind, config string) (*dnsmodule.Provider, error) {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `INSERT INTO dns_provider_connections(name,type,config,created_at,updated_at) VALUES(?,?,?,?,?)`, name, kind, config, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &dnsmodule.Provider{ID: id, Name: name, Type: kind, Config: config, CreatedAt: now, UpdatedAt: now}, nil
}
func (r *Repository) Get(ctx context.Context, id int64) (*dnsmodule.Provider, error) {
	var p dnsmodule.Provider
	var c, u string
	err := r.db.QueryRowContext(ctx, `SELECT id,name,type,config,created_at,updated_at FROM dns_provider_connections WHERE id=?`, id).Scan(&p.ID, &p.Name, &p.Type, &p.Config, &c, &u)
	if err != nil {
		return nil, err
	}
	p.CreatedAt, p.UpdatedAt = parseTime(c), parseTime(u)
	return &p, nil
}
func (r *Repository) List(ctx context.Context) ([]dnsmodule.Provider, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,type,config,created_at,updated_at FROM dns_provider_connections ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []dnsmodule.Provider{}
	for rows.Next() {
		var p dnsmodule.Provider
		var c, u string
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.Config, &c, &u); err != nil {
			return nil, err
		}
		p.CreatedAt, p.UpdatedAt = parseTime(c), parseTime(u)
		items = append(items, p)
	}
	return items, rows.Err()
}
func (r *Repository) ListSummaries(ctx context.Context) ([]dnsmodule.ProviderSummary, error) {
	items, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dnsmodule.ProviderSummary, 0, len(items))
	for _, p := range items {
		out = append(out, dnsmodule.ProviderSummary{ID: p.ID, Name: p.Name, Type: p.Type, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt})
	}
	return out, nil
}
func (r *Repository) Update(ctx context.Context, id int64, name, kind, config string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE dns_provider_connections SET name=?,type=?,config=?,updated_at=? WHERE id=?`, name, kind, config, time.Now(), id)
	return err
}
func (r *Repository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM dns_provider_connections WHERE id=?`, id)
	return err
}
func parseTime(v string) time.Time {
	for _, f := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05-07:00", "2006-01-02 15:04:05"} {
		if t, e := time.Parse(f, v); e == nil {
			return t
		}
	}
	return time.Time{}
}
