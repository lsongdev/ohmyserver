package stores

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lsongdev/forward-go/forward"
	"github.com/lsongdev/forward-go/store"
)

// ForwardRules keeps the port forwarder's rules in the shared database
// (forward-go keeps them in a YAML file when it runs on its own). The
// table is the one forward-go's own SQLite store created, so existing rules
// carry over.
type ForwardRules struct{ db *sql.DB }

var forwardMigrations = []string{`
CREATE TABLE forward_rules (
	id         INTEGER PRIMARY KEY,
	name       TEXT NOT NULL UNIQUE,
	protocol   TEXT NOT NULL CHECK (protocol IN ('tcp', 'udp')),
	listen     TEXT NOT NULL,
	target     TEXT NOT NULL,
	enabled    INTEGER NOT NULL,
	created_at INTEGER NOT NULL, -- unix nanoseconds
	updated_at INTEGER NOT NULL,
	UNIQUE (protocol, listen)
)`}

// NewForwardRules migrates the forward_ tables in db.
func NewForwardRules(ctx context.Context, db *sql.DB) (*ForwardRules, error) {
	if err := migrate(ctx, db, "forward_schema", forwardMigrations); err != nil {
		return nil, fmt.Errorf("forward rules: %w", err)
	}
	return &ForwardRules{db: db}, nil
}

const forwardColumns = `id, name, protocol, listen, target, enabled, created_at, updated_at`

func scanRule(row interface{ Scan(...any) error }) (forward.Rule, error) {
	var r forward.Rule
	var created, updated int64
	err := row.Scan(&r.ID, &r.Name, &r.Protocol, &r.Listen, &r.Target, &r.Enabled, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return r, store.ErrNotFound
	}
	r.CreatedAt, r.UpdatedAt = time.Unix(0, created).UTC(), time.Unix(0, updated).UTC()
	return r, err
}

// List returns every rule by name.
func (s *ForwardRules) List(ctx context.Context) ([]forward.Rule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+forwardColumns+` FROM forward_rules ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := []forward.Rule{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// Get returns one rule.
func (s *ForwardRules) Get(ctx context.Context, id int64) (forward.Rule, error) {
	return scanRule(s.db.QueryRowContext(ctx, `SELECT `+forwardColumns+` FROM forward_rules WHERE id = ?`, id))
}

// Create inserts r and sets its ID and timestamps.
func (s *ForwardRules) Create(ctx context.Context, r *forward.Rule) error {
	next := *r
	if err := next.Normalize(); err != nil {
		return err
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `INSERT INTO forward_rules (name, protocol, listen, target, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, next.Name, next.Protocol, next.Listen, next.Target, next.Enabled, now.UnixNano(), now.UnixNano())
	if err != nil {
		return forwardConflict(err)
	}
	if next.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	next.CreatedAt, next.UpdatedAt = now, now
	*r = next
	return nil
}

// Update replaces r's fields, keeping its creation time.
func (s *ForwardRules) Update(ctx context.Context, r *forward.Rule) error {
	next := *r
	if err := next.Normalize(); err != nil {
		return err
	}
	old, err := s.Get(ctx, next.ID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `UPDATE forward_rules SET name = ?, protocol = ?, listen = ?, target = ?, enabled = ?, updated_at = ?
		WHERE id = ?`, next.Name, next.Protocol, next.Listen, next.Target, next.Enabled, now.UnixNano(), next.ID)
	if err != nil {
		return forwardConflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	next.CreatedAt, next.UpdatedAt = old.CreatedAt, now
	*r = next
	return nil
}

// Delete removes a rule; removing a missing rule succeeds.
func (s *ForwardRules) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM forward_rules WHERE id = ?`, id)
	return err
}

func forwardConflict(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		if strings.Contains(err.Error(), "forward_rules.name") {
			return fmt.Errorf("%w: a rule with this name exists", store.ErrConflict)
		}
		return fmt.Errorf("%w: another rule already listens on this address", store.ErrConflict)
	}
	return err
}
