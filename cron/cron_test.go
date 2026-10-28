package cron

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/modules/cron/runner"
	sshapi "github.com/lsongdev/ohmyserver/modules/ssh/api"
	sshweb "github.com/lsongdev/ohmyserver/modules/ssh/web"
	"golang.org/x/crypto/ssh"
	_ "modernc.org/sqlite"
)

// execServer accepts password "secret" and runs "exec" requests with the
// local /bin/sh, reporting the exit status.
func execServer(t *testing.T) string {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	config := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
		if string(p) == "secret" {
			return nil, nil
		}
		return nil, ssh.ErrNoAuth
	}}
	config.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
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
							if req.Type != "exec" {
								req.Reply(false, nil)
								continue
							}
							req.Reply(true, nil)
							n := binary.BigEndian.Uint32(req.Payload)
							cmd := exec.Command("/bin/sh", "-c", string(req.Payload[4:4+n]))
							cmd.Stdout, cmd.Stderr = ch, ch.Stderr()
							cmd.Run()
							ch.SendRequest("exit-status", false, binary.BigEndian.AppendUint32(nil, uint32(cmd.ProcessState.ExitCode())))
							return
						}
					}()
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func TestSSHHosts(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "ohmyserver.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	terminals, err := sshweb.New(ctx, db, sshweb.Options{})
	if err != nil {
		t.Fatal(err)
	}
	host, portText, _ := net.SplitHostPort(execServer(t))
	port, _ := strconv.Atoi(portText)
	h, err := terminals.Service.Create(ctx, sshapi.Input{Name: "box", Address: host, Port: port, User: "alice", Auth: "password", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	r := SSHHosts{Service: terminals.Service}
	targets, err := r.Targets(ctx)
	target := "ssh:" + strconv.FormatInt(h.ID, 10)
	if err != nil || len(targets) != 1 || targets[0].ID != target || targets[0].Name != "box" {
		t.Fatalf("targets %+v %v", targets, err)
	}

	var out bytes.Buffer
	code, err := r.Run(ctx, target, runner.Command{Script: "pwd; echo $((6*7)); exit 5", Dir: "/nonexistent/it's here"}, &out)
	// The directory does not exist: nothing of the script runs.
	if err != nil || code != 1 || bytes.Contains(out.Bytes(), []byte("42")) {
		t.Fatalf("missing directory: code %d err %v out %q", code, err, out.String())
	}
	out.Reset()
	code, err = r.Run(ctx, target, runner.Command{Script: "pwd; echo $((6*7)); exit 5", Dir: "/"}, &out)
	if err != nil || code != 5 || out.String() != "/\n42\n" {
		t.Fatalf("code %d err %v out %q", code, err, out.String())
	}
	// The first run pinned the host key.
	if v, _ := terminals.Service.Get(ctx, h.ID); v.HostKeySHA256 == "" {
		t.Fatal("host key not pinned")
	}

	cctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := r.Run(cctx, target, runner.Command{Script: "sleep 10"}, &out); err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("cancel: %v after %v", err, time.Since(start))
	}
	if _, err := r.Run(ctx, "ssh:999", runner.Command{Script: "true"}, &out); err == nil {
		t.Fatal("unknown host ran")
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Fatal(got)
	}
}
