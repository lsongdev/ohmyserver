package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ErrNameTaken reports a channel name in use.
var ErrNameTaken = errors.New("a channel with this name exists")

// Delivery states.
const (
	DeliveryPending = "pending"
	DeliverySent    = "sent"
	DeliveryFailed  = "failed"  // given up on
	DeliveryMerged  = "merged"  // sent as part of a summary
	DeliveryDropped = "dropped" // its channel was turned off
)

// Channel is somewhere events go, and which ones. Config holds its secrets, and
// stays in this module.
type Channel struct {
	ID            int64
	Name          string
	Type          string
	Config        string // JSON, kept opaque here
	MinLevel      string
	Sources       []string // empty: all
	Enabled       bool
	LastError     string
	LastErrorAt   *time.Time
	LastSuccessAt *time.Time
	Created       time.Time
}

// wants says whether an event of this level and source goes to the channel.
func (c Channel) wants(level, source string) bool {
	return Rank(level) >= Rank(c.MinLevel) && (len(c.Sources) == 0 || slices.Contains(c.Sources, source))
}

func joinSources(list []string) string { return strings.Join(list, ",") }

func splitSources(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, ",")
}

const channelColumns = `id, name, type, config, min_level, sources, enabled, last_error, last_error_at, last_success_at, created_at`

func millis(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	at := time.UnixMilli(v.Int64)
	return &at
}

func scanChannel(row interface{ Scan(...any) error }) (Channel, error) {
	var c Channel
	var sources string
	var enabled int
	var failed, succeeded sql.NullInt64
	var created int64
	if err := row.Scan(&c.ID, &c.Name, &c.Type, &c.Config, &c.MinLevel, &sources, &enabled, &c.LastError, &failed, &succeeded, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c, ErrNotFound
		}
		return c, err
	}
	c.Sources, c.Enabled, c.Created = splitSources(sources), enabled != 0, time.UnixMilli(created)
	c.LastErrorAt, c.LastSuccessAt = millis(failed), millis(succeeded)
	return c, nil
}

func nameTaken(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrNameTaken
	}
	return err
}

func flag(b bool) int {
	if b {
		return 1
	}
	return 0
}

// CreateChannel stores a channel and fills in its id.
func (s *SQLite) CreateChannel(ctx context.Context, c *Channel, now time.Time) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO notify_channels (name, type, config, min_level, sources, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, c.Name, c.Type, c.Config, c.MinLevel, joinSources(c.Sources), flag(c.Enabled), now.UnixMilli())
	if err != nil {
		return nameTaken(err)
	}
	c.ID, err = res.LastInsertId()
	c.Created = now
	return err
}

// UpdateChannel replaces what can be edited.
func (s *SQLite) UpdateChannel(ctx context.Context, c Channel) error {
	res, err := s.db.ExecContext(ctx, `UPDATE notify_channels SET name = ?, type = ?, config = ?, min_level = ?, sources = ?, enabled = ? WHERE id = ?`,
		c.Name, c.Type, c.Config, c.MinLevel, joinSources(c.Sources), flag(c.Enabled), c.ID)
	if err != nil {
		return nameTaken(err)
	}
	return mustChange(res)
}

