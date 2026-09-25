package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRepositoryOwnsESXiSchemaAndCredentialLifecycle(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "esxi.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	repo := NewRepository(db)
	ctx := context.Background()
	if err := repo.Initialize(ctx); err != nil {
		t.Fatal(err)
	}

	created := &Server{Name: "lab", Host: "192.0.2.5", Username: "root", Password: "secret"}
	if err := repo.Create(ctx, created); err != nil {
		t.Fatal(err)
	}
	if created.Port != 443 || created.ID == 0 {
		t.Fatalf("created record defaults: %+v", created)
	}
	loaded, err := repo.Get(ctx, created.ID)
	if err != nil || loaded.Password != "secret" {
		t.Fatalf("get record=%+v err=%v", loaded, err)
	}
	encoded, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || strings.Contains(string(encoded), "secret") {
		t.Fatalf("credential leaked in JSON: %s", encoded)
	}
	loaded.Name = "lab-updated"
	if err := repo.Update(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	items, err := repo.List(ctx)
	if err != nil || len(items) != 1 || items[0].Name != "lab-updated" {
		t.Fatalf("list=%+v err=%v", items, err)
	}
	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	items, err = repo.List(ctx)
	if err != nil || len(items) != 0 {
		t.Fatalf("list after delete=%+v err=%v", items, err)
	}
}
