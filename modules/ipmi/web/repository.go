package web

import (
	"context"
	"database/sql"
	ipmi "github.com/lsongdev/ohmyserver/modules/ipmi"
	"time"
)

type Server = ipmi.Server

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Initialize(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS ipmi_servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE, bmc_ip TEXT NOT NULL,
		port INTEGER NOT NULL DEFAULT 623, username TEXT NOT NULL, password TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active', description TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	); CREATE INDEX IF NOT EXISTS idx_ipmi_servers_name ON ipmi_servers(name); CREATE INDEX IF NOT EXISTS idx_ipmi_servers_bmc_ip ON ipmi_servers(bmc_ip);`)
	return err
}

func (r *Repository) Create(ctx context.Context, item *Server) error {
	if item.Port == 0 {
		item.Port = 623
	}
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `INSERT INTO ipmi_servers(name,bmc_ip,port,username,password,status,description,created_at,updated_at) VALUES(?,?,?,?,?,'active',?,?,?)`, item.Name, item.BMCIP, item.Port, item.Username, item.Password, item.Description, now, now)
	if err != nil {
		return err
	}
	item.ID, err = result.LastInsertId()
	item.Status = "active"
	item.CreatedAt, item.UpdatedAt = now, now
	return err
}
func (r *Repository) List(ctx context.Context) ([]Server, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,bmc_ip,port,username,password,status,description,created_at,updated_at FROM ipmi_servers ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Server, 0)
	for rows.Next() {
		item, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}
func (r *Repository) Get(ctx context.Context, id int64) (*Server, error) {
	return scanServer(r.db.QueryRowContext(ctx, `SELECT id,name,bmc_ip,port,username,password,status,description,created_at,updated_at FROM ipmi_servers WHERE id=?`, id))
}
func (r *Repository) Update(ctx context.Context, item *Server) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ipmi_servers SET name=?,bmc_ip=?,port=?,username=?,password=?,description=?,updated_at=? WHERE id=?`, item.Name, item.BMCIP, item.Port, item.Username, item.Password, item.Description, time.Now(), item.ID)
	return err
}
func (r *Repository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM ipmi_servers WHERE id=?`, id)
	return err
}
func scanServer(row interface{ Scan(...any) error }) (*Server, error) {
	var item Server
	var created, updated string
	err := row.Scan(&item.ID, &item.Name, &item.BMCIP, &item.Port, &item.Username, &item.Password, &item.Status, &item.Description, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return &item, nil
}
func parseTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05-07:00"} {
		if value, err := time.Parse(layout, value); err == nil {
			return value
		}
	}
	return time.Time{}
}
