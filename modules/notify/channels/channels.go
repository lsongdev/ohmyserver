// Package channels sends messages out: to any web hook, with presets for the
// common services, and to Telegram. Sending goes one way only, so each message
// stands alone and needs nothing from the service first.
package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrInvalid marks a configuration that cannot work.
var ErrInvalid = errors.New("invalid channel")

// Channel types and web hook presets.
const (
	TypeWebhook  = "webhook"
	TypeTelegram = "telegram"

	PresetJSON   = "json"
	PresetFeishu = "feishu"
	PresetWeCom  = "wecom"
	PresetBark   = "bark"
	PresetNtfy   = "ntfy"
)

// Config says where a channel sends. URL and Token are secrets: a chat bot's
// web hook address is as good as a password, so they are never shown back.
type Config struct {
	Preset string `json:"preset,omitempty"`  // of a web hook
	URL    string `json:"url,omitempty"`     // of a web hook
	Token  string `json:"token,omitempty"`   // bearer token, or Telegram's bot token
	ChatID string `json:"chat_id,omitempty"` // Telegram
}

// Validate checks a configuration of a type.
func (c Config) Validate(typ string) error {
	switch typ {
	case TypeWebhook:
		switch c.Preset {
		case PresetJSON, PresetFeishu, PresetWeCom, PresetBark, PresetNtfy:
		default:
			return fmt.Errorf("%w: unknown preset %q", ErrInvalid, c.Preset)
		}
		u, err := url.Parse(c.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("%w: url must be an http or https address", ErrInvalid)
		}
	case TypeTelegram:
		if !strings.Contains(c.Token, ":") || strings.ContainsAny(c.Token, "/ ?#") {
			return fmt.Errorf("%w: a bot token looks like 123456:ABC-DEF", ErrInvalid)
		}
		if strings.TrimSpace(c.ChatID) == "" {
			return fmt.Errorf("%w: chat id required", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: unknown type %q", ErrInvalid, typ)
	}
	return nil
}

// Target says where a channel sends, without its secrets.
func (c Config) Target(typ string) string {
	if typ == TypeTelegram {
		return "chat " + c.ChatID
	}
	if u, err := url.Parse(c.URL); err == nil && u.Host != "" {
		return u.Scheme + "://" + u.Host + "/…"
	}
	return ""
}

// Message is what gets sent.
type Message struct {
	Source string    `json:"source"`
	Level  string    `json:"level"`
	Kind   string    `json:"kind"` // message, opened, escalated, resolved, test, summary
	Title  string    `json:"title"`
	Body   string    `json:"body,omitempty"`
	Time   time.Time `json:"time"`
}

// Text is the message as one block of plain text.
func (m Message) Text() string {
	label := strings.ToUpper(m.Level)
	if m.Kind == "resolved" {
		label = "RESOLVED"
	}
	text := "[" + label + "] " + m.Title
	if m.Body != "" {
		text += "\n" + m.Body
	}
	if m.Source != "" {
		text += "\n— " + m.Source + ", " + m.Time.Format("2006-01-02 15:04:05")
	}
	return text
}

// Sender sends messages over one channel.
type Sender struct {
	typ    string
	config Config
	client *http.Client
	tgAPI  string
}

// Options configures senders; zero values select the defaults.
type Options struct {
	Client      *http.Client // with a timeout, and no redirects
	TelegramAPI string       // for tests
}

// New returns a sender for a configuration.
func New(typ string, config Config, opts Options) (*Sender, error) {
	if err := config.Validate(typ); err != nil {
		return nil, err
	}
	s := &Sender{typ: typ, config: config, client: opts.Client, tgAPI: opts.TelegramAPI}
	if s.client == nil {
		s.client = DefaultClient()
	}
	if s.tgAPI == "" {
		s.tgAPI = "https://api.telegram.org"
	}
	return s, nil
}

// DefaultClient gives up after 15 seconds and follows no redirects, which could
// carry a secret somewhere else.
func DefaultClient() *http.Client {
	return &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// Send sends a message. Its errors never contain the channel's address or
// token: they get stored and shown.
func (s *Sender) Send(ctx context.Context, m Message) error {
	var req *http.Request
	var err error
	if s.typ == TypeTelegram {
		req, err = s.telegram(ctx, m)
	} else {
		req, err = s.webhook(ctx, m)
	}
	if err != nil {
		return err
	}
	res, err := s.client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return errors.New(s.scrub(urlErr.Err.Error())) // url.Error quotes the URL
		}
		return errors.New(s.scrub(err.Error()))
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("the service answered %d: %s", res.StatusCode, s.scrub(strings.TrimSpace(string(body))))
	}
	return s.checkBody(body)
}

// scrub removes the secrets from text coming back from elsewhere.
func (s *Sender) scrub(text string) string {
	for _, secret := range []string{s.config.URL, s.config.Token} {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "…")
		}
	}
	return text
}

