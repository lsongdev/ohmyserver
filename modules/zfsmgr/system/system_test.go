package system

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fake answers from a table keyed by the command line, and records it.
type fake struct {
	out   map[string]string
	fail  map[string]string
	calls []string
}

func (f *fake) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	line := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, line)
	if msg, ok := f.fail[line]; ok {
		return nil, &ExitError{Command: name, Code: 1, Output: msg}
	}
	for key, out := range f.out {
		if strings.HasPrefix(line, key) {
			return []byte(out), nil
		}
	}
	return nil, nil
}

func (f *fake) Lines(ctx context.Context, line func(string) bool, name string, args ...string) error {
	out, err := f.Run(ctx, name, args...)
	if err != nil {
		return err
	}
	for l := range strings.Lines(string(out)) {
		if !line(strings.TrimRight(l, "\n")) {
			return nil
		}
	}
	return nil
}

func (f *fake) mutating() []string {
	var result []string
	for _, c := range f.calls {
		if !strings.HasPrefix(c, "zfs list") && !strings.HasPrefix(c, "zfs get") {
			result = append(result, c)
		}
	}
	return result
}

func TestSplitSnapshotRefusesWhatIsNotOneSnapshot(t *testing.T) {
	for _, bad := range []string{
		"data", "data/media", "", "@x", "data@", "data@a@b", "data@a,b", "data@a%b", "data@-r", "-r@x",
		"data@a/b", "data/@x", "/data@x", "data@../x", "data@ x", "data@a\nb",
	} {
		if _, _, err := SplitSnapshot(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("SplitSnapshot(%q) = %v, want invalid", bad, err)
		}
	}
	dataset, name, err := SplitSnapshot("data/my media@auto-daily-20261102-0000")
	if err != nil || dataset != "data/my media" || name != "auto-daily-20261102-0000" {
		t.Errorf("got %q %q %v", dataset, name, err)
	}
}

