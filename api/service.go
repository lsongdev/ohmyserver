// Package api samples every monitored host on an interval, keeps their
// history, and serves both as a JSON API with relative routes.
package api

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/lsongdev/monitor-go/collector"
	"github.com/lsongdev/monitor-go/metrics"
	"github.com/lsongdev/monitor-go/store"
)

// Options configures the service; zero values select the defaults.
type Options struct {
	// Collectors read hosts; collector.Local is added unless NoLocal.
	Collectors []collector.Collector
	NoLocal    bool
	Interval   time.Duration // between readings of a host (15s)
	Logger     *slog.Logger
}

// Host statuses.
const (
	StatusOnline   = "online"
	StatusOffline  = "offline"  // the last reading failed, or none came for a while
	StatusPending  = "pending"  // waiting for a second reading to compute rates
	StatusDisabled = "disabled" // not monitored
	StatusGone     = "gone"     // its collector no longer offers it; only history remains
)

// HostView is a host with its current state.
type HostView struct {
	collector.Host
	Enabled  bool            `json:"enabled"`
	Status   string          `json:"status"`
	Error    string          `json:"error,omitempty"`
	LastSeen time.Time       `json:"last_seen,omitzero"`
	Info     metrics.Info    `json:"info"`
	Latest   *metrics.Sample `json:"latest,omitempty"`
	Disks    []metrics.Disk  `json:"disks,omitempty"`
	Uptime   int64           `json:"uptime,omitempty"` // seconds
	Spark    *Spark          `json:"spark,omitempty"`
}

// Spark is the last hour of CPU and memory use, for list views.
type Spark struct {
	CPU []float64 `json:"cpu"`
	Mem []float64 `json:"mem"` // percent
}

// Summary describes the hosts for an overview, with the local host's
// figures.
type Summary struct {
	Hosts   int       `json:"hosts"`
	Online  int       `json:"online"`
	Offline int       `json:"offline"`
	Local   *HostView `json:"local,omitempty"`
}

// ErrNotFound reports an unknown host.
var ErrNotFound = errors.New("host not found")

// Service samples hosts.
type Service struct {
	store      *store.SQLite
	collectors map[string]collector.Collector
	interval   time.Duration
	log        *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.Mutex
	hosts  []collector.Host // as last offered by the collectors
	states map[string]*state
}

// state is what the service knows about a host between readings.
type state struct {
	prev    *metrics.Reading
	latest  *metrics.Sample
	disks   []metrics.Disk
	uptime  time.Duration
	err     string
	lastOK  time.Time
	reading bool
}

// NewService returns a service; call Start to begin sampling.
func NewService(st *store.SQLite, opts Options) *Service {
	if opts.Interval == 0 {
		opts.Interval = 15 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &Service{
		store: st, collectors: map[string]collector.Collector{}, interval: opts.Interval, log: opts.Logger,
		states: map[string]*state{},
	}
	if !opts.NoLocal {
		s.collectors["local"] = &collector.Local{}
	}
	for _, c := range opts.Collectors {
		s.collectors[c.Kind()] = c
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s
}

// Start samples every enabled host now and then every interval, and
// compacts the history every five minutes.
func (s *Service) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		tick := time.NewTicker(s.interval)
		defer tick.Stop()
		compact := time.NewTicker(5 * time.Minute)
		defer compact.Stop()
		s.poll()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-tick.C:
				s.poll()
			case now := <-compact.C:
				if err := s.store.Compact(s.ctx, now); err != nil && s.ctx.Err() == nil {
					s.log.Error("monitor: compacting history failed", "err", err)
				}
			}
		}
	}()
}

// Close stops sampling and waits for readings in progress.
func (s *Service) Close() {
	s.cancel()
	s.wg.Wait()
}

