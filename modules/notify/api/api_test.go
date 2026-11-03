package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/modules/notify/api"
	"github.com/lsongdev/ohmyserver/modules/notify/store"
	_ "modernc.org/sqlite"
)

type env struct {
	t   *testing.T
	s   *api.Service
	now time.Time
}

func setup(t *testing.T) *env {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "n.db")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	st, err := store.New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	e.s = api.NewService(st, api.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return e.now }})
	return e
}

func (e *env) notify(ev api.Event) {
	e.t.Helper()
	if err := e.s.Notify(context.Background(), "zfs", ev); err != nil {
		e.t.Fatal(err)
	}
}

// kinds lists the events oldest first, as "kind level".
func (e *env) kinds() []string {
	e.t.Helper()
	list, err := e.s.Events(context.Background(), store.EventQuery{})
	if err != nil {
		e.t.Fatal(err)
	}
	var out []string
	for i := len(list) - 1; i >= 0; i-- {
		out = append(out, list[i].Kind+" "+list[i].Level)
	}
	return out
}

func (e *env) open() []store.Issue {
	e.t.Helper()
	list, err := e.s.Issues(context.Background(), store.StateOpen)
	if err != nil {
		e.t.Fatal(err)
	}
	return list
}

func (e *env) expect(want ...string) {
	e.t.Helper()
	if got := e.kinds(); strings.Join(got, ",") != strings.Join(want, ",") {
		e.t.Fatalf("events %q, want %q", got, want)
	}
}

var bad = api.Event{Level: "critical", Title: "Pool degraded", Key: "pool:data:health", Link: "/pool?name=data"}
var good = api.Event{Key: "pool:data:health", Resolved: true}

func TestMessageIsOneEvent(t *testing.T) {
	e := setup(t)
	e.notify(api.Event{Level: "info", Title: "Done"})
	e.notify(api.Event{Level: "info", Title: "Done"})
	e.expect("message info", "message info")
	if len(e.open()) != 0 {
		t.Fatal("a message opened an issue")
	}
}

func TestRepeatsAreQuiet(t *testing.T) {
	e := setup(t)
	e.notify(good) // healthy from the start, on every check: nothing at all
	e.expect()
	e.notify(bad)
	for range 5 {
		e.now = e.now.Add(30 * time.Second)
		e.notify(bad)
	}
	e.expect("opened critical")
	i := e.open()
	if len(i) != 1 || i[0].Occurrences != 6 || !i[0].LastSeen.After(i[0].FirstSeen) {
		t.Fatalf("%+v", i)
	}
}

func TestLevelChanges(t *testing.T) {
	e := setup(t)
	e.notify(api.Event{Level: "warning", Title: "80% full", Key: "cap"})
	e.notify(api.Event{Level: "critical", Title: "90% full", Key: "cap"})
	e.notify(api.Event{Level: "warning", Title: "85% full", Key: "cap"}) // easing is not news
	e.expect("opened warning", "escalated critical")
	if i := e.open(); i[0].Level != "warning" || i[0].Title != "85% full" {
		t.Fatalf("%+v", i[0])
	}
}

func TestResolvedIsAnnouncedAfterTheWindow(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.notify(bad)
	e.now = e.now.Add(time.Minute)
	e.notify(good)
	e.notify(good) // repeated
	if len(e.open()) != 0 {
		t.Fatal("still open")
	}
	e.now = e.now.Add(4 * time.Minute)
	e.s.Flush(ctx)
	e.expect("opened critical")
	e.now = e.now.Add(2 * time.Minute)
	e.s.Flush(ctx)
	e.s.Flush(ctx)
	e.expect("opened critical", "resolved info")
	list, _ := e.s.Events(ctx, store.EventQuery{})
	if want := time.Date(2026, 11, 3, 12, 1, 0, 0, time.UTC); !list[0].Created.Equal(want) {
		t.Fatalf("dated %v, want when it was resolved", list[0].Created)
	}
}

func TestFlappingStaysQuiet(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.notify(bad)
	for range 10 {
		e.now = e.now.Add(time.Minute)
		e.notify(good)
		e.s.Flush(ctx)
		e.now = e.now.Add(time.Minute)
		e.notify(bad)
		e.s.Flush(ctx)
	}
	e.expect("opened critical")
	if i := e.open(); len(i) != 1 || i[0].Occurrences != 11 {
		t.Fatalf("%+v", i)
	}
}

func TestBackAfterTheWindowOpensAgain(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.notify(bad)
	e.notify(good)
	e.now = e.now.Add(10 * time.Minute)
	e.s.Flush(ctx)
	e.now = e.now.Add(time.Hour)
	e.notify(bad)
	e.expect("opened critical", "resolved info", "opened critical")
	i := e.open()
	if len(i) != 1 || i[0].Occurrences != 1 || !i[0].FirstSeen.Equal(e.now) || i[0].ResolvedAt != nil {
		t.Fatalf("%+v", i)
	}
}

