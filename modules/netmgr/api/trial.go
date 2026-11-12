package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lsongdev/ohmyserver/modules/netmgr/system"
)

var errNoTrial = errors.New("no change is on trial; it may have reverted already")

// Trial is a change in effect that reverts by itself unless it is kept.
// Subject is what it changes: an interface, or later a connection.
type Trial struct {
	Subject     string    `json:"subject"`
	Target      string    `json:"target"`
	Deadline    time.Time `json:"deadline"`
	Seconds     int       `json:"seconds"` // left, so a client's clock does not matter
	RevertError string    `json:"revert_error,omitempty"`
}

// Outcome is how the latest trial ended.
type Outcome struct {
	Subject string    `json:"subject"`
	Result  string    `json:"result"` // kept, reverted or expired
	At      time.Time `json:"at"`
}

// TrialStatus answers GET /trial.
type TrialStatus struct {
	Trial *Trial   `json:"trial"`
	Last  *Outcome `json:"last,omitempty"`
}

// record is what survives a restart.
type record struct {
	Subject  string      `json:"subject"`
	Target   string      `json:"target"`
	Deadline time.Time   `json:"deadline"`
	Undo     system.Undo `json:"undo"`
}

const recordFile = "trial.json"

// trials runs at most one trial at a time. Its record reaches disk before
// the change does: a panel that dies in between reverts at its next start,
// which at worst re-applies the settings already in place.
type trials struct {
	dir      string
	run      system.Runner
	override system.Backend // for tests
	logger   *slog.Logger

	mu     sync.Mutex
	active *record
	failed string
	timer  *time.Timer
	last   *Outcome
	closed bool
}

func newTrials(dir string, run system.Runner, override system.Backend, logger *slog.Logger) *trials {
	t := &trials{dir: dir, run: run, override: override, logger: logger}
	t.recover()
	return t
}

func (t *trials) path() string { return filepath.Join(t.dir, recordFile) }

func (t *trials) recover() {
	if t.dir == "" {
		return
	}
	data, err := os.ReadFile(t.path())
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		t.logger.Error("network trial record unreadable", "err", err)
		return
	}
	var rec record
	if err := json.Unmarshal(data, &rec); err != nil {
		t.logger.Error("network trial record unreadable", "err", err)
		return
	}
	t.logger.Warn("network change still on trial from before this start", "subject", rec.Subject, "deadline", rec.Deadline)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active = &rec
	t.arm(time.Until(rec.Deadline))
}

func (t *trials) save(rec *record) error {
	if err := os.MkdirAll(t.dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	tmp := t.path() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, t.path())
}

func (t *trials) forget() {
	if t.dir != "" {
		if err := os.Remove(t.path()); err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.logger.Error("removing network trial record", "err", err)
		}
	}
}

// arm starts the countdown; the caller holds mu.
func (t *trials) arm(after time.Duration) {
	if t.timer != nil {
		t.timer.Stop()
	}
	if after < 0 {
		after = 0
	}
	t.timer = time.AfterFunc(after, t.expire)
}

func (t *trials) expire() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.active == nil || t.closed {
		return
	}
	if err := t.revert("expired"); err != nil {
		// Try for as long as it takes: this may be what restores the network.
		t.logger.Error("reverting network change failed; retrying", "subject", t.active.Subject, "err", err)
		t.arm(15 * time.Second)
	}
}

