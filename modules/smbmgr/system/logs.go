package system

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const maxLogBytes = 1 << 20
const maxLogEntries = 5000

// smbdLogPath uses Samba's build-time log directory.
func (s *Samba) smbdLogPath(ctx context.Context) string {
	base := "/var/log/samba"
	if out, _, err := s.Run.Run(ctx, "", "smbd", "-b"); err == nil {
		for line := range strings.Lines(string(out)) {
			if key, value, ok := strings.Cut(strings.TrimSpace(line), ":"); ok && key == "LOGFILEBASE" && filepath.IsAbs(strings.TrimSpace(value)) {
				base = strings.TrimSpace(value)
				break
			}
		}
	}
	return filepath.Join(base, "smbd.log")
}

type LogQuery struct {
	Search string
	Limit  int
}

type LogResult struct {
	Path      string    `json:"path"`
	Entries   []string  `json:"entries"`
	Truncated bool      `json:"truncated"`
	ReadAt    time.Time `json:"read_at"`
}

// Logs reads a bounded recent window, with whole-record text matching so
// a file's timestamp and the following error message stay together.
func (s *Samba) Logs(ctx context.Context, q LogQuery) (LogResult, error) {
	if q.Limit == 0 {
		q.Limit = 500
	}
	if q.Limit < 1 || q.Limit > maxLogEntries || len(q.Search) > 256 {
		return LogResult{}, fmt.Errorf("%w: limit must be 1–5000 and search at most 256 bytes", ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return LogResult{}, err
	}
	path := s.smbdLogPath(ctx)
	data, truncated, err := tailLog(path)
	if os.IsNotExist(err) {
		return LogResult{}, fmt.Errorf("%w: %s does not exist yet", ErrNotFound, path)
	}
	if err != nil {
		return LogResult{}, err
	}
	result := LogResult{Path: path, Entries: []string{}, Truncated: truncated, ReadAt: time.Now().UTC()}
	search := strings.ToLower(q.Search)
	for _, entry := range logRecords(string(data)) {
		if search == "" || strings.Contains(strings.ToLower(entry), search) {
			result.Entries = append(result.Entries, entry)
		}
	}
	if len(result.Entries) > q.Limit {
		result.Entries = result.Entries[len(result.Entries)-q.Limit:]
		result.Truncated = true
	}
	return result, ctx.Err()
}

func tailLog(path string) ([]byte, bool, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, false, err
	}
	if !before.Mode().IsRegular() {
		return nil, false, fmt.Errorf("%w: log source is not a regular file", ErrInvalid)
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(before, info) {
		return nil, false, fmt.Errorf("%w: log file changed; refresh and try again", ErrConflict)
	}
	offset := max(int64(0), info.Size()-maxLogBytes)
	data, err := io.ReadAll(io.NewSectionReader(f, offset, maxLogBytes))
	if offset > 0 {
		// The first line may begin halfway through a message or UTF-8 rune.
		if i := strings.IndexByte(string(data), '\n'); i >= 0 {
			data = data[i+1:]
		} else {
			data = nil
		}
	}
	return data, offset > 0, err
}

var sambaLogHeader = regexp.MustCompile(`^\[\d{4}/\d{2}/\d{2} `)

func logRecords(text string) []string {
	entries := []string{}
	for line := range strings.Lines(strings.ToValidUTF8(text, "�")) {
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		if len(entries) > 0 && !sambaLogHeader.MatchString(line) && (line[0] == ' ' || line[0] == '\t') {
			entries[len(entries)-1] += "\n" + line
		} else {
			entries = append(entries, line)
		}
	}
	return entries
}
