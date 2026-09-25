// Package dashboard exposes the panel's small read-only overview composition.
package dashboard

import (
	"context"
)

type Widget struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Config    string `json:"config"`
	SortOrder int    `json:"sort_order"`
}
type SystemSummary struct {
	Hostname    string  `json:"hostname"`
	Timestamp   string  `json:"timestamp"`
	CPUUsage    float64 `json:"cpu_usage"`
	CPUCores    int     `json:"cpu_cores"`
	MemoryTotal uint64  `json:"memory_total"`
	MemoryUsed  uint64  `json:"memory_used"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskTotal   uint64  `json:"disk_total"`
	DiskUsed    uint64  `json:"disk_used"`
	DiskUsage   float64 `json:"disk_usage"`
	Uptime      uint64  `json:"uptime_seconds"`
	Load1       float64 `json:"load_1"`
	Load5       float64 `json:"load_5"`
	Load15      float64 `json:"load_15"`
}
type Overview struct {
	Widgets []Widget      `json:"widgets"`
	System  SystemSummary `json:"system"`
}

type Service interface {
	Overview(context.Context) (Overview, error)
}
