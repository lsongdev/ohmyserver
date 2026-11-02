package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/lsongdev/ohmyserver/modules/zfsmgr/system"
)

const (
	datasets  = "data\tfilesystem\t10\t20\t10\t3\t/data\tyes\ndata/vol\tvolume\t1\t2\t1\t0\tnone\t-\n"
	policies  = "data\thourly=24,daily=0,weekly=0,monthly=0\tlocal\ndata/vol\thourly=24,daily=0,weekly=0,monthly=0\tinherited from data\n"
	snapshots = "data@b\t200\t1\t2\t3\t0\t-\ndata@a\t100\t1\t2\t3\t0\t-\n"
)

type fake struct{ calls []string }

func (f *fake) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	line := name + " " + strings.Join(args, " ")
	switch {
	case strings.HasPrefix(line, "zfs list -Hp -t filesystem,volume"):
		return []byte(datasets), nil
	case strings.HasPrefix(line, "zfs get -Hp"):
		return []byte(policies), nil
	case strings.HasPrefix(line, "zfs list -Hp -t snapshot"):
		return []byte(snapshots), nil
	}
	f.calls = append(f.calls, line)
	if strings.Contains(line, "destroy data@busy") {
		return nil, &system.ExitError{Command: name, Code: 1, Output: "dataset is busy"}
	}
	return nil, nil
}

func (f *fake) Lines(ctx context.Context, line func(string) bool, name string, args ...string) error {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	line("M\tF\t/data/file")
	return nil
}

func newServer(t *testing.T) (*httptest.Server, *fake) {
	t.Helper()
	f := &fake{}
	yes := true
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(New(Options{Runner: f, Installed: &yes, Logger: logger}).Handler())
	t.Cleanup(srv.Close)
	return srv, f
}

func call(t *testing.T, srv *httptest.Server, method, path string, body, out any) int {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = strings.NewReader(string(data))
	}
	req, _ := http.NewRequest(method, srv.URL+path, reader)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		var envelope struct{ Data json.RawMessage }
		json.NewDecoder(res.Body).Decode(&envelope)
		json.Unmarshal(envelope.Data, out)
	}
	return res.StatusCode
}

func TestDatasets(t *testing.T) {
	srv, _ := newServer(t)
	var list []system.Dataset
	if code := call(t, srv, "GET", "/datasets", nil, &list); code != 200 {
		t.Fatal(code)
	}
	if len(list) != 2 || list[0].Snapshots != 2 || list[0].PolicySource != "local" || list[0].Policy.Hourly != 24 {
		t.Fatalf("%+v", list)
	}
	if list[1].PolicySource != "data" || list[1].Mountpoint != "" {
		t.Fatalf("%+v", list[1])
	}
}

func TestSnapshotDetailShowsNewerAndPath(t *testing.T) {
	srv, _ := newServer(t)
	var d SnapshotDetail
	if code := call(t, srv, "GET", "/snapshot?name=data@a", nil, &d); code != 200 {
		t.Fatal(code)
	}
	if d.Path != "/data/.zfs/snapshot/a" || len(d.Newer) != 1 || d.Newer[0].Name != "data@b" {
		t.Fatalf("%+v", d)
	}
	if code := call(t, srv, "GET", "/snapshot?name=data@nope", nil, nil); code != 404 {
		t.Fatal(code)
	}
}

func TestBulkDestroyRefusesEverythingOverOneBadName(t *testing.T) {
	srv, f := newServer(t)
	code := call(t, srv, "POST", "/snapshots/destroy", map[string]any{"names": []string{"data@a", "data"}}, nil)
	if code != 400 || len(f.calls) != 0 {
		t.Fatalf("code %d, ran %v", code, f.calls)
	}
}

func TestBulkDestroyContinuesPastFailures(t *testing.T) {
	srv, f := newServer(t)
	var result DestroyResult
	code := call(t, srv, "POST", "/snapshots/destroy", map[string]any{"names": []string{"data@busy", "data@a"}}, &result)
	if code != 200 || !slices.Equal(result.Destroyed, []string{"data@a"}) || result.Failed[0].Error != "dataset is busy" {
		t.Fatalf("code %d, %+v", code, result)
	}
	if want := []string{"zfs destroy data@busy", "zfs destroy data@a"}; !slices.Equal(f.calls, want) {
		t.Fatalf("ran %v", f.calls)
	}
}

