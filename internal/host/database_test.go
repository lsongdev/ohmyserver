package host

import (
	"database/sql"
	"testing"
)

func TestSQLiteConnectionHookSetsBusyTimeout(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var timeout int
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&timeout); err != nil {
		t.Fatal(err)
	}
	if timeout != 5000 {
		t.Fatalf("SQLite busy timeout = %d ms, want 5000 ms", timeout)
	}
}
