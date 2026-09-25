package web

import (
	"context"
	"database/sql"
	pvemodule "github.com/lsongdev/ohmyserver/modules/pve"
	_ "modernc.org/sqlite"
	"testing"
)

func TestLocalServiceRetainsCredentialWhenUpdateOmitsPassword(t *testing.T) {
	db, e := sql.Open("sqlite", "file:pve-service-test?mode=memory&cache=shared")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	repo := NewRepository(db)
	ctx := context.Background()
	if e = repo.Initialize(ctx); e != nil {
		t.Fatal(e)
	}
	s := NewLocalService(repo)
	secret := "top-secret"
	v, e := s.CreatePVE(ctx, pvemodule.Input{Name: "cluster", Host: "pve.local", Username: "root@pam", Password: &secret})
	if e != nil {
		t.Fatal(e)
	}
	if !v.HasCredential {
		t.Fatalf("missing credential flag: %+v", v)
	}
	updated, e := s.UpdatePVE(ctx, v.ID, pvemodule.Input{Name: "cluster2", Host: "pve.local", Username: "root@pam"})
	if e != nil {
		t.Fatal(e)
	}
	stored, e := repo.Get(ctx, v.ID)
	if e != nil {
		t.Fatal(e)
	}
	if stored.Password != secret || !updated.HasCredential {
		t.Fatalf("credential not retained: stored=%+v dto=%+v", stored, updated)
	}
}
