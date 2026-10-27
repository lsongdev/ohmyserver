// Package api is the service manager's JSON API: systemd services, their
// state, actions and unit files, and the journal, which it can also stream
// as server-sent events.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/lsongdev/ohmyserver/svcmgr/system"
)

// Options configures the API.
type Options struct {
	Logger *slog.Logger
	// Runner runs commands; nil runs them on this host.
	Runner system.Runner
	// Root is the file system root, "/" unless testing.
	Root string
}

// ErrSelf refuses stopping or disabling the service running this process.
var ErrSelf = errors.New("this service runs the panel")

// actionTimeout bounds waiting for systemctl: a service that takes longer
// to start or stop keeps going, and the page shows its state.
const actionTimeout = 90 * time.Second

// Service serves the API.
type Service struct {
	systemd *system.Systemd
	running bool   // systemd manages this system
	self    string // the unit running this process, if any
	logger  *slog.Logger
}

// New checks for systemd and finds the unit running this process.
func New(opts Options) *Service {
	run := opts.Runner
	if run == nil {
		run = system.Exec{}
	}
	root := opts.Root
	if root == "/" {
		root = ""
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{systemd: &system.Systemd{Run: run}, running: system.Running(root), self: system.SelfUnit(root), logger: logger}
	if !s.running {
		logger.Warn("services: " + system.ErrUnsupported.Error())
	}
	return s
}

// Info describes the system.
type Info struct {
	Systemd bool   `json:"systemd"`
	Version int    `json:"version,omitempty"`
	Self    string `json:"self,omitempty"` // the service running the panel
}

// Detail is a service's state, marked when it runs the panel.
type Detail struct {
	*system.Detail
	Self    bool `json:"self,omitempty"`
	Guarded bool `json:"guarded,omitempty"` // starting it shuts down or reboots the system
}

func (s *Service) need() error {
	if !s.running {
		return system.ErrUnsupported
	}
	return nil
}

func (s *Service) detail(ctx context.Context, name string) (*Detail, error) {
	d, err := s.systemd.Show(ctx, name)
	if err != nil {
		return nil, err
	}
	return &Detail{Detail: d, Self: d.Name == s.self, Guarded: system.Guarded[d.Name]}, nil
}

// act runs an action. The panel's own service may be restarted (without
// waiting, since the restart ends this process) but not stopped or
// disabled from here.
func (s *Service) act(ctx context.Context, name, action string) (*Detail, error) {
	self := name == s.self
	if self && (action == "stop" || action == "disable") {
		return nil, fmt.Errorf("%w: %s it from a shell, where you can start it again", ErrSelf, action)
	}
	actx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()
	if err := s.systemd.Act(actx, name, action, self && (action == "restart" || action == "reload")); err != nil {
		return nil, err
	}
	s.logger.Info("services: "+action, "unit", name)
	return s.detail(ctx, name)
}

// Handler serves the API. Successful responses are {"data": ...}; failures
// are {"error": {"code": ..., "message": ...}} with a matching status.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /system", func(w http.ResponseWriter, r *http.Request) {
		info := Info{Systemd: s.running, Self: s.self}
		if s.running {
			info.Version = s.systemd.Version(r.Context())
		}
		respond(w, http.StatusOK, info, nil)
	})
	mux.HandleFunc("GET /services", func(w http.ResponseWriter, r *http.Request) {
		if err := s.need(); err != nil {
			respond(w, 0, nil, err)
			return
		}
		units, err := s.systemd.Services(r.Context())
		respond(w, http.StatusOK, units, err)
	})
	mux.HandleFunc("GET /services/{name}", func(w http.ResponseWriter, r *http.Request) {
		name, err := s.unit(r)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		d, err := s.detail(r.Context(), name)
		if err == nil && d.Load == "not-found" {
			err = fmt.Errorf("%w: %s", errNotFound, name)
		}
		respond(w, http.StatusOK, d, err)
	})
	mux.HandleFunc("GET /services/{name}/file", func(w http.ResponseWriter, r *http.Request) {
		name, err := s.unit(r)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		text, err := s.systemd.File(r.Context(), name)
		respond(w, http.StatusOK, map[string]string{"text": text}, err)
	})
	mux.HandleFunc("POST /services/{name}/{action}", func(w http.ResponseWriter, r *http.Request) {
		name, err := s.unit(r)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		d, err := s.act(r.Context(), name, r.PathValue("action"))
		respond(w, http.StatusOK, d, err)
	})
	mux.HandleFunc("GET /logs", func(w http.ResponseWriter, r *http.Request) {
		q, err := s.query(r)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		entries, err := s.systemd.Journal(r.Context(), q)
		if entries == nil {
			entries = []system.Entry{}
		}
		respond(w, http.StatusOK, entries, err)
	})
	mux.HandleFunc("GET /logs/stream", s.stream)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
	return mux
}