// discover asks every collector for its hosts, records new ones (enabled)
// and returns them.
func (s *Service) discover(ctx context.Context) []collector.Host {
	var hosts []collector.Host
	for _, kind := range s.kinds() {
		list, err := s.collectors[kind].Hosts(ctx)
		if err != nil {
			s.log.Warn("monitor: listing hosts failed", "kind", kind, "err", err)
			// Keep the hosts this collector offered last time.
			s.mu.Lock()
			for _, h := range s.hosts {
				if h.Kind == kind {
					hosts = append(hosts, h)
				}
			}
			s.mu.Unlock()
			continue
		}
		hosts = append(hosts, list...)
	}
	for _, h := range hosts {
		if err := s.store.AddHost(ctx, h.ID, true); err != nil && ctx.Err() == nil {
			s.log.Error("monitor: recording a host failed", "host", h.ID, "err", err)
		}
	}
	s.mu.Lock()
	s.hosts = hosts
	s.mu.Unlock()
	return hosts
}

// kinds returns the collector kinds, local first.
func (s *Service) kinds() []string {
	kinds := make([]string, 0, len(s.collectors))
	for k := range s.collectors {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool {
		if (kinds[i] == "local") != (kinds[j] == "local") {
			return kinds[i] == "local"
		}
		return kinds[i] < kinds[j]
	})
	return kinds
}

// poll starts a reading of every enabled host that is not being read.
func (s *Service) poll() {
	hosts := s.discover(s.ctx)
	stored, err := s.store.Hosts(s.ctx)
	if err != nil {
		if s.ctx.Err() == nil {
			s.log.Error("monitor: loading hosts failed", "err", err)
		}
		return
	}
	for _, h := range hosts {
		if !stored[h.ID].Enabled {
			continue
		}
		s.mu.Lock()
		st := s.states[h.ID]
		if st == nil {
			st = &state{}
			s.states[h.ID] = st
		}
		busy := st.reading
		st.reading = true
		s.mu.Unlock()
		if busy {
			continue // a slow host: skip this round
		}
		s.wg.Add(1)
		go func(h collector.Host, st *state) {
			defer s.wg.Done()
			s.read(h, st)
		}(h, st)
	}
}

