package system

import (
	"context"
	"runtime"
	"time"
)

type ServiceOptions struct {
	Info      Info
	StartedAt time.Time
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

func (s localService) SystemInfo(ctx context.Context) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	info := s.options.Info
	info.GoVersion = runtime.Version()
	info.OS = runtime.GOOS
	info.Arch = runtime.GOARCH
	info.NumCPU = runtime.NumCPU()
	info.Uptime = time.Since(s.options.StartedAt).Round(time.Second).String()
	return info, nil
}
