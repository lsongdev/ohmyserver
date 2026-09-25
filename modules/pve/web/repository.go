package web

import (
	"context"
	"database/sql"
	pvemodule "github.com/lsongdev/ohmyserver/modules/pve"
	"time"
)

type Server = pvemodule.Server
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Initialize(ctx context.Context) error {
	_, e := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS pve_servers(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,host TEXT NOT NULL,port INTEGER NOT NULL DEFAULT 8006,username TEXT NOT NULL,password TEXT NOT NULL,insecure BOOLEAN NOT NULL DEFAULT 1,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);CREATE INDEX IF NOT EXISTS idx_pve_servers_name ON pve_servers(name);`)
	return e
}
func (r *Repository) Create(ctx context.Context, v *Server) error {
	if v.Port == 0 {
		v.Port = 8006
	}
	now := time.Now()
	res, e := r.db.ExecContext(ctx, `INSERT INTO pve_servers(name,host,port,username,password,insecure,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, v.Name, v.Host, v.Port, v.Username, v.Password, v.Insecure, now, now)
	if e != nil {
		return e
	}
	v.ID, e = res.LastInsertId()
	v.CreatedAt = now
	v.UpdatedAt = now
	return e
}
func (r *Repository) List(ctx context.Context) ([]Server, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT id,name,host,port,username,password,insecure,created_at,updated_at FROM pve_servers ORDER BY created_at DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]Server, 0)
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}
func (r *Repository) Get(ctx context.Context, id int64) (*Server, error) {
	return scan(r.db.QueryRowContext(ctx, `SELECT id,name,host,port,username,password,insecure,created_at,updated_at FROM pve_servers WHERE id=?`, id))
}
func (r *Repository) Update(ctx context.Context, v *Server) error {
	_, e := r.db.ExecContext(ctx, `UPDATE pve_servers SET name=?,host=?,port=?,username=?,password=?,insecure=?,updated_at=? WHERE id=?`, v.Name, v.Host, v.Port, v.Username, v.Password, v.Insecure, time.Now(), v.ID)
	return e
}
func (r *Repository) Delete(ctx context.Context, id int64) error {
	_, e := r.db.ExecContext(ctx, `DELETE FROM pve_servers WHERE id=?`, id)
	return e
}
func scan(row interface{ Scan(...any) error }) (*Server, error) {
	var v Server
	var created, updated string
	e := row.Scan(&v.ID, &v.Name, &v.Host, &v.Port, &v.Username, &v.Password, &v.Insecure, &created, &updated)
	v.CreatedAt = parseTime(created)
	v.UpdatedAt = parseTime(updated)
	return &v, e
}
func parseTime(v string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05-07:00"} {
		if t, e := time.Parse(layout, v); e == nil {
			return t
		}
	}
	return time.Time{}
}
