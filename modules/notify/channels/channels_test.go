package channels

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var sample = Message{Source: "zfs", Level: "critical", Kind: "opened", Title: "Pool data 已降级", Body: "sdb: offline", Time: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}

type captured struct {
	path, auth, title, priority string
	body                        string
}

func serve(t *testing.T, status int, answer string) (*httptest.Server, *captured) {
	t.Helper()
	got := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		got.path, got.auth, got.title, got.priority, got.body = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Title"), r.Header.Get("Priority"), string(data)
		w.WriteHeader(status)
		io.WriteString(w, answer)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func send(t *testing.T, typ string, c Config, opts Options, m Message) error {
	t.Helper()
	s, err := New(typ, c, opts)
	if err != nil {
		t.Fatal(err)
	}
	return s.Send(context.Background(), m)
}

func TestPresetsSendWhatTheServicesExpect(t *testing.T) {
	for preset, check := range map[string]func(map[string]any) bool{
		PresetJSON: func(b map[string]any) bool {
			return b["title"] == sample.Title && b["level"] == "critical" && b["source"] == "zfs"
		},
		PresetFeishu: func(b map[string]any) bool {
			return b["msg_type"] == "text" && strings.Contains(b["content"].(map[string]any)["text"].(string), "[CRITICAL] Pool data")
		},
		PresetWeCom: func(b map[string]any) bool {
			return b["msgtype"] == "text" && strings.Contains(b["text"].(map[string]any)["content"].(string), "sdb: offline")
		},
		PresetBark: func(b map[string]any) bool {
			return b["title"] == sample.Title && b["body"] == sample.Body && b["level"] == "timeSensitive"
		},
	} {
		srv, got := serve(t, 200, `{"code":0,"errcode":0}`)
		if preset == PresetBark {
			srv, got = serve(t, 200, `{"code":200}`)
		}
		if err := send(t, TypeWebhook, Config{Preset: preset, URL: srv.URL + "/hook"}, Options{}, sample); err != nil {
			t.Errorf("%s: %v", preset, err)
			continue
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(got.body), &body); err != nil || !check(body) {
			t.Errorf("%s sent %s", preset, got.body)
		}
	}
}

func TestNtfy(t *testing.T) {
	srv, got := serve(t, 200, `{}`)
	if err := send(t, TypeWebhook, Config{Preset: PresetNtfy, URL: srv.URL + "/topic", Token: "tk"}, Options{}, sample); err != nil {
		t.Fatal(err)
	}
	title, _ := new(mime.WordDecoder).DecodeHeader(got.title)
	if title != sample.Title || got.priority != "5" || got.auth != "Bearer tk" || !strings.HasPrefix(got.body, "sdb") {
		t.Fatalf("%+v", got)
	}
}

func TestTelegram(t *testing.T) {
	srv, got := serve(t, 200, `{"ok":true}`)
	if err := send(t, TypeTelegram, Config{Token: "123:abc", ChatID: "-100"}, Options{TelegramAPI: srv.URL}, sample); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	json.Unmarshal([]byte(got.body), &body)
	if got.path != "/bot123:abc/sendMessage" || body["chat_id"] != "-100" || !strings.Contains(body["text"].(string), "Pool data") {
		t.Fatalf("%+v", got)
	}
}

func TestResolvedText(t *testing.T) {
	m := sample
	m.Kind, m.Level, m.Body = "resolved", "info", ""
	if text := m.Text(); !strings.HasPrefix(text, "[RESOLVED] Pool data") || !strings.Contains(text, "— zfs") {
		t.Fatal(text)
	}
}

func TestRefusalsAtStatus200AreFailures(t *testing.T) {
	for preset, answer := range map[string]string{
		PresetFeishu: `{"code":19021,"msg":"sign match fail"}`,
		PresetWeCom:  `{"errcode":93000,"errmsg":"invalid webhook url"}`,
		PresetBark:   `{"code":400,"message":"bad key"}`,
	} {
		srv, _ := serve(t, 200, answer)
		if err := send(t, TypeWebhook, Config{Preset: preset, URL: srv.URL}, Options{}, sample); err == nil {
			t.Errorf("%s: refusal not noticed", preset)
		}
	}
	srv, _ := serve(t, 200, `{"code":5}`)
	if err := send(t, TypeWebhook, Config{Preset: PresetJSON, URL: srv.URL}, Options{}, sample); err != nil {
		t.Errorf("a generic hook's answer means nothing here: %v", err)
	}
	srv, _ = serve(t, 200, `not json`)
	if err := send(t, TypeWebhook, Config{Preset: PresetFeishu, URL: srv.URL}, Options{}, sample); err != nil {
		t.Errorf("%v", err)
	}
}

func TestErrorsNeverContainSecrets(t *testing.T) {
	const secret = "s3cret-key"
	srv, _ := serve(t, 500, "bad request to /hook/"+secret)
	err := send(t, TypeWebhook, Config{Preset: PresetJSON, URL: srv.URL + "/hook/" + secret}, Options{}, sample)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("%v", err)
	}
	srv.Close()
	err = send(t, TypeWebhook, Config{Preset: PresetJSON, URL: srv.URL + "/hook/" + secret}, Options{}, sample)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("leaked: %v", err)
	}
	err = send(t, TypeTelegram, Config{Token: "123:" + secret, ChatID: "1"}, Options{TelegramAPI: srv.URL}, sample)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("leaked: %v", err)
	}
	s := Config{Preset: PresetJSON, URL: "https://hooks.example/x/" + secret}.Target(TypeWebhook)
	if strings.Contains(s, secret) || s != "https://hooks.example/…" {
		t.Fatal(s)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	followed := false
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { followed = true }))
	defer elsewhere.Close()
	srv := httptest.NewServer(http.RedirectHandler(elsewhere.URL, http.StatusTemporaryRedirect))
	defer srv.Close()
	if err := send(t, TypeWebhook, Config{Preset: PresetJSON, URL: srv.URL}, Options{}, sample); err == nil || followed {
		t.Fatalf("followed=%v err=%v", followed, err)
	}
}

