package web

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	ntp "github.com/lsongdev/ohmyserver/modules/ntp"
)

// ApplyPolicy lets a host check shared resources such as listener conflicts
// and persist its service preference while the module retains NTP ownership.
type ApplyPolicy func(context.Context, ntp.Settings) error

type LocalService struct {
	repo    *Repository
	runtime *ntp.Runtime
	policy  ApplyPolicy
	mu      sync.Mutex
}

func NewLocalService(repo *Repository, policies ...ApplyPolicy) *LocalService {
	service := &LocalService{repo: repo, runtime: &ntp.Runtime{}}
	if len(policies) > 0 {
		service.policy = policies[0]
	}
	return service
}

func normalizeSettings(settings ntp.Settings) (ntp.Settings, error) {
	if err := ntp.ValidateSettings(settings); err != nil {
		return settings, err
	}
	settings.ListenAddr = strings.TrimSpace(settings.ListenAddr)
	settings.ReferenceID = strings.ToUpper(strings.TrimSpace(settings.ReferenceID))
	return settings, nil
}

func (s *LocalService) SaveSettings(ctx context.Context, settings ntp.Settings) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	settings, err := normalizeSettings(settings)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.repo.Update(ctx, settings)
}

func (s *LocalService) UpdateNTPConfiguration(ctx context.Context, settings ntp.Settings) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	settings, err := normalizeSettings(settings)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, err := s.repo.Get(ctx)
	if err != nil {
		return err
	}
	if err = s.repo.Update(ctx, settings); err != nil {
		return err
	}
	if s.policy != nil {
		if err = s.policy(ctx, settings); err != nil {
			rollbackCtx := context.WithoutCancel(ctx)
			return errors.Join(fmt.Errorf("apply NTP policy: %w", err), s.repo.Update(rollbackCtx, *old), s.policy(rollbackCtx, *old))
		}
	}
	if err = s.runtime.Start(settings); err != nil {
		rollbackCtx := context.WithoutCancel(ctx)
		rollbackErr := errors.Join(s.repo.Update(rollbackCtx, *old), s.runtime.Start(*old))
		if s.policy != nil {
			rollbackErr = errors.Join(rollbackErr, s.policy(rollbackCtx, *old))
		}
		return errors.Join(fmt.Errorf("apply NTP settings: %w", err), rollbackErr)
	}
	return nil
}

func (s *LocalService) NTPRuntimeStatus(ctx context.Context) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	status := s.runtime.Status()
	return map[string]any{"runtime": status, "enabled": settings.Enabled, "last_error": status.LastError}, nil
}

func (s *LocalService) Reload(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return err
	}
	return s.runtime.Start(*settings)
}

func (s *LocalService) Stop() error                      { return s.runtime.Stop() }
func (s *LocalService) RuntimeStatus() ntp.RuntimeStatus { return s.runtime.Status() }

func (s *LocalService) NTPAction(ctx context.Context, action string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return err
	}
	switch strings.ToLower(action) {
	case "start":
		settings.Enabled = true
	case "stop":
		settings.Enabled = false
	case "reload":
		return s.UpdateNTPConfiguration(ctx, *settings)
	default:
		return fmt.Errorf("unknown NTP service action %q", action)
	}
	return s.UpdateNTPConfiguration(ctx, *settings)
}

func (s *LocalService) Close() error { return s.runtime.Stop() }
