package web

import (
	"context"
	"database/sql"
	"time"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Initialize(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS dhcp_settings (
		id INTEGER PRIMARY KEY CHECK(id=1), enabled INTEGER NOT NULL DEFAULT 0, listen_addr TEXT NOT NULL DEFAULT ':67',
		server_ip TEXT NOT NULL DEFAULT '192.168.1.1', subnet_mask TEXT NOT NULL DEFAULT '255.255.255.0',
		pool_start TEXT NOT NULL DEFAULT '192.168.1.100', pool_end TEXT NOT NULL DEFAULT '192.168.1.200', router TEXT NOT NULL DEFAULT '192.168.1.1',
		dns_servers TEXT NOT NULL DEFAULT '1.1.1.1
8.8.8.8', ntp_servers TEXT NOT NULL DEFAULT '', domain_name TEXT NOT NULL DEFAULT 'lan',
		broadcast_address TEXT NOT NULL DEFAULT '192.168.1.255', lease_seconds INTEGER NOT NULL DEFAULT 86400,
		renewal_seconds INTEGER NOT NULL DEFAULT 43200, rebinding_seconds INTEGER NOT NULL DEFAULT 75600, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);
	INSERT OR IGNORE INTO dhcp_settings (id) VALUES (1);
	CREATE TABLE IF NOT EXISTS dhcp_static_leases (id INTEGER PRIMARY KEY AUTOINCREMENT, mac TEXT NOT NULL UNIQUE COLLATE NOCASE, ip TEXT NOT NULL UNIQUE, hostname TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);
	CREATE TABLE IF NOT EXISTS dhcp_leases (id INTEGER PRIMARY KEY AUTOINCREMENT, mac TEXT NOT NULL UNIQUE COLLATE NOCASE, ip TEXT NOT NULL UNIQUE, hostname TEXT NOT NULL DEFAULT '', state TEXT NOT NULL DEFAULT 'active', expires_at DATETIME NOT NULL, last_seen DATETIME NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);
	CREATE INDEX IF NOT EXISTS idx_dhcp_leases_expires ON dhcp_leases(expires_at);`)
	if err != nil {
		return err
	}
	return nil
}

// ImportStaticLeases persists records read by an application-level migration
// adapter. The DHCP module itself never reads legacy core-owned tables.
func (r *Repository) ImportStaticLeases(ctx context.Context, leases []StaticLease) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, lease := range leases {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO dhcp_static_leases(id,mac,ip,hostname,description) VALUES(?,?,?,?,?)`, lease.ID, lease.MAC, lease.IP, lease.Hostname, lease.Description); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (r *Repository) Settings(ctx context.Context) (*Settings, error) {
	var x Settings
	var enabled int
	var updated string
	err := r.db.QueryRowContext(ctx, `SELECT enabled,listen_addr,server_ip,subnet_mask,pool_start,pool_end,router,dns_servers,ntp_servers,domain_name,broadcast_address,lease_seconds,renewal_seconds,rebinding_seconds,updated_at FROM dhcp_settings WHERE id=1`).Scan(&enabled, &x.ListenAddr, &x.ServerIP, &x.SubnetMask, &x.PoolStart, &x.PoolEnd, &x.Router, &x.DNSServers, &x.NTPServers, &x.DomainName, &x.BroadcastAddress, &x.LeaseSeconds, &x.RenewalSeconds, &x.RebindingSeconds, &updated)
	if err != nil {
		return nil, err
	}
	x.Enabled = enabled != 0
	x.UpdatedAt = parseTime(updated)
	return &x, nil
}
func (r *Repository) UpdateSettings(ctx context.Context, x Settings) error {
	_, err := r.db.ExecContext(ctx, `UPDATE dhcp_settings SET enabled=?,listen_addr=?,server_ip=?,subnet_mask=?,pool_start=?,pool_end=?,router=?,dns_servers=?,ntp_servers=?,domain_name=?,broadcast_address=?,lease_seconds=?,renewal_seconds=?,rebinding_seconds=?,updated_at=? WHERE id=1`, x.Enabled, x.ListenAddr, x.ServerIP, x.SubnetMask, x.PoolStart, x.PoolEnd, x.Router, x.DNSServers, x.NTPServers, x.DomainName, x.BroadcastAddress, x.LeaseSeconds, x.RenewalSeconds, x.RebindingSeconds, time.Now())
	return err
}
func (r *Repository) StaticLeases(ctx context.Context) ([]StaticLease, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,mac,ip,hostname,description,created_at,updated_at FROM dhcp_static_leases ORDER BY ip`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StaticLease{}
	for rows.Next() {
		var x StaticLease
		var c, u string
		if err := rows.Scan(&x.ID, &x.MAC, &x.IP, &x.Hostname, &x.Description, &c, &u); err != nil {
			return nil, err
		}
		x.CreatedAt, x.UpdatedAt = parseTime(c), parseTime(u)
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) StaticLease(ctx context.Context, id int64) (*StaticLease, error) {
	var x StaticLease
	var c, u string
	err := r.db.QueryRowContext(ctx, `SELECT id,mac,ip,hostname,description,created_at,updated_at FROM dhcp_static_leases WHERE id=?`, id).Scan(&x.ID, &x.MAC, &x.IP, &x.Hostname, &x.Description, &c, &u)
	if err != nil {
		return nil, err
	}
	x.CreatedAt, x.UpdatedAt = parseTime(c), parseTime(u)
	return &x, nil
}
func (r *Repository) StaticLeaseByMAC(ctx context.Context, mac string) (*StaticLease, error) {
	var x StaticLease
	var c, u string
	err := r.db.QueryRowContext(ctx, `SELECT id,mac,ip,hostname,description,created_at,updated_at FROM dhcp_static_leases WHERE mac=? COLLATE NOCASE`, mac).Scan(&x.ID, &x.MAC, &x.IP, &x.Hostname, &x.Description, &c, &u)
	if err != nil {
		return nil, err
	}
	x.CreatedAt, x.UpdatedAt = parseTime(c), parseTime(u)
	return &x, nil
}
func (r *Repository) SaveStaticLease(ctx context.Context, id int64, x StaticLease) error {
	now := time.Now()
	if id == 0 {
		_, err := r.db.ExecContext(ctx, `INSERT INTO dhcp_static_leases(mac,ip,hostname,description,created_at,updated_at) VALUES(?,?,?,?,?,?)`, x.MAC, x.IP, x.Hostname, x.Description, now, now)
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE dhcp_static_leases SET mac=?,ip=?,hostname=?,description=?,updated_at=? WHERE id=?`, x.MAC, x.IP, x.Hostname, x.Description, now, id)
	return err
}
func (r *Repository) DeleteStaticLease(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM dhcp_static_leases WHERE id=?`, id)
	return err
}
func (r *Repository) Leases(ctx context.Context) ([]Lease, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,mac,ip,hostname,state,expires_at,last_seen,created_at,updated_at FROM dhcp_leases ORDER BY ip`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Lease{}
	for rows.Next() {
		var x Lease
		var e, l, c, u string
		if err := rows.Scan(&x.ID, &x.MAC, &x.IP, &x.Hostname, &x.State, &e, &l, &c, &u); err != nil {
			return nil, err
		}
		x.ExpiresAt, x.LastSeen, x.CreatedAt, x.UpdatedAt = parseTime(e), parseTime(l), parseTime(c), parseTime(u)
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) UpsertLease(ctx context.Context, mac, ip, hostname, state string, seconds int) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `INSERT INTO dhcp_leases(mac,ip,hostname,state,expires_at,last_seen,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(mac) DO UPDATE SET ip=excluded.ip,hostname=excluded.hostname,state=excluded.state,expires_at=excluded.expires_at,last_seen=excluded.last_seen,updated_at=excluded.updated_at`, mac, ip, hostname, state, now.Add(time.Duration(seconds)*time.Second), now, now, now)
	return err
}
func (r *Repository) DeleteLeaseByIP(ctx context.Context, ip string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM dhcp_leases WHERE ip=?`, ip)
	return err
}
func (r *Repository) ActiveLeaseMAC(ctx context.Context, ip string) (string, error) {
	var mac string
	err := r.db.QueryRowContext(ctx, `SELECT mac FROM dhcp_leases WHERE ip=? AND expires_at>?`, ip, time.Now()).Scan(&mac)
	return mac, err
}
func (r *Repository) DeleteLease(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM dhcp_leases WHERE id=?`, id)
	return err
}
func (r *Repository) CleanupExpired(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM dhcp_leases WHERE expires_at<=?`, time.Now())
	return err
}
func parseTime(v string) time.Time {
	for _, f := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05-07:00"} {
		if t, e := time.Parse(f, v); e == nil {
			return t
		}
	}
	return time.Time{}
}
