// Package api is the firewall manager's JSON API. A change to the firewall
// that could cut this machine off takes effect on trial: it reverts by
// itself unless it is kept in time, which needs a working connection to the
// panel.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/lsongdev/ohmyserver/modules/fwmgr/system"
)

// DefaultWindow is how long a change waits to be kept.
const DefaultWindow = 2 * time.Minute

// applyTimeout bounds commands that change the firewall. It does not depend
// on the request: the change may well end the client's connection.
const applyTimeout = 30 * time.Second

// Engine applies a configuration. system.Firewall is the real one.
type Engine interface {
	Available() bool
	Check(ctx context.Context, c system.Config) error
	Apply(ctx context.Context, c system.Config) error
	Rules(ctx context.Context) (string, error)
}

// Options configures the API.
type Options struct {
	Logger *slog.Logger
	// Runner runs commands; nil runs them on this host.
	Runner system.Runner
	// Engine overrides nft, for tests.
	Engine Engine
	// Supported overrides the check for Linux, for tests.
	Supported *bool
	// StateDir keeps the saved configuration, and a change on trial across
	// restarts. Without it, changes are refused.
	StateDir string
	// Window overrides DefaultWindow.
	Window time.Duration
}

// Service serves the API.
type Service struct {
	run       system.Runner
	engine    Engine
	supported bool
	dir       string
	window    time.Duration
	logger    *slog.Logger

	mu     sync.Mutex
	saved  system.Config
	active *record
	failed string
	timer  *time.Timer
	last   *Outcome
	closed bool
}

// New loads the saved configuration and puts it in effect: a firewall is
// kept by the panel, not by the kernel, which forgets it at every boot. A
// change left on trial by a previous run goes on where it left off.
func New(opts Options) *Service {
	run := opts.Runner
	if run == nil {
		run = system.Exec{}
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	engine := opts.Engine
	if engine == nil {
		engine = &system.Firewall{Run: run}
	}
	window := opts.Window
	if window <= 0 {
		window = DefaultWindow
	}
	supported := runtime.GOOS == "linux"
	if opts.Supported != nil {
		supported = *opts.Supported
	}
	s := &Service{run: run, engine: engine, supported: supported, dir: opts.StateDir, window: window, logger: logger}
	if supported && engine.Available() {
		s.start()
	}
	return s
}

// Supported reports whether this is a machine with a firewall to manage.
func (s *Service) Supported() bool { return s.supported }

// Close stops waiting on a change on trial. Its record stays, so the next
// start picks it up.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
	}
	return nil
}

func (s *Service) path(name string) string { return filepath.Join(s.dir, name) }

func (s *Service) readJSON(name string, v any) (bool, error) {
	if s.dir == "" {
		return false, nil
	}
	data, err := os.ReadFile(s.path(name))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(data, v)
}

func (s *Service) writeJSON(name string, v any) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := s.path(name) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(name))
}
