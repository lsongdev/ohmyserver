package web

import (
	"context"
	"database/sql"
	"testing"

	sshmodule "github.com/lsongdev/ohmyserver/modules/ssh"
	_ "modernc.org/sqlite"
)

func newTestService(t *testing.T) *LocalService {
	t.Helper()
	db, e := sql.Open("sqlite", ":memory:")
	if e != nil {
		t.Fatal(e)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	if e = repo.Initialize(context.Background()); e != nil {
		t.Fatal(e)
	}
	return NewLocalService(repo)
}
func TestLocalServiceCredentialRetentionAndHostKeyReset(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	enabled := true
	created, e := s.SaveSSHProfile(ctx, 0, sshmodule.Input{Name: "nas", Host: "nas.local", Username: "admin", AuthType: "password", Password: "secret", Enabled: &enabled})
	if e != nil {
		t.Fatal(e)
	}
	if !created.HasCredential || created.Port != 22 {
		t.Fatalf("created profile=%+v", created)
	}
	p, e := s.repo.Get(ctx, created.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.repo.SetHostKey(ctx, created.ID, "SHA256:old"); e != nil {
		t.Fatal(e)
	}
	_, e = s.SaveSSHProfile(ctx, created.ID, sshmodule.Input{Name: "nas", Host: "nas.local", Username: "admin", AuthType: "password"})
	if e != nil {
		t.Fatal(e)
	}
	p, e = s.repo.Get(ctx, created.ID)
	if e != nil {
		t.Fatal(e)
	}
	if p.Password != "secret" || p.HostKey != "SHA256:old" {
		t.Fatalf("unchanged profile lost credential or host key: %+v", p)
	}
	_, e = s.SaveSSHProfile(ctx, created.ID, sshmodule.Input{Name: "nas", Host: "nas-new.local", Username: "admin", AuthType: "password"})
	if e != nil {
		t.Fatal(e)
	}
	p, e = s.repo.Get(ctx, created.ID)
	if e != nil {
		t.Fatal(e)
	}
	if p.Password != "secret" || p.HostKey != "" {
		t.Fatalf("host change did not reset only the pin: %+v", p)
	}
	got, e := s.GetSSHProfile(ctx, created.ID)
	if e != nil {
		t.Fatal(e)
	}
	if !got.HasCredential || got.HostKey != "" {
		t.Fatalf("public profile=%+v", got)
	}
}
