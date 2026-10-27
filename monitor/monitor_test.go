package monitor

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	sshapi "github.com/lsongdev/ohmyserver/modules/ssh/api"
	sshweb "github.com/lsongdev/ohmyserver/modules/ssh/web"
	"golang.org/x/crypto/ssh"
	_ "modernc.org/sqlite"
)

// fakeHost answers every exec request with output, and counts the
// connections it accepted. close drops every open connection.
type fakeHost struct {
	addr   string
	output atomic.Value // string
	conns  atomic.Int32
	mu     sync.Mutex
	open   []net.Conn
}

func newFakeHost(t *testing.T, output string) *fakeHost {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	config := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil }}
	config.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	f := &fakeHost{addr: ln.Addr().String()}
	f.output.Store(output)
	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			f.conns.Add(1)
			f.mu.Lock()
			f.open = append(f.open, raw)
			f.mu.Unlock()
			go func() {
				conn, chans, reqs, err := ssh.NewServerConn(raw, config)
				if err != nil {
					return
				}
				defer conn.Close()
				go ssh.DiscardRequests(reqs)
				for nc := range chans {
					ch, requests, _ := nc.Accept()
					go func() {
						defer ch.Close()
						for req := range requests {
							req.Reply(req.Type == "exec", nil)
							if req.Type == "exec" {
								ch.Write([]byte(f.output.Load().(string)))
								ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
								return
							}
						}
					}()
				}
			}()
		}
	}()
	return f
}

func (f *fakeHost) close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.open {
		c.Close()
	}
	f.open = nil
}

func TestSSHHosts(t *testing.T) {
	debian, err := os.ReadFile("../../monitor-go/procfs/testdata/debian.txt")
	if err != nil {
		t.Skip("monitor-go checkout not found:", err)
	}
	ctx := context.Background()
	db, _ := sql.Open("sqlite", filepath.Join(t.TempDir(), "ohmyserver.db")+"?_pragma=busy_timeout(5000)")
	defer db.Close()
	terminals, err := sshweb.New(ctx, db, sshweb.Options{})
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeHost(t, string(debian))
	host, portText, _ := net.SplitHostPort(fake.addr)
	port, _ := strconv.Atoi(portText)
	h, err := terminals.Service.Create(ctx, sshapi.Input{Name: "nas", Address: host, Port: port, User: "root", Auth: "password", Password: "x"})
	if err != nil {
		t.Fatal(err)
	}
	c := &SSHHosts{Service: terminals.Service}
	defer c.Close()
	list, err := c.Hosts(ctx)
	id := "ssh:" + strconv.FormatInt(h.ID, 10)
	if err != nil || len(list) != 1 || list[0].ID != id || list[0].Name != "nas" {
		t.Fatalf("hosts %+v %v", list, err)
	}

	for range 3 {
		r, info, err := c.Read(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if r.MemTotal == 0 || r.CPUTotal == 0 || !strings.Contains(info.OS, "Debian") {
			t.Fatalf("reading %+v info %+v", r, info)
		}
	}
	if n := fake.conns.Load(); n != 1 {
		t.Fatalf("%d connections for three readings, want 1", n)
	}

	// A dropped connection is replaced on the next reading.
	fake.close()
	if _, _, err := c.Read(ctx, id); err != nil {
		t.Fatalf("after the connection dropped: %v", err)
	}
	if n := fake.conns.Load(); n != 2 {
		t.Fatalf("%d connections, want 2", n)
	}

	// A host without /proc is reported, not misread.
	fake.output.Store("@stat\nhead: /proc/stat: No such file or directory\n@end\n")
	if _, _, err := c.Read(ctx, id); err == nil || !strings.Contains(err.Error(), "not a Linux host") {
		t.Fatalf("non-Linux host: %v", err)
	}

	// Deleting the host drops its connection.
	terminals.Service.Delete(ctx, h.ID)
	if list, _ := c.Hosts(ctx); len(list) != 0 || len(c.conns) != 0 {
		t.Fatalf("after delete: %+v, %d connections", list, len(c.conns))
	}
}
