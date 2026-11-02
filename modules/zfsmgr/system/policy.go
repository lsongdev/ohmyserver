package system

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Classes of automatic snapshots, shortest period first.
var Classes = []string{"hourly", "daily", "weekly", "monthly"}

// MaxKeep bounds how many snapshots of a class a policy may keep.
const MaxKeep = 1000

// Policy says how many automatic snapshots of each class a dataset keeps;
// zero takes none of that class.
type Policy struct {
	Hourly  int `json:"hourly"`
	Daily   int `json:"daily"`
	Weekly  int `json:"weekly"`
	Monthly int `json:"monthly"`
}

// Enabled reports whether the policy takes any snapshots.
func (p Policy) Enabled() bool { return p != Policy{} }

func (p Policy) keep(class string) int {
	switch class {
	case "hourly":
		return p.Hourly
	case "daily":
		return p.Daily
	case "weekly":
		return p.Weekly
	case "monthly":
		return p.Monthly
	}
	return 0
}

// Validate checks the counts.
func (p Policy) Validate() error {
	for _, class := range Classes {
		if n := p.keep(class); n < 0 || n > MaxKeep {
			return fmt.Errorf("%w: %s must be from 0 to %d", ErrInvalid, class, MaxKeep)
		}
	}
	return nil
}

// String is the property value: hourly=24,daily=14,weekly=4,monthly=6.
func (p Policy) String() string {
	parts := make([]string, len(Classes))
	for i, class := range Classes {
		parts[i] = class + "=" + strconv.Itoa(p.keep(class))
	}
	return strings.Join(parts, ",")
}

// ParsePolicy reads a property value. Any part it does not understand makes
// the whole value invalid, and an invalid policy takes and removes nothing.
func ParsePolicy(value string) (Policy, error) {
	var p Policy
	for part := range strings.SplitSeq(value, ",") {
		key, count, ok := strings.Cut(part, "=")
		n, err := strconv.Atoi(count)
		if !ok || err != nil {
			return Policy{}, fmt.Errorf("%w policy %q", ErrInvalid, value)
		}
		switch key {
		case "hourly":
			p.Hourly = n
		case "daily":
			p.Daily = n
		case "weekly":
			p.Weekly = n
		case "monthly":
			p.Monthly = n
		default:
			return Policy{}, fmt.Errorf("%w policy %q", ErrInvalid, value)
		}
	}
	if err := p.Validate(); err != nil {
		return Policy{}, err
	}
	return p, nil
}

// Automatic snapshots have exactly this form, and retention touches nothing
// else: renaming one keeps it for good.
var autoName = regexp.MustCompile(`^auto-(hourly|daily|weekly|monthly)-\d{8}-\d{4}$`)

func classOf(short string) string {
	if m := autoName.FindStringSubmatch(short); m != nil {
		return m[1]
	}
	return ""
}

func autoSnapshotName(class string, t time.Time) string {
	return "auto-" + class + "-" + t.Format("20060102-1504")
}

// periodStart returns when the current period of a class began: the hour,
// the day, the week from Monday, or the month.
func periodStart(class string, t time.Time) time.Time {
	y, m, d := t.Date()
	switch class {
	case "hourly":
		return time.Date(y, m, d, t.Hour(), 0, 0, 0, t.Location())
	case "daily":
		return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	case "weekly":
		sinceMonday := (int(t.Weekday()) + 6) % 7
		return time.Date(y, m, d-sinceMonday, 0, 0, 0, 0, t.Location())
	default:
		return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
	}
}
