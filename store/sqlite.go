// Package store keeps SSH hosts and the panel key in SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lsongdev/ssh-go/hosts"
	_ "modernc.org/sqlite" // registers the "sqlite" driver for Open
)

var (
	// ErrNotFound reports a host that does not exist.
	ErrNotFound = errors.New("host not found")
	// ErrConflict reports a duplicate host name.
	ErrConflict = errors.New("a host with this name exists")
)

// SQLite stores hosts. Its tables are prefixed "ssh_", so the database can
// be shared with other modules. Passwords and private keys are stored as
// given: protect the database file.
type SQLite struct {
	db    *sql.DB
	owned bool // opened by Open, so Close closes it
}

// Open opens (creating if needed) a database file of its own at path.
func Open(ctx context.Context, path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s, err := New(ctx, db)
	if err != nil {
		db.Close()
		return nil, err
	}
	s.owned = true
	return s, nil
}

// New migrates the ssh_ tables in db, which the caller owns and may share
// with other modules.
func New(ctx context.Context, db *sql.DB) (*SQLite, error) {
	s := &SQLite{db: db}
	if err := s.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return s, nil
}

// Close closes the database if Open opened it.
func (s *SQLite) Close() error {
	if s.owned {
		return s.db.Close()
	}
	return nil
}

// migrations are applied in order; ssh_schema records how many have run.
// Append new migrations, never edit old ones.
var migrations = []string{`
CREATE TABLE ssh_hosts (
	id                INTEGER PRIMARY KEY,
	name              TEXT NOT NULL UNIQUE,
	address           TEXT NOT NULL,
	port              INTEGER NOT NULL,
	user              TEXT NOT NULL,
	auth              TEXT NOT NULL CHECK (auth IN ('password', 'key', 'panel')),
	password          TEXT NOT NULL DEFAULT '',
	private_key       TEXT NOT NULL DEFAULT '',
	passphrase        TEXT NOT NULL DEFAULT '',
	host_key          TEXT NOT NULL DEFAULT '',  -- pinned, authorized_keys format
	description       TEXT NOT NULL DEFAULT '',
	last_connected_at INTEGER NOT NULL DEFAULT 0, -- unix nanoseconds
	created_at        INTEGER NOT NULL,
	updated_at        INTEGER NOT NULL
);
CREATE TABLE ssh_settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
)`}

func (s *SQLite) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS ssh_schema (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM ssh_schema`).Scan(&version); err != nil {
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
	if _, err := tx.ExecContext(ctx, `DELETE FROM ssh_schema`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO ssh_schema (version) VALUES (?)`, len(migrations)); err != nil {
		return err
	}
	return tx.Commit()
}

const columns = `id, name, address, port, user, auth, password, private_key, passphrase, host_key, description, last_connected_at, created_at, updated_at`

func scan(row interface{ Scan(...any) error }) (hosts.Host, error) {
	var h hosts.Host
	var last, created, updated int64
	err := row.Scan(&h.ID, &h.Name, &h.Address, &h.Port, &h.User, &h.Auth, &h.Password, &h.PrivateKey, &h.Passphrase,
		&h.HostKey, &h.Description, &last, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return h, ErrNotFound
	}
	if last > 0 {
		h.LastConnectedAt = time.Unix(0, last)
	}
	h.CreatedAt, h.UpdatedAt = time.Unix(0, created), time.Unix(0, updated)
	return h, err
}

// List returns every host by name.
func (s *SQLite) List(ctx context.Context) ([]hosts.Host, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+columns+` FROM ssh_hosts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []hosts.Host{}
	for rows.Next() {
		h, err := scan(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, h)
	}
	return list, rows.Err()
}

// Get returns one host.
func (s *SQLite) Get(ctx context.Context, id int64) (hosts.Host, error) {
	return scan(s.db.QueryRowContext(ctx, `SELECT `+columns+` FROM ssh_hosts WHERE id = ?`, id))
}

// Create inserts h and sets its ID and timestamps.
func (s *SQLite) Create(ctx context.Context, h *hosts.Host) error {
	now := time.Now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO ssh_hosts
		(name, address, port, user, auth, password, private_key, passphrase, host_key, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.Name, h.Address, h.Port, h.User, h.Auth, h.Password, h.PrivateKey, h.Passphrase, h.HostKey, h.Description, now.UnixNano(), now.UnixNano())
	if err != nil {
		return conflict(err)
	}
	h.ID, err = res.LastInsertId()
	h.CreatedAt, h.UpdatedAt = now, now
	return err
}

// Update replaces h's fields, including its pinned host key.
func (s *SQLite) Update(ctx context.Context, h *hosts.Host) error {
	now := time.Now()
	res, err := s.db.ExecContext(ctx, `UPDATE ssh_hosts SET name = ?, address = ?, port = ?, user = ?, auth = ?, password = ?,
		private_key = ?, passphrase = ?, host_key = ?, description = ?, updated_at = ? WHERE id = ?`,
		h.Name, h.Address, h.Port, h.User, h.Auth, h.Password, h.PrivateKey, h.Passphrase, h.HostKey, h.Description, now.UnixNano(), h.ID)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	h.UpdatedAt = now
	return nil
}

// Connected records a successful connection and pins hostKey if the host
// had none.
func (s *SQLite) Connected(ctx context.Context, id int64, hostKey string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE ssh_hosts SET last_connected_at = ?,
		host_key = CASE WHEN host_key = '' THEN ? ELSE host_key END WHERE id = ?`, time.Now().UnixNano(), hostKey, id)
	return err
}

// ForgetHostKey clears a host's pinned key, so the next connection pins
// whatever the server offers.
func (s *SQLite) ForgetHostKey(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE ssh_hosts SET host_key = '' WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a host; removing a missing host succeeds.
func (s *SQLite) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM ssh_hosts WHERE id = ?`, id)
	return err
}

// Setting returns a stored setting, or "" when unset.
func (s *SQLite) Setting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM ssh_settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// SetSetting stores a setting.
func (s *SQLite) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO ssh_settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func conflict(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrConflict
	}
	return err
}
