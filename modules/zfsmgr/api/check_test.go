package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/modules/zfsmgr/system"
)

type recorder struct{ events []Event }

func (r *recorder) Notify(_ context.Context, e Event) error {
	r.events = append(r.events, e)
	return nil
}

// said lists "key" for problems and "key ok" for what was reported as fine.
func (r *recorder) said() []string {
	var out []string
	for _, e := range r.events {
		if e.Resolved {
			out = append(out, e.Key+" ok")
		} else {
			out = append(out, e.Key)
		}
	}
	slices.Sort(out)
	return out
}

// poolHost answers zpool from fixed state.
type poolHost struct {
	fake
	list, status string
	statusErr    error
	listErr      error
}

func (h *poolHost) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	line := name + " " + strings.Join(args, " ")
	switch {
	case strings.HasPrefix(line, "zpool list"):
		return []byte(h.list), h.listErr
	case strings.HasPrefix(line, "zpool status"):
		return []byte(h.status), h.statusErr
	case strings.HasPrefix(line, "zpool get"):
		return []byte(fmt.Sprint(time.Now().Add(-100*24*time.Hour).Unix()) + "\n"), nil
	}
	return h.fake.Run(ctx, name, args...)
}

func statusDoc(state string, errors int, scan string) string {
	doc := map[string]any{"pools": map[string]any{"tank": map[string]any{
		"name": "tank", "state": state, "error_count": errors,
		"vdevs": map[string]any{"tank": map[string]any{"name": "tank", "vdev_type": "root", "vdevs": map[string]any{
			"mirror-0": map[string]any{"name": "mirror-0", "vdev_type": "mirror", "state": state, "vdevs": map[string]any{
				"/tmp/a": map[string]any{"name": "/tmp/a", "vdev_type": "file", "state": "ONLINE", "path": "/tmp/a"},
				"/tmp/b": map[string]any{"name": "/tmp/b", "vdev_type": "file", "state": "OFFLINE", "path": "/tmp/b"},
			}},
		}}},
	}}}
	data, _ := json.Marshal(doc)
	if scan != "" {
		return strings.Replace(string(data), `"name":"tank","state"`, `"scan_stats":`+scan+`,"name":"tank","state"`, 1)
	}
	return string(data)
}

func recent() string {
	return fmt.Sprintf(`{"function":"SCRUB","state":"FINISHED","start_time":%d,"end_time":%d}`, time.Now().Unix()-100, time.Now().Unix()-50)
}

func checkService(h *poolHost) (*Service, *recorder) {
	r := &recorder{}
	yes := true
	return New(Options{Runner: h, Installed: &yes, Notifier: r, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}), r
}

func TestCheckHealthyPoolReportsAllClear(t *testing.T) {
	h := &poolHost{list: "tank\t100\t10\t90\t10\tONLINE\n", status: statusDoc("ONLINE", 0, recent())}
	s, r := checkService(h)
	s.CheckPools(context.Background())
	want := []string{"pool:tank:capacity ok", "pool:tank:errors ok", "pool:tank:health ok", "pool:tank:scrub ok", "zpool:unavailable ok"}
	if got := r.said(); !slices.Equal(got, want) {
		t.Fatalf("%q", got)
	}
}

func TestCheckProblems(t *testing.T) {
	old := fmt.Sprintf(`{"function":"SCRUB","state":"FINISHED","start_time":1,"end_time":%d}`, time.Now().Add(-40*24*time.Hour).Unix())
	h := &poolHost{list: "tank\t100\t92\t8\t92\tDEGRADED\n", status: statusDoc("DEGRADED", 3, old)}
	s, r := checkService(h)
	s.CheckPools(context.Background())
	want := []string{"pool:tank:capacity", "pool:tank:errors", "pool:tank:health", "pool:tank:scrub", "zpool:unavailable ok"}
	if got := r.said(); !slices.Equal(got, want) {
		t.Fatalf("%q", got)
	}
	by := map[string]Event{}
	for _, e := range r.events {
		by[e.Key] = e
	}
	health := by["pool:tank:health"]
	if health.Level != "critical" || health.Title != "Pool tank is degraded" || health.Link != "/pool?name=tank" || !strings.Contains(health.Body, "/tmp/b: offline") {
		t.Fatalf("%+v", health)
	}
	if by["pool:tank:capacity"].Level != "critical" || by["pool:tank:scrub"].Level != "warning" || !strings.Contains(by["pool:tank:scrub"].Title, "40 days") {
		t.Fatalf("%+v", by)
	}
}

func TestCheckCapacityLevels(t *testing.T) {
	h := &poolHost{list: "tank\t100\t80\t20\t80\tONLINE\n", status: statusDoc("ONLINE", 0, recent())}
	s, r := checkService(h)
	s.CheckPools(context.Background())
	for _, e := range r.events {
		if e.Key == "pool:tank:capacity" && (e.Resolved || e.Level != "warning") {
			t.Fatalf("%+v", e)
		}
	}
}

func TestCheckNeverScrubbedCountsFromCreation(t *testing.T) {
	h := &poolHost{list: "tank\t100\t10\t90\t10\tONLINE\n", status: statusDoc("ONLINE", 0, "")}
	s, r := checkService(h)
	s.CheckPools(context.Background())
	if !slices.Contains(r.said(), "pool:tank:scrub") {
		t.Fatalf("%q", r.said())
	}
}

func TestCheckRunningScrubIsNotOverdue(t *testing.T) {
	running := `{"function":"SCRUB","state":"SCANNING","start_time":1,"end_time":0}`
	h := &poolHost{list: "tank\t100\t10\t90\t10\tONLINE\n", status: statusDoc("ONLINE", 0, running)}
	s, r := checkService(h)
	s.CheckPools(context.Background())
	if !slices.Contains(r.said(), "pool:tank:scrub ok") {
		t.Fatalf("%q", r.said())
	}
}

// What cannot be read must not be reported as fine: that would close real issues.
func TestCheckUnreadableReportsNothingAsFine(t *testing.T) {
	h := &poolHost{listErr: &system.ExitError{Command: "zpool", Code: 1, Output: "broken"}}
	s, r := checkService(h)
	s.CheckPools(context.Background())
	if got := r.said(); !slices.Equal(got, []string{"zpool:unavailable"}) {
		t.Fatalf("%q", got)
	}

	h = &poolHost{list: "tank\t100\t10\t90\t10\tONLINE\n", statusErr: &system.ExitError{Command: "zpool", Code: 2, Output: "invalid option 'j'"}}
	s, r = checkService(h)
	s.CheckPools(context.Background())
	for _, said := range r.said() {
		if strings.Contains(said, "errors") || strings.Contains(said, "scrub") {
			t.Fatalf("details reported without being read: %q", r.said())
		}
	}
}

func TestCheckClosesIssuesOfAPoolThatWentAway(t *testing.T) {
	h := &poolHost{list: "tank\t100\t10\t90\t10\tONLINE\n", status: statusDoc("ONLINE", 0, recent())}
	s, r := checkService(h)
	s.CheckPools(context.Background())
	r.events = nil
	h.list, h.status = "", `{"pools":{}}`
	s.CheckPools(context.Background())
	want := []string{"pool:tank:capacity ok", "pool:tank:errors ok", "pool:tank:health ok", "pool:tank:scrub ok", "zpool:unavailable ok"}
	if got := r.said(); !slices.Equal(got, want) {
		t.Fatalf("%q", got)
	}
	r.events = nil
	s.CheckPools(context.Background())
	if got := r.said(); !slices.Equal(got, []string{"zpool:unavailable ok"}) {
		t.Fatalf("closed again: %q", got)
	}
}
