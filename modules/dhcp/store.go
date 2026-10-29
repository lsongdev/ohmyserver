// Package dhcp adapts the DHCP module to OhMyServer's shared database.
package dhcp

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

// SQLite stores the module's state. Its tables are prefixed "dhcp_", so
// the database can be shared with other modules.
type SQLite struct {
	db *sql.DB
}

// NewStore migrates the dhcp_ tables in db, which the caller owns and may share
// with other modules.
func NewStore(ctx context.Context, db *sql.DB) (*SQLite, error) {
	s := &SQLite{db: db}
	if err := s.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return s, nil
}

// migrations are applied in order; dhcp_schema records how many have run.
// Append new migrations, never edit old ones.
var migrations = []string{`
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

func (s *SQLite) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS dhcp_schema (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM dhcp_schema`).Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("schema version %d is newer than this program (%d)", version, len(migrations))
	}
	if version == len(migrations) {
		return nil
	}
	for i := version; i < len(migrations); i++ {
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM dhcp_schema`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO dhcp_schema (version) VALUES (?)`, len(migrations)); err != nil {
		return err
	}
	return tx.Commit()
}

const configKey = "config"

// Config returns the saved configuration; ok is false before the first
// save.
func (s *SQLite) Config(ctx context.Context) (cfg server.Config, ok bool, err error) {
	var v string
	err = s.db.QueryRowContext(ctx, `SELECT value FROM dhcp_settings WHERE key = ?`, configKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return cfg, false, nil
	}
	if err != nil {
		return cfg, false, err
	}
	return cfg, true, json.Unmarshal([]byte(v), &cfg)
}

// SaveConfig stores the configuration.
func (s *SQLite) SaveConfig(ctx context.Context, cfg server.Config) error {
	v, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO dhcp_settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, configKey, string(v))
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
	r.CreatedAt, r.UpdatedAt = time.Unix(0, created), time.Unix(0, updated)
	return r, err
}

// Reservations returns every reservation.
func (s *SQLite) Reservations(ctx context.Context) ([]server.Reservation, error) {
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
func (s *SQLite) Reservation(ctx context.Context, id int64) (server.Reservation, error) {
	return scanReservation(s.db.QueryRowContext(ctx, `SELECT `+reservationColumns+` FROM dhcp_reservations WHERE id = ?`, id))
}

// CreateReservation inserts r and sets its ID and timestamps.
func (s *SQLite) CreateReservation(ctx context.Context, r *server.Reservation) error {
	now := time.Now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO dhcp_reservations (mac, ip, hostname, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, r.MAC, r.IP, r.Hostname, r.Description, now.UnixNano(), now.UnixNano())
	if err != nil {
		return conflict(err)
	}
	r.ID, err = res.LastInsertId()
	r.CreatedAt, r.UpdatedAt = now, now
	return err
}

// UpdateReservation replaces r's fields.
func (s *SQLite) UpdateReservation(ctx context.Context, r *server.Reservation) error {
	now := time.Now()
	res, err := s.db.ExecContext(ctx, `UPDATE dhcp_reservations SET mac = ?, ip = ?, hostname = ?, description = ?, updated_at = ?
		WHERE id = ?`, r.MAC, r.IP, r.Hostname, r.Description, now.UnixNano(), r.ID)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	r.UpdatedAt = now
	return nil
}

// DeleteReservation removes a reservation; removing a missing one succeeds.
func (s *SQLite) DeleteReservation(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM dhcp_reservations WHERE id = ?`, id)
	return err
}

// Leases returns every stored lease.
func (s *SQLite) Leases(ctx context.Context) ([]server.Lease, error) {
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
		l.ExpiresAt, l.UpdatedAt = time.Unix(0, expires), time.Unix(0, updated)
		list = append(list, l)
	}
	return list, rows.Err()
}

// SaveLease implements server.LeaseStore.
func (s *SQLite) SaveLease(l server.Lease) error {
	_, err := s.db.Exec(`INSERT INTO dhcp_leases (ip, mac, hostname, expires_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (ip) DO UPDATE SET mac = excluded.mac, hostname = excluded.hostname,
		expires_at = excluded.expires_at, updated_at = excluded.updated_at`,
		l.IP, l.MAC, l.Hostname, l.ExpiresAt.UnixNano(), l.UpdatedAt.UnixNano())
	return err
}

// DeleteLease implements server.LeaseStore.
func (s *SQLite) DeleteLease(ip string) error {
	_, err := s.db.Exec(`DELETE FROM dhcp_leases WHERE ip = ?`, ip)
	return err
}

// PruneLeases deletes leases that expired before t.
func (s *SQLite) PruneLeases(ctx context.Context, t time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM dhcp_leases WHERE expires_at < ?`, t.UnixNano())
	return err
}

func conflict(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return store.ErrConflict
	}
	return err
}

var _ store.Store = (*SQLite)(nil)
