package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"

	"github.com/lsongdev/ohmyserver/modules/ssh/hosts"
)

// Close codes of the terminal socket beyond the standard ones, so the UI
// can offer the right next step.
const (
	StatusConnectFailed  websocket.StatusCode = 4000 // reason: the error
	StatusHostKeyChanged websocket.StatusCode = 4001 // forget the pinned key to continue
)

// terminalControl is a text frame sent by the client.
type terminalControl struct {
	Type string `json:"type"` // "resize"
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// terminal opens a shell on a host over WebSocket, with the same protocol
// as docker-go's container terminal: binary frames carry raw terminal
// bytes both ways, the client sends {"type":"resize","cols":…,"rows":…}
// as text frames, and the socket closes with the shell's exit status as
// the reason.
func (s *Service) terminal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := s.store.Get(r.Context(), id); err != nil {
		respond(w, 0, nil, err)
		return
	}
	q := r.URL.Query()
	cols, rows := dimension(q.Get("cols"), 80), dimension(q.Get("rows"), 24)
	ws, err := websocket.Accept(w, r, nil) // rejects cross-origin requests
	if err != nil {
		return
	}
	defer ws.CloseNow()
	ctx := r.Context()

	conn, err := s.Connect(ctx, id)
	if err != nil {
		var hk *hosts.HostKeyError
		if errors.As(err, &hk) {
			closeWith(ws, StatusHostKeyChanged, hk.Offered)
		} else {
			closeWith(ws, StatusConnectFailed, errors.Unwrap(err).Error())
		}
		return
	}
	defer conn.Close()
	go conn.KeepAlive(keepAliveInterval)

	session, err := conn.NewSession()
	if err != nil {
		closeWith(ws, StatusConnectFailed, err.Error())
		return
	}
	defer session.Close()
	stdin, err := session.StdinPipe()
	if err != nil {
		closeWith(ws, websocket.StatusInternalError, err.Error())
		return
	}
	out := &socketWriter{ctx: ctx, ws: ws}
	session.Stdout, session.Stderr = out, out
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		closeWith(ws, StatusConnectFailed, "request a terminal: "+err.Error())
		return
	}
	if err := session.Shell(); err != nil {
		closeWith(ws, StatusConnectFailed, "start the shell: "+err.Error())
		return
	}
	s.sessions.Add(1)
	defer s.sessions.Add(-1)

	// Browser input → shell. When the browser goes away, closing the SSH
	// connection ends the shell and Wait below.
	go func() {
		defer conn.Close()
		for {
			kind, data, err := ws.Read(ctx)
			if err != nil {
				return
			}
			if kind == websocket.MessageBinary {
				if _, err := stdin.Write(data); err != nil {
					return
				}
				continue
			}
			var ctl terminalControl
			if json.Unmarshal(data, &ctl) == nil && ctl.Type == "resize" && ctl.Cols > 0 && ctl.Rows > 0 {
				session.WindowChange(ctl.Rows, ctl.Cols)
			}
		}
	}()

	reason := "session closed"
	var exit *ssh.ExitError
	switch err := session.Wait(); {
	case err == nil:
		reason = "exited with code 0"
	case errors.As(err, &exit):
		reason = fmt.Sprintf("exited with code %d", exit.ExitStatus())
	}
	closeWith(ws, websocket.StatusNormalClosure, reason)
}

// socketWriter sends shell output as binary frames. The SSH session copies
// stdout and stderr concurrently, so writes are serialized.
type socketWriter struct {
	ctx context.Context
	ws  *websocket.Conn
	mu  sync.Mutex
}

func (w *socketWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.ws.Write(w.ctx, websocket.MessageBinary, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func dimension(v string, fallback int) int {
	n, err := strconv.ParseUint(v, 10, 16)
	if err != nil || n == 0 {
		return fallback
	}
	return int(n)
}

// closeWith closes the socket; close reasons are limited to 123 bytes.
func closeWith(ws *websocket.Conn, code websocket.StatusCode, reason string) {
	if len(reason) > 120 {
		reason = reason[:120]
	}
	ws.Close(code, reason)
}
