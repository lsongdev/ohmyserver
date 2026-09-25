package host

import (
	"context"
	"fmt"
	"strings"

	ntpmodule "github.com/lsongdev/ohmyserver/modules/ntp"
	ntpweb "github.com/lsongdev/ohmyserver/modules/ntp/web"
)

func (s *Server) initNTPSchema() error {
	return s.localNTPModule().Initialize(context.Background())
}

func (s *Server) localNTPModule() *ntpweb.LocalModule {
	s.ntpModuleOnce.Do(func() { s.ntpModule = ntpweb.NewLocalModule(s.db, s.applyNTPPolicy) })
	return s.ntpModule
}

func (s *Server) NTPRepository() *ntpweb.Repository { return s.localNTPModule().Repository() }

func (s *Server) ntpModuleService() *ntpweb.LocalService {
	return s.localNTPModule().Service()
}

// NTPModuleService returns the module API capability with host-wide policy
// injected without passing the Server itself into the module constructor.
func (s *Server) NTPModuleService() ntpmodule.Service { return s.ntpModuleService() }

func (s *Server) reloadNTP() error {
	return s.reloadNTPContext(context.Background())
}

func (s *Server) reloadNTPContext(ctx context.Context) error {
	return s.ntpModuleService().Reload(ctx)
}

func (s *Server) applyNTPPolicy(ctx context.Context, settings ntpmodule.Settings) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	previous, err := s.getServicePreferenceContext(ctx, "ntp", false)
	if err != nil {
		return err
	}
	if err = s.setServiceDesiredContext(ctx, "ntp", settings.Enabled); err != nil {
		return err
	}
	rollback := func() { _ = s.setServiceDesiredContext(context.WithoutCancel(ctx), "ntp", previous.Enabled) }
	if settings.Enabled {
		conflicts, conflictErr := s.configuredServiceConflicts("ntp")
		if conflictErr != nil {
			rollback()
			return conflictErr
		}
		if len(conflicts) > 0 {
			rollback()
			return fmt.Errorf("listener conflict: %s is also used by %s", conflicts[0].Endpoint, strings.Join(conflicts[0].Services, ", "))
		}
	}
	return ctx.Err()
}

func (s *Server) startNTPService() error {
	_ = s.ntpModuleService()
	if !s.serviceDesiredEnabled("ntp", false) {
		return nil
	}
	err := s.reloadNTP()
	s.setServiceError("ntp", err)
	return err
}

func (s *Server) stopNTPService() error {
	return s.ntpModuleService().Close()
}
