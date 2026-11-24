// Package api owns bounded background NAT checks and their JSON API.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/lsongdev/ohmyserver/modules/nat/discovery"
)

type Check struct {
	ID       string           `json:"id"`
	Status   string           `json:"status"`
	Started  time.Time        `json:"started"`
	Finished *time.Time       `json:"finished,omitempty"`
	Config   discovery.Config `json:"config"`
	Result   discovery.Result `json:"result"`
}

type Options struct{ Detector *discovery.Detector }

type Service struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	detector *discovery.Detector
	checks   []*Check
	active   string
	stop     context.CancelFunc
}

func New(ctx context.Context, opts Options) *Service {
	ctx, cancel := context.WithCancel(ctx)
	d := opts.Detector
	if d == nil {
		d = discovery.New()
	}
	return &Service{ctx: ctx, cancel: cancel, detector: d}
}
func (s *Service) Close() error { s.mu.Lock(); s.cancel(); s.mu.Unlock(); s.wg.Wait(); return nil }

func (s *Service) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /defaults", func(w http.ResponseWriter, r *http.Request) {
		addresses := []string{}
		if addrs, e := net.InterfaceAddrs(); e == nil {
			for _, a := range addrs {
				if ip, _, e := net.ParseCIDR(a.String()); e == nil && ip.To4() != nil && !ip.IsLoopback() {
					addresses = append(addresses, ip.String())
				}
			}
		}
		respond(w, 200, map[string]any{"servers": discovery.DefaultServers(), "addresses": addresses, "family": "ipv4", "retention": 20})
	})
	m.HandleFunc("GET /checks", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		list := make([]*Check, 0, len(s.checks))
		for i := len(s.checks) - 1; i >= 0; i-- {
			list = append(list, s.checks[i])
		}
		respond(w, 200, list)
	})
	m.HandleFunc("GET /checks/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if c := s.find(r.PathValue("id")); c != nil {
			respond(w, 200, c)
		} else {
			fail(w, 404, "not_found", "Check not found. Checks are retained until the process restarts, up to 20 results.", nil)
		}
	})
	m.HandleFunc("POST /checks", s.start)
	m.HandleFunc("POST /checks/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		c := s.find(r.PathValue("id"))
		if c == nil {
			fail(w, 404, "not_found", "Check not found.", nil)
			return
		}
		if s.active == c.ID {
			s.stop()
		}
		respond(w, 200, c)
	})
	return m
}

func (s *Service) find(id string) *Check {
	for _, c := range s.checks {
		if c.ID == id {
			return c
		}
	}
	return nil
}
func (s *Service) start(w http.ResponseWriter, r *http.Request) {
	var cfg discovery.Config
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		fail(w, 400, "bad_request", "Enter valid detection settings.", nil)
		return
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		fail(w, 400, "bad_request", "Expected one JSON object.", nil)
		return
	}
	if fields := discovery.Validate(&cfg); len(fields) > 0 {
		fail(w, 422, "invalid", "Check the detection settings.", fields)
		return
	}
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		fail(w, 500, "internal", "Cannot create a check.", nil)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		fail(w, 503, "unavailable", "The detector is shutting down.", nil)
		return
	}
	if s.active != "" {
		fail(w, 409, "in_use", "A NAT check is already running. Open it from recent checks or wait for it to finish.", nil)
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 60*time.Second)
	c := &Check{ID: hex.EncodeToString(id[:]), Status: "running", Started: time.Now(), Config: cfg, Result: discovery.Result{Mapping: "unknown", Filtering: "unknown", NATType: "unknown", Public: []string{}, Notes: []string{}, Steps: []discovery.Step{}}}
	s.checks = append(s.checks, c)
	if len(s.checks) > 20 {
		s.checks = s.checks[len(s.checks)-20:]
	}
	s.active = c.ID
	s.stop = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		result := s.detector.Run(ctx, cfg, func(result discovery.Result) { s.mu.Lock(); c.Result = result; s.mu.Unlock() })
		s.mu.Lock()
		defer s.mu.Unlock()
		c.Result = result
		now := time.Now()
		c.Finished = &now
		c.Status = "completed"
		if ctx.Err() != nil {
			c.Status = "cancelled"
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				c.Status = "timed_out"
			}
		}
		s.active = ""
		s.stop = nil
	}()
	respond(w, 202, c)
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"data": v})
}
func fail(w http.ResponseWriter, status int, code, message string, fields map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": message, "fields": fields}})
}