func mustChange(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteChannel removes a channel with its deliveries.
func (s *SQLite) DeleteChannel(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM notify_channels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return mustChange(res)
}

// Channel returns one channel.
func (s *SQLite) Channel(ctx context.Context, id int64) (Channel, error) {
	return scanChannel(s.db.QueryRowContext(ctx, `SELECT `+channelColumns+` FROM notify_channels WHERE id = ?`, id))
}

// Channels lists the channels by name.
func (s *SQLite) Channels(ctx context.Context) ([]Channel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+channelColumns+` FROM notify_channels ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Channel{}
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// ChannelResult records how the latest send went. Only that: a channel failing
// must never produce an event, or a dead channel would feed itself.
func (s *SQLite) ChannelResult(ctx context.Context, id int64, failure string, now time.Time) error {
	var err error
	if failure == "" {
		_, err = s.db.ExecContext(ctx, `UPDATE notify_channels SET last_error = '', last_success_at = ? WHERE id = ?`, now.UnixMilli(), id)
	} else {
		_, err = s.db.ExecContext(ctx, `UPDATE notify_channels SET last_error = ?, last_error_at = ? WHERE id = ?`, failure, now.UnixMilli(), id)
	}
	return err
}

// enqueue queues an event for its channels: those whose rule wants it, or for a
// resolution, those told about the problem, whatever its level says.
func enqueue(ctx context.Context, tx *sql.Tx, event int64, now time.Time, r Report, kind string, issue int64) error {
	var targets []int64
	if kind == KindResolved {
		rows, err := tx.QueryContext(ctx, `SELECT DISTINCT d.channel_id FROM notify_deliveries d
			JOIN notify_events e ON e.id = d.event_id JOIN notify_channels c ON c.id = d.channel_id
			WHERE e.issue_id = ? AND c.enabled = 1 AND d.state IN (?, ?, ?)`, issue, DeliveryPending, DeliverySent, DeliveryMerged)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			targets = append(targets, id)
		}
		if err := rows.Close(); err != nil {
			return err
		}
	} else {
		rows, err := tx.QueryContext(ctx, `SELECT `+channelColumns+` FROM notify_channels WHERE enabled = 1`)
		if err != nil {
			return err
		}
		for rows.Next() {
			c, err := scanChannel(rows)
			if err != nil {
				rows.Close()
				return err
			}
			if c.wants(r.Level, r.Source) {
				targets = append(targets, c.ID)
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	for _, channel := range targets {
		if _, err := tx.ExecContext(ctx, `INSERT INTO notify_deliveries (event_id, channel_id, state, next_attempt_at, created_at) VALUES (?, ?, ?, ?, ?)`,
			event, channel, DeliveryPending, now.UnixMilli(), now.UnixMilli()); err != nil {
			return err
		}
	}
	return nil
}

// Due is a delivery ready to go, with its event.
type Due struct {
	ID       int64
	Attempts int
	Created  time.Time
	Event    Event
}

// ChannelsDue lists the channels with deliveries ready.
func (s *SQLite) ChannelsDue(ctx context.Context, now time.Time) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT channel_id FROM notify_deliveries WHERE state = ? AND next_attempt_at <= ?`, DeliveryPending, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		list = append(list, id)
	}
	return list, rows.Err()
}

// DueFor lists a channel's deliveries ready to go, oldest first, stopping at the
// first that is not: a channel sends strictly in order, because a stale
// "resolved" arriving after a newer "opened" would say the opposite of the truth.
func (s *SQLite) DueFor(ctx context.Context, channel int64, now time.Time) ([]Due, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT d.id, d.attempts, d.created_at, d.next_attempt_at, e.id, e.created_at, e.source, e.level, e.kind, e.title, e.body, e.link
		FROM notify_deliveries d JOIN notify_events e ON e.id = d.event_id
		WHERE d.channel_id = ? AND d.state = ? ORDER BY e.id`, channel, DeliveryPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Due
	for rows.Next() {
		var d Due
		var created, next, at int64
		if err := rows.Scan(&d.ID, &d.Attempts, &created, &next, &d.Event.ID, &at, &d.Event.Source, &d.Event.Level, &d.Event.Kind, &d.Event.Title, &d.Event.Body, &d.Event.Link); err != nil {
			return nil, err
		}
		if next > now.UnixMilli() {
			break
		}
		d.Created, d.Event.Created = time.UnixMilli(created), time.UnixMilli(at)
		list = append(list, d)
	}
	return list, rows.Err()
}

// SentSince counts what went out over a channel from a time on.
func (s *SQLite) SentSince(ctx context.Context, channel int64, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notify_deliveries WHERE channel_id = ? AND state = ? AND finished_at >= ?`,
		channel, DeliverySent, since.UnixMilli()).Scan(&n)
	return n, err
}

// FinishDeliveries ends deliveries in a state.
func (s *SQLite) FinishDeliveries(ctx context.Context, ids []int64, state, failure string, now time.Time) error {
	for _, id := range ids {
		if _, err := s.db.ExecContext(ctx, `UPDATE notify_deliveries SET state = ?, error = ?, finished_at = ? WHERE id = ?`, state, failure, now.UnixMilli(), id); err != nil {
			return err
		}
	}
	return nil
}

