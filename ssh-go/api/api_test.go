package api_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"

	"github.com/lsongdev/ssh-go/api"
	"github.com/lsongdev/ssh-go/hosts"
	"github.com/lsongdev/ssh-go/store"
)

// sshServer is a minimal SSH server: it accepts user "alice" with password
// "secret" or with any key in keys, and runs a "shell" that echoes input
// prefixed with "echo:" until it reads "exit", then exits with status 3.
type sshServer struct {
	addr string

	mu      sync.Mutex
	keys    []ssh.PublicKey
	resized []string // "COLSxROWS" of pty-req and window-change
	hostKey ssh.Signer
	ln      net.Listener
}

func newSSHServer(t *testing.T) *sshServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &sshServer{addr: ln.Addr().String(), ln: ln}
	s.setHostKey(t)
	t.Cleanup(func() { ln.Close() })
	go s.serve()
	return s
}

func (s *sshServer) setHostKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.hostKey = signer
	s.mu.Unlock()
}

func (s *sshServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *sshServer) handle(raw net.Conn) {
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == "alice" && string(pass) == "secret" {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			for _, k := range s.keys {
				if c.User() == "alice" && bytes.Equal(k.Marshal(), key.Marshal()) {
					return nil, nil
				}
			}
			return nil, errors.New("denied")
		},
	}
	s.mu.Lock()
	config.AddHostKey(s.hostKey)
	s.mu.Unlock()
	conn, chans, reqs, err := ssh.NewServerConn(raw, config)
	if err != nil {
		raw.Close()
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "no")
			continue
		}
		ch, requests, err := nc.Accept()
		if err != nil {
			return
		}
		go s.session(ch, requests)
	}
}

func (s *sshServer) session(ch ssh.Channel, requests <-chan *ssh.Request) {
	defer ch.Close()
	shell := make(chan struct{})
	go func() {
		for req := range requests {
			switch req.Type {
			case "pty-req":
				// string term, uint32 cols, uint32 rows, ...
				n := binary.BigEndian.Uint32(req.Payload)
				p := req.Payload[4+n:]
				s.record(binary.BigEndian.Uint32(p), binary.BigEndian.Uint32(p[4:]))
				req.Reply(true, nil)
			case "window-change":
				s.record(binary.BigEndian.Uint32(req.Payload), binary.BigEndian.Uint32(req.Payload[4:]))
			case "shell":
				req.Reply(true, nil)
				close(shell)
			default:
				req.Reply(false, nil)
			}
		}
	}()
	<-shell
	buf := make([]byte, 1024)
	for {
		n, err := ch.Read(buf)
		if err != nil {
			return
		}
		in := string(buf[:n])
		if strings.TrimSpace(in) == "exit" {
			ch.SendRequest("exit-status", false, binary.BigEndian.AppendUint32(nil, 3))
			return
		}
		ch.Write([]byte("echo:" + in))
	}
}

func (s *sshServer) record(cols, rows uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resized = append(s.resized, strconv.Itoa(int(cols))+"x"+strconv.Itoa(int(rows)))
}

type env struct {
	t      *testing.T
	svc    *api.Service
	server *httptest.Server
	ssh    *sshServer
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	st, err := store.New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := api.NewService(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(svc.Handler())
	t.Cleanup(server.Close)
	return &env{t: t, svc: svc, server: server, ssh: newSSHServer(t)}
}

func (e *env) call(method, path string, body any, want int, out any) string {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, e.server.URL+path, r)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		e.t.Fatalf("%s %s: %d, want %d: %s", method, path, res.StatusCode, want, data)
	}
	if out != nil {
		var envelope struct{ Data json.RawMessage }
		json.Unmarshal(data, &envelope)
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			e.t.Fatalf("%s %s: %v: %s", method, path, err, data)
		}
	}
	return string(data)
}

func (e *env) hostInput(auth string) map[string]any {
	host, port, _ := net.SplitHostPort(e.ssh.addr)
	p, _ := strconv.Atoi(port)
	in := map[string]any{"name": "box", "address": host, "port": p, "user": "alice", "auth": auth}
	if auth == "password" {
		in["password"] = "secret"
	}
	return in
}

// terminal opens the WebSocket terminal of host id.
func (e *env) terminal(id int64) *websocket.Conn {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(e.server.URL, "http") + "/hosts/" + strconv.FormatInt(id, 10) + "/terminal?cols=100&rows=30"
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return ws
}

// readUntil reads binary frames until their concatenation contains want.
func readUntil(t *testing.T, ws *websocket.Conn, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var got strings.Builder
	for !strings.Contains(got.String(), want) {
		_, data, err := ws.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for %q, got %q: %v", want, got.String(), err)
		}
		got.Write(data)
	}
}

// closeStatus reads until the socket closes and returns its close frame.
func closeStatus(t *testing.T, ws *websocket.Conn) (websocket.StatusCode, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, _, err := ws.Read(ctx)
		if err == nil {
			continue
		}
		var ce websocket.CloseError
		if errors.As(err, &ce) {
			return ce.Code, ce.Reason
		}
		t.Fatalf("socket ended without a close frame: %v", err)
	}
}

