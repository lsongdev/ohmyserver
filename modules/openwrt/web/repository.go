package web

import (
	"context"
	"database/sql"
	openwrtmodule "github.com/lsongdev/ohmyserver/modules/openwrt"
	"time"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Initialize(ctx context.Context) error {
	_, e := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS openwrt_servers(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,host TEXT NOT NULL,port INTEGER DEFAULT 80,username TEXT NOT NULL,password TEXT NOT NULL,protocol TEXT NOT NULL DEFAULT 'http',status TEXT NOT NULL DEFAULT 'active',description TEXT DEFAULT '',created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);CREATE INDEX IF NOT EXISTS idx_openwrt_servers_name ON openwrt_servers(name);CREATE INDEX IF NOT EXISTS idx_openwrt_servers_host ON openwrt_servers(host);`)
	return e
}
func (r *Repository) Create(ctx context.Context, v *openwrtmodule.Server) error {
	if v.Port == 0 {
		v.Port = 80
	}
	if v.Protocol == "" {
		v.Protocol = "http"
	}
	now := time.Now()
	res, e := r.db.ExecContext(ctx, `INSERT INTO openwrt_servers(name,host,port,username,password,protocol,status,description,created_at,updated_at) VALUES(?,?,?,?,?,?,'active',?,?,?)`, v.Name, v.Host, v.Port, v.Username, v.Password, v.Protocol, v.Description, now, now)
	if e != nil {
		return e
	}
	v.ID, e = res.LastInsertId()
	v.Status = "active"
	v.CreatedAt = now
	v.UpdatedAt = now
	return e
}
func (r *Repository) List(ctx context.Context) ([]openwrtmodule.Server, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT id,name,host,port,username,password,protocol,status,description,created_at,updated_at FROM openwrt_servers ORDER BY created_at DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]openwrtmodule.Server, 0)
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}
func (r *Repository) Get(ctx context.Context, id int64) (*openwrtmodule.Server, error) {
	return scan(r.db.QueryRowContext(ctx, `SELECT id,name,host,port,username,password,protocol,status,description,created_at,updated_at FROM openwrt_servers WHERE id=?`, id))
}
func (r *Repository) Update(ctx context.Context, v *openwrtmodule.Server) error {
	_, e := r.db.ExecContext(ctx, `UPDATE openwrt_servers SET name=?,host=?,port=?,username=?,password=?,protocol=?,description=?,updated_at=? WHERE id=?`, v.Name, v.Host, v.Port, v.Username, v.Password, v.Protocol, v.Description, time.Now(), v.ID)
	return e
}
func (r *Repository) Delete(ctx context.Context, id int64) error {
	_, e := r.db.ExecContext(ctx, `DELETE FROM openwrt_servers WHERE id=?`, id)
	return e
}
func scan(row interface{ Scan(...any) error }) (*openwrtmodule.Server, error) {
	var v openwrtmodule.Server
	var created, updated string
	e := row.Scan(&v.ID, &v.Name, &v.Host, &v.Port, &v.Username, &v.Password, &v.Protocol, &v.Status, &v.Description, &created, &updated)
	v.CreatedAt = parseTime(created)
	v.UpdatedAt = parseTime(updated)
	return &v, e
}
func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05-07:00"} {
		if t, e := time.Parse(layout, s); e == nil {
			return t
		}
	}
	return time.Time{}
}