// revert undoes the active trial; the caller holds mu.
func (t *trials) revert(result string) error {
	rec := t.active
	backend := t.override
	if backend == nil {
		var err error
		if backend, err = system.BackendFor(rec.Undo.Backend, t.run); err != nil {
			t.failed = err.Error()
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	if err := backend.Revert(ctx, rec.Undo); err != nil {
		t.failed = err.Error()
		return err
	}
	t.logger.Info("network change reverted", "subject", rec.Subject, "result", result)
	t.finish(result)
	return nil
}

// finish ends the active trial; the caller holds mu.
func (t *trials) finish(result string) {
	if t.timer != nil {
		t.timer.Stop()
	}
	t.last = &Outcome{Subject: t.active.Subject, Result: result, At: time.Now()}
	t.active, t.failed = nil, ""
	t.forget()
}

func (t *trials) status() TrialStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	status := TrialStatus{Last: t.last}
	if t.active != nil {
		seconds := int(time.Until(t.active.Deadline).Round(time.Second).Seconds())
		if seconds < 0 {
			seconds = 0
		}
		status.Trial = &Trial{
			Subject: t.active.Subject, Target: t.active.Target, Deadline: t.active.Deadline,
			Seconds: seconds, RevertError: t.failed,
		}
	}
	return status
}

// change is one thing to try: what it changes, how to capture what it
// replaces, and how to make it. The trial does not know what kind of change
// it is, so interface settings, Wi-Fi and dial-up connections share it.
type change struct {
	subject string
	// begin runs with the trial lock held, before anything changes. It
	// refuses a change that cannot be made (a stale version, an interface
	// someone else manages), names its target, and captures the undo.
	begin func(ctx context.Context) (target string, backend system.Backend, undo system.Undo, err error)
	// apply makes the change and returns once it is in effect.
	apply func(ctx context.Context, backend system.Backend) error
}

func (t *trials) start(ctx context.Context, c change, window time.Duration) (any, error) {
	if t.dir == "" {
		return nil, &errInUse{"This module has no state directory, so a change could not revert after a crash; it is refused."}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil, &errInUse{"The panel is shutting down."}
	}
	if t.active != nil {
		return nil, &errInUse{fmt.Sprintf("A change to %s is on trial; keep or revert it first.", t.active.Subject)}
	}
	target, backend, undo, err := c.begin(ctx)
	if err != nil {
		return nil, err
	}
	rec := &record{Subject: c.subject, Target: target, Deadline: time.Now().Add(window), Undo: undo}
	if err := t.save(rec); err != nil {
		return nil, fmt.Errorf("recording the change: %w", err)
	}
	if err := c.apply(ctx, backend); err != nil {
		// Whatever part of it happened goes back at once.
		revertCtx, cancel := context.WithTimeout(context.Background(), applyTimeout)
		defer cancel()
		if revertErr := backend.Revert(revertCtx, undo); revertErr != nil {
			t.logger.Error("restoring after a failed change; retrying", "subject", c.subject, "err", revertErr)
			t.active, t.failed = rec, revertErr.Error()
			t.arm(15 * time.Second)
			return nil, fmt.Errorf("%w; restoring the previous settings failed too and is being retried: %v", err, revertErr)
		}
		t.forget()
		return nil, err
	}
	t.logger.Info("network change on trial", "subject", c.subject, "target", target, "deadline", rec.Deadline)
	t.active = rec
	t.arm(window)
	return t.statusLocked(), nil
}

func (t *trials) statusLocked() TrialStatus {
	seconds := int(time.Until(t.active.Deadline).Round(time.Second).Seconds())
	return TrialStatus{Trial: &Trial{Subject: t.active.Subject, Target: t.active.Target, Deadline: t.active.Deadline, Seconds: seconds}}
}

func (t *trials) keep() (any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.active == nil {
		return nil, errNoTrial
	}
	t.logger.Info("network change kept", "subject", t.active.Subject)
	for _, file := range t.active.Undo.Cleanup {
		if err := os.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.logger.Error("removing a file kept for the revert", "err", err)
		}
	}
	t.finish("kept")
	return TrialStatus{Last: t.last}, nil
}

func (t *trials) revertNow() (any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.active == nil {
		return nil, errNoTrial
	}
	if err := t.revert("reverted"); err != nil {
		return nil, err
	}
	return TrialStatus{Last: t.last}, nil
}

func (t *trials) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	if t.timer != nil {
		t.timer.Stop()
	}
}
