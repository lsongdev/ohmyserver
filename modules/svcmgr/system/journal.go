package system

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Entry is one journal record.
type Entry struct {
	Time       time.Time `json:"time"`
	Priority   int       `json:"priority"` // 0 emergency … 7 debug
	Unit       string    `json:"unit,omitempty"`
	Identifier string    `json:"identifier,omitempty"`
	PID        int       `json:"pid,omitempty"`
	Message    string    `json:"message"`
	Cursor     string    `json:"cursor"`
}

// Query selects journal entries.
type Query struct {
	Unit     string        // a service; empty reads every entry
	Lines    int           // the newest entries to return; 200 by default, at most 5000
	Priority int           // the least important priority to include (1 alert … 7 debug); 0 includes all
	Since    time.Duration // how far back; zero reads the whole journal
	Search   string        // entries whose message contains this, ignoring case
	After    string        // a cursor: only entries after it
}

const (
	defaultLines = 200
	maxLines     = 5000
	// searchScan is how many entries a search reads to find its matches.
	searchScan = 20000
)

var validCursor = regexp.MustCompile(`^[A-Za-z0-9=;_-]+$`)

// args builds journalctl's arguments for q.
func (q *Query) args(follow bool) ([]string, error) {
	args := []string{"--no-pager", "-o", "json"}
	if q.Unit != "" {
		unit, err := UnitName(q.Unit)
		if err != nil {
			return nil, err
		}
		args = append(args, "-u", unit)
	}
	if q.Priority < 0 || q.Priority > 7 {
		return nil, fmt.Errorf("%w priority %d", ErrInvalid, q.Priority)
	}
	if q.Priority > 0 && q.Priority < 7 {
		args = append(args, "-p", strconv.Itoa(q.Priority))
	}
	if q.Since < 0 || q.Since > 366*24*time.Hour {
		return nil, fmt.Errorf("%w time range", ErrInvalid)
	}
	if q.Since > 0 {
		args = append(args, "--since", time.Now().Add(-q.Since).Format("2006-01-02 15:04:05"))
	}
	if q.After != "" {
		if !validCursor.MatchString(q.After) {
			return nil, fmt.Errorf("%w cursor", ErrInvalid)
		}
		args = append(args, "--after-cursor", q.After)
	}
	if follow {
		return append(args, "-f", "-n", "0"), nil
	}
	if q.Lines <= 0 {
		q.Lines = defaultLines
	}
	q.Lines = min(q.Lines, maxLines)
	n := q.Lines
	if q.Search != "" {
		n = searchScan
	}
	return append(args, "-n", strconv.Itoa(n)), nil
}

// Journal returns the newest entries q selects, oldest first.
func (s *Systemd) Journal(ctx context.Context, q Query) ([]Entry, error) {
	args, err := q.args(false)
	if err != nil {
		return nil, err
	}
	out, err := s.Run.Output(ctx, nil, "journalctl", args...)
	if err != nil {
		return nil, err
	}
	var result []Entry
	search := strings.ToLower(q.Search)
	for _, line := range lines(out) {
		if e, ok := parseEntry(line); ok && (search == "" || strings.Contains(strings.ToLower(e.Message), search)) {
			result = append(result, e)
		}
	}
	if len(result) > q.Lines {
		result = result[len(result)-q.Lines:]
	}
	return result, nil
}

// Follow sends entries q selects as they are written, until ctx is done.
func (s *Systemd) Follow(ctx context.Context, q Query, send func(Entry)) error {
	args, err := q.args(true)
	if err != nil {
		return err
	}
	search := strings.ToLower(q.Search)
	err = s.Run.Stream(ctx, nil, func(line string) {
		if e, ok := parseEntry(line); ok && (search == "" || strings.Contains(strings.ToLower(e.Message), search)) {
			send(e)
		}
	}, "journalctl", args...)
	if ctx.Err() != nil {
		return nil // the reader went away
	}
	return err
}

// parseEntry reads one line of journalctl -o json. MESSAGE is a string, or
// an array of bytes when it is not valid UTF-8 or has control characters.
func parseEntry(line string) (Entry, bool) {
	var raw map[string]json.RawMessage
	if json.Unmarshal([]byte(line), &raw) != nil {
		return Entry{}, false
	}
	str := func(key string) string {
		var s string
		if json.Unmarshal(raw[key], &s) == nil {
			return s
		}
		var b []byte
		var ints []int
		if json.Unmarshal(raw[key], &ints) == nil {
			for _, n := range ints {
				b = append(b, byte(n))
			}
			return strings.ToValidUTF8(string(b), "�")
		}
		return ""
	}
	e := Entry{Message: strings.TrimRight(str("MESSAGE"), "\n"), Cursor: str("__CURSOR"),
		Unit: str("_SYSTEMD_UNIT"), Identifier: str("SYSLOG_IDENTIFIER")}
	if e.Identifier == "" {
		e.Identifier = str("_COMM")
	}
	if us, err := strconv.ParseInt(str("__REALTIME_TIMESTAMP"), 10, 64); err == nil {
		e.Time = time.UnixMicro(us).UTC()
	}
	e.Priority = 6 // info, when unset
	if p, err := strconv.Atoi(str("PRIORITY")); err == nil {
		e.Priority = p
	}
	e.PID, _ = strconv.Atoi(str("_PID"))
	return e, e.Cursor != ""
}