func TestValidate(t *testing.T) {
	for name, bad := range map[string]struct {
		typ string
		c   Config
	}{
		"unknown type":   {"sms", Config{}},
		"unknown preset": {TypeWebhook, Config{Preset: "x", URL: "https://a"}},
		"no url":         {TypeWebhook, Config{Preset: PresetJSON}},
		"file url":       {TypeWebhook, Config{Preset: PresetJSON, URL: "file:///etc/passwd"}},
		"no token":       {TypeTelegram, Config{ChatID: "1"}},
		"odd token":      {TypeTelegram, Config{Token: "a:b/../c", ChatID: "1"}},
		"no chat":        {TypeTelegram, Config{Token: "1:a"}},
	} {
		if bad.c.Validate(bad.typ) == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func telegramServer(t *testing.T, routes map[string]string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		for suffix, answer := range routes {
			if strings.HasSuffix(r.URL.Path, suffix) {
				io.WriteString(w, answer)
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDiscoverTelegram(t *testing.T) {
	srv := telegramServer(t, map[string]string{
		"/getMe": `{"ok":true,"result":{"username":"nas_bot","first_name":"NAS"}}`,
		"/getUpdates": `{"ok":true,"result":[
			{"update_id":1,"message":{"chat":{"id":123456789,"type":"private","first_name":"Li","last_name":"Song"}}},
			{"update_id":2,"my_chat_member":{"chat":{"id":-1001234567890123,"type":"supergroup","title":"Ops"}}},
			{"update_id":3,"message":{"chat":{"id":123456789,"type":"private","first_name":"Li","last_name":"Song"}}},
			{"update_id":4,"channel_post":{"chat":{"id":-1009,"type":"channel","title":"Alerts"}}},
			{"update_id":5,"poll":{"id":"x"}}
		]}`,
	}, 200)
	bot, chats, err := DiscoverTelegram(context.Background(), "1:tok", Options{TelegramAPI: srv.URL})
	if err != nil || bot.Username != "nas_bot" {
		t.Fatalf("%+v %v", bot, err)
	}
	want := []TelegramChat{
		{ID: "-1009", Title: "Alerts", Type: "channel"},
		{ID: "123456789", Title: "Li Song", Type: "private"},
		{ID: "-1001234567890123", Title: "Ops", Type: "supergroup"}, // exact, past a float's precision
	}
	if len(chats) != len(want) {
		t.Fatalf("%+v", chats)
	}
	for i := range want {
		if chats[i] != want[i] {
			t.Errorf("%+v, want %+v", chats[i], want[i])
		}
	}
}

func TestDiscoverTelegramSaysWhatIsWrong(t *testing.T) {
	const token = "1:s3cret"
	for status, phrase := range map[int]string{401: "does not know this token", 409: "something else is reading"} {
		srv := telegramServer(t, map[string]string{"/getMe": `{"ok":false}`}, status)
		_, _, err := DiscoverTelegram(context.Background(), token, Options{TelegramAPI: srv.URL})
		if err == nil || !strings.Contains(err.Error(), phrase) {
			t.Errorf("%d: %v", status, err)
		}
	}
	srv := telegramServer(t, nil, 200)
	srv.Close()
	_, _, err := DiscoverTelegram(context.Background(), token, Options{TelegramAPI: srv.URL})
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("leaked: %v", err)
	}
	if _, _, err := DiscoverTelegram(context.Background(), "no-colon", Options{TelegramAPI: srv.URL}); err == nil {
		t.Fatal("accepted a malformed token")
	}
}

// A conflict getting updates still names the bot, so the page can say whose it is.
func TestDiscoverTelegramKeepsTheBotOnConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			io.WriteString(w, `{"ok":true,"result":{"username":"nas_bot","first_name":"NAS"}}`)
			return
		}
		w.WriteHeader(409)
		io.WriteString(w, `{"ok":false,"description":"Conflict"}`)
	}))
	defer srv.Close()
	bot, _, err := DiscoverTelegram(context.Background(), "1:t", Options{TelegramAPI: srv.URL})
	if bot.Username != "nas_bot" || err == nil {
		t.Fatalf("%+v %v", bot, err)
	}
}
