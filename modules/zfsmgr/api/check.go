package api

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lsongdev/ohmyserver/modules/zfsmgr/system"
)

const (
	// checkInterval is how often pools are looked at. Both commands read
	// kernel state, and touch no disk.
	checkInterval = 30 * time.Second
	checkTimeout  = 20 * time.Second
	scrubOverdue  = 35 * 24 * time.Hour

	levelWarning  = "warning"
	levelCritical = "critical"
)

// Event is something worth telling someone. Without Key it is a message. With
// Key it is the state of a problem, and is repeated on every check: repeats
// change nothing, and Resolved says it is over.
type Event struct {
	Level    string
	Title    string
	Body     string
	Link     string // a path in this module
	Key      string
	Resolved bool
}

// Notifier passes events on. It must not block for long.
type Notifier interface {
	Notify(ctx context.Context, e Event) error
}

// checker remembers which pools it has seen, to close their issues once if one
// goes away. Anything it does not know, it says nothing about: only what it has
// read as healthy is reported as resolved.
type checker struct {
	mu   sync.Mutex
	seen map[string]bool
}

func (s *Service) report(ctx context.Context, e Event) {
	if err := s.notifier.Notify(ctx, e); err != nil && ctx.Err() == nil {
		s.logger.Warn("zfs: reporting", "key", e.Key, "err", err)
	}
}

// state reports a problem as present, or as over.
func (s *Service) state(ctx context.Context, key string, bad bool, e Event) {
	e.Key = key
	if !bad {
		e = Event{Key: key, Resolved: true}
	}
	s.report(ctx, e)
}

// startChecks looks at the pools on an interval, if anyone is listening.
func (s *Service) startChecks() {
	ctx, cancel := context.WithCancel(context.Background())
	s.stopChecks, s.checksDone = cancel, make(chan struct{})
	go func() {
		defer close(s.checksDone)
		s.CheckPools(ctx)
		tick := time.NewTicker(checkInterval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				s.CheckPools(ctx)
			}
		}
	}()
}

const zpoolKey = "zpool:unavailable"

// CheckPools makes one pass over the pools.
func (s *Service) CheckPools(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	pools, err := s.zfs.Pools(ctx)
	if ctx.Err() != nil {
		return
	}
	s.state(ctx, zpoolKey, err != nil, Event{
		Level: levelCritical, Title: "Cannot read ZFS pools", Link: "/",
		Body: fmt.Sprintf("%v\nA pool may be suspended, which usually means its disks stopped responding.", err),
	})
	if err != nil {
		return
	}
	detailsErr := s.zfs.AddHealth(ctx, pools)
	if ctx.Err() != nil {
		return
	}
	current := map[string]bool{}
	for _, p := range pools {
		current[p.Name] = true
		s.checkPool(ctx, p, detailsErr == nil)
	}
	s.closeGone(ctx, current)
}

func (s *Service) checkPool(ctx context.Context, p system.Pool, details bool) {
	link := "/pool?name=" + p.Name
	s.state(ctx, "pool:"+p.Name+":health", p.Health != "ONLINE", Event{
		Level: levelCritical, Title: fmt.Sprintf("Pool %s is %s", p.Name, strings.ToLower(p.Health)), Link: link,
		Body: healthBody(p),
	})
	level := levelWarning
	if p.Capacity >= 90 {
		level = levelCritical
	}
	s.state(ctx, "pool:"+p.Name+":capacity", p.Capacity >= 80, Event{
		Level: level, Title: fmt.Sprintf("Pool %s is %d%% full", p.Name, p.Capacity), Link: link,
		Body: "Above about 80% a pool slows down. Snapshots keep deleted files' space in use until they go too.",
	})
	if !details {
		return // what cannot be read is not reported as fine
	}
	s.state(ctx, "pool:"+p.Name+":errors", p.Errors > 0, Event{
		Level: levelCritical, Title: fmt.Sprintf("Pool %s has %d known data errors", p.Name, p.Errors), Link: link,
		Body: "Run zpool status -v " + p.Name + " on the server for the affected files.",
	})
	if age, known := s.scrubAge(ctx, p); known {
		days := int(age.Hours() / 24)
		s.state(ctx, "pool:"+p.Name+":scrub", age > scrubOverdue, Event{
			Level: levelWarning, Title: fmt.Sprintf("Pool %s has not been scrubbed for %d days", p.Name, days), Link: link,
			Body: "A scrub checks the pool's data against its checksums and repairs what redundancy allows.",
		})
	}
}

// scrubAge says how long ago the pool was last checked, counting from its
// creation if it never was, and whether that could be told.
func (s *Service) scrubAge(ctx context.Context, p system.Pool) (time.Duration, bool) {
	since := time.Time{}
	switch {
	case p.Scan == nil:
		created, err := s.zfs.PoolCreated(ctx, p.Name)
		if err != nil {
			return 0, false
		}
		since = created
	case p.Scan.State == "SCANNING":
		return 0, true
	case p.Scan.End > 0:
		since = time.Unix(p.Scan.End, 0)
	default:
		since = time.Unix(p.Scan.Start, 0)
	}
	return time.Since(since), true
}

// healthBody says what zpool says, and which devices are behind it.
func healthBody(p system.Pool) string {
	var lines []string
	if p.Status != "" {
		lines = append(lines, p.Status)
	}
	if p.Action != "" {
		lines = append(lines, p.Action)
	}
	var walk func(vdevs []system.Vdev)
	walk = func(vdevs []system.Vdev) {
		for _, v := range vdevs {
			if len(v.Children) > 0 {
				walk(v.Children)
				continue
			}
			if v.State != "ONLINE" && v.State != "AVAIL" && v.State != "INUSE" {
				name := v.Disk
				if name == "" {
					name = v.Name
				}
				lines = append(lines, name+": "+strings.ToLower(v.State))
			}
		}
	}
	walk(p.Vdevs)
	return strings.Join(lines, "\n")
}

// closeGone closes the issues of pools that have gone away: exported or
// destroyed on purpose, as far as can be told. Only pools seen since this panel
// started are known; after a restart such an issue has to be closed by hand.
func (s *Service) closeGone(ctx context.Context, current map[string]bool) {
	s.check.mu.Lock()
	var gone []string
	for name := range s.check.seen {
		if !current[name] {
			gone = append(gone, name)
		}
	}
	s.check.seen = current
	s.check.mu.Unlock()
	for _, name := range gone {
		for _, part := range []string{"health", "capacity", "errors", "scrub"} {
			s.report(ctx, Event{Key: "pool:" + name + ":" + part, Resolved: true})
		}
	}
}

// reportSchedules passes on what the last retention pass found, by dataset.
func (s *Service) reportSchedules(ctx context.Context) {
	datasets, err := s.zfs.Datasets(ctx)
	if err != nil {
		return
	}
	failed := s.scheduler.Errors()
	for _, d := range datasets {
		problem := failed[d.Name]
		s.state(ctx, "dataset:"+d.Name+":schedule", problem != "", Event{
			Level: levelWarning, Title: "Snapshot schedule of " + d.Name + " failed", Body: problem,
			Link: "/dataset?name=" + d.Name,
		})
	}
}