func TestHostsAndTerminal(t *testing.T) {
	e := newEnv(t)
	var h hosts.View
	e.call("POST", "/hosts", e.hostInput("password"), 201, &h)
	if h.ID == 0 || !h.HasPassword || h.HostKeySHA256 != "" {
		t.Fatalf("created %+v", h)
	}
	if body := e.call("GET", "/hosts", nil, 200, nil); strings.Contains(body, "secret") {
		t.Fatalf("password leaked: %s", body)
	}
	e.call("POST", "/hosts", e.hostInput("password"), 409, nil)
	bad := e.hostInput("password")
	delete(bad, "password")
	e.call("POST", "/hosts", bad, 422, nil)

	// Testing does not pin.
	var res api.TestResult
	e.call("POST", "/test", map[string]any{"id": h.ID, "name": "box", "address": h.Address, "port": h.Port, "user": "alice", "auth": "password"}, 200, &res)
	if res.Pinned || !strings.HasPrefix(res.HostKeySHA256, "SHA256:") {
		t.Fatalf("test %+v", res)
	}
	wrong := e.hostInput("password")
	wrong["password"] = "nope"
	e.call("POST", "/test", wrong, 502, nil)

	ws := e.terminal(h.ID)
	ws.Write(context.Background(), websocket.MessageBinary, []byte("hello"))
	readUntil(t, ws, "echo:hello")
	ws.Write(context.Background(), websocket.MessageText, []byte(`{"type":"resize","cols":120,"rows":40}`))
	time.Sleep(100 * time.Millisecond)
	ws.Write(context.Background(), websocket.MessageBinary, []byte("exit"))
	if code, reason := closeStatus(t, ws); code != websocket.StatusNormalClosure || reason != "exited with code 3" {
		t.Fatalf("close %d %q", code, reason)
	}
	e.ssh.mu.Lock()
	if got := strings.Join(e.ssh.resized, " "); got != "100x30 120x40" {
		t.Errorf("sizes %q", got)
	}
	e.ssh.mu.Unlock()

	// The first connection pinned the host key.
	e.call("GET", "/hosts/1", nil, 200, &h)
	if h.HostKeySHA256 != res.HostKeySHA256 || h.LastConnectedAt.IsZero() {
		t.Fatalf("after connecting %+v", h)
	}

	// A new server key is refused until the pinned one is forgotten.
	e.ssh.setHostKey(t)
	if code, reason := closeStatus(t, e.terminal(h.ID)); code != api.StatusHostKeyChanged || !strings.HasPrefix(reason, "SHA256:") {
		t.Fatalf("changed key: %d %q", code, reason)
	}
	e.call("DELETE", "/hosts/1/host-key", nil, 204, nil)
	ws = e.terminal(h.ID)
	ws.Write(context.Background(), websocket.MessageBinary, []byte("again"))
	readUntil(t, ws, "echo:again")
	ws.Close(websocket.StatusNormalClosure, "")

	var sum api.Summary
	e.call("GET", "/summary", nil, 200, &sum)
	if sum.Hosts != 1 {
		t.Fatalf("summary %+v", sum)
	}
	e.call("DELETE", "/hosts/1", nil, 204, nil)
	e.call("GET", "/hosts/1/terminal", nil, 404, nil)
}

// Hosts using the panel key log in once its public key is authorized.
func TestPanelKey(t *testing.T) {
	e := newEnv(t)
	var key struct {
		PublicKey string `json:"public_key"`
	}
	e.call("GET", "/key", nil, 200, &key)
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key.PublicKey))
	if err != nil || pub.Type() != ssh.KeyAlgoED25519 {
		t.Fatalf("key %q: %v", key.PublicKey, err)
	}
	var h hosts.View
	e.call("POST", "/hosts", e.hostInput("panel"), 201, &h)
	if code, _ := closeStatus(t, e.terminal(h.ID)); code != api.StatusConnectFailed {
		t.Fatalf("unauthorized key: %d", code)
	}
	e.ssh.mu.Lock()
	e.ssh.keys = append(e.ssh.keys, pub)
	e.ssh.mu.Unlock()
	ws := e.terminal(h.ID)
	ws.Write(context.Background(), websocket.MessageBinary, []byte("hi"))
	readUntil(t, ws, "echo:hi")
	ws.Close(websocket.StatusNormalClosure, "")
}

// Updating keeps secrets left empty and forgets the pinned key when the
// address changes.
func TestUpdateKeepsSecrets(t *testing.T) {
	e := newEnv(t)
	var h hosts.View
	e.call("POST", "/hosts", e.hostInput("password"), 201, &h)
	ws := e.terminal(h.ID)
	ws.Write(context.Background(), websocket.MessageBinary, []byte("x"))
	readUntil(t, ws, "echo:x")
	ws.Close(websocket.StatusNormalClosure, "")

	in := e.hostInput("password")
	delete(in, "password")
	in["description"] = "renamed"
	e.call("PUT", "/hosts/1", in, 200, &h)
	if !h.HasPassword || h.HostKeySHA256 == "" || h.Description != "renamed" {
		t.Fatalf("update %+v", h)
	}
	in["address"] = "localhost"
	h = hosts.View{} // empty fields are omitted from JSON
	e.call("PUT", "/hosts/1", in, 200, &h)
	if h.HostKeySHA256 != "" {
		t.Fatal("pinned key kept after the address changed")
	}
	// Switching to the panel key drops the stored password.
	in["auth"] = "panel"
	h = hosts.View{}
	e.call("PUT", "/hosts/1", in, 200, &h)
	if h.HasPassword {
		t.Fatal("password kept for panel auth")
	}
}
