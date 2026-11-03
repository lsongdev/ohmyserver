package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lsongdev/ohmyserver/modules/notify/channels"
	"github.com/lsongdev/ohmyserver/modules/notify/store"
)

// ErrSendFailed reports that a test message did not get through.
var ErrSendFailed = errors.New("sending failed")

const (
	defaultRate = 20
	giveUpAfter = 24 * time.Hour
	maxSummary  = 8 // titles in a summary
)

// backoff says how long to wait after a failed attempt.
func backoff(attempts int) time.Duration {
	steps := []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}
	if attempts < len(steps) {
		return steps[attempts]
	}
	return time.Hour
}

// nudge says a delivery may be due. It never waits.
func (s *Service) nudge() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// dispatch starts sending on each channel with something due. A channel sends
// one message at a time; a slow or dead one holds up only itself.
func (s *Service) dispatch(ctx context.Context) {
	due, err := s.store.ChannelsDue(ctx, s.now())
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("notify: reading deliveries", "err", err)
		}
		return
	}
	for _, id := range due {
		s.busy.Lock()
		if s.active[id] {
			s.busy.Unlock()
			continue
		}
		s.active[id] = true
		s.busy.Unlock()
		s.sending.Add(1)
		go func() {
			defer s.sending.Done()
			defer func() {
				s.busy.Lock()
				delete(s.active, id)
				s.busy.Unlock()
			}()
			s.sendDue(ctx, id)
		}()
	}
}

// Dispatch sends what is due and waits for it, for tests.
func (s *Service) Dispatch(ctx context.Context) {
	s.dispatch(ctx)
	s.sending.Wait()
}

func (s *Service) sender(c store.Channel) (*channels.Sender, error) {
	var config channels.Config
	if err := json.Unmarshal([]byte(c.Config), &config); err != nil {
		return nil, fmt.Errorf("reading its configuration: %w", err)
	}
	return channels.New(c.Type, config, s.send)
}

func message(e store.Event) channels.Message {
	return channels.Message{Source: e.Source, Level: e.Level, Kind: e.Kind, Title: e.Title, Body: e.Body, Time: e.Created}
}

func ids(list []store.Due) []int64 {
	out := make([]int64, len(list))
	for i, d := range list {
		out[i] = d.ID
	}
	return out
}

// sendDue sends a channel's due deliveries. Errors here go to the log and the
// channel's status, never to an event.
func (s *Service) sendDue(ctx context.Context, channel int64) {
	if err := s.trySendDue(ctx, channel); err != nil && ctx.Err() == nil {
		s.logger.Warn("notify: sending", "channel", channel, "err", err)
	}
}

func (s *Service) trySendDue(ctx context.Context, id int64) error {
	now := s.now()
	due, err := s.store.DueFor(ctx, id, now)
	if err != nil || len(due) == 0 {
		return err
	}
	c, err := s.store.Channel(ctx, id)
	if err != nil {
		return err
	}
	if !c.Enabled {
		return s.store.FinishDeliveries(ctx, ids(due), store.DeliveryDropped, "the channel was turned off", now)
	}
	sender, err := s.sender(c)
	if err != nil {
		s.store.ChannelResult(ctx, id, err.Error(), now)
		return s.store.Postpone(ctx, ids(due), now.Add(time.Hour))
	}
	// More than a minute's worth at once: one summary says more than a flood.
	if len(due) > s.rate {
		return s.sendSummary(ctx, c, sender, due, now)
	}
	sent, err := s.store.SentSince(ctx, id, now.Add(-time.Minute))
	if err != nil {
		return err
	}
	for _, d := range due {
		if sent >= s.rate {
			return s.store.Postpone(ctx, []int64{d.ID}, now.Add(time.Minute))
		}
		failure := sender.Send(ctx, message(d.Event))
		if ctx.Err() != nil {
			return nil // shutting down: it stays queued
		}
		now = s.now()
		if failure == nil {
			s.store.ChannelResult(ctx, id, "", now)
			if err := s.store.FinishDeliveries(ctx, []int64{d.ID}, store.DeliverySent, "", now); err != nil {
				return err
			}
			sent++
			continue
		}
		s.store.ChannelResult(ctx, id, failure.Error(), now)
		if now.Sub(d.Created) >= giveUpAfter {
			if err := s.store.FinishDeliveries(ctx, []int64{d.ID}, store.DeliveryFailed, failure.Error(), now); err != nil {
				return err
			}
			continue
		}
		// What comes after waits behind it: see DueFor.
		return s.store.RetryLater(ctx, d.ID, failure.Error(), now.Add(backoff(d.Attempts)))
	}
	return nil
}

