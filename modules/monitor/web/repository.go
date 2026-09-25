package web

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Initialize(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS system_stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT, hostname TEXT NOT NULL DEFAULT '', timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		cpu_usage REAL NOT NULL DEFAULT 0, cpu_cores INTEGER NOT NULL DEFAULT 0,
		memory_total INTEGER NOT NULL DEFAULT 0, memory_used INTEGER NOT NULL DEFAULT 0,
		memory_free INTEGER NOT NULL DEFAULT 0, memory_usage REAL NOT NULL DEFAULT 0,
		disk_total INTEGER NOT NULL DEFAULT 0, disk_used INTEGER NOT NULL DEFAULT 0,
		disk_free INTEGER NOT NULL DEFAULT 0, disk_usage REAL NOT NULL DEFAULT 0,
		net_bytes_sent INTEGER NOT NULL DEFAULT 0, net_bytes_recv INTEGER NOT NULL DEFAULT 0,
		uptime INTEGER NOT NULL DEFAULT 0, load_1 REAL NOT NULL DEFAULT 0,
		load_5 REAL NOT NULL DEFAULT 0, load_15 REAL NOT NULL DEFAULT 0)`)
	if err != nil {
		return err
	}
	columns := []struct{ name, definition string }{
		{"hostname", `TEXT NOT NULL DEFAULT ''`}, {"cpu_cores", `INTEGER NOT NULL DEFAULT 0`},
		{"disk_total", `INTEGER NOT NULL DEFAULT 0`}, {"disk_used", `INTEGER NOT NULL DEFAULT 0`}, {"disk_free", `INTEGER NOT NULL DEFAULT 0`}, {"disk_usage", `REAL NOT NULL DEFAULT 0`},
		{"net_bytes_sent", `INTEGER NOT NULL DEFAULT 0`}, {"net_bytes_recv", `INTEGER NOT NULL DEFAULT 0`}, {"uptime", `INTEGER NOT NULL DEFAULT 0`},
		{"load_1", `REAL NOT NULL DEFAULT 0`}, {"load_5", `REAL NOT NULL DEFAULT 0`}, {"load_15", `REAL NOT NULL DEFAULT 0`},
	}
	for _, column := range columns {
		var count int
		if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('system_stats') WHERE name=?`, column.name).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE system_stats ADD COLUMN `+column.name+` `+column.definition); err != nil {
				return fmt.Errorf("add system_stats.%s: %w", column.name, err)
			}
		}
	}
	_, err = r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_system_stats_timestamp ON system_stats(timestamp); CREATE INDEX IF NOT EXISTS idx_system_stats_host_timestamp ON system_stats(hostname,timestamp)`)
	return err
}

const statsColumns = `id,COALESCE(hostname,''),timestamp,COALESCE(cpu_usage,0),COALESCE(cpu_cores,0),COALESCE(memory_total,0),COALESCE(memory_used,0),COALESCE(memory_free,0),COALESCE(memory_usage,0),COALESCE(disk_total,0),COALESCE(disk_used,0),COALESCE(disk_free,0),COALESCE(disk_usage,0),COALESCE(net_bytes_sent,0),COALESCE(net_bytes_recv,0),COALESCE(uptime,0),COALESCE(load_1,0),COALESCE(load_5,0),COALESCE(load_15,0)`

func (r *Repository) Insert(ctx context.Context, s *Stats) error {
	if s.Timestamp.IsZero() {
		s.Timestamp = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO system_stats(hostname,timestamp,cpu_usage,cpu_cores,memory_total,memory_used,memory_free,memory_usage,disk_total,disk_used,disk_free,disk_usage,net_bytes_sent,net_bytes_recv,uptime,load_1,load_5,load_15) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, s.Hostname, s.Timestamp, s.CPUUsage, s.CPUCores, s.MemoryTotal, s.MemoryUsed, s.MemoryFree, s.MemoryUsage, s.DiskTotal, s.DiskUsed, s.DiskFree, s.DiskUsage, s.NetBytesSent, s.NetBytesRecv, s.Uptime, s.Load1, s.Load5, s.Load15)
	return err
}
func (r *Repository) Recent(ctx context.Context, hostname string, duration time.Duration) ([]Stats, error) {
	since := time.Now().Add(-duration)
	query := `SELECT ` + statsColumns + ` FROM system_stats WHERE timestamp>=?`
	args := []any{since}
	if hostname != "" {
		query += ` AND hostname=?`
		args = append(args, hostname)
	}
	query += ` ORDER BY timestamp ASC`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStats(rows)
}

