package sysinfo

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// Metrics is the flat JSON format for system metrics
type Metrics struct {
	Hostname     string  `json:"hostname"`
	CPUUsage     float64 `json:"cpu_usage"`
	CPUCores     int     `json:"cpu_cores"`
	MemoryTotal  uint64  `json:"memory_total"`
	MemoryUsed   uint64  `json:"memory_used"`
	MemoryFree   uint64  `json:"memory_free"`
	MemoryUsage  float64 `json:"memory_usage"`
	DiskTotal    uint64  `json:"disk_total"`
	DiskUsed     uint64  `json:"disk_used"`
	DiskFree     uint64  `json:"disk_free"`
	DiskUsage    float64 `json:"disk_usage"`
	NetBytesSent uint64  `json:"net_bytes_sent"`
	NetBytesRecv uint64  `json:"net_bytes_recv"`
	Uptime       uint64  `json:"uptime"`
	Load1        float64 `json:"load_1"`
	Load5        float64 `json:"load_5"`
	Load15       float64 `json:"load_15"`
	Token        string  `json:"token,omitempty"`
}

// ParseMetrics decodes a JSON stream into Metrics
func ParseMetrics(r io.Reader) (*Metrics, error) {
	var m Metrics
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	if m.Hostname == "" {
		return nil, fmt.Errorf("hostname is required")
	}
	return &m, nil
}

// Collect gathers local system metrics using gopsutil.
// This is the shared core used by both the built-in server agent and the standalone agent.
func Collect() (*Metrics, error) {
	hostInfo, _ := host.Info()
	hostname := "unknown"
	uptime := uint64(0)
	if hostInfo != nil {
		hostname = hostInfo.Hostname
		uptime = hostInfo.Uptime
	}

	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil {
		return nil, fmt.Errorf("cpu: %w", err)
	}
	cpuUsage := 0.0
	if len(cpuPercent) > 0 {
		cpuUsage = cpuPercent[0]
	}

	cpuInfo, _ := cpu.Info()
	cpuCores := runtime.NumCPU()
	if len(cpuInfo) > 0 && cpuInfo[0].Cores > 0 {
		cpuCores = int(cpuInfo[0].Cores)
	}

	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}

	diskInfo, err := disk.Usage("/")
	if err != nil {
		return nil, fmt.Errorf("disk: %w", err)
	}

	netIO, err := net.IOCounters(false)
	netSent, netRecv := uint64(0), uint64(0)
	if err == nil && len(netIO) > 0 {
		netSent = netIO[0].BytesSent
		netRecv = netIO[0].BytesRecv
	}

	loadAvg, _ := load.Avg()
	load1, load5, load15 := 0.0, 0.0, 0.0
	if loadAvg != nil {
		load1 = loadAvg.Load1
		load5 = loadAvg.Load5
		load15 = loadAvg.Load15
	}

	return &Metrics{
		Hostname:     hostname,
		CPUUsage:     cpuUsage,
		CPUCores:     cpuCores,
		MemoryTotal:  memInfo.Total,
		MemoryUsed:   memInfo.Used,
		MemoryFree:   memInfo.Free,
		MemoryUsage:  memInfo.UsedPercent,
		DiskTotal:    diskInfo.Total,
		DiskUsed:     diskInfo.Used,
		DiskFree:     diskInfo.Free,
		DiskUsage:    diskInfo.UsedPercent,
		NetBytesSent: netSent,
		NetBytesRecv: netRecv,
		Uptime:       uptime,
		Load1:        load1,
		Load5:        load5,
		Load15:       load15,
	}, nil
}
