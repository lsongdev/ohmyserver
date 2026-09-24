package web

import (
	"context"
	"database/sql"
	"time"

	yeelight "github.com/lsongdev/ohmyserver/modules/yeelight"
)

type Device = yeelight.Device
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Initialize(ctx context.Context) error {
	_, e := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS yeelight_devices(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,host TEXT NOT NULL,port INTEGER DEFAULT 55443,status TEXT NOT NULL DEFAULT 'offline',last_seen DATETIME,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);CREATE INDEX IF NOT EXISTS idx_yeelight_devices_name ON yeelight_devices(name);CREATE INDEX IF NOT EXISTS idx_yeelight_devices_host ON yeelight_devices(host);`)
	return e
}
func (r *Repository) Create(ctx context.Context, v *Device) error {
	if v.Port == 0 {
		v.Port = 55443
	}
	now := time.Now()
	res, e := r.db.ExecContext(ctx, `INSERT INTO yeelight_devices(name,host,port,status,created_at,updated_at) VALUES(?,?,?,'offline',?,?)`, v.Name, v.Host, v.Port, now, now)
	if e != nil {
		return e
	}
	v.ID, e = res.LastInsertId()
	v.Status = "offline"
	v.CreatedAt = now
	v.UpdatedAt = now
	return e
}
func (r *Repository) List(ctx context.Context) ([]Device, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT id,name,host,port,status,last_seen,created_at,updated_at FROM yeelight_devices ORDER BY created_at DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]Device, 0)
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}
func (r *Repository) Get(ctx context.Context, id int64) (*Device, error) {
	return scan(r.db.QueryRowContext(ctx, `SELECT id,name,host,port,status,last_seen,created_at,updated_at FROM yeelight_devices WHERE id=?`, id))
}
func (r *Repository) Update(ctx context.Context, v *Device) error {
	_, e := r.db.ExecContext(ctx, `UPDATE yeelight_devices SET name=?,host=?,port=?,updated_at=? WHERE id=?`, v.Name, v.Host, v.Port, time.Now(), v.ID)
	return e
}
func (r *Repository) Delete(ctx context.Context, id int64) error {
	_, e := r.db.ExecContext(ctx, `DELETE FROM yeelight_devices WHERE id=?`, id)
	return e
}
func (r *Repository) SetStatus(ctx context.Context, id int64, online bool) error {
	status := "offline"
	if online {
		status = "online"
	}
	_, e := r.db.ExecContext(ctx, `UPDATE yeelight_devices SET status=?,last_seen=?,updated_at=? WHERE id=?`, status, time.Now(), time.Now(), id)
	return e
}
func scan(row interface{ Scan(...any) error }) (*Device, error) {
	var v Device
	var seen sql.NullString
	var created, updated string
	e := row.Scan(&v.ID, &v.Name, &v.Host, &v.Port, &v.Status, &seen, &created, &updated)
	if seen.Valid {
		v.LastSeen = parseTime(seen.String)
	}
	v.CreatedAt = parseTime(created)
	v.UpdatedAt = parseTime(updated)
	return &v, e
}
func parseTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05-07:00"} {
		if t, e := time.Parse(layout, value); e == nil {
			return t
		}
	}
	return time.Time{}
}
