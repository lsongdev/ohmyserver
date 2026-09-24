package monitor

import (
	"context"
	"time"
)

type Stats struct {
	ID           int64     `json:"id"`
	Hostname     string    `json:"hostname"`
	Timestamp    time.Time `json:"timestamp"`
	CPUUsage     float64   `json:"cpu_usage"`
	CPUCores     int       `json:"cpu_cores"`
	MemoryTotal  uint64    `json:"memory_total"`
	MemoryUsed   uint64    `json:"memory_used"`
	MemoryFree   uint64    `json:"memory_free"`
	MemoryUsage  float64   `json:"memory_usage"`
	DiskTotal    uint64    `json:"disk_total"`
	DiskUsed     uint64    `json:"disk_used"`
	DiskFree     uint64    `json:"disk_free"`
	DiskUsage    float64   `json:"disk_usage"`
	NetBytesSent uint64    `json:"net_bytes_sent"`
	NetBytesRecv uint64    `json:"net_bytes_recv"`
	Uptime       uint64    `json:"uptime"`
	Load1        float64   `json:"load_1"`
	Load5        float64   `json:"load_5"`
	Load15       float64   `json:"load_15"`
}

type Collector interface {
	CollectMonitorStats(context.Context) (*Stats, error)
}
