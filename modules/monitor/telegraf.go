package monitor

import (
	telegraflib "github.com/lsongdev/ohmyserver/modules/monitor/telegraf"
	"time"
)

func StatsFromTelegraf(batch *telegraflib.Batch) *Stats {
	hostname := ""
	stats := &Stats{Timestamp: time.Now()}

	for _, m := range batch.Metrics {
		if h := m.Tags["host"]; h != "" {
			hostname = h
		}
		switch m.Name {
		case "cpu":
			if m.Tags["cpu"] != "" && m.Tags["cpu"] != "cpu-total" {
				continue
			}
			if v, ok := telegraflib.ToFloat64(m.Fields["usage_active"]); ok {
				stats.CPUUsage = v
			} else if v, ok := telegraflib.ToFloat64(m.Fields["usage_idle"]); ok {
				stats.CPUUsage = 100 - v
			}
		case "mem":
			if v, ok := telegraflib.ToFloat64(m.Fields["used_percent"]); ok {
				stats.MemoryUsage = v
			}
			if v, ok := telegraflib.ToUint64(m.Fields["total"]); ok {
				stats.MemoryTotal = v
			}
			if v, ok := telegraflib.ToUint64(m.Fields["used"]); ok {
				stats.MemoryUsed = v
			}
			if v, ok := telegraflib.ToUint64(m.Fields["free"]); ok {
				stats.MemoryFree = v
			}
		case "disk":
			if v, ok := telegraflib.ToFloat64(m.Fields["used_percent"]); ok {
				stats.DiskUsage = v
			}
			if v, ok := telegraflib.ToUint64(m.Fields["total"]); ok {
				stats.DiskTotal = v
			}
			if v, ok := telegraflib.ToUint64(m.Fields["used"]); ok {
				stats.DiskUsed = v
			}
			if v, ok := telegraflib.ToUint64(m.Fields["free"]); ok {
				stats.DiskFree = v
			}
		case "net":
			if m.Tags["interface"] == "" {
				continue
			}
			if v, ok := telegraflib.ToUint64(m.Fields["bytes_sent"]); ok {
				stats.NetBytesSent = v
			}
			if v, ok := telegraflib.ToUint64(m.Fields["bytes_recv"]); ok {
				stats.NetBytesRecv = v
			}
		case "system":
			if v, ok := telegraflib.ToUint64(m.Fields["uptime"]); ok {
				stats.Uptime = v
			}
			if v, ok := telegraflib.ToFloat64(m.Fields["load1"]); ok {
				stats.Load1 = v
			}
			if v, ok := telegraflib.ToFloat64(m.Fields["load5"]); ok {
				stats.Load5 = v
			}
			if v, ok := telegraflib.ToFloat64(m.Fields["load15"]); ok {
				stats.Load15 = v
			}
		}
	}

	if hostname == "" {
		return nil
	}
	stats.Hostname = hostname
	return stats
}
