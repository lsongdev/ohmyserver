// Package store keeps DNS provider accounts in SQLite.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for Open
)

var (
	// ErrNotFound reports an account that does not exist.
	ErrNotFound = errors.New("account not found")
	// ErrConflict reports a duplicate account name.
	ErrConflict = errors.New("an account with this name exists")
)

// Account is a provider account. Credentials are stored as given: protect
// the database file.
type Account struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Kind        string            `json:"kind"` // cloudflare, …
	Credentials map[string]string `json:"-"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// SQLite stores accounts. Its tables are prefixed "domains_", so the
// database can be shared with other modules.
type SQLite struct {
	db    *sql.DB
	owned bool
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

// New migrates the domains_ tables in db, which the caller owns.
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

// migrations are applied in order; append, never edit.
var migrations = []string{`
CREATE TABLE domains_accounts (
	id          INTEGER PRIMARY KEY,
	name        TEXT NOT NULL UNIQUE,
	kind        TEXT NOT NULL,
	credentials TEXT NOT NULL, -- JSON object
	created_at  INTEGER NOT NULL, -- unix nanoseconds
	updated_at  INTEGER NOT NULL
)`}

func (s *SQLite) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS domains_schema (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM domains_schema`).Scan(&version); err != nil {
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
	if _, err := tx.ExecContext(ctx, `DELETE FROM domains_schema`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO domains_schema (version) VALUES (?)`, len(migrations)); err != nil {
		return err
	}
	return tx.Commit()
}

func scan(row interface{ Scan(...any) error }) (Account, error) {
	var a Account
	var creds string
	var created, updated int64
	err := row.Scan(&a.ID, &a.Name, &a.Kind, &creds, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	a.CreatedAt, a.UpdatedAt = time.Unix(0, created), time.Unix(0, updated)
	return a, json.Unmarshal([]byte(creds), &a.Credentials)
}

const columns = `id, name, kind, credentials, created_at, updated_at`

// List returns every account by name.
func (s *SQLite) List(ctx context.Context) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+columns+` FROM domains_accounts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Account{}
	for rows.Next() {
		a, err := scan(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// Get returns one account.
func (s *SQLite) Get(ctx context.Context, id int64) (Account, error) {
	return scan(s.db.QueryRowContext(ctx, `SELECT `+columns+` FROM domains_accounts WHERE id = ?`, id))
}

// Create inserts a and sets its ID and timestamps.
func (s *SQLite) Create(ctx context.Context, a *Account) error {
	creds, err := json.Marshal(a.Credentials)
	if err != nil {
		return err
	}
	now := time.Now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO domains_accounts (name, kind, credentials, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, a.Name, a.Kind, string(creds), now.UnixNano(), now.UnixNano())
	if err != nil {
		return conflict(err)
	}
	a.ID, err = res.LastInsertId()
	a.CreatedAt, a.UpdatedAt = now, now
	return err
}

// Update replaces a's name and credentials.
func (s *SQLite) Update(ctx context.Context, a *Account) error {
	creds, err := json.Marshal(a.Credentials)
	if err != nil {
		return err
	}
	now := time.Now()
	res, err := s.db.ExecContext(ctx, `UPDATE domains_accounts SET name = ?, credentials = ?, updated_at = ? WHERE id = ?`,
		a.Name, string(creds), now.UnixNano(), a.ID)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	a.UpdatedAt = now
	return nil
}

// Delete removes an account; removing a missing one succeeds.
func (s *SQLite) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM domains_accounts WHERE id = ?`, id)
	return err
}

func conflict(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrConflict
	}
	return err
}
