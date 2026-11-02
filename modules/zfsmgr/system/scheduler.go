package system

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"sync"
	"time"
)

// Scheduler applies the datasets' retention policies: each minute it takes
// the automatic snapshots whose period has started, including any missed
// while the server was off, then destroys the oldest beyond each class's
// count. A class set to zero, or a policy removed, leaves existing
// snapshots alone. A snapshot that is held or has clones cannot be
// destroyed and stays; older ones are still removed.
type Scheduler struct {
	ZFS    *ZFS
	Logger *slog.Logger

	mu     sync.Mutex
	errors map[string]string // dataset → what last failed
	cancel context.CancelFunc
	done   chan struct{}
}

// Start runs the scheduler until Stop.
func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.done = cancel, make(chan struct{})
	go func() {
		defer close(s.done)
		// Not at once: a crash-looping panel must not take a snapshot each start.
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-timer.C:
				s.Tick(ctx, now)
				timer.Reset(time.Minute)
			}
		}
	}()
}

// Stop waits for a running pass to end.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
		<-s.done
	}
}

// Errors returns what failed in the last pass, by dataset.
func (s *Scheduler) Errors() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.errors)
}

// Tick makes one pass over the datasets.
func (s *Scheduler) Tick(ctx context.Context, now time.Time) {
	failed := map[string]string{}
	datasets, err := s.ZFS.Datasets(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.Logger.Warn("zfs: listing datasets", "err", err)
		}
		return // keep the previous errors: nothing was checked
	}
	for _, d := range datasets {
		if d.PolicyInvalid != "" {
			failed[d.Name] = "unreadable policy " + d.PolicyInvalid + "; taking and removing nothing"
			continue
		}
		if !d.Policy.Enabled() {
			continue
		}
		if err := s.apply(ctx, d, now); err != nil {
			if ctx.Err() != nil {
				return
			}
			s.Logger.Warn("zfs: applying policy", "dataset", d.Name, "err", err)
			failed[d.Name] = err.Error()
		}
	}
	s.mu.Lock()
	s.errors = failed
	s.mu.Unlock()
}

func (s *Scheduler) apply(ctx context.Context, d Dataset, now time.Time) error {
	snapshots, err := s.ZFS.Snapshots(ctx, d.Name)
	if err != nil {
		return err
	}
	var failures []error
	for _, class := range Classes {
		keep := d.Policy.keep(class)
		if keep == 0 {
			continue
		}
		var existing []Snapshot // newest first
		for _, snap := range snapshots {
			if snap.Class == class {
				existing = append(existing, snap)
			}
		}
		start := periodStart(class, now)
		if len(existing) == 0 || time.Unix(existing[0].Created, 0).Before(start) {
			name := autoSnapshotName(class, now)
			if err := s.ZFS.Create(ctx, d.Name, name, false); err != nil {
				// Without a new snapshot, removing old ones would shrink
				// the history while nothing replaces it.
				failures = append(failures, err)
				continue
			}
			s.Logger.Info("zfs: took a snapshot", "dataset", d.Name, "name", name)
			existing = append([]Snapshot{{Name: d.Name + "@" + name}}, existing...)
		}
		for _, old := range existing[min(keep, len(existing)):] {
			if old.Holds > 0 || old.Clones != "" {
				continue
			}
			if err := s.ZFS.Destroy(ctx, old.Name); err != nil {
				failures = append(failures, err)
				continue
			}
			s.Logger.Info("zfs: removed an old snapshot", "snapshot", old.Name)
		}
	}
	return errors.Join(failures...)
}
