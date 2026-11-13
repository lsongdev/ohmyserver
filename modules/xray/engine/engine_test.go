package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func loopbackPort(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func proxyConfig(addr net.Addr, route string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{
		"inbounds":[{"tag":"local","listen":"127.0.0.1","port":%d,"protocol":"http"}],
		"outbounds":[{"tag":"direct","protocol":"freedom"},{"tag":"blocked","protocol":"blackhole","settings":{"response":{"type":"http"}}}],
		"routing":{"rules":[{"type":"field","inboundTag":["local"],"outboundTag":%q}]}
	}`, addr.(*net.TCPAddr).Port, route))
}

func TestDraftApplyRollbackAndRestart(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	initial := e.Snapshot()
	if initial.Running || initial.Enabled {
		t.Fatal("fresh install should be stopped")
	}
	if _, err := e.Save(initial.Revision, json.RawMessage(`{"outbounds":[{"protocol":"invalid"}]}`)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid configuration: %v", err)
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "via xray")
	}))
	defer target.Close()
	available := loopbackPort(t)
	addr := available.Addr()
	available.Close()
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		Proxy:             http.ProxyURL(&url.URL{Scheme: "http", Host: addr.String()}),
		DisableKeepAlives: true,
	}}
	assertProxy := func(want bool) {
		t.Helper()
		resp, err := client.Get(target.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil || (string(body) == "via xray") != want {
			t.Fatalf("proxy response %d %q, want forwarding=%v: %v", resp.StatusCode, body, want, err)
		}
	}
	saved, err := e.Save(initial.Revision, proxyConfig(addr, "direct"))
	if err != nil || saved.Running || !saved.Pending {
		t.Fatalf("save draft: %+v %v", saved, err)
	}
	if _, err := e.Apply(initial.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale apply: %v", err)
	}
	active, err := e.Apply(saved.Revision)
	if err != nil || !active.Running || active.Pending {
		t.Fatalf("apply: %+v %v", active, err)
	}
	assertProxy(true)

	occupied := loopbackPort(t)
	next, err := e.Save(active.Revision, proxyConfig(occupied.Addr(), "blocked"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Apply(next.Revision); err == nil {
		t.Fatal("apply should fail with occupied port")
	}
	if got := e.Snapshot(); !got.Running || !got.Pending || got.Error == "" || got.Revision != next.Revision {
		t.Fatalf("rollback state: %+v", got)
	}
	assertProxy(true)

	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if got := e.Snapshot(); !got.Running || !got.Pending {
		t.Fatalf("restart should use applied config, not draft: %+v", got)
	}
	assertProxy(true)
	draft, err := e.Save(e.Snapshot().Revision, proxyConfig(addr, "blocked"))
	if err != nil {
		t.Fatal(err)
	}
	applied, err := e.Apply(draft.Revision)
	if err != nil {
		t.Fatal(err)
	}
	assertProxy(false)
	if stopped, err := e.Stop(applied.Revision); err != nil || stopped.Enabled || stopped.Running {
		t.Fatalf("stop: %+v %v", stopped, err)
	}
	if c, err := net.DialTimeout("tcp", addr.String(), time.Second); err == nil {
		c.Close()
		t.Fatal("stopped inbound still accepts connections")
	}
}
