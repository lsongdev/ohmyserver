// Package monitor connects the monitor-go host monitor to the other
// modules. It implements monitor-go's collector.Collector for the hosts
// saved in ssh-go: each reading runs procfs.Script over SSH, so a saved
// Linux host is monitored without installing anything on it.
package monitor

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lsongdev/ohmyserver/monitor-go/collector"
	"github.com/lsongdev/ohmyserver/monitor-go/metrics"
	"github.com/lsongdev/ohmyserver/monitor-go/procfs"
	sshapi "github.com/lsongdev/ohmyserver/ssh-go/api"
	"github.com/lsongdev/ohmyserver/ssh-go/hosts"
)

// SSHHosts reads ssh-go's saved hosts. It keeps one connection per host
// and runs a new session on it for each reading, reconnecting when the
// connection fails. Host IDs are "ssh:<host id>".
type SSHHosts struct {
	Service *sshapi.Service

	mu    sync.Mutex
	conns map[int64]*hosts.Conn
}

var _ collector.Collector = (*SSHHosts)(nil)

func (*SSHHosts) Kind() string { return "ssh" }

func (s *SSHHosts) Hosts(ctx context.Context) ([]collector.Host, error) {
	list, err := s.Service.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]collector.Host, len(list))
	keep := map[int64]bool{}
	for i, h := range list {
		out[i] = collector.Host{ID: "ssh:" + strconv.FormatInt(h.ID, 10), Name: h.Name, Kind: "ssh"}
		keep[h.ID] = true
	}
	// Drop connections to hosts that were deleted.
	s.mu.Lock()
	for id, c := range s.conns {
		if !keep[id] {
			c.Close()
			delete(s.conns, id)
		}
	}
	s.mu.Unlock()
	return out, nil
}

func (s *SSHHosts) Read(ctx context.Context, host string) (metrics.Reading, metrics.Info, error) {
	_, idText, _ := strings.Cut(host, ":")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return metrics.Reading{}, metrics.Info{}, fmt.Errorf("invalid host %q", host)
	}
	out, err := s.run(ctx, id)
	if err != nil {
		return metrics.Reading{}, metrics.Info{}, err
	}
	return procfs.Parse(out, time.Now())
}

// run runs procfs.Script on host id, over a kept connection if it still
// works and a new one otherwise.
func (s *SSHHosts) run(ctx context.Context, id int64) (string, error) {
	for attempt := 0; ; attempt++ {
		conn, err := s.conn(ctx, id)
		if err != nil {
			return "", err
		}
		out, err := runScript(ctx, conn)
		if err == nil {
			return out, nil
		}
		s.drop(id, conn)
		if attempt > 0 || ctx.Err() != nil {
			return "", err
		}
		// The kept connection may have died since the last reading: retry
		// once on a fresh one.
	}
}

func (s *SSHHosts) conn(ctx context.Context, id int64) (*hosts.Conn, error) {
	s.mu.Lock()
	if c := s.conns[id]; c != nil {
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()
	c, err := s.Service.Connect(ctx, id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conns == nil {
		s.conns = map[int64]*hosts.Conn{}
	}
	if old := s.conns[id]; old != nil { // connected concurrently
		c.Close()
		return old, nil
	}
	s.conns[id] = c
	return c, nil
}

func (s *SSHHosts) drop(id int64, c *hosts.Conn) {
	c.Close()
	s.mu.Lock()
	if s.conns[id] == c {
		delete(s.conns, id)
	}
	s.mu.Unlock()
}

// Close closes the kept connections.
func (s *SSHHosts) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, c := range s.conns {
		c.Close()
		delete(s.conns, id)
	}
	return nil
}

// runScript runs procfs.Script in a session; cancelling the context closes
// the session.
func runScript(ctx context.Context, conn *hosts.Conn) (string, error) {
	session, err := conn.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	stop := context.AfterFunc(ctx, func() { session.Close() })
	defer stop()
	var out bytes.Buffer
	session.Stdout = &out
	// The script's exit status reflects its last optional command; what
	// matters is whether Parse finds its sections.
	session.Run("sh -c " + shellQuote(procfs.Script))
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if out.Len() == 0 {
		return "", fmt.Errorf("the host returned no output")
	}
	return out.String(), nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
