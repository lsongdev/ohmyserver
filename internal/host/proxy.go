package host

import (
	"context"

	forward "github.com/lsongdev/ohmyserver/modules/forward"
	forwardweb "github.com/lsongdev/ohmyserver/modules/forward/web"
)

func (s *Server) forwardLocalService() *forwardweb.LocalService {
	s.forwardServiceOnce.Do(func() {
		s.forwardService = forwardweb.NewLocalService(s.ForwardRepository())
		s.forwardService.Close()
	})
	return s.forwardService
}

type forwardServiceAdapter struct{ server *Server }

func (a forwardServiceAdapter) CreateForwardListener(ctx context.Context, item *forward.Listener) error {
	if a.server.serviceDesiredEnabled("forward", true) {
		if err := a.server.validateServiceStart("forward"); err != nil {
			return err
		}
	}
	return a.server.forwardLocalService().CreateForwardListener(ctx, item)
}

func (a forwardServiceAdapter) UpdateForwardListener(ctx context.Context, item forward.Listener) error {
	if a.server.serviceDesiredEnabled("forward", true) {
		if err := a.server.validateServiceStart("forward"); err != nil {
			return err
		}
	}
	return a.server.forwardLocalService().UpdateForwardListener(ctx, item)
}

func (a forwardServiceAdapter) DeleteForwardListener(ctx context.Context, id int64) error {
	return a.server.forwardLocalService().DeleteForwardListener(ctx, id)
}

func (a forwardServiceAdapter) ForwardListenerAction(ctx context.Context, id int64, action string) error {
	if action == "activate" || action == "restart" {
		if a.server.serviceDesiredEnabled("forward", true) {
			if err := a.server.validateServiceStart("forward"); err != nil {
				return err
			}
		}
	}
	return a.server.forwardLocalService().ForwardListenerAction(ctx, id, action)
}

func (s *Server) ForwardModuleService() forward.Service {
	return forwardServiceAdapter{server: s}
}

func (s *Server) startForwardService() error {
	service := s.forwardLocalService()
	if !s.serviceDesiredEnabled("forward", true) {
		return nil
	}
	if err := s.validateServiceStart("forward"); err != nil {
		s.setServiceError("forward", err)
		return err
	}
	err := service.Start(s.monitorContext)
	s.setServiceError("forward", err)
	return err
}

func (s *Server) stopForwardService() error {
	if s.forwardService != nil {
		s.forwardService.Close()
	}
	return nil
}

var _ forward.Service = forwardServiceAdapter{}

func (s *Server) reloadForward(ctx context.Context) error {
	if err := s.validateServiceStart("forward"); err != nil {
		return err
	}
	service := s.forwardLocalService()
	service.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	return service.Start(ctx)
}