// RetryLater records a failed attempt on a delivery.
func (s *SQLite) RetryLater(ctx context.Context, id int64, failure string, next time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notify_deliveries SET attempts = attempts + 1, error = ?, next_attempt_at = ? WHERE id = ?`, failure, next.UnixMilli(), id)
	return err
}

// Postpone moves deliveries to a later time, without counting an attempt.
func (s *SQLite) Postpone(ctx context.Context, ids []int64, next time.Time) error {
	for _, id := range ids {
		if _, err := s.db.ExecContext(ctx, `UPDATE notify_deliveries SET next_attempt_at = ? WHERE id = ?`, next.UnixMilli(), id); err != nil {
			return err
		}
	}
	return nil
}

// RetryDelivery queues a delivery that was given up on again.
func (s *SQLite) RetryDelivery(ctx context.Context, id int64, now time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE notify_deliveries SET state = ?, attempts = 0, error = '', next_attempt_at = ?, finished_at = NULL, created_at = ?
		WHERE id = ? AND state = ?`, DeliveryPending, now.UnixMilli(), now.UnixMilli(), id, DeliveryFailed)
	if err != nil {
		return err
	}
	return mustChange(res)
}

// DeliveryView is a delivery as the log shows it.
type DeliveryView struct {
	ID         int64      `json:"id"`
	State      string     `json:"state"`
	Attempts   int        `json:"attempts"`
	Error      string     `json:"error,omitempty"`
	Created    time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	NextTry    *time.Time `json:"next_attempt_at,omitempty"`
	Channel    string     `json:"channel"`
	ChannelID  int64      `json:"channel_id"`
	Source     string     `json:"source"`
	Level      string     `json:"level"`
	Kind       string     `json:"kind"`
	Title      string     `json:"title"`
}

// DeliveryQuery filters the log; zero values match everything.
type DeliveryQuery struct {
	Channel int64
	State   string
	Before  int64
	Limit   int
}

// Deliveries lists deliveries newest first.
func (s *SQLite) Deliveries(ctx context.Context, q DeliveryQuery) ([]DeliveryView, error) {
	where, args := []string{"1 = 1"}, []any{}
	if q.Channel != 0 {
		where, args = append(where, "d.channel_id = ?"), append(args, q.Channel)
	}
	if q.State != "" {
		where, args = append(where, "d.state = ?"), append(args, q.State)
	}
	if q.Before != 0 {
		where, args = append(where, "d.id < ?"), append(args, q.Before)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT d.id, d.state, d.attempts, d.error, d.created_at, d.finished_at, d.next_attempt_at, c.name, c.id, e.source, e.level, e.kind, e.title
		FROM notify_deliveries d JOIN notify_channels c ON c.id = d.channel_id JOIN notify_events e ON e.id = d.event_id
		WHERE `+strings.Join(where, " AND ")+` ORDER BY d.id DESC LIMIT ?`, append(args, q.Limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []DeliveryView{}
	for rows.Next() {
		var d DeliveryView
		var created, next int64
		var finished sql.NullInt64
		if err := rows.Scan(&d.ID, &d.State, &d.Attempts, &d.Error, &created, &finished, &next, &d.Channel, &d.ChannelID, &d.Source, &d.Level, &d.Kind, &d.Title); err != nil {
			return nil, err
		}
		d.Created, d.FinishedAt = time.UnixMilli(created), millis(finished)
		if d.State == DeliveryPending {
			at := time.UnixMilli(next)
			d.NextTry = &at
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// MuteIssue sets or ends when an issue is muted until.
func (s *SQLite) MuteIssue(ctx context.Context, id int64, until *time.Time) error {
	var value any
	if until != nil {
		value = until.UnixMilli()
	}
	res, err := s.db.ExecContext(ctx, `UPDATE notify_issues SET muted_until = ? WHERE id = ?`, value, id)
	if err != nil {
		return fmt.Errorf("muting: %w", err)
	}
	return mustChange(res)
}
