package system

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"sync"
	"time"
)

// Scheduler applies the datasets' retention policies. Each pass looks at what
// exists and takes the automatic snapshots whose period has started,
// including any missed earlier, then destroys the oldest beyond each class's
// count, so it does not matter how often or how regularly passes run. A class
// set to zero, or a policy removed, leaves existing snapshots alone. A
// snapshot that is held or has clones cannot be destroyed and stays; older
// ones are still removed.
//
// Apply is one pass, for a host's own job scheduler to call. Start runs passes
// each minute for a host without one.
type Scheduler struct {
	ZFS    *ZFS
	Logger *slog.Logger

	pass   sync.Mutex // passes never overlap
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
				s.Apply(ctx, now, io.Discard) // failures are logged and kept in Errors
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

// Apply makes one pass over the datasets, writing what it did to out. It
// returns what failed; the other datasets are still handled.
func (s *Scheduler) Apply(ctx context.Context, now time.Time, out io.Writer) error {
	s.pass.Lock()
	defer s.pass.Unlock()
	datasets, err := s.ZFS.Datasets(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.Logger.Warn("zfs: listing datasets", "err", err)
		}
		return err // keep the previous errors: nothing was checked
	}
	failed := map[string]string{}
	var failures []error
	scheduled := 0
	for _, d := range datasets {
		if d.PolicyInvalid != "" {
			err := fmt.Errorf("%s: unreadable policy %s; taking and removing nothing", d.Name, d.PolicyInvalid)
			fmt.Fprintln(out, err)
			failed[d.Name] = "unreadable policy " + d.PolicyInvalid + "; taking and removing nothing"
			failures = append(failures, err)
			continue
		}
		if !d.Policy.Enabled() {
			continue
		}
		scheduled++
		if err := s.apply(ctx, d, now, out); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			s.Logger.Warn("zfs: applying policy", "dataset", d.Name, "err", err)
			fmt.Fprintf(out, "%s: %v\n", d.Name, err)
			failed[d.Name] = err.Error()
			failures = append(failures, err)
		}
	}
	s.mu.Lock()
	s.errors = failed
	s.mu.Unlock()
	fmt.Fprintf(out, "Checked %d scheduled dataset(s).\n", scheduled)
	return errors.Join(failures...)
}

func (s *Scheduler) apply(ctx context.Context, d Dataset, now time.Time, out io.Writer) error {
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
			fmt.Fprintf(out, "Took %s@%s\n", d.Name, name)
			existing = append([]Snapshot{{Name: d.Name + "@" + name}}, existing...)
		}
		for _, old := range existing[min(keep, len(existing)):] {
			if old.Holds > 0 || old.Clones != "" {
				fmt.Fprintf(out, "Kept %s: it is held or cloned\n", old.Name)
				continue
			}
			if err := s.ZFS.Destroy(ctx, old.Name); err != nil {
				failures = append(failures, err)
				continue
			}
			s.Logger.Info("zfs: removed an old snapshot", "snapshot", old.Name)
			fmt.Fprintf(out, "Removed %s\n", old.Name)
		}
	}
	return errors.Join(failures...)
}
