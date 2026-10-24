// Package stores keeps module state in OhMyServer's shared database for
// modules that keep it in files when they run on their own. Each type
// implements an interface the module defines; tables are named after the
// module they belong to (docs/module-contract.md, Storage).
package stores

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lsongdev/docker-go/apps"
	"github.com/lsongdev/yabber"
)

// Open opens the shared database: WAL journaling so readers never block the
// writer, a busy timeout for concurrent writers, foreign keys, and
// immediate transactions so a transaction never fails upgrading its lock.
func Open(path string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate")
}

// migrate applies migrations recorded in the version table named schema.
func migrate(ctx context.Context, db *sql.DB, schema string, migrations []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+schema+` (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM `+schema).Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("%s: schema version %d is newer than this program (%d)", schema, version, len(migrations))
	}
	if version == len(migrations) {
		return nil
	}
	for i := version; i < len(migrations); i++ {
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			return fmt.Errorf("%s migration %d: %w", schema, i+1, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+schema); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO `+schema+` (version) VALUES (?)`, len(migrations)); err != nil {
		return err
	}
	return tx.Commit()
}

// DockerApps keeps docker-go's install records (apps.Store).
type DockerApps struct{ db *sql.DB }

var _ apps.Store = (*DockerApps)(nil)

// NewDockerApps migrates the docker_ tables and returns the store.
func NewDockerApps(ctx context.Context, db *sql.DB) (*DockerApps, error) {
	err := migrate(ctx, db, "docker_schema", []string{`CREATE TABLE docker_apps (
		id           TEXT PRIMARY KEY,
		version      TEXT NOT NULL,
		port         INTEGER NOT NULL,
		domain       TEXT NOT NULL DEFAULT '',
		settings     TEXT NOT NULL DEFAULT '{}', -- form values as a JSON object
		installed_at INTEGER NOT NULL,           -- unix nanoseconds
		updated_at   INTEGER NOT NULL
	)`})
	if err != nil {
		return nil, err
	}
	return &DockerApps{db: db}, nil
}

const appColumns = `id, version, port, domain, settings, installed_at, updated_at`

func scanApp(row interface{ Scan(...any) error }) (*apps.Record, error) {
	var r apps.Record
	var settings string
	var installed, updated int64
	if err := row.Scan(&r.ID, &r.Version, &r.Port, &r.Domain, &settings, &installed, &updated); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(settings), &r.Values); err != nil {
		return nil, fmt.Errorf("app %s settings: %w", r.ID, err)
	}
	r.InstalledAt, r.UpdatedAt = time.Unix(0, installed).UTC(), time.Unix(0, updated).UTC()
	return &r, nil
}

func (s *DockerApps) ListApps(ctx context.Context) ([]apps.Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+appColumns+` FROM docker_apps ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []apps.Record
	for rows.Next() {
		r, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (s *DockerApps) GetApp(ctx context.Context, id string) (*apps.Record, error) {
	r, err := scanApp(s.db.QueryRowContext(ctx, `SELECT `+appColumns+` FROM docker_apps WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", apps.ErrNotInstalled, id)
	}
	return r, err
}

func (s *DockerApps) PutApp(ctx context.Context, r apps.Record) error {
	settings := []byte("{}")
	if r.Values != nil {
		var err error
		if settings, err = json.Marshal(r.Values); err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO docker_apps (`+appColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET version = excluded.version, port = excluded.port, domain = excluded.domain,
			settings = excluded.settings, installed_at = excluded.installed_at, updated_at = excluded.updated_at`,
		r.ID, r.Version, r.Port, r.Domain, string(settings), r.InstalledAt.UnixNano(), r.UpdatedAt.UnixNano())
	return err
}

func (s *DockerApps) DeleteApp(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM docker_apps WHERE id = ?`, id)
	return err
}

// GatewayConfig keeps the yabber gateway's configuration, which yabber
// keeps in a YAML file when it runs on its own.
type GatewayConfig struct{ db *sql.DB }

// NewGatewayConfig migrates the yabber_ tables and returns the store.
func NewGatewayConfig(ctx context.Context, db *sql.DB) (*GatewayConfig, error) {
	err := migrate(ctx, db, "yabber_schema", []string{`CREATE TABLE yabber_config (
		id         INTEGER PRIMARY KEY CHECK (id = 1), -- a single configuration
		config     TEXT NOT NULL,                      -- yabber.Config as JSON
		updated_at INTEGER NOT NULL
	)`})
	if err != nil {
		return nil, err
	}
	return &GatewayConfig{db: db}, nil
}

// Load returns the saved configuration, or ok=false if none was saved yet.
func (s *GatewayConfig) Load(ctx context.Context) (config yabber.Config, ok bool, err error) {
	var data string
	err = s.db.QueryRowContext(ctx, `SELECT config FROM yabber_config WHERE id = 1`).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return config, false, nil
	}
	if err != nil {
		return config, false, err
	}
	if err := json.Unmarshal([]byte(data), &config); err != nil {
		return config, false, fmt.Errorf("gateway configuration: %w", err)
	}
	return config, true, nil
}

// Save stores config; it matches yabber/api.Options.Save.
func (s *GatewayConfig) Save(config yabber.Config) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO yabber_config (id, config, updated_at) VALUES (1, ?, ?)
		ON CONFLICT (id) DO UPDATE SET config = excluded.config, updated_at = excluded.updated_at`, string(data), time.Now().UnixNano())
	return err
}
