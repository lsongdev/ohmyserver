package web

import (
	"context"
	"database/sql"
	"time"
)

type Server struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	Port      int       `json:"port"`
	Insecure  bool      `json:"insecure"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Initialize(ctx context.Context) error {
	_, e := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS esxi_servers(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,host TEXT NOT NULL,username TEXT NOT NULL,password TEXT NOT NULL,port INTEGER NOT NULL DEFAULT 443,insecure BOOLEAN NOT NULL DEFAULT 1,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);CREATE INDEX IF NOT EXISTS idx_esxi_servers_name ON esxi_servers(name);`)
	return e
}
func (r *Repository) Create(ctx context.Context, v *Server) error {
	if v.Port == 0 {
		v.Port = 443
	}
	now := time.Now()
	res, e := r.db.ExecContext(ctx, `INSERT INTO esxi_servers(name,host,username,password,port,insecure,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, v.Name, v.Host, v.Username, v.Password, v.Port, v.Insecure, now, now)
	if e != nil {
		return e
	}
	v.ID, e = res.LastInsertId()
	v.CreatedAt = now
	v.UpdatedAt = now
	return e
}
func (r *Repository) List(ctx context.Context) ([]Server, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT id,name,host,username,password,port,insecure,created_at,updated_at FROM esxi_servers ORDER BY created_at DESC`)
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
	return scan(r.db.QueryRowContext(ctx, `SELECT id,name,host,username,password,port,insecure,created_at,updated_at FROM esxi_servers WHERE id=?`, id))
}
func (r *Repository) Update(ctx context.Context, v *Server) error {
	_, e := r.db.ExecContext(ctx, `UPDATE esxi_servers SET name=?,host=?,username=?,password=?,port=?,insecure=?,updated_at=? WHERE id=?`, v.Name, v.Host, v.Username, v.Password, v.Port, v.Insecure, time.Now(), v.ID)
	return e
}
func (r *Repository) Delete(ctx context.Context, id int64) error {
	_, e := r.db.ExecContext(ctx, `DELETE FROM esxi_servers WHERE id=?`, id)
	return e
}
func scan(row interface{ Scan(...any) error }) (*Server, error) {
	var v Server
	var created, updated string
	err := row.Scan(&v.ID, &v.Name, &v.Host, &v.Username, &v.Password, &v.Port, &v.Insecure, &created, &updated)
	v.CreatedAt, v.UpdatedAt = parseTime(created), parseTime(updated)
	return &v, err
}
func parseTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05-07:00"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
