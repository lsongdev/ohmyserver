package stores

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/lsongdev/forward-go/api"
	"github.com/lsongdev/forward-go/forward"
	"github.com/lsongdev/forward-go/store"
)

var _ api.Store = (*ForwardRules)(nil)

func TestForwardRules(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "ohmyserver.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rules, err := NewForwardRules(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	// Migrating again, as on every start, is a no-op.
	if _, err := NewForwardRules(ctx, db); err != nil {
		t.Fatal(err)
	}

	ssh := forward.Rule{Name: " nas-ssh ", Protocol: "TCP", Listen: "2222", Target: "192.168.8.100:22", Enabled: true}
	if err := rules.Create(ctx, &ssh); err != nil {
		t.Fatal(err)
	}
	if ssh.ID == 0 || ssh.Name != "nas-ssh" || ssh.Protocol != "tcp" || ssh.Listen != ":2222" || ssh.CreatedAt.IsZero() {
		t.Fatalf("created %+v", ssh)
	}
	got, err := rules.Get(ctx, ssh.ID)
	if err != nil || got.Name != ssh.Name || !got.CreatedAt.Equal(ssh.CreatedAt) {
		t.Fatalf("get %+v, %v; want %+v", got, err, ssh)
	}

	dup := forward.Rule{Name: "other", Protocol: "tcp", Listen: ":2222", Target: "127.0.0.1:22"}
	if err := rules.Create(ctx, &dup); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("same listen address: %v", err)
	}
	dup = forward.Rule{Name: "nas-ssh", Protocol: "udp", Listen: ":53", Target: "1.1.1.1:53"}
	if err := rules.Create(ctx, &dup); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("same name: %v", err)
	}

	update := ssh
	update.Enabled = false
	if err := rules.Update(ctx, &update); err != nil {
		t.Fatal(err)
	}
	if update.Enabled || !update.CreatedAt.Equal(ssh.CreatedAt) {
		t.Fatalf("updated %+v", update)
	}
	missing := forward.Rule{ID: 99, Name: "x", Protocol: "tcp", Listen: ":1", Target: "127.0.0.1:1"}
	if err := rules.Update(ctx, &missing); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}

	if list, err := rules.List(ctx); err != nil || len(list) != 1 {
		t.Fatalf("list %+v, %v", list, err)
	}
	if err := rules.Delete(ctx, ssh.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := rules.Get(ctx, ssh.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted rule: %v", err)
	}
}
