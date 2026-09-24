package host

import (
	"context"
	"fmt"
	"strings"
	"time"

	dnsmodule "github.com/lsongdev/ohmyserver/modules/dns"
	dnsweb "github.com/lsongdev/ohmyserver/modules/dns/web"
)

func (s *Server) initDNSSchema() error {
	repository := s.DNSRepository()
	if err := repository.InitializeModule(context.Background()); err != nil {
		return err
	}
	return s.importLegacyDNSProviders()
}

func (s *Server) applyDNSPolicy(ctx context.Context, item dnsmodule.Settings) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	previous, err := s.getServicePreferenceContext(ctx, "dns", item.Enabled)
	if err != nil {
		return err
	}
	rollback := func() {
		_ = s.setServiceDesiredContext(context.WithoutCancel(ctx), "dns", previous.Enabled)
	}
	if err := s.setServiceDesiredContext(ctx, "dns", item.Enabled); err != nil {
		return err
	}
	if item.Enabled {
		conflicts, conflictErr := s.configuredServiceConflicts("dns")
		if conflictErr != nil {
			rollback()
			return conflictErr
		}
		if len(conflicts) > 0 {
			rollback()
			return fmt.Errorf("listener conflict: %s is also used by %s", conflicts[0].Endpoint, strings.Join(conflicts[0].Services, ", "))
		}
	}
	if err := ctx.Err(); err != nil {
		rollback()
		return err
	}
	return nil
}

func (s *Server) DNSRuntimeStatus(ctx context.Context) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	status := s.dnsLocalService(ctx).RuntimeStatus()
	pref, err := s.getServicePreferenceContext(ctx, "dns", false)
	if err != nil {
		return nil, err
	}
	return map[string]any{"runtime": status, "enabled": pref.Enabled, "last_error": pref.LastError}, nil
}

type dnsServiceAdapter struct{ server *Server }

func (a dnsServiceAdapter) UpdateDNSConfiguration(ctx context.Context, settings dnsmodule.Settings) error {
	err := a.server.dnsLocalService(ctx).UpdateDNSConfiguration(ctx, settings)
	a.server.setServiceError("dns", err)
	return err
}
func (a dnsServiceAdapter) DNSRuntimeStatus(ctx context.Context) (any, error) {
	return a.server.DNSRuntimeStatus(ctx)
}
func (a dnsServiceAdapter) DNSAction(ctx context.Context, action string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if action == "start" || action == "reload" {
		if err := a.server.validateServiceStart("dns"); err != nil {
			a.server.setServiceError("dns", err)
			return err
		}
	}
	err := a.server.dnsLocalService(ctx).DNSAction(ctx, action)
	a.server.setServiceError("dns", err)
	return err
}
func (a dnsServiceAdapter) ReloadDNS(ctx context.Context) error { return a.server.ReloadDNS(ctx) }
func (s *Server) DNSModuleService() dnsweb.Service              { return dnsServiceAdapter{server: s} }

func (s *Server) startDNSService() error {
	s.dnsLocalService(s.monitorContext)
	if s.serviceDesiredEnabled("dns", false) {
		if err := s.validateServiceStart("dns"); err != nil {
			s.setServiceError("dns", err)
			return err
		}
	}
	err := s.refreshDNSAndReload()
	s.setServiceError("dns", err)
	return err
}

func (s *Server) stopDNSService() error {
	return s.dnsLocalService(s.monitorContext).Close()
}
