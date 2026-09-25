package host

import (
	"context"
	"time"

	monitormodule "github.com/lsongdev/ohmyserver/modules/monitor"
	monitorweb "github.com/lsongdev/ohmyserver/modules/monitor/web"
	sshweb "github.com/lsongdev/ohmyserver/modules/ssh/web"
)

type SystemStats = monitormodule.Stats

func (s *Server) MonitorRepository() *monitorweb.Repository {
	return monitorweb.NewRepository(s.db)
}

func (s *Server) initMonitorSchema() error {
	return s.MonitorRepository().Initialize(context.Background())
}

type sshMonitorCollector struct{ service *sshweb.LocalService }

func (c sshMonitorCollector) CollectHistoricalStats(ctx context.Context) ([]monitormodule.Stats, error) {
	metrics, err := c.service.CollectEnabledMetrics(ctx)
	out := make([]monitormodule.Stats, 0, len(metrics))
	for _, m := range metrics {
		out = append(out, monitormodule.Stats{
			Hostname: m.Host, Timestamp: time.Now(), CPUUsage: m.CPUUsage, CPUCores: m.CPUCores,
			MemoryTotal: m.MemoryTotal, MemoryUsed: m.MemoryUsed, MemoryFree: m.MemoryFree, MemoryUsage: m.MemoryUsage,
			DiskTotal: m.DiskTotal, DiskUsed: m.DiskUsed, DiskFree: m.DiskFree, DiskUsage: m.DiskUsage,
			NetBytesSent: m.NetworkSent, NetBytesRecv: m.NetworkReceived, Uptime: m.Uptime,
			Load1: m.Load1, Load5: m.Load5, Load15: m.Load15,
		})
	}
	return out, err
}

func (s *Server) startMonitorCollection() error {
	if s.monitorContext == nil {
		return nil
	}
	interval := time.Duration(s.config.CollectionInterval) * time.Second
	s.monitorCollection = monitorweb.NewCollectionService(s.monitorContext, s.MonitorRepository(), monitormodule.LocalCollector{}, sshMonitorCollector{service: s.SSHService()}, interval)
	return s.monitorCollection.Start()
}

var _ monitorweb.HistoricalCollector = sshMonitorCollector{}
