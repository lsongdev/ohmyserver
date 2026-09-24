package monitor

import (
	"context"
	"time"

	"github.com/lsongdev/ohmyserver/modules/monitor/sysinfo"
)

type LocalCollector struct{}

func (LocalCollector) CollectMonitorStats(ctx context.Context) (*Stats, error) {
	return CollectLocalStats(ctx)
}
func CollectLocalStats(ctx context.Context) (*Stats, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m, err := sysinfo.Collect()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Stats{Hostname: m.Hostname, Timestamp: time.Now(), CPUUsage: m.CPUUsage, CPUCores: m.CPUCores, MemoryTotal: m.MemoryTotal, MemoryUsed: m.MemoryUsed, MemoryFree: m.MemoryFree, MemoryUsage: m.MemoryUsage, DiskTotal: m.DiskTotal, DiskUsed: m.DiskUsed, DiskFree: m.DiskFree, DiskUsage: m.DiskUsage, NetBytesSent: m.NetBytesSent, NetBytesRecv: m.NetBytesRecv, Uptime: m.Uptime, Load1: m.Load1, Load5: m.Load5, Load15: m.Load15}, nil
}