// RecentSampled returns at most about 1200 evenly grouped time buckets so long
// history ranges remain practical to render in a browser. Gauge values are
// averaged; cumulative counters and uptime use their bucket maximum.
func (r *Repository) RecentSampled(ctx context.Context, hostname string, duration time.Duration) ([]Stats, int64, error) {
	step := int64((duration.Seconds() + 1199) / 1200)
	if step < 1 {
		step = 1
	}
	query := `SELECT MAX(id),COALESCE(hostname,''),MAX(timestamp),AVG(cpu_usage),MAX(cpu_cores),CAST(AVG(memory_total) AS INTEGER),CAST(AVG(memory_used) AS INTEGER),CAST(AVG(memory_free) AS INTEGER),AVG(memory_usage),CAST(AVG(disk_total) AS INTEGER),CAST(AVG(disk_used) AS INTEGER),CAST(AVG(disk_free) AS INTEGER),AVG(disk_usage),MAX(net_bytes_sent),MAX(net_bytes_recv),MAX(uptime),AVG(load_1),AVG(load_5),AVG(load_15) FROM system_stats WHERE timestamp>=?`
	args := []any{time.Now().Add(-duration)}
	if hostname != "" {
		query += ` AND hostname=?`
		args = append(args, hostname)
	}
	query += ` GROUP BY CAST(strftime('%s',timestamp) AS INTEGER)/? ORDER BY MAX(timestamp) ASC`
	args = append(args, step)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, step, err
	}
	defer rows.Close()
	items, err := scanStats(rows)
	return items, step, err
}
func (r *Repository) LatestForHost(ctx context.Context, hostname string) (*Stats, error) {
	return scanStat(r.db.QueryRowContext(ctx, `SELECT `+statsColumns+` FROM system_stats WHERE hostname=? ORDER BY timestamp DESC,id DESC LIMIT 1`, hostname))
}
func (r *Repository) LatestForAllHosts(ctx context.Context) ([]Stats, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+statsColumns+` FROM system_stats s WHERE s.hostname<>'' AND s.id=(SELECT s2.id FROM system_stats s2 WHERE s2.hostname=s.hostname ORDER BY s2.timestamp DESC,s2.id DESC LIMIT 1) ORDER BY s.hostname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStats(rows)
}
func (r *Repository) Hosts(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT hostname FROM system_stats WHERE hostname<>'' ORDER BY hostname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var host string
		if err := rows.Scan(&host); err != nil {
			return nil, err
		}
		out = append(out, host)
	}
	return out, rows.Err()
}
func (r *Repository) PruneBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM system_stats WHERE timestamp<?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type scanner interface{ Scan(...any) error }

func scanStats(rows *sql.Rows) ([]Stats, error) {
	out := []Stats{}
	for rows.Next() {
		v, err := scanStat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}
func scanStat(row scanner) (*Stats, error) {
	var v Stats
	var stamp string
	err := row.Scan(&v.ID, &v.Hostname, &stamp, &v.CPUUsage, &v.CPUCores, &v.MemoryTotal, &v.MemoryUsed, &v.MemoryFree, &v.MemoryUsage, &v.DiskTotal, &v.DiskUsed, &v.DiskFree, &v.DiskUsage, &v.NetBytesSent, &v.NetBytesRecv, &v.Uptime, &v.Load1, &v.Load5, &v.Load15)
	if err != nil {
		return nil, err
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05-07:00"} {
		if t, e := time.Parse(layout, stamp); e == nil {
			v.Timestamp = t
			break
		}
	}
	return &v, nil
}