var errNotFound = errors.New("not found")

func (s *Service) unit(r *http.Request) (string, error) {
	if err := s.need(); err != nil {
		return "", err
	}
	return system.UnitName(r.PathValue("name"))
}

// query reads ?unit=&lines=&priority=&since=1h&q=&after=.
func (s *Service) query(r *http.Request) (system.Query, error) {
	if err := s.need(); err != nil {
		return system.Query{}, err
	}
	v := r.URL.Query()
	q := system.Query{Unit: v.Get("unit"), Search: v.Get("q"), After: v.Get("after")}
	var err error
	if n := v.Get("lines"); n != "" {
		if q.Lines, err = strconv.Atoi(n); err != nil {
			return q, fmt.Errorf("%w lines", system.ErrInvalid)
		}
	}
	if p := v.Get("priority"); p != "" {
		if q.Priority, err = strconv.Atoi(p); err != nil {
			return q, fmt.Errorf("%w priority", system.ErrInvalid)
		}
	}
	if since := v.Get("since"); since != "" {
		if q.Since, err = time.ParseDuration(since); err != nil {
			return q, fmt.Errorf("%w time range %q", system.ErrInvalid, since)
		}
	}
	return q, nil
}

// stream follows the journal as server-sent "entry" events, each a JSON
// Entry, starting after the cursor in ?after= so a page that loaded the
// newest entries misses none.
func (s *Service) stream(w http.ResponseWriter, r *http.Request) {
	q, err := s.query(r)
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "streaming is not supported")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": following the journal\n\n")
	flusher.Flush()

	ctx := r.Context()
	entries := make(chan system.Entry, 256)
	done := make(chan error, 1)
	go func() { done <- s.systemd.Follow(ctx, q, func(e system.Entry) { entries <- e }) }()
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case e := <-entries:
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "event: entry\ndata: %s\n\n", data)
			// Send what has queued up in one write.
			for drained := false; !drained; {
				select {
				case e := <-entries:
					data, _ := json.Marshal(e)
					fmt.Fprintf(w, "event: entry\ndata: %s\n\n", data)
				default:
					drained = true
				}
			}
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case err := <-done:
			if err != nil && ctx.Err() == nil {
				data, _ := json.Marshal(map[string]string{"message": err.Error()})
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
				flusher.Flush()
			}
			return
		case <-ctx.Done():
			// Drain so Follow's sender never blocks while it stops.
			go func() {
				for range entries {
				}
			}()
			<-done
			close(entries)
			return
		}
	}
}

func respond(w http.ResponseWriter, status int, data any, err error) {
	switch {
	case err == nil:
		writeJSON(w, status, map[string]any{"data": data})
	case errors.Is(err, system.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, errNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrSelf):
		writeError(w, http.StatusConflict, "self", err.Error())
	case errors.Is(err, system.ErrGuarded):
		writeError(w, http.StatusConflict, "guarded", err.Error())
	case errors.Is(err, system.ErrUnsupported):
		writeError(w, http.StatusServiceUnavailable, "unsupported", "systemd is not running on this system")
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
