package web

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	dnsmodule "github.com/lsongdev/ohmyserver/modules/dns"
	dnsruntime "github.com/lsongdev/ohmyserver/modules/dns/runtime"
)

// LocalService provides the DNS runtime and settings behavior for an
// independently running DNS module. A host may supply its own lifecycle adapter.
type LocalService struct {
	ctx         context.Context
	repo        *Repository
	listDir     string
	runtime     *dnsruntime.DNSRuntime
	runPolicy   RunPolicy
	applyPolicy ApplyPolicy
	reloadMu    sync.Mutex
}

// RunPolicy lets an embedding host apply its service preference and shared
// listener policy while the DNS module retains runtime ownership.
type RunPolicy func(context.Context, bool) (bool, error)

// ApplyPolicy lets a host persist application-level preferences and validate
// shared listener constraints when DNS settings change.
type ApplyPolicy func(context.Context, dnsmodule.Settings) error

func NewLocalService(ctx context.Context, repo *Repository, listDir string, policies ...RunPolicy) *LocalService {
	service := &LocalService{ctx: ctx, repo: repo, listDir: listDir, runtime: &dnsruntime.DNSRuntime{}}
	if len(policies) > 0 {
		service.runPolicy = policies[0]
	}
	service.runtime.StartRefreshLoop(ctx, service.ReloadDNS)
	return service
}

func (s *LocalService) SetApplyPolicy(policy ApplyPolicy) { s.applyPolicy = policy }

func (s *LocalService) Close() error { return s.runtime.Close() }

func (s *LocalService) Stop() error { return s.runtime.Stop() }

func (s *LocalService) RuntimeStatus() dnsruntime.DNSRuntimeStatus { return s.runtime.Status() }

func (s *LocalService) ReloadDNS(ctx context.Context) error {
	return s.Reload(ctx, true)
}

func (s *LocalService) Reload(ctx context.Context, refreshSubscriptions bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	cfg, enabled, err := BuildConfig(ctx, s.repo, s.listDir, refreshSubscriptions)
	if err != nil {
		return err
	}
	if s.runPolicy != nil {
		enabled, err = s.runPolicy(ctx, enabled)
		if err != nil {
			return err
		}
	}
	if !enabled {
		return s.runtime.Stop()
	}
	return s.runtime.Start(s.ctx, cfg)
}

func (s *LocalService) UpdateDNSConfiguration(ctx context.Context, settings dnsmodule.Settings) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	settings, err := NormalizeSettings(settings)
	if err != nil {
		return err
	}
	previous, err := s.repo.GetSettings(ctx)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateSettings(ctx, settings); err != nil {
		return err
	}
	rollback := func(cause error) error {
		rollbackCtx := context.WithoutCancel(ctx)
		rollbackErr := s.repo.UpdateSettings(rollbackCtx, *previous)
		if s.applyPolicy != nil {
			rollbackErr = errors.Join(rollbackErr, s.applyPolicy(rollbackCtx, *previous))
		}
		rollbackErr = errors.Join(rollbackErr, s.ReloadDNS(rollbackCtx))
		return errors.Join(cause, rollbackErr)
	}
	if s.applyPolicy != nil {
		if err := s.applyPolicy(ctx, settings); err != nil {
			return rollback(fmt.Errorf("apply DNS policy: %w", err))
		}
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}
	if err := s.ReloadDNS(ctx); err != nil {
		return rollback(fmt.Errorf("apply DNS configuration: %w", err))
	}
	return nil
}

func (s *LocalService) DNSRuntimeStatus(ctx context.Context) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	settings, err := s.repo.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"runtime": s.runtime.Status(), "enabled": settings.Enabled}, nil
}

func (s *LocalService) DNSAction(ctx context.Context, action string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	settings, err := s.repo.GetSettings(ctx)
	if err != nil {
		return err
	}
	switch strings.ToLower(action) {
	case "start":
		settings.Enabled = true
	case "stop":
		settings.Enabled = false
	case "reload":
		return s.ReloadDNS(ctx)
	default:
		return fmt.Errorf("unknown DNS service action %q", action)
	}
	return s.UpdateDNSConfiguration(ctx, *settings)
}