// checkBody reads the answers of services that report failure with status 200.
func (s *Sender) checkBody(body []byte) error {
	var answer struct {
		Code    *int   `json:"code"`    // feishu 0, bark 200
		ErrCode *int   `json:"errcode"` // wecom 0
		Msg     string `json:"msg"`
		Message string `json:"message"`
		ErrMsg  string `json:"errmsg"`
	}
	if json.Unmarshal(body, &answer) != nil {
		return nil
	}
	code, ok := 0, false
	switch s.config.Preset {
	case PresetFeishu:
		if answer.Code != nil {
			code, ok = *answer.Code, *answer.Code != 0
		}
	case PresetWeCom:
		if answer.ErrCode != nil {
			code, ok = *answer.ErrCode, *answer.ErrCode != 0
		}
	case PresetBark:
		if answer.Code != nil {
			code, ok = *answer.Code, *answer.Code != 200
		}
	}
	if ok {
		return fmt.Errorf("the service refused it (%d): %s", code, s.scrub(answer.Msg+answer.Message+answer.ErrMsg))
	}
	return nil
}

func post(ctx context.Context, address string, payload any) (*http.Request, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("invalid address")
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (s *Sender) telegram(ctx context.Context, m Message) (*http.Request, error) {
	return post(ctx, s.tgAPI+"/bot"+s.config.Token+"/sendMessage", map[string]any{
		"chat_id": s.config.ChatID, "text": limit(m.Text(), 4000), "disable_web_page_preview": true,
	})
}

func (s *Sender) webhook(ctx context.Context, m Message) (*http.Request, error) {
	var payload any
	switch s.config.Preset {
	case PresetFeishu:
		payload = map[string]any{"msg_type": "text", "content": map[string]string{"text": limit(m.Text(), 4000)}}
	case PresetWeCom:
		payload = map[string]any{"msgtype": "text", "text": map[string]string{"content": limit(m.Text(), 2000)}}
	case PresetBark:
		level := "active"
		if m.Level == "critical" && m.Kind != "resolved" {
			level = "timeSensitive"
		}
		payload = map[string]string{"title": m.Title, "body": limit(m.Body, 2000), "group": m.Source, "level": level}
	case PresetNtfy:
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.URL, strings.NewReader(limit(m.Body+" ", 3000)))
		if err != nil {
			return nil, errors.New("invalid address")
		}
		req.Header.Set("Title", mime.BEncoding.Encode("UTF-8", m.Title))
		req.Header.Set("Priority", strconv.Itoa(map[string]int{"critical": 5, "warning": 4, "info": 3, "debug": 2}[m.Level]))
		if s.config.Token != "" {
			req.Header.Set("Authorization", "Bearer "+s.config.Token)
		}
		return req, nil
	default:
		payload = m
	}
	req, err := post(ctx, s.config.URL, payload)
	if err != nil {
		return nil, err
	}
	if s.config.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.config.Token)
	}
	return req, nil
}

func limit(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + "…"
}
