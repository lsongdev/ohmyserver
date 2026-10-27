package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRunner answers systemctl and journalctl; Stream (journalctl -f) sends
// one entry and then waits until it is stopped.
type fakeRunner struct {
	mu  sync.Mutex
	ran []string
}

func (f *fakeRunner) Output(_ context.Context, _ []string, name string, args ...string) ([]byte, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	f.mu.Lock()
	f.ran = append(f.ran, line)
	f.mu.Unlock()
	switch {
	case strings.HasPrefix(line, "systemctl show ohmyserver.service"):
		return []byte("Id=ohmyserver.service\nLoadState=loaded\nActiveState=active\nSubState=running\n"), nil
	case strings.HasPrefix(line, "systemctl show nginx.service"):
		return []byte("Id=nginx.service\nLoadState=loaded\nActiveState=active\nSubState=running\n"), nil
	case strings.HasPrefix(line, "systemctl show"):
		return []byte("Id=" + args[1] + "\nLoadState=not-found\nActiveState=inactive\n"), nil
	case strings.HasPrefix(line, "journalctl"):
		return []byte(`{"__CURSOR":"s=1","MESSAGE":"hello","PRIORITY":"6","__REALTIME_TIMESTAMP":"1793073823314313"}` + "\n"), nil
	}
	return nil, nil
}

func (f *fakeRunner) Stream(ctx context.Context, env []string, out func(string), name string, args ...string) error {
	f.Output(ctx, env, name, args...)
	out(`{"__CURSOR":"s=2","MESSAGE":"live","PRIORITY":"3","__REALTIME_TIMESTAMP":"1793073823314313"}`)
	<-ctx.Done()
	return ctx.Err()
}

func newTestService(t *testing.T) (*Service, *fakeRunner) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "run", "systemd", "system"), 0o755)
	os.MkdirAll(filepath.Join(root, "proc", "self"), 0o755)
	os.WriteFile(filepath.Join(root, "proc", "self", "cgroup"), []byte("0::/system.slice/ohmyserver.service\n"), 0o644)
	run := &fakeRunner{}
	return New(Options{Runner: run, Root: root}), run
}

func call(t *testing.T, h http.Handler, method, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec.Code, rec.Body.String()
}

func TestProtectsItsOwnService(t *testing.T) {
	s, run := newTestService(t)
	h := s.Handler()
	if status, body := call(t, h, "GET", "/system"); !strings.Contains(body, `"self":"ohmyserver.service"`) {
		t.Fatalf("system: %d %s", status, body)
	}
	for _, action := range []string{"stop", "disable"} {
		if status, _ := call(t, h, "POST", "/services/ohmyserver/"+action); status != http.StatusConflict {
			t.Errorf("%s self: %d", action, status)
		}
	}
	if status, body := call(t, h, "POST", "/services/ohmyserver/restart"); status != 200 || !strings.Contains(body, `"self":true`) {
		t.Fatalf("restart self: %d %s", status, body)
	}
	if status, _ := call(t, h, "POST", "/services/nginx/stop"); status != 200 {
		t.Fatalf("stop nginx: %d", status)
	}
	want := []string{"systemctl restart --no-block ohmyserver.service", "systemctl stop nginx.service"}
	var acted []string
	for _, line := range run.ran {
		if strings.HasPrefix(line, "systemctl restart") || strings.HasPrefix(line, "systemctl stop") || strings.HasPrefix(line, "systemctl disable") {
			acted = append(acted, line)
		}
	}
	if strings.Join(acted, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ran %q", acted)
	}
}

func TestRejectsBadInput(t *testing.T) {
	s, _ := newTestService(t)
	h := s.Handler()
	for path, want := range map[string]int{
		"/services/-x":                400,
		"/services/nope":              404,
		"/logs?priority=x":            400,
		"/logs?since=forever":         400,
		"/logs?unit=a%20b":            400,
		"/logs?after=%24(id)":         400,
		"/services/nginx/mask":        404, // GET on an action path
		"/logs?unit=nginx&since=1h&q": 200,
	} {
		method := "GET"
		if status, body := call(t, h, method, path); status != want {
			t.Errorf("%s: %d %s", path, status, body)
		}
	}
	if status, _ := call(t, h, "POST", "/services/nginx/mask"); status != 400 {
		t.Errorf("mask: %d", status)
	}
}

func TestUnsupported(t *testing.T) {
	s := New(Options{Runner: &fakeRunner{}, Root: t.TempDir()})
	if status, body := call(t, s.Handler(), "GET", "/services"); status != 503 || !strings.Contains(body, "unsupported") {
		t.Fatalf("%d %s", status, body)
	}
}

func TestStream(t *testing.T) {
	s, run := newTestService(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/logs/stream?unit=nginx&after=s%3D1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type %q", resp.Header.Get("Content-Type"))
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			var e struct{ Message, Cursor string }
			json.Unmarshal([]byte(data), &e)
			if e.Message != "live" || e.Cursor != "s=2" {
				t.Fatalf("entry %s", data)
			}
			break
		}
	}
	cancel()
	run.mu.Lock()
	defer run.mu.Unlock()
	if !strings.Contains(strings.Join(run.ran, "\n"), "journalctl --no-pager -o json -u nginx.service --after-cursor s=1 -f -n 0") {
		t.Fatalf("ran %q", run.ran)
	}
}