func (s *Service) sendSummary(ctx context.Context, c store.Channel, sender *channels.Sender, due []store.Due, now time.Time) error {
	ordered := slices.Clone(due)
	slices.SortStableFunc(ordered, func(a, b store.Due) int { return store.Rank(b.Event.Level) - store.Rank(a.Event.Level) })
	counts := map[string]int{}
	var lines []string
	for i, d := range ordered {
		counts[d.Event.Level]++
		if i < maxSummary {
			lines = append(lines, "• "+d.Event.Title)
		}
	}
	if len(ordered) > maxSummary {
		lines = append(lines, fmt.Sprintf("… and %d more. See Notifications in the panel.", len(ordered)-maxSummary))
	}
	level := store.LevelInfo
	if len(ordered) > 0 {
		level = ordered[0].Event.Level
	}
	failure := sender.Send(ctx, channels.Message{
		Source: "notifications", Level: level, Kind: "summary", Time: now,
		Title: fmt.Sprintf("%d events: %d critical, %d warning", len(due), counts[store.LevelCritical], counts[store.LevelWarning]),
		Body:  strings.Join(lines, "\n"),
	})
	if ctx.Err() != nil {
		return nil
	}
	if failure != nil {
		s.store.ChannelResult(ctx, c.ID, failure.Error(), now)
		return s.store.Postpone(ctx, ids(due), now.Add(backoff(0)))
	}
	s.store.ChannelResult(ctx, c.ID, "", now)
	return s.store.FinishDeliveries(ctx, ids(due), store.DeliveryMerged, "", now)
}

// RetryDelivery queues again a delivery that was given up on.
func (s *Service) RetryDelivery(ctx context.Context, id int64) error {
	err := s.store.RetryDelivery(ctx, id, s.now())
	s.nudge()
	return err
}

// ChannelInput is a channel as a form sends it. On an update, an empty URL or
// token keeps the stored one, so that secrets need not travel back and forth.
type ChannelInput struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Preset   string   `json:"preset"`
	URL      string   `json:"url"`
	Token    string   `json:"token"`
	ChatID   string   `json:"chat_id"`
	MinLevel string   `json:"min_level"`
	Sources  []string `json:"sources"`
	Enabled  bool     `json:"enabled"`
}

