package web

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	dhcpmodule "github.com/lsongdev/ohmyserver/modules/dhcp"
)

type LocalService struct {
	ctx     context.Context
	repo    *Repository
	runtime *Runtime
	policy  ApplyPolicy
	mu      sync.Mutex
}

// ApplyPolicy lets a host coordinate application-wide settings such as
// service preferences and shared listener conflicts.
type ApplyPolicy func(context.Context, Settings) error

func NewLocalService(ctx context.Context, repo *Repository, policies ...ApplyPolicy) *LocalService {
	service := &LocalService{ctx: ctx, repo: repo, runtime: &Runtime{}}
	if len(policies) > 0 {
		service.policy = policies[0]
	}
	return service
}
func (s *LocalService) Runtime() *Runtime { return s.runtime }
func (s *LocalService) Close() error      { return s.runtime.Stop() }
func (s *LocalService) UpdateDHCPConfiguration(ctx context.Context, item Settings) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	item, err := dhcpmodule.NormalizeSettings(item)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, err := s.repo.Settings(ctx)
	if err != nil {
		return err
	}
	if err = s.repo.UpdateSettings(ctx, item); err != nil {
		return err
	}
	rollback := func(cause error) error {
		rollbackCtx := context.WithoutCancel(ctx)
		rollbackErr := s.repo.UpdateSettings(rollbackCtx, *previous)
		if s.policy != nil {
			rollbackErr = errors.Join(rollbackErr, s.policy(rollbackCtx, *previous))
		}
		rollbackErr = errors.Join(rollbackErr, s.runtime.Start(s.ctx, s.repo, *previous))
		return errors.Join(cause, rollbackErr)
	}
	if s.policy != nil {
		if err = s.policy(ctx, item); err != nil {
			return rollback(fmt.Errorf("apply DHCP policy: %w", err))
		}
	}
	if err = ctx.Err(); err != nil {
		return rollback(err)
	}
	if err = s.runtime.Start(s.ctx, s.repo, item); err != nil {
		return rollback(fmt.Errorf("apply DHCP settings: %w", err))
	}
	return nil
}
func (s *LocalService) SaveReservation(ctx context.Context, id int64, item StaticLease) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	mac, err := dhcpmodule.NormalizeMAC(item.MAC)
	if err != nil {
		return err
	}
	settings, err := s.repo.Settings(ctx)
	if err != nil {
		return err
	}
	if err := dhcpmodule.ValidateStaticLeaseIP(item.IP, *settings); err != nil {
		return err
	}
	leaseMAC, err := s.repo.ActiveLeaseMAC(ctx, strings.TrimSpace(item.IP))
	if err == nil && !strings.EqualFold(leaseMAC, mac) {
		return fmt.Errorf("address %s is currently leased to %s", item.IP, leaseMAC)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	item.MAC, item.IP = mac, strings.TrimSpace(item.IP)
	item.Hostname, item.Description = strings.TrimSpace(item.Hostname), strings.TrimSpace(item.Description)
	return s.repo.SaveStaticLease(ctx, id, item)
}
func (s *LocalService) DeleteReservation(ctx context.Context, id int64) error {
	return s.repo.DeleteStaticLease(ctx, id)
}
func (s *LocalService) DeleteDHCPLease(ctx context.Context, id int64) error {
	return s.repo.DeleteLease(ctx, id)
}
func (s *LocalService) DHCPRuntimeStatus(ctx context.Context) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	settings, err := s.repo.Settings(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"runtime": s.runtime.Status(), "enabled": settings.Enabled, "last_error": s.runtime.Status().LastError}, nil
}
func (s *LocalService) DHCPAction(ctx context.Context, action string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.repo.Settings(ctx)
	if err != nil {
		return err
	}
	switch strings.ToLower(action) {
	case "start":
		settings.Enabled = true
	case "stop":
		settings.Enabled = false
	case "reload":
		if !settings.Enabled {
			return errors.New("DHCP service is disabled")
		}
		return s.runtime.Start(s.ctx, s.repo, *settings)
	default:
		return fmt.Errorf("unknown DHCP service action %q", action)
	}
	if err := s.repo.UpdateSettings(ctx, *settings); err != nil {
		return err
	}
	return s.runtime.Start(s.ctx, s.repo, *settings)
}
