// Package api is the notification center: it records what modules and scripts
// report, keeps track of the problems still open, and serves both as a JSON API
// with relative routes.
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/lsongdev/ohmyserver/modules/notify/channels"
	"github.com/lsongdev/ohmyserver/modules/notify/store"
)

// ErrInvalid marks an event that fails validation.
var ErrInvalid = errors.New("invalid event")

const (
	maxKey   = 200
	maxTitle = 300
	maxBody  = 4000
	maxLink  = 500

	defaultLimit = 100
	maxLimit     = 500
)

// Event is what a source reports. Without Key it is a one-off message. With
// Key it is the state of an ongoing problem, and may be repeated on every
// check: repeats change nothing, and Resolved closes it if it is open.
type Event struct {
	Level    string // debug, info, warning or critical; not needed with Resolved
	Title    string // one line; not needed with Resolved
	Body     string // details, as plain text
	Link     string // a path within the source module, such as /pool?name=data
	Key      string // the same problem keeps the same key
	Resolved bool
}

// Options configures the service; zero values select the defaults.
type Options struct {
	Logger *slog.Logger
	// FlapWindow is how long a resolution waits to be announced, so that a
	// problem coming and going stays quiet (5 minutes).
	FlapWindow time.Duration
	// Retention says how long events are kept (see DefaultRetention).
	Retention *store.Retention
	// Now is the clock, for tests.
	Now func() time.Time
	// Unauthenticated says the host serves this without a sign-in, so anyone
	// who can reach it can read what channels are for and change them. The
	// pages warn about it.
	Unauthenticated bool
	// Sending configures how channels send.
	Sending channels.Options
	// RateLimit is the most messages a channel gets in a minute (20).
	RateLimit int
}

// DefaultRetention keeps less of what matters less.
func DefaultRetention() store.Retention {
	const day = 24 * time.Hour
	return store.Retention{
		Ages: map[string]time.Duration{
			store.LevelDebug: 7 * day, store.LevelInfo: 30 * day,
			store.LevelWarning: 90 * day, store.LevelCritical: 365 * day,
		},
		Max: 100_000,
	}
}

// Service records events and tracks issues.
type Service struct {
	store  *store.SQLite
	logger *slog.Logger
	window time.Duration
	keep   store.Retention
	now    func() time.Time
	open   bool // served without a sign-in
	send   channels.Options
	rate   int

	mu     sync.Mutex // one change at a time: a report reads, then writes
	cancel context.CancelFunc
	done   chan struct{}

	wake    chan struct{}  // a delivery may be due
	active  map[int64]bool // channels sending now, each one at a time
	sending sync.WaitGroup
	busy    sync.Mutex // guards active
}

