package uptime

import "time"

type MonitorType string

const (
	Ping MonitorType = "ping"
	TCP  MonitorType = "tcp"
	HTTP MonitorType = "http"
)

type Monitor struct {
	ID             int64       `json:"id"`
	Name           string      `json:"name"`
	Type           MonitorType `json:"type"`
	Target         string      `json:"target"`
	Port           int         `json:"port"`
	Interval       int         `json:"interval"`
	Timeout        int         `json:"timeout"`
	ExpectedStatus int         `json:"expected_status"`
	Enabled        bool        `json:"enabled"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

type Check struct {
	ID           int64     `json:"id"`
	MonitorID    int64     `json:"monitor_id"`
	Status       string    `json:"status"`
	ResponseTime int64     `json:"response_time"`
	StatusCode   int       `json:"status_code"`
	Message      string    `json:"message"`
	CheckedAt    time.Time `json:"checked_at"`
}

type WithStatus struct {
	Monitor
	LastCheck *Check  `json:"last_check"`
	UptimePct float64 `json:"uptime_pct"`
}
