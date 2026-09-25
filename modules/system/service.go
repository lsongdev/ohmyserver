package system

import (
	"context"
	"errors"
	"runtime"
	"time"
)

var ErrUpdateUnavailable = errors.New("system update scheduler is unavailable")

type ServiceOptions struct {
	Info      Info
	StartedAt time.Time
	Check     func(context.Context) (UpdateInfo, error)
	Queue     func(context.Context) error
}

type localService struct {
	options ServiceOptions
}

// NewService provides the module-owned system information behavior while
// accepting host-specific metadata and update capabilities as narrow inputs.
func NewService(options ServiceOptions) Service {
	if options.StartedAt.IsZero() {
		options.StartedAt = time.Now()
	}
	return localService{options: options}
}

func (s localService) SystemInfo(ctx context.Context, check bool) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	info := s.options.Info
	info.GoVersion = runtime.Version()
	info.OS = runtime.GOOS
	info.Arch = runtime.GOARCH
	info.NumCPU = runtime.NumCPU()
	info.Uptime = time.Since(s.options.StartedAt).Round(time.Second).String()
	if check && s.options.Check != nil {
		update, err := s.options.Check(ctx)
		if err != nil {
			return Info{}, err
		}
		info.Update = update
	}
	return info, nil
}

func (s localService) QueueSystemUpdate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.options.Queue == nil {
		return ErrUpdateUnavailable
	}
	return s.options.Queue(ctx)
}