func TestRollbackConflict(t *testing.T) {
	srv, f := newServer(t)
	if code := call(t, srv, "POST", "/snapshots/rollback", map[string]any{"name": "data@a"}, nil); code != 409 {
		t.Fatal(code)
	}
	if len(f.calls) != 0 {
		t.Fatalf("ran %v", f.calls)
	}
	body := map[string]any{"name": "data@a", "destroy_newer": []string{"data@b"}}
	if code := call(t, srv, "POST", "/snapshots/rollback", body, nil); code != 200 {
		t.Fatal(code)
	}
	if want := []string{"zfs rollback -r data@a"}; !slices.Equal(f.calls, want) {
		t.Fatalf("ran %v", f.calls)
	}
}

func TestPolicy(t *testing.T) {
	srv, f := newServer(t)
	body := map[string]any{"dataset": "data", "policy": system.Policy{Daily: 7}}
	if code := call(t, srv, "PUT", "/policy", body, nil); code != 200 {
		t.Fatal(code)
	}
	if code := call(t, srv, "PUT", "/policy", map[string]any{"dataset": "data"}, nil); code != 200 {
		t.Fatal(code)
	}
	bad := map[string]any{"dataset": "data", "policy": system.Policy{Daily: -1}}
	if code := call(t, srv, "PUT", "/policy", bad, nil); code != 400 {
		t.Fatal(code)
	}
	want := []string{"zfs set " + system.PolicyProperty + "=hourly=0,daily=7,weekly=0,monthly=0 data", "zfs inherit " + system.PolicyProperty + " data"}
	if !slices.Equal(f.calls, want) {
		t.Fatalf("ran %v", f.calls)
	}
}

func TestUnsupportedWithoutZFS(t *testing.T) {
	no := false
	srv := httptest.NewServer(New(Options{Runner: &fake{}, Installed: &no}).Handler())
	defer srv.Close()
	if code := call(t, srv, "GET", "/snapshots", nil, nil); code != 503 {
		t.Fatal(code)
	}
	if code := call(t, srv, "GET", "/info", nil, nil); code != 200 {
		t.Fatal(code)
	}
}

type fakeTrigger struct {
	runs   int
	status TriggerStatus
}

func (f *fakeTrigger) Status(context.Context) (TriggerStatus, error) { return f.status, nil }
func (f *fakeTrigger) Run(context.Context) error                     { f.runs++; return nil }

func TestScheduleAndTrigger(t *testing.T) {
	srv, _ := newServer(t)
	var own Schedule
	if code := call(t, srv, "GET", "/schedule", nil, &own); code != 200 || own.External {
		t.Fatalf("code %d, %+v", code, own)
	}

	yes := true
	trigger := &fakeTrigger{status: TriggerStatus{JobID: 7, Enabled: true, RunStatus: "succeeded"}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	withJob := httptest.NewServer(New(Options{Runner: &fake{}, Installed: &yes, Logger: logger, Trigger: trigger}).Handler())
	defer withJob.Close()
	var got Schedule
	if code := call(t, withJob, "GET", "/schedule", nil, &got); code != 200 || !got.External || got.JobID != 7 {
		t.Fatalf("code %d, %+v", code, got)
	}
	body := map[string]any{"dataset": "data", "policy": system.Policy{Daily: 7}}
	if code := call(t, withJob, "PUT", "/policy", body, nil); code != 200 || trigger.runs != 1 {
		t.Fatalf("code %d, %d runs", code, trigger.runs)
	}
	if code := call(t, withJob, "PUT", "/policy", map[string]any{"dataset": "bad@name"}, nil); code != 400 || trigger.runs != 1 {
		t.Fatalf("code %d, %d runs after a refused change", code, trigger.runs)
	}
}
