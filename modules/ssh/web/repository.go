package web

import (
	"context"
	"database/sql"
	"errors"
	sshmodule "github.com/lsongdev/ohmyserver/modules/ssh"
	"time"
)

type Profile = sshmodule.Profile

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Initialize(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS ssh_servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		host TEXT NOT NULL,
		port INTEGER DEFAULT 22,
		username TEXT NOT NULL,
		auth_type TEXT NOT NULL DEFAULT 'password',
		password TEXT,
		private_key TEXT,
		host_key TEXT DEFAULT '',
		description TEXT,
		enabled INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_ssh_servers_enabled ON ssh_servers(enabled);
	`)
	if err != nil {
		return err
	}
	// Older installations predate persisted host-key pinning.
	rows, err := r.db.QueryContext(ctx, `PRAGMA table_info(ssh_servers)`)
	if err != nil {
		return err
	}
	hasHostKey := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "host_key" {
			hasHostKey = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !hasHostKey {
		if _, err := r.db.ExecContext(ctx, `ALTER TABLE ssh_servers ADD COLUMN host_key TEXT DEFAULT ''`); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) List(ctx context.Context) ([]Profile, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,host,port,username,auth_type,password,private_key,host_key,description,enabled,created_at,updated_at FROM ssh_servers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Profile, 0)
	for rows.Next() {
		item, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id int64) (Profile, error) {
	item, err := scanProfile(r.db.QueryRowContext(ctx, `SELECT id,name,host,port,username,auth_type,password,private_key,host_key,description,enabled,created_at,updated_at FROM ssh_servers WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, sshmodule.ErrNotFound
	}
	return item, err
}

type scanner interface{ Scan(...any) error }

func scanProfile(row scanner) (Profile, error) {
	var p Profile
	var enabled int
	err := row.Scan(&p.ID, &p.Name, &p.Host, &p.Port, &p.Username, &p.AuthType, &p.Password, &p.PrivateKey, &p.HostKey, &p.Description, &enabled, &p.CreatedAt, &p.UpdatedAt)
	p.Enabled = enabled == 1
	return p, err
}

func (r *Repository) Create(ctx context.Context, p *Profile) error {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `INSERT INTO ssh_servers(name,host,port,username,auth_type,password,private_key,host_key,description,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, p.Name, p.Host, p.Port, p.Username, p.AuthType, p.Password, p.PrivateKey, p.HostKey, p.Description, p.Enabled, now, now)
	if err != nil {
		return err
	}
	p.ID, err = result.LastInsertId()
	p.CreatedAt, p.UpdatedAt = now, now
	return err
}

func (r *Repository) Update(ctx context.Context, p *Profile) error {
	result, err := r.db.ExecContext(ctx, `UPDATE ssh_servers SET name=?,host=?,port=?,username=?,auth_type=?,password=?,private_key=?,host_key=?,description=?,enabled=?,updated_at=? WHERE id=?`, p.Name, p.Host, p.Port, p.Username, p.AuthType, p.Password, p.PrivateKey, p.HostKey, p.Description, p.Enabled, time.Now(), p.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return sshmodule.ErrNotFound
	}
	p.UpdatedAt = time.Now()
	return err
}

func (r *Repository) SetHostKey(ctx context.Context, id int64, key string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ssh_servers SET host_key=?,updated_at=? WHERE id=?`, key, time.Now(), id)
	return err
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM ssh_servers WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return sshmodule.ErrNotFound
	}
	return err
}
