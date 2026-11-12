package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/lsongdev/ohmyserver/modules/fwmgr/system"
)

const (
	configFile = "config.json"
	trialFile  = "trial.json"
)

var errNoTrial = errors.New("no change is on trial; it may have reverted already")

// errInUse refuses what cannot be done now.
type errInUse struct{ message string }

func (e *errInUse) Error() string { return e.message }

// Trial is a change in effect that reverts by itself unless it is kept.
type Trial struct {
	Deadline    time.Time `json:"deadline"`
	Seconds     int       `json:"seconds"` // left, so a client's clock does not matter
	RevertError string    `json:"revert_error,omitempty"`
}

// Outcome is how the latest trial ended.
type Outcome struct {
	Result string    `json:"result"` // kept, reverted or expired
	At     time.Time `json:"at"`
}

// TrialStatus answers GET /trial.
type TrialStatus struct {
	Trial *Trial   `json:"trial"`
	Last  *Outcome `json:"last,omitempty"`
}

// record is what survives a restart: what to go back to, what is being
// tried, and until when. It reaches disk before the change is made, so a
// panel that dies in between reverts at its next start.
type record struct {
	Previous system.Config `json:"previous"`
	Next     system.Config `json:"next"`
	Deadline time.Time     `json:"deadline"`
}

func (s *Service) applyNow(c system.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	return s.engine.Apply(ctx, c)
}

// start puts the saved configuration in effect, and takes up a change left
// on trial. A trial whose time has passed is reverted without ever applying
// what it tried again.
func (s *Service) start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.readJSON(configFile, &s.saved); err != nil {
		s.logger.Error("firewall configuration unreadable", "err", err)
	}
	var rec record
	switch ok, err := s.readJSON(trialFile, &rec); {
	case err != nil:
		s.logger.Error("firewall trial record unreadable", "err", err)
	case ok && reflect.DeepEqual(rec.Next, s.saved):
		// It was kept, and the panel stopped before it cleared the record.
		os.Remove(s.path(trialFile))
	case ok:
		s.logger.Warn("firewall change still on trial from before this start", "deadline", rec.Deadline)
		s.active = &rec
		if time.Until(rec.Deadline) <= 0 {
			s.expireLocked()
			return
		}
		if err := s.applyNow(rec.Next); err != nil {
			s.logger.Error("applying the firewall change on trial", "err", err)
		}
		s.arm(time.Until(rec.Deadline))
		return
	}
	if err := s.applyNow(s.saved); err != nil {
		s.logger.Error("applying the saved firewall", "err", err)
	}
}

// arm starts the countdown; the caller holds mu.
func (s *Service) arm(after time.Duration) {
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(max(after, 0), s.expire)
}

func (s *Service) expire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil && !s.closed {
		s.expireLocked()
	}
}

func (s *Service) expireLocked() {
	if err := s.revert("expired"); err != nil {
		// Try for as long as it takes: this may be what restores access.
		s.logger.Error("reverting the firewall change failed; retrying", "err", err)
		s.arm(15 * time.Second)
	}
}

// revert puts back what the trial replaced; the caller holds mu.
func (s *Service) revert(result string) error {
	if err := s.applyNow(s.active.Previous); err != nil {
		s.failed = err.Error()
		return err
	}
	s.logger.Info("firewall change reverted", "result", result)
	s.finish(result)
	return nil
}

func (s *Service) finish(result string) {
	if s.timer != nil {
		s.timer.Stop()
	}
	s.last = &Outcome{Result: result, At: time.Now()}
	s.active, s.failed = nil, ""
	if err := os.Remove(s.path(trialFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.logger.Error("removing the firewall trial record", "err", err)
	}
}

// try puts c in effect on trial. The record reaches disk first.
func (s *Service) try(c system.Config) (TrialStatus, error) {
	if s.dir == "" {
		return TrialStatus{}, &errInUse{"This module has no state directory, so a change could not revert after a crash; it is refused."}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return TrialStatus{}, &errInUse{"The panel is shutting down."}
	}
	if s.active != nil {
		return TrialStatus{}, &errInUse{"A firewall change is on trial; keep or revert it first."}
	}
	rec := &record{Previous: s.saved, Next: c, Deadline: time.Now().Add(s.window)}
	if err := s.writeJSON(trialFile, rec); err != nil {
		return TrialStatus{}, fmt.Errorf("recording the change: %w", err)
	}
	if err := s.applyNow(c); err != nil {
		// Whatever part of it happened goes back at once.
		if back := s.applyNow(rec.Previous); back != nil {
			s.logger.Error("restoring the firewall after a failed change; retrying", "err", back)
			s.active, s.failed = rec, back.Error()
			s.arm(15 * time.Second)
			return TrialStatus{}, fmt.Errorf("%w; restoring the previous firewall failed too and is being retried: %v", err, back)
		}
		os.Remove(s.path(trialFile))
		return TrialStatus{}, err
	}
	s.logger.Info("firewall change on trial", "deadline", rec.Deadline)
	s.active = rec
	s.arm(s.window)
	return s.statusLocked(), nil
}

func (s *Service) status() TrialStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *Service) statusLocked() TrialStatus {
	status := TrialStatus{Last: s.last}
	if s.active != nil {
		status.Trial = &Trial{
			Deadline: s.active.Deadline, RevertError: s.failed,
			Seconds: max(int(time.Until(s.active.Deadline).Round(time.Second).Seconds()), 0),
		}
	}
	return status
}

// keep makes the change on trial the saved configuration. The file is
// replaced before the record is removed: stopping between the two leaves a
// record that start recognises as already kept.
func (s *Service) keep() (TrialStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil {
		return TrialStatus{}, errNoTrial
	}
	if err := s.writeJSON(configFile, s.active.Next); err != nil {
		return TrialStatus{}, fmt.Errorf("saving the firewall: %w", err)
	}
	s.saved = s.active.Next
	s.logger.Info("firewall change kept")
	s.finish("kept")
	return TrialStatus{Last: s.last}, nil
}

func (s *Service) revertNow() (TrialStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil {
		return TrialStatus{}, errNoTrial
	}
	if err := s.revert("reverted"); err != nil {
		return TrialStatus{}, err
	}
	return TrialStatus{Last: s.last}, nil
}
