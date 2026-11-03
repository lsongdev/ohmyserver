package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/modules/notify/api"
	"github.com/lsongdev/ohmyserver/modules/notify/channels"
	"github.com/lsongdev/ohmyserver/modules/notify/store"
)

// receiver records what reaches it, and fails while told to.
type receiver struct {
	mu   sync.Mutex
	got  []map[string]any
	fail bool
	srv  *httptest.Server
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()
	r := &receiver{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		data, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.fail {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		var m map[string]any
		json.Unmarshal(data, &m)
		r.got = append(r.got, m)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) titles() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, m := range r.got {
		out = append(out, fmt.Sprint(m["kind"], ":", m["title"]))
	}
	return strings.Join(out, "|")
}

func (r *receiver) setFail(fail bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fail = fail
}

func (e *env) channel(r *receiver, change func(*api.ChannelInput)) api.ChannelView {
	e.t.Helper()
	in := api.ChannelInput{Name: "hook" + fmt.Sprint(time.Now().UnixNano()), Type: "webhook", Preset: "json", URL: r.srv.URL + "/secret-path", MinLevel: "warning", Enabled: true}
	if change != nil {
		change(&in)
	}
	v, err := e.s.CreateChannel(context.Background(), in)
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}

func (e *env) states() string {
	e.t.Helper()
	list, err := e.s.Deliveries(context.Background(), store.DeliveryQuery{Limit: 500})
	if err != nil {
		e.t.Fatal(err)
	}
	var out []string
	for i := len(list) - 1; i >= 0; i-- {
		out = append(out, list[i].State)
	}
	return strings.Join(out, ",")
}

func TestOnlyWhatTheRuleWantsGoesOut(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	r := newReceiver(t)
	e.channel(r, nil)
	cronOnly := newReceiver(t)
	e.channel(cronOnly, func(in *api.ChannelInput) { in.MinLevel = "info"; in.Sources = []string{"cron"} })
	off := newReceiver(t)
	e.channel(off, func(in *api.ChannelInput) { in.Enabled = false })

	e.notify(api.Event{Level: "info", Title: "quiet"})
	e.notify(api.Event{Level: "warning", Title: "loud"})
	if err := e.s.Notify(ctx, "cron", api.Event{Level: "info", Title: "cron says"}); err != nil {
		t.Fatal(err)
	}
	e.s.Dispatch(ctx)
	if got := r.titles(); got != "message:loud" {
		t.Fatalf("default rule: %s", got)
	}
	if got := cronOnly.titles(); got != "message:cron says" {
		t.Fatalf("source rule: %s", got)
	}
	if off.titles() != "" {
		t.Fatal("a channel turned off got something")
	}
}

func TestIssueGoesOutOnceAndItsResolutionFollows(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	r := newReceiver(t)
	e.channel(r, nil)
	e.notify(bad)
	e.notify(bad)
	e.notify(bad)
	e.s.Dispatch(ctx)
	if got := r.titles(); got != "opened:Pool degraded" {
		t.Fatal(got)
	}
	e.notify(good)
	e.s.Flush(ctx)
	e.s.Dispatch(ctx)
	if got := r.titles(); got != "opened:Pool degraded" {
		t.Fatalf("announced within the window: %s", got)
	}
	e.now = e.now.Add(6 * time.Minute)
	e.s.Flush(ctx)
	e.s.Dispatch(ctx)
	if got := r.titles(); got != "opened:Pool degraded|resolved:Pool degraded" {
		t.Fatal(got)
	}
}

// A resolution is info, below this channel's rule, but it was told of the problem.
// One added since was not, and gets no "resolved" out of the blue.
func TestResolutionGoesToWhoeverWasTold(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	told := newReceiver(t)
	e.channel(told, func(in *api.ChannelInput) { in.MinLevel = "critical" })
	e.notify(bad)
	e.s.Dispatch(ctx)
	later := newReceiver(t)
	e.channel(later, nil)
	e.notify(good)
	e.now = e.now.Add(6 * time.Minute)
	e.s.Flush(ctx)
	e.s.Dispatch(ctx)
	if !strings.Contains(told.titles(), "resolved:") || later.titles() != "" {
		t.Fatalf("told %s, later %s", told.titles(), later.titles())
	}
}

func TestFlappingSendsNothingMore(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	r := newReceiver(t)
	e.channel(r, nil)
	e.notify(bad)
	for range 5 {
		e.now = e.now.Add(time.Minute)
		e.notify(good)
		e.s.Flush(ctx)
		e.s.Dispatch(ctx)
		e.now = e.now.Add(time.Minute)
		e.notify(bad)
		e.s.Dispatch(ctx)
	}
	if got := r.titles(); got != "opened:Pool degraded" {
		t.Fatal(got)
	}
}

func TestEscalationGoesOut(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	r := newReceiver(t)
	e.channel(r, nil)
	e.notify(api.Event{Level: "warning", Title: "80%", Key: "cap"})
	e.notify(api.Event{Level: "critical", Title: "90%", Key: "cap"})
	e.notify(api.Event{Level: "warning", Title: "85%", Key: "cap"})
	e.s.Dispatch(ctx)
	if got := r.titles(); got != "opened:80%|escalated:90%" {
		t.Fatal(got)
	}
}

func TestMutedIssueIsRecordedButNotSent(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	r := newReceiver(t)
	e.channel(r, nil)
	e.notify(api.Event{Level: "warning", Title: "80%", Key: "cap"})
	e.s.Dispatch(ctx)
	id := e.open()[0].ID
	until := e.now.Add(time.Hour)
	if err := e.s.MuteIssue(ctx, id, &until); err != nil {
		t.Fatal(err)
	}
	e.notify(api.Event{Level: "critical", Title: "90%", Key: "cap"})
	e.s.Dispatch(ctx)
	if got := r.titles(); got != "opened:80%" {
		t.Fatal(got)
	}
	e.expect("opened warning", "escalated critical")
	e.now = e.now.Add(2 * time.Hour)
	e.notify(api.Event{Level: "warning", Title: "85%", Key: "cap"})
	e.notify(api.Event{Key: "cap", Resolved: true})
	e.now = e.now.Add(10 * time.Minute)
	e.s.Flush(ctx)
	e.s.Dispatch(ctx)
	if got := r.titles(); got != "opened:80%|resolved:85%" {
		t.Fatal(got)
	}
}

func TestFailuresRetryWithBackoffAndNeverMakeEvents(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	r := newReceiver(t)
	v := e.channel(r, nil)
	r.setFail(true)
	e.notify(api.Event{Level: "critical", Title: "first"})
	e.notify(api.Event{Level: "critical", Title: "second"})
	e.s.Dispatch(ctx)
	e.s.Dispatch(ctx) // not yet due
	list, _ := e.s.Deliveries(ctx, store.DeliveryQuery{Limit: 10})
	if len(list) != 2 || list[1].Attempts != 1 || list[0].Attempts != 0 || !strings.Contains(list[1].Error, "503") {
		t.Fatalf("%+v", list) // one attempt: the second waits with it, without its own timeout
	}
	view, _ := e.s.Channel(ctx, v.ID)
	if !strings.Contains(view.LastError, "503") || view.LastErrorAt == nil {
		t.Fatalf("%+v", view)
	}
	e.expect("message critical", "message critical") // no event about the failure
	r.setFail(false)
	e.now = e.now.Add(31 * time.Second)
	e.s.Dispatch(ctx)
	if got := r.titles(); got != "message:first|message:second" {
		t.Fatal(got)
	}
	view, _ = e.s.Channel(ctx, v.ID)
	if view.LastError != "" || view.LastSuccessAt == nil {
		t.Fatalf("%+v", view)
	}
	if got := e.states(); got != "sent,sent" {
		t.Fatal(got)
	}
}

func TestGivesUpAfterADayAndCanBeRetried(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	r := newReceiver(t)
	e.channel(r, nil)
	r.setFail(true)
	e.notify(api.Event{Level: "critical", Title: "x"})
	for range 30 {
		e.s.Dispatch(ctx)
		e.now = e.now.Add(time.Hour)
	}
	if got := e.states(); got != "failed" {
		t.Fatal(got)
	}
	list, _ := e.s.Deliveries(ctx, store.DeliveryQuery{Limit: 1})
	r.setFail(false)
	if err := e.s.RetryDelivery(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	e.s.Dispatch(ctx)
	if got := r.titles(); got != "message:x" || e.states() != "sent" {
		t.Fatalf("%s %s", got, e.states())
	}
	if err := e.s.RetryDelivery(ctx, list[0].ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("retried what was sent: %v", err)
	}
}

func TestFloodBecomesOneSummary(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	r := newReceiver(t)
	e.channel(r, nil)
	for i := range 25 {
		level := "warning"
		if i == 24 {
			level = "critical"
		}
		e.notify(api.Event{Level: level, Title: fmt.Sprint("event ", i)})
	}
	e.s.Dispatch(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.got) != 1 || r.got[0]["kind"] != "summary" || r.got[0]["level"] != "critical" ||
		!strings.Contains(r.got[0]["title"].(string), "25 events: 1 critical, 24 warning") ||
		!strings.HasPrefix(r.got[0]["body"].(string), "• event 24") { // the worst first
		t.Fatalf("%+v", r.got)
	}
}

func TestRateLimitPostponesTheRest(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	r := newReceiver(t)
	e.channel(r, nil)
	send := func(n int) {
		for i := range n {
			e.notify(api.Event{Level: "warning", Title: fmt.Sprint("e", i)})
		}
		e.s.Dispatch(ctx)
	}
	send(15)
	send(15) // 30 in all: 20 go now, 10 wait
	if n := strings.Count(r.titles(), "message:"); n != 20 {
		t.Fatalf("%d sent", n)
	}
	e.now = e.now.Add(61 * time.Second)
	e.s.Dispatch(ctx)
	if n := strings.Count(r.titles(), "message:"); n != 30 {
		t.Fatalf("%d sent", n)
	}
}

func TestTurningAChannelOffDropsWhatWaits(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	r := newReceiver(t)
	v := e.channel(r, nil)
	e.notify(api.Event{Level: "warning", Title: "x"})
	if _, err := e.s.UpdateChannel(ctx, v.ID, api.ChannelInput{Name: v.Name, Type: "webhook", Preset: "json", MinLevel: "warning", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	e.s.Dispatch(ctx)
	if r.titles() != "" || e.states() != "dropped" {
		t.Fatalf("%s %s", r.titles(), e.states())
	}
}

func TestSecretsStayIn(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	r := newReceiver(t)
	v := e.channel(r, func(in *api.ChannelInput) { in.Token = "tok-secret" })
	data, _ := json.Marshal(v)
	if s := string(data); strings.Contains(s, "secret-path") || strings.Contains(s, "tok-secret") || !v.HasToken {
		t.Fatal(s)
	}
	// Saving with them empty keeps them.
	if _, err := e.s.UpdateChannel(ctx, v.ID, api.ChannelInput{Name: "renamed", Type: "webhook", Preset: "json", MinLevel: "warning", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := e.s.TestChannel(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.titles(), "test:Test from OhMyServer") {
		t.Fatal(r.titles())
	}
	// Another type keeps nothing of the old: its URL is not a Telegram token.
	if _, err := e.s.UpdateChannel(ctx, v.ID, api.ChannelInput{Name: "x", Type: "telegram", ChatID: "1", MinLevel: "warning"}); err == nil {
		t.Fatal("kept a secret across types")
	}
}

func TestTestChannelSaysWhyNot(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	r := newReceiver(t)
	v := e.channel(r, nil)
	r.setFail(true)
	err := e.s.TestChannel(ctx, v.ID)
	if !errors.Is(err, api.ErrSendFailed) || !strings.Contains(err.Error(), "503") {
		t.Fatal(err)
	}
	e.expect() // and no event of it
}

func TestChannelValidation(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	r := newReceiver(t)
	v := e.channel(r, nil)
	for name, in := range map[string]api.ChannelInput{
		"no name":      {Type: "webhook", Preset: "json", URL: "https://a", MinLevel: "warning"},
		"bad level":    {Name: "a", Type: "webhook", Preset: "json", URL: "https://a", MinLevel: "loud"},
		"bad url":      {Name: "a", Type: "webhook", Preset: "json", URL: "ftp://a", MinLevel: "warning"},
		"bad source":   {Name: "a", Type: "webhook", Preset: "json", URL: "https://a", MinLevel: "warning", Sources: []string{"a,b"}},
		"unknown type": {Name: "a", Type: "sms", MinLevel: "warning"},
	} {
		if _, err := e.s.CreateChannel(ctx, in); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := e.s.CreateChannel(ctx, api.ChannelInput{Name: v.Name, Type: "webhook", Preset: "json", URL: "https://a", MinLevel: "warning"}); !errors.Is(err, store.ErrNameTaken) {
		t.Fatal(err)
	}
}

// A stale "resolved" must never arrive after a newer "opened": it would say the
// problem is over when it is not.
func TestMessagesStayInOrderThroughFailures(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	r := newReceiver(t)
	e.channel(r, nil)
	e.notify(bad)
	e.s.Dispatch(ctx)
	e.notify(good)
	e.now = e.now.Add(6 * time.Minute)
	e.s.Flush(ctx)
	r.setFail(true)
	e.s.Dispatch(ctx) // the resolution fails once
	e.now = e.now.Add(31 * time.Second)
	e.s.Dispatch(ctx) // and again: it now waits two minutes
	e.notify(bad)     // back, past the window: a new message, with no wait of its own
	e.s.Dispatch(ctx)
	r.setFail(false)
	e.s.Dispatch(ctx)
	if got := r.titles(); got != "opened:Pool degraded" {
		t.Fatalf("overtook the waiting resolution: %s", got)
	}
	e.now = e.now.Add(3 * time.Minute)
	e.s.Dispatch(ctx)
	if got := r.titles(); got != "opened:Pool degraded|resolved:Pool degraded|opened:Pool degraded" {
		t.Fatal(got)
	}
}

func TestDiscoverTelegramUsesASavedToken(t *testing.T) {
	ctx := context.Background()
	var asked []string
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			io.WriteString(w, `{"ok":true,"result":{"username":"nas_bot"}}`)
			return
		}
		io.WriteString(w, `{"ok":true,"result":[]}`)
	}))
	defer tg.Close()
	e := setupWith(t, api.Options{Sending: channels.Options{TelegramAPI: tg.URL}})
	v, err := e.s.CreateChannel(ctx, api.ChannelInput{Name: "tg", Type: "telegram", Token: "1:saved", ChatID: "5", MinLevel: "warning", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	found, err := e.s.DiscoverTelegram(ctx, "", v.ID)
	if err != nil || found.Bot.Username != "nas_bot" || found.Chats == nil || !strings.Contains(asked[0], "1:saved") {
		t.Fatalf("%+v %v %v", found, err, asked)
	}
	if _, err := e.s.DiscoverTelegram(ctx, "", 0); !errors.Is(err, api.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := e.s.DiscoverTelegram(ctx, "malformed", 0); !errors.Is(err, channels.ErrInvalid) {
		t.Fatal(err)
	}
}