// read takes one reading of h and stores the sample it completes.
func (s *Service) read(h collector.Host, st *state) {
	ctx, cancel := context.WithTimeout(s.ctx, s.interval)
	defer cancel()
	r, info, err := s.collectors[h.Kind].Read(ctx, h.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	st.reading = false
	if err != nil {
		if s.ctx.Err() == nil {
			if st.err != err.Error() {
				s.log.Warn("monitor: reading failed", "host", h.ID, "err", err)
			}
			st.err = err.Error()
		}
		return
	}
	st.err, st.lastOK, st.disks, st.uptime = "", r.Time, r.Disks, r.Uptime
	if st.prev != nil {
		sample := metrics.Between(*st.prev, r)
		st.latest = &sample
		if err := s.store.Add(s.ctx, h.ID, sample); err != nil && s.ctx.Err() == nil {
			s.log.Error("monitor: storing a sample failed", "host", h.ID, "err", err)
		}
	}
	st.prev = &r
	if err := s.store.Seen(s.ctx, h.ID, info, r.Time); err != nil && s.ctx.Err() == nil {
		s.log.Error("monitor: recording host info failed", "host", h.ID, "err", err)
	}
}

// view builds a host's view; callers hold s.mu.
func (s *Service) viewLocked(h collector.Host, stored store.Host, offered bool) HostView {
	v := HostView{Host: h, Enabled: stored.Enabled, Info: stored.Info, LastSeen: stored.LastSeen}
	st := s.states[h.ID]
	if st != nil && stored.Enabled {
		v.Latest, v.Disks, v.Uptime, v.Error = st.latest, st.disks, int64(st.uptime.Seconds()), st.err
	}
	switch {
	case !offered:
		v.Status = StatusGone
	case !stored.Enabled:
		v.Status = StatusDisabled
	case st == nil || st.lastOK.IsZero() && st.err == "":
		v.Status = StatusPending
	case st.err != "" || time.Since(st.lastOK) > 3*s.interval:
		v.Status = StatusOffline
	case st.latest == nil:
		v.Status = StatusPending
	default:
		v.Status = StatusOnline
	}
	return v
}

// Hosts returns every host with its state: the ones the collectors offer,
// then the ones only history remains of. With spark, each carries its last
// hour of CPU and memory use.
func (s *Service) Hosts(ctx context.Context, spark bool) ([]HostView, error) {
	stored, err := s.store.Hosts(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	var out []HostView
	seen := map[string]bool{}
	for _, h := range s.hosts {
		seen[h.ID] = true
		out = append(out, s.viewLocked(h, stored[h.ID], true))
	}
	var gone []string
	for id := range stored {
		if !seen[id] {
			gone = append(gone, id)
		}
	}
	slices.Sort(gone)
	for _, id := range gone {
		name := stored[id].Info.Hostname
		if name == "" {
			name = id
		}
		out = append(out, s.viewLocked(collector.Host{ID: id, Name: name, Kind: collector.Kind(id)}, stored[id], false))
	}
	s.mu.Unlock()
	if spark {
		now := time.Now()
		for i := range out {
			if out[i].Status == StatusDisabled {
				continue
			}
			series, _, err := s.store.Series(ctx, out[i].ID, now.Add(-time.Hour), now, 60)
			if err != nil {
				return nil, err
			}
			sp := &Spark{CPU: []float64{}, Mem: []float64{}}
			for _, x := range series {
				sp.CPU = append(sp.CPU, round(x.CPU))
				sp.Mem = append(sp.Mem, round(percent(x.MemUsed, x.MemTotal)))
			}
			out[i].Spark = sp
		}
	}
	if out == nil {
		out = []HostView{}
	}
	return out, nil
}

// Host returns one host.
func (s *Service) Host(ctx context.Context, id string) (HostView, error) {
	hosts, err := s.Hosts(ctx, false)
	if err != nil {
		return HostView{}, err
	}
	for _, h := range hosts {
		if h.ID == id {
			return h, nil
		}
	}
	return HostView{}, ErrNotFound
}

// SetEnabled turns monitoring of a host on or off. Disabling keeps its
// history.
func (s *Service) SetEnabled(ctx context.Context, id string, enabled bool) (HostView, error) {
	if _, err := s.Host(ctx, id); err != nil {
		return HostView{}, err
	}
	if err := s.store.SetEnabled(ctx, id, enabled); err != nil {
		return HostView{}, err
	}
	s.mu.Lock()
	delete(s.states, id) // start over: a stale previous reading gives wrong rates
	s.mu.Unlock()
	return s.Host(ctx, id) // the next round reads it
}

// DeleteHost removes the history of a host its collector no longer offers.
func (s *Service) DeleteHost(ctx context.Context, id string) error {
	h, err := s.Host(ctx, id)
	if err != nil {
		return err
	}
	if h.Status != StatusGone {
		return errors.New("only hosts that no longer exist can be deleted; disable monitoring instead")
	}
	s.mu.Lock()
	delete(s.states, id)
	s.mu.Unlock()
	return s.store.DeleteHost(ctx, id)
}

// Ranges are the periods Series accepts.
var Ranges = map[string]time.Duration{
	"1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour, "90d": 90 * 24 * time.Hour,
}

// Series returns about 300 samples of a host over the last period.
func (s *Service) Series(ctx context.Context, id string, period time.Duration) ([]metrics.Sample, time.Duration, error) {
	if _, err := s.Host(ctx, id); err != nil {
		return nil, 0, err
	}
	now := time.Now()
	return s.store.Series(ctx, id, now.Add(-period), now, 300)
}

// Summary counts hosts by status and includes the local host.
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	hosts, err := s.Hosts(ctx, false)
	var sum Summary
	for i, h := range hosts {
		if h.Status == StatusGone || h.Status == StatusDisabled {
			continue
		}
		sum.Hosts++
		switch h.Status {
		case StatusOnline, StatusPending:
			sum.Online++
		case StatusOffline:
			sum.Offline++
		}
		if h.ID == "local" {
			sum.Local = &hosts[i]
		}
	}
	return sum, err
}

func percent(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

func round(v float64) float64 { return float64(int(v*10+0.5)) / 10 }
