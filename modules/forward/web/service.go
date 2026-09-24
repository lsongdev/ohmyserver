package web

import (
	"context"
	"fmt"
	"sync"

	forward "github.com/lsongdev/ohmyserver/modules/forward"
	proxy "github.com/lsongdev/ohmyserver/modules/forward/proxy"
)

type LocalService struct {
	repo    *Repository
	proxy   *proxy.TCPProxy
	mu      sync.Mutex
	enabled bool
}

func NewLocalService(repo *Repository) *LocalService {
	return &LocalService{repo: repo, proxy: proxy.NewTCPProxy(), enabled: true}
}
func (s *LocalService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = true
	return s.startAll(ctx)
}
func (s *LocalService) startAll(ctx context.Context) error {
	items, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].Status == "active" {
			if err := s.start(ctx, &items[i]); err != nil {
				s.proxy.StopAllListeners()
				s.enabled = false
				return err
			}
		}
	}
	return nil
}
func (s *LocalService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = false
	s.proxy.StopAllListeners()
}
func (s *LocalService) start(ctx context.Context, x *forward.Listener) error {
	return s.proxy.StartListener(ctx, &proxy.TCPProxyConfig{ID: x.ID, Name: x.Name, Protocol: x.Protocol, ListenPort: x.ListenPort, ForwardHost: x.ForwardHost, ForwardPort: x.ForwardPort})
}
func (s *LocalService) CreateForwardListener(ctx context.Context, x *forward.Listener) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.repo.Create(ctx, x); err != nil {
		return err
	}
	if s.enabled && x.Status == "active" {
		if err := s.start(ctx, x); err != nil {
			_ = s.repo.Delete(ctx, x.ID)
			return err
		}
	}
	return nil
}
func (s *LocalService) UpdateForwardListener(ctx context.Context, x forward.Listener) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, err := s.repo.Get(ctx, x.ID)
	if err != nil {
		return err
	}
	s.proxy.StopListener(old.ListenPort)
	if err = s.repo.Update(ctx, x); err != nil {
		_ = s.start(ctx, old)
		return err
	}
	if s.enabled && x.Status == "active" {
		if err = s.start(ctx, &x); err != nil {
			_ = s.repo.Update(ctx, *old)
			_ = s.start(ctx, old)
			return err
		}
	}
	return nil
}
func (s *LocalService) DeleteForwardListener(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	s.proxy.StopListener(item.ListenPort)
	return s.repo.Delete(ctx, id)
}
func (s *LocalService) ForwardListenerAction(ctx context.Context, id int64, action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	switch action {
	case "activate":
		if err = s.repo.SetStatus(ctx, id, "active"); err != nil {
			return err
		}
		if s.enabled {
			if err = s.start(ctx, item); err != nil {
				_ = s.repo.SetStatus(ctx, id, "inactive")
				return err
			}
		}
	case "deactivate":
		s.proxy.StopListener(item.ListenPort)
		return s.repo.SetStatus(ctx, id, "inactive")
	case "restart":
		if !s.enabled {
			return nil
		}
		s.proxy.StopListener(item.ListenPort)
		if err = s.start(ctx, item); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown forward listener action %q", action)
	}
	return nil
}
