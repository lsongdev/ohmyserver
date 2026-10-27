// Package metrics defines what monitor-go measures: a Reading is what a
// collector reads from a host at one moment (with cumulative counters), and
// a Sample is what two consecutive readings say about the time between
// them (usage percentages and rates).
package metrics

import "time"

// Reading is a host's state at one moment. Counters are cumulative since
// boot; Sample turns two of them into rates.
type Reading struct {
	Time time.Time
	// CPUBusy and CPUTotal are cumulative CPU time in any unit (seconds,
	// jiffies); only their ratio over an interval matters.
	CPUBusy, CPUTotal    float64
	Load1, Load5, Load15 float64
	MemTotal, MemUsed    uint64 // bytes; used excludes caches and buffers
	SwapTotal, SwapUsed  uint64
	Disks                []Disk
	DiskRead, DiskWrite  uint64 // cumulative bytes, physical disks only
	NetRecv, NetSent     uint64 // cumulative bytes, physical interfaces only
	Uptime               time.Duration
}

// Disk is one mounted file system.
type Disk struct {
	Mount  string `json:"mount"`
	Device string `json:"device"`
	FSType string `json:"fstype"`
	Total  uint64 `json:"total"`
	Used   uint64 `json:"used"`
}

// Info describes a host; it rarely changes.
type Info struct {
	Hostname  string   `json:"hostname"`
	OS        string   `json:"os"` // e.g. "Ubuntu 24.04.1 LTS", "macOS 15.1"
	Kernel    string   `json:"kernel"`
	Arch      string   `json:"arch"`
	CPUModel  string   `json:"cpu_model"`
	CPUCores  int      `json:"cpu_cores"`
	Addresses []string `json:"addresses,omitempty"`
}

// Sample is what a host did between two readings.
type Sample struct {
	Time      time.Time `json:"t"`
	CPU       float64   `json:"cpu"` // percent of all cores, 0-100
	Load1     float64   `json:"load1"`
	Load5     float64   `json:"load5"`
	Load15    float64   `json:"load15"`
	MemTotal  uint64    `json:"mem_total"`
	MemUsed   uint64    `json:"mem_used"`
	SwapTotal uint64    `json:"swap_total"`
	SwapUsed  uint64    `json:"swap_used"`
	DiskTotal uint64    `json:"disk_total"` // all file systems
	DiskUsed  uint64    `json:"disk_used"`
	DiskRead  float64   `json:"disk_read"`  // bytes per second
	DiskWrite float64   `json:"disk_write"` // bytes per second
	NetRecv   float64   `json:"net_recv"`   // bytes per second
	NetSent   float64   `json:"net_sent"`   // bytes per second
}

// Between returns the sample for the interval from prev to cur. Counters
// that went backwards (a reboot, a counter wrap) count as zero.
func Between(prev, cur Reading) Sample {
	s := Sample{
		Time: cur.Time, Load1: cur.Load1, Load5: cur.Load5, Load15: cur.Load15,
		MemTotal: cur.MemTotal, MemUsed: cur.MemUsed, SwapTotal: cur.SwapTotal, SwapUsed: cur.SwapUsed,
	}
	for _, d := range cur.Disks {
		s.DiskTotal += d.Total
		s.DiskUsed += d.Used
	}
	if total := cur.CPUTotal - prev.CPUTotal; total > 0 {
		busy := cur.CPUBusy - prev.CPUBusy
		s.CPU = clamp(busy/total*100, 0, 100)
	}
	if secs := cur.Time.Sub(prev.Time).Seconds(); secs > 0 {
		rate := func(a, b uint64) float64 {
			if b < a {
				return 0
			}
			return float64(b-a) / secs
		}
		s.DiskRead, s.DiskWrite = rate(prev.DiskRead, cur.DiskRead), rate(prev.DiskWrite, cur.DiskWrite)
		s.NetRecv, s.NetSent = rate(prev.NetRecv, cur.NetRecv), rate(prev.NetSent, cur.NetSent)
	}
	return s
}

func clamp(v, lo, hi float64) float64 { return max(lo, min(hi, v)) }