// ChannelView is a channel as shown: Target instead of its secrets.
type ChannelView struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	Type          string     `json:"type"`
	Preset        string     `json:"preset,omitempty"`
	ChatID        string     `json:"chat_id,omitempty"`
	Target        string     `json:"target"`
	HasToken      bool       `json:"has_token"`
	MinLevel      string     `json:"min_level"`
	Sources       []string   `json:"sources"`
	Enabled       bool       `json:"enabled"`
	LastError     string     `json:"last_error,omitempty"`
	LastErrorAt   *time.Time `json:"last_error_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
}

func viewChannel(c store.Channel) ChannelView {
	var config channels.Config
	json.Unmarshal([]byte(c.Config), &config)
	sources := c.Sources
	if sources == nil {
		sources = []string{}
	}
	return ChannelView{
		ID: c.ID, Name: c.Name, Type: c.Type, Preset: config.Preset, ChatID: config.ChatID,
		Target: config.Target(c.Type), HasToken: config.Token != "", MinLevel: c.MinLevel, Sources: sources, Enabled: c.Enabled,
		LastError: c.LastError, LastErrorAt: c.LastErrorAt, LastSuccessAt: c.LastSuccessAt,
	}
}

// channelFrom validates an input into a channel, over the stored config if any.
func channelFrom(in ChannelInput, stored channels.Config) (store.Channel, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 100 {
		return store.Channel{}, fmt.Errorf("%w: a name of up to 100 characters is required", ErrInvalid)
	}
	switch in.MinLevel {
	case store.LevelDebug, store.LevelInfo, store.LevelWarning, store.LevelCritical:
	default:
		return store.Channel{}, fmt.Errorf("%w: unknown level %q", ErrInvalid, in.MinLevel)
	}
	for _, source := range in.Sources {
		if source == "" || strings.Contains(source, ",") {
			return store.Channel{}, fmt.Errorf("%w: invalid source %q", ErrInvalid, source)
		}
	}
	config := channels.Config{Preset: in.Preset, URL: strings.TrimSpace(in.URL), Token: strings.TrimSpace(in.Token), ChatID: strings.TrimSpace(in.ChatID)}
	if config.URL == "" {
		config.URL = stored.URL
	}
	if config.Token == "" {
		config.Token = stored.Token
	}
	if err := config.Validate(in.Type); err != nil {
		return store.Channel{}, err
	}
	data, err := json.Marshal(config)
	if err != nil {
		return store.Channel{}, err
	}
	return store.Channel{Name: name, Type: in.Type, Config: string(data), MinLevel: in.MinLevel, Sources: in.Sources, Enabled: in.Enabled}, nil
}

// Channels lists the channels, without secrets.
func (s *Service) Channels(ctx context.Context) ([]ChannelView, error) {
	list, err := s.store.Channels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelView, len(list))
	for i, c := range list {
		out[i] = viewChannel(c)
	}
	return out, nil
}

// Channel returns one channel, without secrets.
func (s *Service) Channel(ctx context.Context, id int64) (ChannelView, error) {
	c, err := s.store.Channel(ctx, id)
	if err != nil {
		return ChannelView{}, err
	}
	return viewChannel(c), nil
}

// CreateChannel adds a channel. It gets what happens from now on.
func (s *Service) CreateChannel(ctx context.Context, in ChannelInput) (ChannelView, error) {
	c, err := channelFrom(in, channels.Config{})
	if err != nil {
		return ChannelView{}, err
	}
	if err := s.store.CreateChannel(ctx, &c, s.now()); err != nil {
		return ChannelView{}, err
	}
	return viewChannel(c), nil
}

// UpdateChannel changes a channel. Empty secrets keep the stored ones, unless
// the type changed, since they would mean something else then.
func (s *Service) UpdateChannel(ctx context.Context, id int64, in ChannelInput) (ChannelView, error) {
	old, err := s.store.Channel(ctx, id)
	if err != nil {
		return ChannelView{}, err
	}
	var stored channels.Config
	if old.Type == in.Type {
		json.Unmarshal([]byte(old.Config), &stored)
	}
	c, err := channelFrom(in, stored)
	if err != nil {
		return ChannelView{}, err
	}
	c.ID = id
	if err := s.store.UpdateChannel(ctx, c); err != nil {
		return ChannelView{}, err
	}
	return s.Channel(ctx, id)
}

// DeleteChannel removes a channel and its log.
func (s *Service) DeleteChannel(ctx context.Context, id int64) error {
	return s.store.DeleteChannel(ctx, id)
}

// TelegramDiscovery is who a bot token belongs to and who has written to it.
// Problem says why chats could not be listed, while the bot may still be known.
type TelegramDiscovery struct {
	Bot     channels.TelegramBot    `json:"bot"`
	Chats   []channels.TelegramChat `json:"chats"`
	Problem string                  `json:"problem,omitempty"`
}

// DiscoverTelegram looks a bot up by its token, or a saved channel's if channel
// is given, which keeps the token from travelling back to the browser.
func (s *Service) DiscoverTelegram(ctx context.Context, token string, channel int64) (TelegramDiscovery, error) {
	if token == "" && channel != 0 {
		c, err := s.store.Channel(ctx, channel)
		if err != nil {
			return TelegramDiscovery{}, err
		}
		var config channels.Config
		if c.Type == channels.TypeTelegram {
			json.Unmarshal([]byte(c.Config), &config)
		}
		token = config.Token
	}
	if token == "" {
		return TelegramDiscovery{}, fmt.Errorf("%w: a bot token is required", ErrInvalid)
	}
	bot, chats, err := channels.DiscoverTelegram(ctx, token, s.send)
	if errors.Is(err, channels.ErrInvalid) {
		return TelegramDiscovery{}, err
	}
	found := TelegramDiscovery{Bot: bot, Chats: chats}
	if found.Chats == nil {
		found.Chats = []channels.TelegramChat{}
	}
	if err != nil {
		found.Problem = err.Error()
	}
	return found, nil
}

// TestChannel sends a test message right away, and records how it went.
func (s *Service) TestChannel(ctx context.Context, id int64) error {
	c, err := s.store.Channel(ctx, id)
	if err != nil {
		return err
	}
	sender, err := s.sender(c)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSendFailed, err)
	}
	now := s.now()
	failure := sender.Send(ctx, channels.Message{
		Source: "notifications", Level: store.LevelInfo, Kind: "test", Time: now,
		Title: "Test from OhMyServer", Body: "If you can read this, the channel " + c.Name + " works.",
	})
	if failure != nil {
		s.store.ChannelResult(ctx, id, failure.Error(), now)
		return fmt.Errorf("%w: %v", ErrSendFailed, failure)
	}
	return s.store.ChannelResult(ctx, id, "", now)
}
