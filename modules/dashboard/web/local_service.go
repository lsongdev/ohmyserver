package web

import (
	"context"

	dashboard "github.com/lsongdev/ohmyserver/modules/dashboard"
)

type WidgetSource interface {
	EnabledWidgets(context.Context) ([]dashboard.Widget, error)
}

type SystemSource interface {
	SystemSummary(context.Context) (dashboard.SystemSummary, error)
}

// LocalService composes the narrow sources a host or standalone command
// provides. Dashboard owns the overview contract and never imports a feature
// module's storage or collector implementation.
type LocalService struct {
	widgets WidgetSource
	system  SystemSource
}

func NewLocalService(widgets WidgetSource, system SystemSource) *LocalService {
	return &LocalService{widgets: widgets, system: system}
}

func (s *LocalService) Overview(ctx context.Context) (dashboard.Overview, error) {
	if err := ctx.Err(); err != nil {
		return dashboard.Overview{}, err
	}
	result := dashboard.Overview{Widgets: []dashboard.Widget{}}
	if s.widgets != nil {
		items, err := s.widgets.EnabledWidgets(ctx)
		if err != nil {
			return dashboard.Overview{}, err
		}
		result.Widgets = items
	}
	if s.system != nil {
		item, err := s.system.SystemSummary(ctx)
		if err != nil {
			return dashboard.Overview{}, err
		}
		result.System = item
	}
	return result, nil
}

var _ dashboard.Service = (*LocalService)(nil)
