package host

import (
	"context"
	"path/filepath"

	dnsweb "github.com/lsongdev/ohmyserver/modules/dns/web"
)

func (s *Server) dnsLocalService(ctx context.Context) *dnsweb.LocalService {
	if s.dnsService == nil {
		lifetime := s.monitorContext
		if lifetime == nil {
			lifetime = context.WithoutCancel(ctx)
		}
		s.dnsService = dnsweb.NewLocalService(lifetime, s.DNSRepository(), s.dnsListDirectory(), s.dnsRunPolicy)
		s.dnsService.SetApplyPolicy(s.applyDNSPolicy)
	}
	return s.dnsService
}

func (s *Server) dnsRunPolicy(ctx context.Context, configuredEnabled bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !configuredEnabled {
		return false, nil
	}
	return s.serviceDesiredEnabled("dns", configuredEnabled), nil
}

func (s *Server) reloadDNS() error { return s.reloadDNSContext(context.Background()) }
func (s *Server) reloadDNSContext(ctx context.Context) error {
	return s.dnsLocalService(ctx).Reload(ctx, false)
}
func (s *Server) refreshDNSAndReload() error {
	return s.refreshDNSAndReloadContext(context.Background())
}
func (s *Server) refreshDNSAndReloadContext(ctx context.Context) error {
	return s.dnsLocalService(ctx).Reload(ctx, true)
}
func (s *Server) dnsListDirectory() string {
	if s.config == nil {
		return "dns-lists"
	}
	return filepath.Join(filepath.Dir(s.config.Database.Path), "dns-lists")
}

// DNSListDirectory provides the module's configured subscription storage path.
func (s *Server) DNSListDirectory() string { return s.dnsListDirectory() }

// ReloadDNS asks the application lifecycle coordinator to rebuild and apply
// DNS configuration after module-owned data changes.
func (s *Server) ReloadDNS(ctx context.Context) error { return s.reloadDNSContext(ctx) }
