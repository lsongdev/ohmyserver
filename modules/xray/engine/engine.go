package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/xtls/xray-core/core"
)

var (
	ErrConflict = errors.New("Xray configuration changed; reload before saving or applying")
	ErrInvalid  = errors.New("invalid Xray configuration")
	ErrClosed   = errors.New("Xray module is closed")
	// Xray's system dialer is process-wide, so two live cores cannot be isolated.
	runtimeLease sync.Mutex
)

type state struct {
	Draft    json.RawMessage `json:"draft"`
	Applied  json.RawMessage `json:"applied,omitempty"`
	Revision uint64          `json:"revision"`
}

type Snapshot struct {
	Config   json.RawMessage `json:"config"`
	Revision uint64          `json:"revision"`
	Enabled  bool            `json:"enabled"`
	Running  bool            `json:"running"`
	Pending  bool            `json:"pending"`
	Error    string          `json:"error,omitempty"`
}

type Engine struct {
	mu      sync.Mutex
	path    string
	state   state
	core    *core.Instance
	lastErr string
	closed  bool
}

func Open(dir string) (*Engine, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	e := &Engine{path: filepath.Join(dir, "state.json"), state: state{
		Draft: json.RawMessage(`{"inbounds":[],"outbounds":[{"tag":"direct","protocol":"freedom","settings":{}}],"routing":{"rules":[]}}`),
	}}
	raw, err := os.ReadFile(e.path)
	if errors.Is(err, fs.ErrNotExist) {
		return e, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &e.state); err != nil {
		return nil, fmt.Errorf("read Xray state: %w", err)
	}
	if len(e.state.Applied) != 0 {
		if err := e.start(e.state.Applied); err != nil {
			e.lastErr = err.Error()
		}
	}
	return e, nil
}

func (e *Engine) snapshot() Snapshot {
	return Snapshot{Config: bytes.Clone(e.state.Draft), Revision: e.state.Revision,
		Enabled: len(e.state.Applied) != 0, Running: e.core != nil, Error: e.lastErr,
		Pending: !bytes.Equal(e.state.Draft, e.state.Applied)}
}

func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshot()
}

func (e *Engine) check(revision uint64) error {
	if e.closed {
		return ErrClosed
	}
	if revision != e.state.Revision {
		return ErrConflict
	}
	return nil
}

func (e *Engine) Save(revision uint64, raw json.RawMessage) (Snapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.check(revision); err != nil {
		return Snapshot{}, err
	}
	if _, err := compile(raw); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return Snapshot{}, err
	}
	next := e.state
	next.Draft = compact.Bytes()
	return e.commit(next)
}

func (e *Engine) start(raw json.RawMessage) error {
	cfg, err := compile(raw)
	if err != nil {
		return err
	}
	if !runtimeLease.TryLock() {
		return errors.New("another Xray core is running in this process")
	}
	e.core, err = start(cfg)
	if err != nil {
		runtimeLease.Unlock()
	}
	return err
}

func (e *Engine) stop() error {
	if e.core == nil {
		return nil
	}
	err := e.core.Close()
	e.core = nil
	runtimeLease.Unlock()
	return err
}

// Apply starts the draft, replacing the running core if necessary. On failure,
// the previous applied configuration stays on disk and the old core is restored.
func (e *Engine) Apply(revision uint64) (Snapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.check(revision); err != nil {
		return Snapshot{}, err
	}
	if e.core != nil && bytes.Equal(e.state.Draft, e.state.Applied) {
		return e.snapshot(), nil
	}
	wasRunning := e.core != nil
	if err := e.stop(); err != nil {
		e.lastErr = err.Error()
		return Snapshot{}, err
	}
	next := e.state
	next.Applied = next.Draft
	err := e.start(next.Applied)
	if err == nil {
		if _, err = e.commit(next); err == nil {
			e.lastErr = ""
			return e.snapshot(), nil
		}
		err = errors.Join(err, e.stop())
	}
	if wasRunning {
		if rollback := e.start(e.state.Applied); rollback != nil {
			err = errors.Join(err, fmt.Errorf("restore previous configuration: %w", rollback))
		}
	}
	e.lastErr = err.Error()
	return Snapshot{}, err
}

func (e *Engine) Stop(revision uint64) (Snapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.check(revision); err != nil {
		return Snapshot{}, err
	}
	next := e.state
	next.Applied = nil
	if _, err := e.commit(next); err != nil {
		return Snapshot{}, err
	}
	if err := e.stop(); err != nil {
		e.lastErr = err.Error()
		return Snapshot{}, err
	}
	e.lastErr = ""
	return e.snapshot(), nil
}

func (e *Engine) commit(next state) (Snapshot, error) {
	next.Revision++
	raw, err := json.Marshal(next)
	if err != nil {
		return Snapshot{}, err
	}
	f, err := os.CreateTemp(filepath.Dir(e.path), ".state-*")
	if err != nil {
		return Snapshot{}, err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(raw)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return Snapshot{}, err
	}
	if err := os.Rename(f.Name(), e.path); err != nil {
		return Snapshot{}, err
	}
	e.state = next
	return e.snapshot(), nil
}

func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closed = true
	return e.stop()
}
