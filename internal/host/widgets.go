package host

import (
	"context"
	"time"

	dashboardmodule "github.com/lsongdev/ohmyserver/modules/dashboard"
	monitormodule "github.com/lsongdev/ohmyserver/modules/monitor"
	widgetsweb "github.com/lsongdev/ohmyserver/modules/widgets/web"
)

func (s *Server) WidgetsRepository() *widgetsweb.Repository {
	return widgetsweb.NewRepository(s.db)
}

func (s *Server) Overview(ctx context.Context) (dashboardmodule.Overview, error) {
	items, err := s.WidgetsRepository().List(ctx, true)
	if err != nil {
		return dashboardmodule.Overview{}, err
	}
	result := dashboardmodule.Overview{Widgets: make([]dashboardmodule.Widget, 0, len(items))}
	for _, item := range items {
		result.Widgets = append(result.Widgets, dashboardmodule.Widget{ID: item.ID, Name: item.Name, Type: item.Type, Config: item.Config, SortOrder: item.SortOrder})
	}
	stats, err := monitormodule.CollectLocalStats(ctx)
	if err != nil {
		return dashboardmodule.Overview{}, err
	}
	result.System = dashboardmodule.SystemSummary{
		Hostname: stats.Hostname, Timestamp: stats.Timestamp.UTC().Format(time.RFC3339),
		CPUUsage: stats.CPUUsage, CPUCores: stats.CPUCores,
		MemoryTotal: stats.MemoryTotal, MemoryUsed: stats.MemoryUsed, MemoryUsage: stats.MemoryUsage,
		DiskTotal: stats.DiskTotal, DiskUsed: stats.DiskUsed, DiskUsage: stats.DiskUsage,
		Uptime: stats.Uptime, Load1: stats.Load1, Load5: stats.Load5, Load15: stats.Load15,
	}
	return result, nil
}

type dashboardServiceAdapter struct{ server *Server }

func (a dashboardServiceAdapter) Overview(ctx context.Context) (dashboardmodule.Overview, error) {
	return a.server.Overview(ctx)
}

func (s *Server) DashboardModuleService() dashboardmodule.Service {
	return dashboardServiceAdapter{server: s}
}

func (s *Server) initWidgetSchema() error {
	return s.WidgetsRepository().Initialize(context.Background())
}