func TestDestroyNeverReachesZfsWithADatasetName(t *testing.T) {
	f := &fake{}
	z := &ZFS{Run: f}
	for _, bad := range []string{"data", "data/media", "data@a%b", "data@a,b", "-r"} {
		if err := z.Destroy(context.Background(), bad); err == nil {
			t.Errorf("Destroy(%q) succeeded", bad)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("ran %v", f.calls)
	}
	if err := z.Destroy(context.Background(), "data@old"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"zfs destroy data@old"}; !slices.Equal(f.calls, want) {
		t.Fatalf("ran %v, want %v", f.calls, want)
	}
}

const listing = "data@c\t300\t1\t2\t3\t0\t-\ndata@b\t200\t1\t2\t3\t1\tdata/clone\ndata@a\t100\t1\t2\t3\t0\t-\n"

func TestRollbackOnlyWithTheNewerSnapshotsTheCallerSaw(t *testing.T) {
	ctx := context.Background()
	newZ := func() (*ZFS, *fake) {
		f := &fake{out: map[string]string{"zfs list -Hp -t snapshot": listing}}
		return &ZFS{Run: f}, f
	}

	z, f := newZ()
	if err := z.Rollback(ctx, "data@a", []string{"data@c"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("partial list: %v", err)
	}
	if err := z.Rollback(ctx, "data@a", nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("no list: %v", err)
	}
	if got := f.mutating(); got != nil {
		t.Fatalf("ran %v", got)
	}

	z, f = newZ()
	if err := z.Rollback(ctx, "data@a", []string{"data@c", "data@b"}); err != nil {
		t.Fatal(err)
	}
	if got, want := f.mutating(), []string{"zfs rollback -r data@a"}; !slices.Equal(got, want) {
		t.Fatalf("ran %v, want %v", got, want)
	}

	z, f = newZ()
	if err := z.Rollback(ctx, "data@c", []string{}); err != nil {
		t.Fatal(err)
	}
	if got, want := f.mutating(), []string{"zfs rollback data@c"}; !slices.Equal(got, want) {
		t.Fatalf("ran %v, want %v", got, want)
	}
}

func TestParsePolicy(t *testing.T) {
	p, err := ParsePolicy("hourly=24,daily=14,weekly=0,monthly=6")
	if want := (Policy{Hourly: 24, Daily: 14, Monthly: 6}); err != nil || p != want {
		t.Fatalf("got %+v %v", p, err)
	}
	if p.String() != "hourly=24,daily=14,weekly=0,monthly=6" {
		t.Fatal(p.String())
	}
	for _, bad := range []string{"-", "", "hourly", "hourly=x", "yearly=1", "hourly=-1", "hourly=1001"} {
		if _, err := ParsePolicy(bad); err == nil {
			t.Errorf("ParsePolicy(%q) accepted", bad)
		}
	}
}

func TestPeriodStart(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 11, 1, 15, 30, 0, 0, loc) // a Sunday
	for class, want := range map[string]time.Time{
		"hourly":  time.Date(2026, 11, 1, 15, 0, 0, 0, loc),
		"daily":   time.Date(2026, 11, 1, 0, 0, 0, 0, loc),
		"weekly":  time.Date(2026, 10, 26, 0, 0, 0, 0, loc),
		"monthly": time.Date(2026, 11, 1, 0, 0, 0, 0, loc),
	} {
		if got := periodStart(class, now); !got.Equal(want) {
			t.Errorf("%s: %v, want %v", class, got, want)
		}
	}
}

func TestDiffDecodesAndLimits(t *testing.T) {
	f := &fake{out: map[string]string{"zfs diff": "M\t/\t/data/a\\0040b\n+\tF\t/data/\\0344\\0270\\0255\n-\tF\t/data/x\nR\tF\t/data/o\t/data/n\n"}}
	z := &ZFS{Run: f}
	changes, more, err := z.Diff(context.Background(), "data@a", 2)
	if err != nil || !more || len(changes) != 2 || changes[0].Path != "/data/a b" || changes[1].Path != "/data/中" {
		t.Fatalf("%+v %v %v", changes, more, err)
	}
	changes, more, err = z.Diff(context.Background(), "data@a", 10)
	if err != nil || more || len(changes) != 4 || changes[3].To != "/data/n" {
		t.Fatalf("%+v %v %v", changes, more, err)
	}
}

// schedulerFor builds a scheduler over one dataset with a policy value.
func schedulerFor(policy, snapshots string, fail map[string]string) (*Scheduler, *fake) {
	f := &fake{fail: fail, out: map[string]string{
		"zfs list -Hp -t filesystem,volume": "data\tfilesystem\t1\t1\t1\t0\t/data\tyes\tlz4\t0\n",
		"zfs get -Hp":                       "data\t" + policy + "\tlocal\n",
		"zfs list -Hp -t snapshot":          snapshots,
	}}
	return &Scheduler{ZFS: &ZFS{Run: f}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, f
}

func snapRow(name string, created time.Time, holds, clones string) string {
	return name + "\t" + strconv.FormatInt(created.Unix(), 10) + "\t1\t1\t1\t" + holds + "\t" + clones + "\n"
}

func TestSchedulerTakesDueAndPrunesOnlyAutoSnapshots(t *testing.T) {
	now := time.Date(2026, 11, 2, 15, 30, 0, 0, time.Local)
	h := func(ago int) time.Time { return now.Add(-time.Duration(ago) * time.Hour) }
	list := snapRow("data@manual-old", h(100), "0", "-") +
		snapRow("data@auto-hourly-20261102-1400", h(1), "0", "-") +
		snapRow("data@auto-hourly-20261102-1300", h(2), "0", "-") +
		snapRow("data@auto-hourly-20261102-1200", h(3), "1", "-") + // held
		snapRow("data@auto-hourly-20261102-1100", h(4), "0", "-") +
		snapRow("data@auto-monthly-20260101-0000", h(5000), "0", "-")
	s, f := schedulerFor("hourly=2,daily=0,weekly=0,monthly=0", list, nil)
	var out strings.Builder
	if err := s.Apply(context.Background(), now, &out); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"Took data@auto-hourly-20261102-1530", "Removed data@auto-hourly-20261102-1300", "Kept data@auto-hourly-20261102-1200"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("output lacks %q:\n%s", line, out.String())
		}
	}
	want := []string{
		"zfs snapshot data@auto-hourly-20261102-1530",
		"zfs destroy data@auto-hourly-20261102-1300",
		"zfs destroy data@auto-hourly-20261102-1100",
	}
	if got := f.mutating(); !slices.Equal(got, want) {
		t.Fatalf("ran %q\nwant %q", got, want)
	}
	if len(s.Errors()) != 0 {
		t.Fatalf("errors: %v", s.Errors())
	}
}

func TestSchedulerSkipsWhatIsNotDue(t *testing.T) {
	now := time.Date(2026, 11, 2, 15, 30, 0, 0, time.Local)
	list := snapRow("data@auto-hourly-20261102-1500", now.Add(-20*time.Minute), "0", "-")
	s, f := schedulerFor("hourly=24,daily=0,weekly=0,monthly=0", list, nil)
	s.Apply(context.Background(), now, io.Discard)
	if got := f.mutating(); got != nil {
		t.Fatalf("ran %v", got)
	}
}

