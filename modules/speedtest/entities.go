package speedtest

import "time"

type Result struct {
	ID         int64     `json:"id"`
	Type       string    `json:"type"`
	ServerID   string    `json:"server_id"`
	ServerName string    `json:"server_name"`
	ServerHost string    `json:"server_host"`
	Download   float64   `json:"download"`
	Upload     float64   `json:"upload"`
	Ping       float64   `json:"ping"`
	Jitter     float64   `json:"jitter"`
	IP         string    `json:"ip"`
	ISP        string    `json:"isp"`
	Country    string    `json:"country"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
type ServerInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
}
type Progress struct {
	Stage    string  `json:"stage"`
	Progress float64 `json:"progress"`
	Download float64 `json:"download"`
	Upload   float64 `json:"upload"`
	Ping     float64 `json:"ping"`
	Jitter   float64 `json:"jitter"`
	Message  string  `json:"message"`
}
type RunRequest struct {
	Type     string `json:"type"`
	ServerID string `json:"server_id"`
}