func TestClosingByHand(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.notify(bad)
	id := e.open()[0].ID
	if err := e.s.CloseIssue(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := e.s.CloseIssue(ctx, id); err != nil {
		t.Fatal("closing twice should be fine:", err)
	}
	e.expect("opened critical", "resolved info")
	if err := e.s.CloseIssue(ctx, 999); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	e.notify(bad) // the source still says so
	if len(e.open()) != 1 {
		t.Fatal("did not reopen")
	}
}

func TestSameKeyFromAnotherSourceIsAnotherIssue(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.notify(bad)
	if err := e.s.Notify(ctx, "disks", bad); err != nil {
		t.Fatal(err)
	}
	if len(e.open()) != 2 {
		t.Fatalf("%+v", e.open())
	}
}

func TestInvalid(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	for name, ev := range map[string]api.Event{
		"no level":        {Title: "x"},
		"unknown level":   {Level: "fatal", Title: "x"},
		"no title":        {Level: "info", Title: "  "},
		"resolved no key": {Resolved: true},
		"outside link":    {Level: "info", Title: "x", Link: "https://evil.example"},
		"network path":    {Level: "info", Title: "x", Link: "//evil.example"},
		"long key":        {Level: "info", Title: "x", Key: strings.Repeat("k", 201)},
	} {
		if err := e.s.Notify(ctx, "zfs", ev); !errors.Is(err, api.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := e.s.Notify(ctx, "", api.Event{Level: "info", Title: "x"}); !errors.Is(err, api.ErrInvalid) {
		t.Error(err)
	}
	e.expect()
	e.notify(api.Event{Level: "info", Title: strings.Repeat("中", 200)}) // cut, on a character
	list, _ := e.s.Events(ctx, store.EventQuery{})
	if !strings.HasSuffix(list[0].Title, "…") || strings.ContainsRune(list[0].Title, '�') {
		t.Fatalf("%q", list[0].Title)
	}
}

func TestPrune(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.notify(api.Event{Level: "debug", Title: "old debug"})
	e.notify(api.Event{Level: "critical", Title: "old critical"})
	e.notify(api.Event{Level: "info", Title: "old info of an open issue", Key: "open"})
	e.notify(api.Event{Level: "info", Title: "closed long ago", Key: "closed"})
	e.notify(api.Event{Key: "closed", Resolved: true})
	e.now = e.now.Add(10 * time.Minute)
	e.s.Flush(ctx)
	e.now = e.now.Add(40 * 24 * time.Hour)
	e.notify(api.Event{Level: "debug", Title: "new debug"})
	if err := e.s.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	list, _ := e.s.Events(ctx, store.EventQuery{})
	var titles []string
	for _, ev := range list {
		titles = append(titles, ev.Title)
	}
	if got := strings.Join(titles, "|"); got != "new debug|old info of an open issue|old critical" {
		t.Fatalf("%s", got)
	}
	// Past the longest age, a resolved issue goes too; an open one stays.
	e.now = e.now.Add(400 * 24 * time.Hour)
	e.s.Prune(ctx)
	resolved, _ := e.s.Issues(ctx, store.StateResolved)
	if len(resolved) != 0 || len(e.open()) != 1 {
		t.Fatalf("resolved %+v, open %+v", resolved, e.open())
	}
}

func TestEventFiltersAndPages(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.notify(api.Event{Level: "info", Title: "first 100%"})
	e.notify(api.Event{Level: "warning", Title: "second", Body: "disk sdb"})
	if err := e.s.Notify(ctx, "cron", api.Event{Level: "info", Title: "third"}); err != nil {
		t.Fatal(err)
	}
	count := func(q store.EventQuery) int {
		list, err := e.s.Events(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		return len(list)
	}
	if count(store.EventQuery{Level: "info"}) != 2 || count(store.EventQuery{Source: "cron"}) != 1 ||
		count(store.EventQuery{Search: "SDB"}) != 1 || count(store.EventQuery{Search: "100%"}) != 1 || count(store.EventQuery{Search: "%"}) != 1 {
		t.Fatal("filters")
	}
	all, _ := e.s.Events(ctx, store.EventQuery{})
	if count(store.EventQuery{Before: all[0].ID}) != 2 || count(store.EventQuery{Limit: 1}) != 1 {
		t.Fatal("pages")
	}
}

func TestHandler(t *testing.T) {
	e := setup(t)
	e.notify(bad)
	e.notify(api.Event{Level: "warning", Title: "Full", Key: "cap"})
	srv := httptest.NewServer(e.s.Handler())
	defer srv.Close()
	get := func(path string, out any) int {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var doc struct{ Data json.RawMessage }
		json.NewDecoder(res.Body).Decode(&doc)
		if out != nil {
			json.Unmarshal(doc.Data, out)
		}
		return res.StatusCode
	}
	var sum api.Summary
	if get("/summary", &sum) != 200 || sum.Open != 2 || sum.Critical != 1 || sum.Warning != 1 || sum.Issues[0].Level != "critical" {
		t.Fatalf("%+v", sum)
	}
	var sources []string
	if get("/sources", &sources) != 200 || len(sources) != 1 || sources[0] != "zfs" {
		t.Fatal(sources)
	}
	if get("/issues?state=bad", nil) != 400 {
		t.Fatal("state")
	}
	res, err := http.Post(srv.URL+"/issues/"+"1/resolve", "application/json", nil)
	if err != nil || res.StatusCode != 200 {
		t.Fatal(res, err)
	}
	var open []store.Issue
	if get("/issues", &open); len(open) != 1 {
		t.Fatalf("%+v", open)
	}
}