// NewService returns a service; call Start for its upkeep.
func NewService(st *store.SQLite, opts Options) *Service {
	s := &Service{
		store: st, logger: opts.Logger, window: opts.FlapWindow, keep: DefaultRetention(), now: opts.Now,
		open: opts.Unauthenticated, send: opts.Sending, rate: opts.RateLimit,
		wake: make(chan struct{}, 1), active: map[int64]bool{},
	}
	if s.rate == 0 {
		s.rate = defaultRate
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.window == 0 {
		s.window = 5 * time.Minute
	}
	if opts.Retention != nil {
		s.keep = *opts.Retention
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// Start announces resolutions as their window passes, and removes old events.
func (s *Service) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.done = cancel, make(chan struct{})
	go func() {
		defer close(s.done)
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		prune := time.NewTicker(time.Hour)
		defer prune.Stop()
		retry := time.NewTicker(5 * time.Second)
		defer retry.Stop()
		s.dispatch(ctx) // what a restart left waiting
		for {
			select {
			case <-ctx.Done():
				s.sending.Wait()
				return
			case <-s.wake:
				s.dispatch(ctx)
			case <-retry.C:
				s.dispatch(ctx)
			case <-tick.C:
				if err := s.Flush(ctx); err != nil && ctx.Err() == nil {
					s.logger.Warn("notify: announcing resolutions", "err", err)
				}
			case <-prune.C:
				if err := s.Prune(ctx); err != nil && ctx.Err() == nil {
					s.logger.Warn("notify: removing old events", "err", err)
				}
			}
		}
	}()
}

// Close stops the upkeep.
func (s *Service) Close() {
	if s.cancel != nil {
		s.cancel()
		<-s.done
	}
}

// Flush announces the resolutions whose window has passed.
func (s *Service) Flush(ctx context.Context) error {
	s.mu.Lock()
	err := s.store.FlushResolved(ctx, s.window, s.now())
	s.mu.Unlock()
	s.nudge()
	return err
}

// Prune removes old events.
func (s *Service) Prune(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.Prune(ctx, s.keep, s.now())
}

func validate(source string, e *Event) error {
	e.Title, e.Key = strings.TrimSpace(e.Title), strings.TrimSpace(e.Key)
	switch {
	case source == "":
		return fmt.Errorf("%w: source required", ErrInvalid)
	case len(e.Key) > maxKey:
		return fmt.Errorf("%w: key longer than %d", ErrInvalid, maxKey)
	case e.Resolved && e.Key == "":
		return fmt.Errorf("%w: resolved needs a key", ErrInvalid)
	}
	if e.Resolved {
		return nil
	}
	switch e.Level {
	case store.LevelDebug, store.LevelInfo, store.LevelWarning, store.LevelCritical:
	default:
		return fmt.Errorf("%w: unknown level %q", ErrInvalid, e.Level)
	}
	if e.Title == "" {
		return fmt.Errorf("%w: title required", ErrInvalid)
	}
	// A link is followed by whoever reads this, so it stays within the panel.
	if e.Link != "" && (!strings.HasPrefix(e.Link, "/") || strings.HasPrefix(e.Link, "//") || len(e.Link) > maxLink) {
		return fmt.Errorf("%w: link must be a path within the module", ErrInvalid)
	}
	e.Title, e.Body = cut(e.Title, maxTitle), cut(e.Body, maxBody)
	return nil
}

func cut(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return strings.ToValidUTF8(s[:limit], "") + "…"
}

// Notify records what source reports. Callers may repeat it freely.
func (s *Service) Notify(ctx context.Context, source string, e Event) error {
	if err := validate(source, &e); err != nil {
		return err
	}
	s.mu.Lock()
	err := s.store.Record(ctx, store.Report{
		Source: source, Key: e.Key, Level: e.Level, Title: e.Title, Body: e.Body, Link: e.Link, Resolved: e.Resolved,
	}, s.now())
	s.mu.Unlock()
	s.nudge()
	return err
}

// MuteIssue keeps an issue from being sent anywhere until a time, which may be
// nil to end that. It is still recorded and shown.
func (s *Service) MuteIssue(ctx context.Context, id int64, until *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.MuteIssue(ctx, id, until)
}

// Events lists events newest first.
func (s *Service) Events(ctx context.Context, q store.EventQuery) ([]store.Event, error) {
	if q.Limit <= 0 {
		q.Limit = defaultLimit
	}
	q.Limit = min(q.Limit, maxLimit)
	return s.store.Events(ctx, q)
}

// Deliveries lists deliveries newest first.
func (s *Service) Deliveries(ctx context.Context, q store.DeliveryQuery) ([]store.DeliveryView, error) {
	return s.store.Deliveries(ctx, q)
}

// Sources lists who has sent events.
func (s *Service) Sources(ctx context.Context) ([]string, error) { return s.store.Sources(ctx) }

// Issues lists issues in a state.
func (s *Service) Issues(ctx context.Context, state string) ([]store.Issue, error) {
	return s.store.Issues(ctx, state)
}

// CloseIssue resolves an open issue by hand.
func (s *Service) CloseIssue(ctx context.Context, id int64) error {
	s.mu.Lock()
	err := s.store.CloseIssue(ctx, id, s.now())
	s.mu.Unlock()
	s.nudge()
	return err
}

// Summary is what an overview shows.
type Summary struct {
	Open     int           `json:"open"`
	Critical int           `json:"critical"`
	Warning  int           `json:"warning"`
	Issues   []store.Issue `json:"issues"` // the most severe
}

// Summary counts the open issues.
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	issues, err := s.store.Issues(ctx, store.StateOpen)
	if err != nil {
		return Summary{}, err
	}
	sum := Summary{Open: len(issues), Issues: issues[:min(len(issues), 5)]}
	for _, i := range issues {
		switch i.Level {
		case store.LevelCritical:
			sum.Critical++
		case store.LevelWarning:
			sum.Warning++
		}
	}
	return sum, nil
}
