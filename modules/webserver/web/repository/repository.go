package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	webservermodule "github.com/lsongdev/ohmyserver/modules/webserver"
)

// Repository owns Web Server tables only. Certificate information is always
// supplied by the application through the CertificateSource capability.
type Listener = webservermodule.Listener
type Host = webservermodule.Host
type Route = webservermodule.Route
type Policy = webservermodule.Policy

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

// Initialize creates and migrates only Web Server-owned tables.
func (r *Repository) Initialize(ctx context.Context) error {
	for _, initialize := range []func(context.Context) error{r.InitializePolicies, r.InitializeListeners, r.InitializeHosts, r.InitializeRoutes} {
		if err := initialize(ctx); err != nil {
			return err
		}
	}
	for _, column := range []struct{ table, name, definition string }{
		{"web_listeners", "trusted_proxies", `TEXT NOT NULL DEFAULT ''`},
		{"web_routes", "worker_script", `TEXT NOT NULL DEFAULT ''`},
		{"web_routes", "request_timeout_seconds", `INTEGER NOT NULL DEFAULT 0`},
		{"web_routes", "response_header_timeout_seconds", `INTEGER NOT NULL DEFAULT 0`},
		{"web_routes", "max_conns_per_host", `INTEGER NOT NULL DEFAULT 0`},
	} {
		if err := r.ensureColumn(ctx, column.table, column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ensureColumn(ctx context.Context, table, name, definition string) error {
	var pragma, alter string
	switch table {
	case "web_listeners":
		pragma = `PRAGMA table_info(web_listeners)`
		alter = `ALTER TABLE web_listeners ADD COLUMN `
	case "web_routes":
		pragma = `PRAGMA table_info(web_routes)`
		alter = `ALTER TABLE web_routes ADD COLUMN `
	default:
		return fmt.Errorf("invalid Web Server table %q", table)
	}
	rows, err := r.db.QueryContext(ctx, pragma)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var columnName, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if columnName == name {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = r.db.ExecContext(ctx, alter+name+" "+definition)
	return err
}

// SetLegacyWorkerScript is used only by the application-level one-time adapter
// that imports historical worker scripts into the route-owned schema.
func (r *Repository) SetLegacyWorkerScript(ctx context.Context, routeID int64, script string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE web_routes SET worker_script=?,updated_at=? WHERE id=? AND handler_type='worker' AND worker_script=''`, script, time.Now(), routeID)
	return err
}

type LegacyWorkerBinding struct{ RouteID, WorkerID int64 }

func (r *Repository) LegacyWorkerBindings(ctx context.Context) ([]LegacyWorkerBinding, error) {
	rows, err := r.db.QueryContext(ctx, `PRAGMA table_info(web_routes)`)
	if err != nil {
		return nil, err
	}
	hasWorkerID := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return nil, err
		}
		if name == "worker_id" {
			hasWorkerID = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if !hasWorkerID {
		return []LegacyWorkerBinding{}, nil
	}
	rows, err = r.db.QueryContext(ctx, `SELECT id,worker_id FROM web_routes WHERE handler_type='worker' AND worker_script='' AND worker_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []LegacyWorkerBinding{}
	for rows.Next() {
		var item LegacyWorkerBinding
		if err := rows.Scan(&item.RouteID, &item.WorkerID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) InitializePolicies(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS web_policies (
		id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,
		max_body_bytes INTEGER NOT NULL DEFAULT 0,request_timeout_seconds INTEGER NOT NULL DEFAULT 0,
		rate_requests_per_second REAL NOT NULL DEFAULT 0,rate_burst INTEGER NOT NULL DEFAULT 0,
		rate_key TEXT NOT NULL DEFAULT 'client_ip',rate_max_keys INTEGER NOT NULL DEFAULT 10000,
		rate_idle_timeout_seconds INTEGER NOT NULL DEFAULT 600,concurrency_max INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	return err
}

func (r *Repository) ListPolicies(ctx context.Context) ([]Policy, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,max_body_bytes,request_timeout_seconds,rate_requests_per_second,rate_burst,rate_key,rate_max_keys,rate_idle_timeout_seconds,concurrency_max FROM web_policies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Policy{}
	for rows.Next() {
		var x Policy
		if err := rows.Scan(&x.ID, &x.Name, &x.MaxBodyBytes, &x.RequestTimeoutSeconds, &x.RateRequestsPerSecond, &x.RateBurst, &x.RateKey, &x.RateMaxKeys, &x.RateIdleTimeoutSeconds, &x.ConcurrencyMax); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}
func (r *Repository) GetPolicy(ctx context.Context, id int64) (*Policy, error) {
	var x Policy
	err := r.db.QueryRowContext(ctx, `SELECT id,name,max_body_bytes,request_timeout_seconds,rate_requests_per_second,rate_burst,rate_key,rate_max_keys,rate_idle_timeout_seconds,concurrency_max FROM web_policies WHERE id=?`, id).Scan(&x.ID, &x.Name, &x.MaxBodyBytes, &x.RequestTimeoutSeconds, &x.RateRequestsPerSecond, &x.RateBurst, &x.RateKey, &x.RateMaxKeys, &x.RateIdleTimeoutSeconds, &x.ConcurrencyMax)
	if err != nil {
		return nil, err
	}
	return &x, nil
}
func (r *Repository) CreatePolicy(ctx context.Context, x Policy) (int64, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO web_policies(name,max_body_bytes,request_timeout_seconds,rate_requests_per_second,rate_burst,rate_key,rate_max_keys,rate_idle_timeout_seconds,concurrency_max) VALUES(?,?,?,?,?,?,?,?,?)`, x.Name, x.MaxBodyBytes, x.RequestTimeoutSeconds, x.RateRequestsPerSecond, x.RateBurst, x.RateKey, x.RateMaxKeys, x.RateIdleTimeoutSeconds, x.ConcurrencyMax)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
func (r *Repository) UpdatePolicy(ctx context.Context, x Policy) error {
	_, err := r.db.ExecContext(ctx, `UPDATE web_policies SET name=?,max_body_bytes=?,request_timeout_seconds=?,rate_requests_per_second=?,rate_burst=?,rate_key=?,rate_max_keys=?,rate_idle_timeout_seconds=?,concurrency_max=?,updated_at=? WHERE id=?`, x.Name, x.MaxBodyBytes, x.RequestTimeoutSeconds, x.RateRequestsPerSecond, x.RateBurst, x.RateKey, x.RateMaxKeys, x.RateIdleTimeoutSeconds, x.ConcurrencyMax, time.Now(), x.ID)
	return err
}
func (r *Repository) DeletePolicy(ctx context.Context, id int64) error {
	var references int
	if err := r.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM web_listener_policies WHERE policy_id=?)+(SELECT COUNT(*) FROM web_host_policies WHERE policy_id=?)+(SELECT COUNT(*) FROM web_route_policies WHERE policy_id=?)`, id, id, id).Scan(&references); err != nil {
		return err
	}
	if references > 0 {
		return fmt.Errorf("policy is used by %d listener, host, or route binding(s)", references)
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM web_policies WHERE id=?`, id)
	return err
}

func (r *Repository) InitializeListeners(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS web_listeners (
		id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,address TEXT NOT NULL,
		protocol TEXT NOT NULL,enabled INTEGER NOT NULL DEFAULT 1,min_tls_version TEXT NOT NULL DEFAULT 'TLS1.2',
		default_certificate_id INTEGER,trusted_proxies TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);
		CREATE INDEX IF NOT EXISTS idx_web_listeners_enabled ON web_listeners(enabled);
		CREATE TABLE IF NOT EXISTS web_listener_policies(listener_id INTEGER NOT NULL,policy_id INTEGER NOT NULL,position INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(listener_id,policy_id),FOREIGN KEY(listener_id) REFERENCES web_listeners(id) ON DELETE CASCADE,FOREIGN KEY(policy_id) REFERENCES web_policies(id) ON DELETE RESTRICT);`)
	return err
}

func (r *Repository) EnsureDefaultListener(ctx context.Context, address string) error {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM web_listeners`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO web_listeners(name,address,protocol,enabled) VALUES('http',?,'http',1)`, address)
	return err
}

func (r *Repository) InitializeHosts(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS web_hosts(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,hostnames TEXT NOT NULL,enabled INTEGER NOT NULL DEFAULT 1,certificate_id INTEGER,force_https INTEGER NOT NULL DEFAULT 0,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);CREATE TABLE IF NOT EXISTS web_host_listeners(host_id INTEGER NOT NULL,listener_id INTEGER NOT NULL,PRIMARY KEY(host_id,listener_id),FOREIGN KEY(host_id) REFERENCES web_hosts(id) ON DELETE CASCADE,FOREIGN KEY(listener_id) REFERENCES web_listeners(id) ON DELETE CASCADE);CREATE TABLE IF NOT EXISTS web_host_policies(host_id INTEGER NOT NULL,policy_id INTEGER NOT NULL,position INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(host_id,policy_id),FOREIGN KEY(host_id) REFERENCES web_hosts(id) ON DELETE CASCADE,FOREIGN KEY(policy_id) REFERENCES web_policies(id) ON DELETE RESTRICT);CREATE INDEX IF NOT EXISTS idx_web_hosts_enabled ON web_hosts(enabled);`)
	return err
}

func (r *Repository) InitializeRoutes(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS web_routes(id INTEGER PRIMARY KEY AUTOINCREMENT,host_id INTEGER NOT NULL,name TEXT NOT NULL,enabled INTEGER NOT NULL DEFAULT 1,method TEXT NOT NULL DEFAULT '',path TEXT NOT NULL DEFAULT '/',handler_type TEXT NOT NULL,upstream_url TEXT NOT NULL DEFAULT '',root TEXT NOT NULL DEFAULT '',strip_prefix TEXT NOT NULL DEFAULT '',response_status INTEGER NOT NULL DEFAULT 0,response_body TEXT NOT NULL DEFAULT '',redirect_location TEXT NOT NULL DEFAULT '',redirect_status INTEGER NOT NULL DEFAULT 0,worker_script TEXT NOT NULL DEFAULT '',request_timeout_seconds INTEGER NOT NULL DEFAULT 0,response_header_timeout_seconds INTEGER NOT NULL DEFAULT 0,max_conns_per_host INTEGER NOT NULL DEFAULT 0,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,FOREIGN KEY(host_id) REFERENCES web_hosts(id) ON DELETE CASCADE,UNIQUE(host_id,name));CREATE TABLE IF NOT EXISTS web_route_policies(route_id INTEGER NOT NULL,policy_id INTEGER NOT NULL,position INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(route_id,policy_id),FOREIGN KEY(route_id) REFERENCES web_routes(id) ON DELETE CASCADE,FOREIGN KEY(policy_id) REFERENCES web_policies(id) ON DELETE RESTRICT);CREATE INDEX IF NOT EXISTS idx_web_routes_host ON web_routes(host_id);`)
	return err
}

const routeColumns = `r.id,r.host_id,h.name,r.name,r.enabled,r.method,r.path,r.handler_type,r.upstream_url,r.root,r.strip_prefix,r.response_status,r.response_body,r.redirect_location,r.redirect_status,r.worker_script,r.request_timeout_seconds,r.response_header_timeout_seconds,r.max_conns_per_host,r.created_at,r.updated_at`

func scanRoute(row interface{ Scan(...any) error }) (*Route, error) {
	var x Route
	var created, updated string
	err := row.Scan(&x.ID, &x.HostID, &x.HostName, &x.Name, &x.Enabled, &x.Method, &x.Path, &x.HandlerType, &x.UpstreamURL, &x.Root, &x.StripPrefix, &x.ResponseStatus, &x.ResponseBody, &x.RedirectLocation, &x.RedirectStatus, &x.WorkerScript, &x.RequestTimeoutSeconds, &x.ResponseHeaderTimeoutSeconds, &x.MaxConnsPerHost, &created, &updated)
	if err != nil {
		return nil, err
	}
	x.CreatedAt = parseWebTime(created)
	x.UpdatedAt = parseWebTime(updated)
	return &x, nil
}
func (r *Repository) ListRoutesByHost(ctx context.Context, hostID int64) ([]Route, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+routeColumns+` FROM web_routes r JOIN web_hosts h ON h.id=r.host_id WHERE r.host_id=? ORDER BY r.created_at ASC`, hostID)
	if err != nil {
		return nil, err
	}
	items := []Route{}
	for rows.Next() {
		x, err := scanRoute(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, *x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range items {
		if err := r.loadRoutePolicies(ctx, &items[i]); err != nil {
			return nil, err
		}
	}
	return items, nil
}
func (r *Repository) GetRoute(ctx context.Context, id int64) (*Route, error) {
	x, err := scanRoute(r.db.QueryRowContext(ctx, `SELECT `+routeColumns+` FROM web_routes r JOIN web_hosts h ON h.id=r.host_id WHERE r.id=?`, id))
	if err != nil {
		return nil, err
	}
	if err := r.loadRoutePolicies(ctx, x); err != nil {
		return nil, err
	}
	return x, nil
}
func (r *Repository) loadRoutePolicies(ctx context.Context, x *Route) error {
	rows, err := r.db.QueryContext(ctx, `SELECT p.id,p.name FROM web_route_policies b JOIN web_policies p ON p.id=b.policy_id WHERE b.route_id=? ORDER BY b.position,p.name`, x.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	x.PolicyIDs = []int64{}
	x.PolicyNames = []string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		x.PolicyIDs = append(x.PolicyIDs, id)
		x.PolicyNames = append(x.PolicyNames, name)
	}
	return rows.Err()
}
func (r *Repository) CreateRoute(ctx context.Context, x Route) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now()
	res, err := tx.ExecContext(ctx, `INSERT INTO web_routes(host_id,name,enabled,method,path,handler_type,upstream_url,root,strip_prefix,response_status,response_body,redirect_location,redirect_status,worker_script,request_timeout_seconds,response_header_timeout_seconds,max_conns_per_host,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, x.HostID, x.Name, x.Enabled, x.Method, x.Path, x.HandlerType, x.UpstreamURL, x.Root, x.StripPrefix, x.ResponseStatus, x.ResponseBody, x.RedirectLocation, x.RedirectStatus, x.WorkerScript, x.RequestTimeoutSeconds, x.ResponseHeaderTimeoutSeconds, x.MaxConnsPerHost, now, now)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err = r.replaceRoutePolicies(ctx, tx, id, x.PolicyIDs); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}
func (r *Repository) UpdateRoute(ctx context.Context, x Route) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE web_routes SET name=?,enabled=?,method=?,path=?,handler_type=?,upstream_url=?,root=?,strip_prefix=?,response_status=?,response_body=?,redirect_location=?,redirect_status=?,worker_script=?,request_timeout_seconds=?,response_header_timeout_seconds=?,max_conns_per_host=?,updated_at=? WHERE id=?`, x.Name, x.Enabled, x.Method, x.Path, x.HandlerType, x.UpstreamURL, x.Root, x.StripPrefix, x.ResponseStatus, x.ResponseBody, x.RedirectLocation, x.RedirectStatus, x.WorkerScript, x.RequestTimeoutSeconds, x.ResponseHeaderTimeoutSeconds, x.MaxConnsPerHost, time.Now(), x.ID); err != nil {
		return err
	}
	if err = r.replaceRoutePolicies(ctx, tx, x.ID, x.PolicyIDs); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) DeleteRoute(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM web_route_policies WHERE route_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM web_routes WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) replaceRoutePolicies(ctx context.Context, tx *sql.Tx, id int64, ids []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM web_route_policies WHERE route_id=?`, id); err != nil {
		return err
	}
	for pos, policy := range ids {
		if _, err := tx.ExecContext(ctx, `INSERT INTO web_route_policies(route_id,policy_id,position) VALUES(?,?,?)`, id, policy, pos); err != nil {
			return err
		}
	}
	return nil
}
func scanHost(row interface{ Scan(...any) error }) (*Host, error) {
	var x Host
	var cert sql.NullInt64
	var created, updated string
	err := row.Scan(&x.ID, &x.Name, &x.Hostnames, &x.Enabled, &cert, &x.ForceHTTPS, &created, &updated)
	if err != nil {
		return nil, err
	}
	if cert.Valid {
		id := cert.Int64
		x.CertificateID = &id
	}
	x.CreatedAt = parseWebTime(created)
	x.UpdatedAt = parseWebTime(updated)
	return &x, nil
}
func parseWebTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999-07:00", "2006-01-02 15:04:05-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}
func (r *Repository) ListHosts(ctx context.Context) ([]Host, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,hostnames,enabled,certificate_id,force_https,created_at,updated_at FROM web_hosts ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	items := []Host{}
	for rows.Next() {
		x, err := scanHost(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, *x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range items {
		if err := r.loadHostRelations(ctx, &items[i]); err != nil {
			return nil, err
		}
	}
	return items, nil
}
func (r *Repository) GetHost(ctx context.Context, id int64) (*Host, error) {
	x, err := scanHost(r.db.QueryRowContext(ctx, `SELECT id,name,hostnames,enabled,certificate_id,force_https,created_at,updated_at FROM web_hosts WHERE id=?`, id))
	if err != nil {
		return nil, err
	}
	if err := r.loadHostRelations(ctx, x); err != nil {
		return nil, err
	}
	return x, nil
}
func (r *Repository) loadHostRelations(ctx context.Context, x *Host) error {
	rows, err := r.db.QueryContext(ctx, `SELECT l.id,l.name FROM web_host_listeners b JOIN web_listeners l ON l.id=b.listener_id WHERE b.host_id=? ORDER BY l.created_at`, x.ID)
	if err != nil {
		return err
	}
	x.ListenerIDs = []int64{}
	x.ListenerNames = []string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return err
		}
		x.ListenerIDs = append(x.ListenerIDs, id)
		x.ListenerNames = append(x.ListenerNames, name)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = r.db.QueryContext(ctx, `SELECT p.id,p.name FROM web_host_policies b JOIN web_policies p ON p.id=b.policy_id WHERE b.host_id=? ORDER BY b.position,p.name`, x.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	x.PolicyIDs = []int64{}
	x.PolicyNames = []string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		x.PolicyIDs = append(x.PolicyIDs, id)
		x.PolicyNames = append(x.PolicyNames, name)
	}
	return rows.Err()
}
func (r *Repository) CreateHost(ctx context.Context, x Host) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now()
	res, err := tx.ExecContext(ctx, `INSERT INTO web_hosts(name,hostnames,enabled,certificate_id,force_https,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, x.Name, x.Hostnames, x.Enabled, nullableCertificate(x.CertificateID), x.ForceHTTPS, now, now)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err = replaceBindings(ctx, tx, "web_host_listeners", "host_id", id, x.ListenerIDs); err != nil {
		return 0, err
	}
	if err = replaceBindings(ctx, tx, "web_host_policies", "host_id", id, x.PolicyIDs); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}
func (r *Repository) UpdateHost(ctx context.Context, x Host) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE web_hosts SET name=?,hostnames=?,enabled=?,certificate_id=?,force_https=?,updated_at=? WHERE id=?`, x.Name, x.Hostnames, x.Enabled, nullableCertificate(x.CertificateID), x.ForceHTTPS, time.Now(), x.ID); err != nil {
		return err
	}
	if err = replaceBindings(ctx, tx, "web_host_listeners", "host_id", x.ID, x.ListenerIDs); err != nil {
		return err
	}
	if err = replaceBindings(ctx, tx, "web_host_policies", "host_id", x.ID, x.PolicyIDs); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) DeleteHost(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM web_route_policies WHERE route_id IN (SELECT id FROM web_routes WHERE host_id=?)`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM web_routes WHERE host_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM web_host_policies WHERE host_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM web_host_listeners WHERE host_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM web_hosts WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) SeedDefaultWorkerHost(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now()
	if _, err = tx.ExecContext(ctx, `INSERT INTO web_listeners(name,address,protocol,enabled,min_tls_version,created_at,updated_at) VALUES('http',':80','http',1,'TLS1.2',?,?) ON CONFLICT(name) DO UPDATE SET address=excluded.address,protocol=excluded.protocol,enabled=1,updated_at=excluded.updated_at`, now, now); err != nil {
		return err
	}
	var listenerID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM web_listeners WHERE name='http'`).Scan(&listenerID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO web_hosts(name,hostnames,enabled,force_https,created_at,updated_at) VALUES('default','*',1,0,?,?) ON CONFLICT(name) DO UPDATE SET hostnames=excluded.hostnames,enabled=1,force_https=0,updated_at=excluded.updated_at`, now, now); err != nil {
		return err
	}
	var hostID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM web_hosts WHERE name='default'`).Scan(&hostID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO web_host_listeners(host_id,listener_id) VALUES(?,?) ON CONFLICT(host_id,listener_id) DO NOTHING`, hostID, listenerID); err != nil {
		return err
	}
	const script = `export default {
  fetch(request, env) {
    return new Response("OhMyServer Web Server", {
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    });
  }
}`
	if _, err = tx.ExecContext(ctx, `INSERT INTO web_routes(host_id,name,enabled,method,path,handler_type,worker_script,created_at,updated_at) VALUES(?,'worker',1,'','/','worker',?,?,?) ON CONFLICT(host_id,name) DO UPDATE SET enabled=1,method='',path='/',handler_type='worker',worker_script=excluded.worker_script,updated_at=excluded.updated_at`, hostID, script, now, now); err != nil {
		return err
	}
	return tx.Commit()
}
func replaceBindings(ctx context.Context, tx *sql.Tx, table, column string, id int64, ids []int64) error {
	if column != "host_id" {
		return fmt.Errorf("invalid webserver binding table")
	}
	switch table {
	case "web_host_listeners":
		if _, err := tx.ExecContext(ctx, `DELETE FROM web_host_listeners WHERE host_id=?`, id); err != nil {
			return err
		}
	case "web_host_policies":
		if _, err := tx.ExecContext(ctx, `DELETE FROM web_host_policies WHERE host_id=?`, id); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid webserver binding table")
	}
	for pos, value := range ids {
		if table == "web_host_listeners" {
			if _, err := tx.ExecContext(ctx, `INSERT INTO web_host_listeners(host_id,listener_id) VALUES(?,?)`, id, value); err != nil {
				return err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `INSERT INTO web_host_policies(host_id,policy_id,position) VALUES(?,?,?)`, id, value, pos); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Repository) ListListeners(ctx context.Context) ([]Listener, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,address,protocol,enabled,min_tls_version,default_certificate_id,trusted_proxies FROM web_listeners ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	items := []Listener{}
	for rows.Next() {
		var x Listener
		var cert sql.NullInt64
		if err := rows.Scan(&x.ID, &x.Name, &x.Address, &x.Protocol, &x.Enabled, &x.MinTLSVersion, &cert, &x.TrustedProxies); err != nil {
			rows.Close()
			return nil, err
		}
		if cert.Valid {
			id := cert.Int64
			x.CertificateID = &id
		}
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range items {
		if err := r.loadListenerPolicies(ctx, &items[i]); err != nil {
			return nil, err
		}
	}
	return items, nil
}
func (r *Repository) GetListener(ctx context.Context, id int64) (*Listener, error) {
	var x Listener
	var cert sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT id,name,address,protocol,enabled,min_tls_version,default_certificate_id,trusted_proxies FROM web_listeners WHERE id=?`, id).Scan(&x.ID, &x.Name, &x.Address, &x.Protocol, &x.Enabled, &x.MinTLSVersion, &cert, &x.TrustedProxies)
	if err != nil {
		return nil, err
	}
	if cert.Valid {
		v := cert.Int64
		x.CertificateID = &v
	}
	if err := r.loadListenerPolicies(ctx, &x); err != nil {
		return nil, err
	}
	return &x, nil
}
func (r *Repository) loadListenerPolicies(ctx context.Context, x *Listener) error {
	rows, err := r.db.QueryContext(ctx, `SELECT p.id,p.name FROM web_listener_policies b JOIN web_policies p ON p.id=b.policy_id WHERE b.listener_id=? ORDER BY b.position,p.name`, x.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	x.PolicyIDs = []int64{}
	x.PolicyNames = []string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		x.PolicyIDs = append(x.PolicyIDs, id)
		x.PolicyNames = append(x.PolicyNames, name)
	}
	return rows.Err()
}
func (r *Repository) CreateListener(ctx context.Context, x Listener) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now()
	res, err := tx.ExecContext(ctx, `INSERT INTO web_listeners(name,address,protocol,enabled,min_tls_version,default_certificate_id,trusted_proxies,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, x.Name, x.Address, x.Protocol, x.Enabled, x.MinTLSVersion, nullableCertificate(x.CertificateID), x.TrustedProxies, now, now)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err = r.replaceListenerPolicies(ctx, tx, id, x.PolicyIDs); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}
func (r *Repository) UpdateListener(ctx context.Context, x Listener) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE web_listeners SET name=?,address=?,protocol=?,enabled=?,min_tls_version=?,default_certificate_id=?,trusted_proxies=?,updated_at=? WHERE id=?`, x.Name, x.Address, x.Protocol, x.Enabled, x.MinTLSVersion, nullableCertificate(x.CertificateID), x.TrustedProxies, time.Now(), x.ID); err != nil {
		return err
	}
	if err = r.replaceListenerPolicies(ctx, tx, x.ID, x.PolicyIDs); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) DeleteListener(ctx context.Context, id int64) error {
	var refs int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM web_host_listeners WHERE listener_id=?`, id).Scan(&refs); err != nil {
		return err
	}
	if refs > 0 {
		return fmt.Errorf("listener is used by %d virtual host(s)", refs)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM web_listener_policies WHERE listener_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM web_listeners WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) replaceListenerPolicies(ctx context.Context, tx *sql.Tx, id int64, ids []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM web_listener_policies WHERE listener_id=?`, id); err != nil {
		return err
	}
	for pos, policyID := range ids {
		if _, err := tx.ExecContext(ctx, `INSERT INTO web_listener_policies(listener_id,policy_id,position) VALUES(?,?,?)`, id, policyID, pos); err != nil {
			return err
		}
	}
	return nil
}
func nullableCertificate(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
