package web

import (
	"context"
	"database/sql"
	"net"
	"strconv"
	"testing"

	forward "github.com/lsongdev/ohmyserver/modules/forward"
	_ "modernc.org/sqlite"
)

func TestLocalServiceDisabledModePersistsWithoutBinding(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", "file:forward-disabled-service-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	service := NewLocalService(repo)
	service.Close()
	listener := &forward.Listener{Name: "disabled listener", Protocol: "tcp", ListenPort: port, ForwardHost: "127.0.0.1", ForwardPort: 1, Status: "active"}
	if err := service.CreateForwardListener(context.Background(), listener); err != nil {
		t.Fatal(err)
	}
	bound, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("disabled module service unexpectedly bound port %d: %v", port, err)
	}
	_ = bound.Close()

	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if bound, err = net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port))); err == nil {
		_ = bound.Close()
		t.Fatalf("enabled module service did not bind port %d", port)
	}
	service.Close()
	bound, err = net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("closed module service still owns port %d: %v", port, err)
	}
	_ = bound.Close()
}