func TestSchedulerNeverPrunesWhenItCouldNotSnapshot(t *testing.T) {
	now := time.Date(2026, 11, 2, 15, 30, 0, 0, time.Local)
	list := snapRow("data@auto-hourly-20261102-1400", now.Add(-time.Hour), "0", "-") +
		snapRow("data@auto-hourly-20261102-1300", now.Add(-2*time.Hour), "0", "-")
	fail := map[string]string{"zfs snapshot data@auto-hourly-20261102-1530": "out of space"}
	s, f := schedulerFor("hourly=1,daily=0,weekly=0,monthly=0", list, fail)
	if err := s.Apply(context.Background(), now, io.Discard); err == nil {
		t.Fatal("a failed snapshot must fail the pass")
	}
	if got, want := f.mutating(), []string{"zfs snapshot data@auto-hourly-20261102-1530"}; !slices.Equal(got, want) {
		t.Fatalf("ran %v", got)
	}
	if s.Errors()["data"] != "out of space" {
		t.Fatalf("errors: %v", s.Errors())
	}
}

func TestSchedulerLeavesEverythingAloneWithAnUnreadablePolicy(t *testing.T) {
	now := time.Date(2026, 11, 2, 15, 30, 0, 0, time.Local)
	list := snapRow("data@auto-hourly-20261102-1400", now.Add(-time.Hour), "0", "-")
	s, f := schedulerFor("hourly=lots", list, nil)
	s.Apply(context.Background(), now, io.Discard)
	if got := f.mutating(); got != nil {
		t.Fatalf("ran %v", got)
	}
	if !strings.Contains(s.Errors()["data"], "unreadable") {
		t.Fatalf("errors: %v", s.Errors())
	}
}

func createFake() *fake {
	return &fake{out: map[string]string{
		"zfs list -Hp -t filesystem,volume": "data\tfilesystem\t1\t1\t1\t0\t/mnt\tyes\tlz4\t0\ndata/vol\tvolume\t1\t1\t1\t0\tnone\t-\tlz4\t-\n",
	}}
}

func TestCreateDataset(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "mnt/empty"), 0o755)
	os.MkdirAll(filepath.Join(root, "mnt/full"), 0o755)
	os.WriteFile(filepath.Join(root, "mnt/full/file"), nil, 0o644)

	f := createFake()
	z := &ZFS{Run: f, Root: root}
	full, err := z.CreateDataset(ctx, NewDataset{Parent: "data", Name: "media"})
	if err != nil || full != "data/media" {
		t.Fatalf("%q %v", full, err)
	}
	if _, err := z.CreateDataset(ctx, NewDataset{Parent: "data", Name: "empty", Mountpoint: "/srv/x", Compression: "zstd", Quota: "2T"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"zfs create data/media", "zfs create -o mountpoint=/srv/x -o compression=zstd -o quota=2T data/empty"}
	if got := f.mutating(); !slices.Equal(got, want) {
		t.Fatalf("ran %q", got)
	}

	f = createFake()
	z = &ZFS{Run: f, Root: root}
	for name, bad := range map[string]NewDataset{
		"folder with files":  {Parent: "data", Name: "full"},
		"chosen mountpoint":  {Parent: "data", Name: "x", Mountpoint: "/mnt/full"},
		"under a volume":     {Parent: "data/vol", Name: "x"},
		"parent missing":     {Parent: "nope", Name: "x"},
		"nested name":        {Parent: "data", Name: "a/b"},
		"option in the name": {Parent: "data", Name: "-o"},
		"relative mount":     {Parent: "data", Name: "x", Mountpoint: "srv"},
		"unclean mount":      {Parent: "data", Name: "x", Mountpoint: "/srv/../etc"},
		"unknown compress":   {Parent: "data", Name: "x", Compression: "-o"},
		"odd quota":          {Parent: "data", Name: "x", Quota: "2T,x"},
	} {
		if _, err := z.CreateDataset(ctx, bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if got := f.mutating(); got != nil {
		t.Fatalf("ran %q", got)
	}
}

func TestScrub(t *testing.T) {
	ctx := context.Background()
	f := &fake{}
	z := &ZFS{Run: f}
	for _, bad := range []string{"", "-s", "data/media", "data@x", "1data"} {
		if err := z.Scrub(ctx, bad, true); err == nil {
			t.Errorf("Scrub(%q) accepted", bad)
		}
	}
	if err := z.Scrub(ctx, "data", true); err != nil {
		t.Fatal(err)
	}
	if err := z.Scrub(ctx, "data", false); err != nil {
		t.Fatal(err)
	}
	if want := []string{"zpool scrub data", "zpool scrub -s data"}; !slices.Equal(f.calls, want) {
		t.Fatalf("ran %q", f.calls)
	}
}
