package stores

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lsongdev/dhcp-go/server"
	"github.com/lsongdev/dhcp-go/store"
)

// DHCP keeps the DHCP server's settings, reservations and leases in the
// shared database (dhcp-go keeps them in YAML files when it runs on its
// own). The tables are the ones dhcp-go's own SQLite store created, so
// existing state carries over.
type DHCP struct{ db *sql.DB }

// NewDHCP migrates the dhcp_ tables in db.
func NewDHCP(ctx context.Context, db *sql.DB) (*DHCP, error) {
	if err := migrate(ctx, db, "dhcp_schema", dhcpMigrations); err != nil {
		return nil, fmt.Errorf("dhcp: %w", err)
	}
	return &DHCP{db: db}, nil
}

var dhcpMigrations = []string{`
CREATE TABLE dhcp_settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE dhcp_reservations (
	id          INTEGER PRIMARY KEY,
	mac         TEXT NOT NULL UNIQUE,
	ip          TEXT NOT NULL UNIQUE,
	hostname    TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	created_at  INTEGER NOT NULL, -- unix nanoseconds
	updated_at  INTEGER NOT NULL
);
CREATE TABLE dhcp_leases (
	ip         TEXT PRIMARY KEY,
	mac        TEXT NOT NULL,
	hostname   TEXT NOT NULL DEFAULT '',
	expires_at INTEGER NOT NULL, -- unix nanoseconds
	updated_at INTEGER NOT NULL
)`}

const dhcpConfigKey = "config"

// Config returns the saved configuration; ok is false before the first
// save.
func (s *DHCP) Config(ctx context.Context) (cfg server.Config, ok bool, err error) {
	var v string
	err = s.db.QueryRowContext(ctx, `SELECT value FROM dhcp_settings WHERE key = ?`, dhcpConfigKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return cfg, false, nil
	}
	if err != nil {
		return cfg, false, err
	}
	return cfg, true, json.Unmarshal([]byte(v), &cfg)
}

// SaveConfig stores the configuration.
func (s *DHCP) SaveConfig(ctx context.Context, cfg server.Config) error {
	v, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO dhcp_settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, dhcpConfigKey, string(v))
	return err
}

const reservationColumns = `id, mac, ip, hostname, description, created_at, updated_at`

func scanReservation(row interface{ Scan(...any) error }) (server.Reservation, error) {
	var r server.Reservation
	var created, updated int64
	err := row.Scan(&r.ID, &r.MAC, &r.IP, &r.Hostname, &r.Description, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return r, store.ErrNotFound
	}
	r.CreatedAt, r.UpdatedAt = time.Unix(0, created).UTC(), time.Unix(0, updated).UTC()
	return r, err
}

// Reservations returns every reservation.
func (s *DHCP) Reservations(ctx context.Context) ([]server.Reservation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+reservationColumns+` FROM dhcp_reservations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []server.Reservation{}
	for rows.Next() {
		r, err := scanReservation(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// Reservation returns one reservation.
func (s *DHCP) Reservation(ctx context.Context, id int64) (server.Reservation, error) {
	return scanReservation(s.db.QueryRowContext(ctx, `SELECT `+reservationColumns+` FROM dhcp_reservations WHERE id = ?`, id))
}

// CreateReservation inserts r and sets its ID and timestamps.
func (s *DHCP) CreateReservation(ctx context.Context, r *server.Reservation) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `INSERT INTO dhcp_reservations (mac, ip, hostname, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, r.MAC, r.IP, r.Hostname, r.Description, now.UnixNano(), now.UnixNano())
	if err != nil {
		return dhcpConflict(err)
	}
	r.ID, err = res.LastInsertId()
	r.CreatedAt, r.UpdatedAt = now, now
	return err
}

// UpdateReservation replaces r's fields.
func (s *DHCP) UpdateReservation(ctx context.Context, r *server.Reservation) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `UPDATE dhcp_reservations SET mac = ?, ip = ?, hostname = ?, description = ?, updated_at = ?
		WHERE id = ?`, r.MAC, r.IP, r.Hostname, r.Description, now.UnixNano(), r.ID)
	if err != nil {
		return dhcpConflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	r.UpdatedAt = now
	return nil
}

// DeleteReservation removes a reservation; removing a missing one succeeds.
func (s *DHCP) DeleteReservation(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM dhcp_reservations WHERE id = ?`, id)
	return err
}

// Leases returns every stored lease.
func (s *DHCP) Leases(ctx context.Context) ([]server.Lease, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ip, mac, hostname, expires_at, updated_at FROM dhcp_leases`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []server.Lease{}
	for rows.Next() {
		var l server.Lease
		var expires, updated int64
		if err := rows.Scan(&l.IP, &l.MAC, &l.Hostname, &expires, &updated); err != nil {
			return nil, err
		}
		l.ExpiresAt, l.UpdatedAt = time.Unix(0, expires).UTC(), time.Unix(0, updated).UTC()
		list = append(list, l)
	}
	return list, rows.Err()
}

// SaveLease implements server.LeaseStore.
func (s *DHCP) SaveLease(l server.Lease) error {
	_, err := s.db.Exec(`INSERT INTO dhcp_leases (ip, mac, hostname, expires_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (ip) DO UPDATE SET mac = excluded.mac, hostname = excluded.hostname,
		expires_at = excluded.expires_at, updated_at = excluded.updated_at`,
		l.IP, l.MAC, l.Hostname, l.ExpiresAt.UnixNano(), l.UpdatedAt.UnixNano())
	return err
}

// DeleteLease implements server.LeaseStore.
func (s *DHCP) DeleteLease(ip string) error {
	_, err := s.db.Exec(`DELETE FROM dhcp_leases WHERE ip = ?`, ip)
	return err
}

// PruneLeases deletes leases that expired before t.
func (s *DHCP) PruneLeases(ctx context.Context, t time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM dhcp_leases WHERE expires_at < ?`, t.UnixNano())
	return err
}

func dhcpConflict(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return store.ErrConflict
	}
	return err
}

var _ store.Store = (*DHCP)(nil)
