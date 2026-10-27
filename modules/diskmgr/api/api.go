// Package api is the disk module's JSON API: capacity of the mounted file
// systems, software RAID arrays, and the disks with their SMART health and
// self-tests.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/lsongdev/ohmyserver/modules/diskmgr/system"
	"github.com/lsongdev/smartctl-go/smartctl"
)

// Options configures the API.
type Options struct {
	Logger *slog.Logger
	// Runner runs commands; nil runs them on this host.
	Runner system.Runner
	// Root is the file system root, "" unless testing.
	Root string
}

const (
	// healthTTL is how long a disk's health is reused: reading SMART data
	// is quick but not free, and pages poll.
	healthTTL = 5 * time.Minute
	// readTimeout bounds reading every disk's health for the overview.
	readTimeout = 20 * time.Second
)

var errNotFound = errors.New("not found")

// Service serves the API.
type Service struct {
	storage *system.Storage
	logger  *slog.Logger

	mu     sync.Mutex
	health map[string]system.Health // by disk name
}

// New returns the API.
func New(opts Options) *Service {
	run := opts.Runner
	if run == nil {
		run = system.Exec{}
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{storage: &system.Storage{Run: run, Root: opts.Root}, logger: logger, health: map[string]system.Health{}}
}

// Info says what the host offers.
type Info struct {
	Linux    bool `json:"linux"`    // lsblk is available
	Smartctl bool `json:"smartctl"` // smartmontools is installed
}

// Disk is a physical disk with what is on it and its health.
type Disk struct {
	system.Block
	Health *system.Health `json:"health,omitempty"`
}

// Overview is everything the overview page shows.
type Overview struct {
	FileSystems []system.FileSystem `json:"filesystems"`
	Arrays      []system.Array      `json:"arrays"`
	Disks       []Disk              `json:"disks"`
}

func (s *Service) need() error {
	if _, err := exec.LookPath("lsblk"); err != nil {
		return system.ErrUnsupported
	}
	return nil
}

func (s *Service) overview(ctx context.Context) (*Overview, error) {
	if err := s.need(); err != nil {
		return nil, err
	}
	blocks, err := s.storage.Blocks(ctx)
	if err != nil {
		return nil, err
	}
	filesystems, err := s.storage.FileSystems(ctx)
	if err != nil {
		return nil, err
	}
	o := &Overview{FileSystems: filesystems, Arrays: s.storage.Arrays(), Disks: []Disk{}}
	if o.Arrays == nil {
		o.Arrays = []system.Array{}
	}
	disks := system.Disks(blocks)
	s.refresh(ctx, disks)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range disks {
		d := Disk{Block: b}
		if h, ok := s.health[b.Name]; ok {
			d.Health = &h
		}
		o.Disks = append(o.Disks, d)
	}
	return o, nil
}

// refresh rereads the health of disks whose cached health is stale, in
// parallel, never waking a disk in standby.
func (s *Service) refresh(ctx context.Context, disks []system.Block) {
	if !system.SmartAvailable() {
		s.mu.Lock()
		for _, d := range disks {
			s.health[d.Name] = system.Health{Error: system.ErrNoSmartctl.Error(), CheckedAt: time.Now().UTC()}
		}
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	var wg sync.WaitGroup
	for _, d := range disks {
		s.mu.Lock()
		h, ok := s.health[d.Name]
		s.mu.Unlock()
		if ok && time.Since(h.CheckedAt) < healthTTL {
			continue
		}
		wg.Go(func() {
			report, err := s.storage.Report(ctx, d.Name, false)
			s.record(d.Name, report, err)
		})
	}
	wg.Wait()
}

// record caches the health a read produced. A disk in standby keeps its
// last health, marked asleep.
func (s *Service) record(disk string, report *smartctl.SmartInfo, err error) system.Health {
	s.mu.Lock()
	defer s.mu.Unlock()
	var h system.Health
	switch {
	case errors.Is(err, smartctl.ErrAsleep):
		h = s.health[disk]
		h.Asleep, h.CheckedAt = true, time.Now().UTC()
	case err != nil:
		h = system.Health{Error: err.Error(), CheckedAt: time.Now().UTC()}
	default:
		h = system.Summarize(report)
	}
	s.health[disk] = h
	return h
}

// disk finds a physical disk by name.
func (s *Service) disk(ctx context.Context, name string) (system.Block, error) {
	if err := s.need(); err != nil {
		return system.Block{}, err
	}
	if _, err := system.DiskName(name); err != nil {
		return system.Block{}, err
	}
	blocks, err := s.storage.Blocks(ctx)
	if err != nil {
		return system.Block{}, err
	}
	for _, b := range system.Disks(blocks) {
		if b.Name == name {
			return b, nil
		}
	}
	return system.Block{}, fmt.Errorf("%w: no disk %s", errNotFound, name)
}

// DiskDetail is a disk with its full SMART report.
type DiskDetail struct {
	Disk
	Report *smartctl.SmartInfo `json:"report,omitempty"`
}

func (s *Service) detail(ctx context.Context, name string, wake bool) (*DiskDetail, error) {
	b, err := s.disk(ctx, name)
	if err != nil {
		return nil, err
	}
	d := &DiskDetail{Disk: Disk{Block: b}}
	var report *smartctl.SmartInfo
	if system.SmartAvailable() {
		report, err = s.storage.Report(ctx, name, wake)
	} else {
		err = system.ErrNoSmartctl
	}
	h := s.record(name, report, err)
	d.Health, d.Report = &h, report
	return d, nil
}

// Handler serves the API. Successful responses are {"data": ...}; failures
// are {"error": {"code": ..., "message": ...}} with a matching status.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /system", func(w http.ResponseWriter, r *http.Request) {
		respond(w, Info{Linux: s.need() == nil, Smartctl: system.SmartAvailable()}, nil)
	})
	mux.HandleFunc("GET /overview", func(w http.ResponseWriter, r *http.Request) {
		o, err := s.overview(r.Context())
		respond(w, o, err)
	})
	mux.HandleFunc("GET /disks/{name}", func(w http.ResponseWriter, r *http.Request) {
		d, err := s.detail(r.Context(), r.PathValue("name"), r.URL.Query().Get("wake") == "1")
		respond(w, d, err)
	})
	mux.HandleFunc("POST /disks/{name}/test", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Kind string `json:"kind"` // short, long, abort
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
			return
		}
		b, err := s.disk(r.Context(), r.PathValue("name"))
		if err == nil {
			err = s.storage.SelfTest(r.Context(), b.Name, req.Kind)
		}
		if err == nil {
			s.logger.Info("disks: self-test", "disk", b.Name, "kind", req.Kind)
			s.mu.Lock()
			delete(s.health, b.Name) // show the test running
			s.mu.Unlock()
		}
		respond(w, map[string]string{"kind": req.Kind}, err)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
	return mux
}

func respond(w http.ResponseWriter, data any, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"data": data})
	case errors.Is(err, system.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, errNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, system.ErrUnsupported):
		writeError(w, http.StatusServiceUnavailable, "unsupported", err.Error())
	case errors.Is(err, system.ErrNoSmartctl):
		writeError(w, http.StatusServiceUnavailable, "no_smartctl", err.Error())
	default:
		writeError(w, http.StatusBadGateway, "command_failed", err.Error())
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
