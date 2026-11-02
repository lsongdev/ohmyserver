package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

type logRunner struct{ base string }

func (r *logRunner) Run(ctx context.Context, stdin, name string, args ...string) ([]byte, []byte, error) {
	if name == "smbd" {
		return []byte("  LOGFILEBASE: " + r.base + "\n"), nil, nil
	}
	return nil, nil, nil
}

func writeLog(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFileLogsSearchKeepsTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "smbd.log")
	writeLog(t, path, "[2026/11/02 10:11:13.252409, 0] allow_access\n  Denied connection from 100.84.176.110\n[2026/11/02 10:12:00.000000, 0] auth\n  NT_STATUS_LOGON_FAILURE\n")
	s := &Samba{Run: &logRunner{base: dir}, Config: "/test/smb.conf"}
	q := LogQuery{Search: "DENIED"}
	result, err := s.Logs(context.Background(), q)
	if err != nil || len(result.Entries) != 1 || !strings.Contains(result.Entries[0], "10:11:13") || !strings.Contains(result.Entries[0], "100.84.176.110") {
		t.Fatalf("filtered records: %+v %v", result, err)
	}
	q.Search, q.Limit = "", 1
	result, err = s.Logs(context.Background(), q)
	if err != nil || !result.Truncated || len(result.Entries) != 1 || !strings.Contains(result.Entries[0], "LOGON_FAILURE") {
		t.Fatalf("newest record: %+v %v", result, err)
	}
	// Reopen on every request: rotation must not leave the reader on the old file.
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	writeLog(t, path, "new log after rotation\n")
	result, err = s.Logs(context.Background(), q)
	if err != nil || !reflect.DeepEqual(result.Entries, []string{"new log after rotation"}) {
		t.Fatalf("rotated: %+v %v", result, err)
	}

}

func TestTailLogBoundAndNonRegularFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "smbd.log")
	writeLog(t, path, strings.Repeat("older record\n", maxLogBytes/5)+"newest record\n")
	data, truncated, err := tailLog(path)
	if err != nil || !truncated || len(data) > maxLogBytes || !strings.HasSuffix(string(data), "newest record\n") || !strings.HasPrefix(string(data), "older record\n") {
		t.Fatalf("tail: bytes=%d truncated=%v error=%v", len(data), truncated, err)
	}
	link := filepath.Join(dir, "link.log")
	os.Symlink(path, link)
	fifo := filepath.Join(dir, "fifo.log")
	syscall.Mkfifo(fifo, 0o600)
	for _, path := range []string{link, fifo, dir} {
		if _, _, err := tailLog(path); !errors.Is(err, ErrInvalid) {
			t.Errorf("non-regular file %q: %v", path, err)
		}
	}
}

func TestLogsMissingAndInvalidQuery(t *testing.T) {
	s := &Samba{Run: &logRunner{base: t.TempDir()}, Config: "/test/smb.conf"}
	if _, err := s.Logs(context.Background(), LogQuery{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing smbd.log: %v", err)
	}
	for _, q := range []LogQuery{{Limit: -1}, {Limit: 5001}, {Search: strings.Repeat("x", 257)}} {
		if _, err := s.Logs(context.Background(), q); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid query accepted: %+v %v", q, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Logs(ctx, LogQuery{}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancellation: %v", err)
	}
}
