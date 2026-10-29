package dhcp

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/lsongdev/dhcp-go/server"
	"github.com/lsongdev/dhcp-go/store"
	_ "modernc.org/sqlite"
)

func TestSharedDatabaseSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "panel.db")
	open := func() *sql.DB {
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { db.Close() })
		return db
	}
	db := open()
	st, err := NewStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	cfg := server.Config{Interface: "eth0", RangeStart: "192.0.2.10", RangeEnd: "192.0.2.20"}
	if err := st.SaveConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	r := server.Reservation{MAC: "00:00:5e:00:53:01", IP: "192.0.2.11"}
	if err := st.CreateReservation(ctx, &r); err != nil {
		t.Fatal(err)
	}
	duplicate := server.Reservation{MAC: r.MAC, IP: "192.0.2.12"}
	if err := st.CreateReservation(ctx, &duplicate); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate: %v", err)
	}
	lease := server.Lease{IP: r.IP, MAC: r.MAC, ExpiresAt: time.Now().Add(time.Hour), UpdatedAt: time.Now()}
	if err := st.SaveLease(lease); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = NewStore(ctx, open())
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := st.Config(ctx)
	if err != nil || !ok || got.Interface != cfg.Interface {
		t.Fatalf("config: %+v %v %v", got, ok, err)
	}
	reservations, err := st.Reservations(ctx)
	if err != nil || len(reservations) != 1 || reservations[0].ID != r.ID {
		t.Fatalf("reservations: %+v %v", reservations, err)
	}
	leases, err := st.Leases(ctx)
	if err != nil || len(leases) != 1 || leases[0].IP != lease.IP || !leases[0].ExpiresAt.Equal(lease.ExpiresAt) {
		t.Fatalf("leases: %+v %v", leases, err)
	}
	module, err := New(ctx, st.db, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := module.Close(); err != nil {
		t.Fatal(err)
	}
	if err := st.db.PingContext(ctx); err != nil {
		t.Fatalf("module closed shared database: %v", err)
	}
}
