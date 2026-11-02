// Package api is the ZFS snapshot manager's JSON API: snapshots to list,
// take, rename, compare, delete and roll back to, and the retention policies
// a scheduler applies to datasets.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/lsongdev/ohmyserver/modules/zfsmgr/system"
)

const (
	// maxChanges bounds a diff: past this, the list is no use to read.
	maxChanges = 5000
	// diffTimeout bounds zfs diff, which walks what changed.
	diffTimeout = 2 * time.Minute
	// changeTimeout bounds a change. Changes do not end with their request:
	// a browser going away must not leave half of a bulk delete undone.
	changeTimeout = 10 * time.Minute
)

// Options configures the API.
type Options struct {
	Logger *slog.Logger
	// Runner runs commands; nil runs them on this host.
	Runner system.Runner
	// Installed overrides looking for zfs and zpool, for tests.
	Installed *bool
}

// Service serves the API.
type Service struct {
	zfs       *system.ZFS
	scheduler *system.Scheduler
	installed bool
	logger    *slog.Logger
}

// New finds ZFS. The scheduler does not run until Start.
func New(opts Options) *Service {
	run := opts.Runner
	if run == nil {
		run = system.Exec{}
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	installed := system.Installed()
	if opts.Installed != nil {
		installed = *opts.Installed
	}
	zfs := &system.ZFS{Run: run}
	return &Service{zfs: zfs, scheduler: &system.Scheduler{ZFS: zfs, Logger: logger}, installed: installed, logger: logger}
}

// Supported reports whether ZFS is installed. Without it every endpoint but
// /info answers 503 "unsupported", so a shell may leave the module out.
func (s *Service) Supported() bool { return s.installed }

// Start starts applying retention policies, if ZFS is installed.
func (s *Service) Start() {
	if s.installed {
		s.scheduler.Start()
	}
}

// Close stops applying retention policies.
func (s *Service) Close() error {
	s.scheduler.Stop()
	return nil
}

// SnapshotDetail is a snapshot with what a rollback to it would destroy and
// where its files are.
type SnapshotDetail struct {
	system.Snapshot
	Newer []system.Snapshot `json:"newer"`
	// Path is the snapshot's read-only directory, when it can be browsed.
	Path string `json:"path,omitempty"`
}

// Diff is what changed since a snapshot.
type Diff struct {
	Changes   []system.Change `json:"changes"`
	Truncated bool            `json:"truncated"`
}

// DestroyResult reports a bulk delete, which continues past failures.
type DestroyResult struct {
	Destroyed []string        `json:"destroyed"`
	Failed    []DestroyFailed `json:"failed"`
}

// DestroyFailed is a snapshot that could not be destroyed.
type DestroyFailed struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

type policyBody struct {
	Dataset string         `json:"dataset"`
	Policy  *system.Policy `json:"policy"` // null removes the dataset's own policy
}

// Handler returns the routes, relative to the module's mount point.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	handle := func(pattern string, fn func(r *http.Request) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if !s.installed {
				writeError(w, http.StatusServiceUnavailable, "unsupported", "ZFS is not installed on this server")
				return
			}
			respond(w, fn, r)
		})
	}
	// change is handle for what alters the system.
	change := func(pattern string, fn func(ctx context.Context, r *http.Request) (any, error)) {
		handle(pattern, func(r *http.Request) (any, error) {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), changeTimeout)
			defer cancel()
			return fn(ctx, r)
		})
	}

	mux.HandleFunc("GET /info", func(w http.ResponseWriter, r *http.Request) {
		info := map[string]any{"installed": s.installed}
		if s.installed {
			info["version"] = s.zfs.Version(r.Context())
		}
		respond(w, func(*http.Request) (any, error) { return info, nil }, r)
	})
	handle("GET /pools", func(r *http.Request) (any, error) { return s.zfs.Pools(r.Context()) })
	handle("GET /datasets", func(r *http.Request) (any, error) {
		list, err := s.zfs.Datasets(r.Context())
		if err != nil {
			return nil, err
		}
		failed := s.scheduler.Errors()
		for i := range list {
			list[i].ScheduleError = failed[list[i].Name]
		}
		return list, nil
	})
	handle("GET /dataset", func(r *http.Request) (any, error) {
		d, err := s.zfs.Dataset(r.Context(), r.URL.Query().Get("name"))
		if err != nil {
			return nil, err
		}
		d.ScheduleError = s.scheduler.Errors()[d.Name]
		return d, nil
	})
	change("PUT /policy", func(ctx context.Context, r *http.Request) (any, error) {
		var body policyBody
		if err := decode(r, &body); err != nil {
			return nil, err
		}
		var err error
		if body.Policy == nil {
			err = s.zfs.InheritPolicy(ctx, body.Dataset)
		} else {
			err = s.zfs.SetPolicy(ctx, body.Dataset, *body.Policy)
		}
		s.logged(err, "changed a retention policy", "dataset", body.Dataset)
		return nil, err
	})

	handle("GET /snapshots", func(r *http.Request) (any, error) {
		return s.zfs.Snapshots(r.Context(), r.URL.Query().Get("dataset"))
	})
	handle("GET /snapshot", func(r *http.Request) (any, error) {
		name := r.URL.Query().Get("name")
		snap, newer, err := s.zfs.Find(r.Context(), name)
		if err != nil {
			return nil, err
		}
		dataset, err := s.zfs.Dataset(r.Context(), snap.Dataset)
		if err != nil {
			return nil, err
		}
		detail := SnapshotDetail{Snapshot: snap, Newer: newer}
		if dataset.Type == "filesystem" && dataset.Mounted && dataset.Mountpoint != "" {
			detail.Path = system.SnapshotPath(dataset.Mountpoint, snap.Short)
		}
		return detail, nil
	})
	handle("GET /snapshot/diff", func(r *http.Request) (any, error) {
		ctx, cancel := context.WithTimeout(r.Context(), diffTimeout)
		defer cancel()
		changes, truncated, err := s.zfs.Diff(ctx, r.URL.Query().Get("name"), maxChanges)
		if err != nil {
			return nil, err
		}
		return Diff{Changes: changes, Truncated: truncated}, nil
	})
	change("POST /snapshots", func(ctx context.Context, r *http.Request) (any, error) {
		var body struct {
			Dataset   string `json:"dataset"`
			Name      string `json:"name"`
			Recursive bool   `json:"recursive"`
		}
		if err := decode(r, &body); err != nil {
			return nil, err
		}
		err := s.zfs.Create(ctx, body.Dataset, body.Name, body.Recursive)
		s.logged(err, "took a snapshot", "dataset", body.Dataset, "name", body.Name, "recursive", body.Recursive)
		return nil, err
	})
	change("POST /snapshots/destroy", func(ctx context.Context, r *http.Request) (any, error) {
		var body struct {
			Names []string `json:"names"`
		}
		if err := decode(r, &body); err != nil {
			return nil, err
		}
		// Nothing runs unless every name is well formed.
		for _, name := range body.Names {
			if _, _, err := system.SplitSnapshot(name); err != nil {
				return nil, err
			}
		}
		result := DestroyResult{Destroyed: []string{}, Failed: []DestroyFailed{}}
		for _, name := range body.Names {
			err := s.zfs.Destroy(ctx, name)
			s.logged(err, "destroyed a snapshot", "snapshot", name)
			if err != nil {
				result.Failed = append(result.Failed, DestroyFailed{Name: name, Error: err.Error()})
				continue
			}
			result.Destroyed = append(result.Destroyed, name)
		}
		return result, nil
	})
	change("POST /snapshots/rename", func(ctx context.Context, r *http.Request) (any, error) {
		var body struct {
			Name    string `json:"name"`
			NewName string `json:"new_name"`
		}
		if err := decode(r, &body); err != nil {
			return nil, err
		}
		err := s.zfs.Rename(ctx, body.Name, body.NewName)
		s.logged(err, "renamed a snapshot", "snapshot", body.Name, "to", body.NewName)
		return nil, err
	})
	change("POST /snapshots/rollback", func(ctx context.Context, r *http.Request) (any, error) {
		var body struct {
			Name         string   `json:"name"`
			DestroyNewer []string `json:"destroy_newer"`
		}
		if err := decode(r, &body); err != nil {
			return nil, err
		}
		err := s.zfs.Rollback(ctx, body.Name, body.DestroyNewer)
		s.logged(err, "rolled back", "snapshot", body.Name, "destroyed", body.DestroyNewer)
		return nil, err
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
	return mux
}

func (s *Service) logged(err error, msg string, args ...any) {
	if err == nil {
		s.logger.Info("zfs: "+msg, args...)
	}
}

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v); err != nil {
		return fmt.Errorf("%w body: %v", system.ErrInvalid, err)
	}
	return nil
}

func respond(w http.ResponseWriter, fn func(*http.Request) (any, error), r *http.Request) {
	data, err := fn(r)
	if err != nil {
		var exit *system.ExitError
		switch {
		case errors.Is(err, system.ErrInvalid):
			writeError(w, http.StatusBadRequest, "invalid", err.Error())
		case errors.Is(err, system.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", strings.TrimPrefix(err.Error(), system.ErrNotFound.Error()+": "))
		case errors.Is(err, system.ErrConflict):
			writeError(w, http.StatusConflict, "conflict", strings.TrimPrefix(err.Error(), system.ErrConflict.Error()+": "))
		case errors.As(err, &exit):
			// zfs refused, and said why in terms worth showing.
			writeError(w, http.StatusUnprocessableEntity, "zfs_error", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
