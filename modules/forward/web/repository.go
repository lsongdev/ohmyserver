package web

import (
	"context"
	"database/sql"
	"time"

	forward "github.com/lsongdev/ohmyserver/modules/forward"
)

type Listener = forward.Listener

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Initialize(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS tcp_listeners(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,protocol TEXT NOT NULL DEFAULT 'tcp',listen_port INTEGER NOT NULL UNIQUE,forward_host TEXT NOT NULL DEFAULT '127.0.0.1',forward_port INTEGER NOT NULL,status TEXT NOT NULL DEFAULT 'active',created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);CREATE INDEX IF NOT EXISTS idx_tcp_listeners_name ON tcp_listeners(name);CREATE INDEX IF NOT EXISTS idx_tcp_listeners_port ON tcp_listeners(listen_port);`)
	return err
}

const columns = `id,name,protocol,listen_port,forward_host,forward_port,status,created_at,updated_at`

func scan(row interface{ Scan(...any) error }) (*Listener, error) {
	var x Listener
	var created, updated string
	err := row.Scan(&x.ID, &x.Name, &x.Protocol, &x.ListenPort, &x.ForwardHost, &x.ForwardPort, &x.Status, &created, &updated)
	if err != nil {
		return nil, err
	}
	x.CreatedAt = parseTime(created)
	x.UpdatedAt = parseTime(updated)
	return &x, nil
}
func parseTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999-07:00", "2006-01-02 15:04:05-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}
func (r *Repository) List(ctx context.Context) ([]Listener, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+columns+` FROM tcp_listeners ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Listener{}
	for rows.Next() {
		x, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *x)
	}
	return items, rows.Err()
}
func (r *Repository) Get(ctx context.Context, id int64) (*Listener, error) {
	return scan(r.db.QueryRowContext(ctx, `SELECT `+columns+` FROM tcp_listeners WHERE id=?`, id))
}
func (r *Repository) ByPort(ctx context.Context, port int) (*Listener, error) {
	return scan(r.db.QueryRowContext(ctx, `SELECT `+columns+` FROM tcp_listeners WHERE listen_port=?`, port))
}
func (r *Repository) Create(ctx context.Context, x *Listener) error {
	if x.ForwardHost == "" {
		x.ForwardHost = "127.0.0.1"
	}
	now := time.Now()
	res, err := r.db.ExecContext(ctx, `INSERT INTO tcp_listeners(name,protocol,listen_port,forward_host,forward_port,status,created_at,updated_at) VALUES(?,?,?,?,?,'active',?,?)`, x.Name, x.Protocol, x.ListenPort, x.ForwardHost, x.ForwardPort, now, now)
	if err != nil {
		return err
	}
	x.ID, err = res.LastInsertId()
	if err == nil {
		x.Status = "active"
		x.CreatedAt = now
		x.UpdatedAt = now
	}
	return err
}
func (r *Repository) Update(ctx context.Context, x Listener) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `UPDATE tcp_listeners SET name=?,protocol=?,listen_port=?,forward_host=?,forward_port=?,updated_at=? WHERE id=?`, x.Name, x.Protocol, x.ListenPort, x.ForwardHost, x.ForwardPort, now, x.ID)
	return err
}
func (r *Repository) SetStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE tcp_listeners SET status=?,updated_at=? WHERE id=?`, status, time.Now(), id)
	return err
}
func (r *Repository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM tcp_listeners WHERE id=?`, id)
	return err
}
