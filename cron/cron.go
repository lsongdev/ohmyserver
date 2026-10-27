// Package cron connects the cron-go scheduler to the other modules. It
// implements cron-go's runner.Runner for the hosts saved in ssh-go, so a
// job can run on any of them; the modules never import one another.
package cron

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/lsongdev/ohmyserver/modules/cron/runner"
	sshapi "github.com/lsongdev/ohmyserver/modules/ssh/api"
	"golang.org/x/crypto/ssh"
)

// SSHHosts runs job commands on ssh-go's saved hosts, with their stored
// credentials and pinned host keys. Target IDs are "ssh:<host id>".
type SSHHosts struct{ Service *sshapi.Service }

var _ runner.Runner = SSHHosts{}

func (SSHHosts) Kind() string { return "ssh" }

func (h SSHHosts) Targets(ctx context.Context) ([]runner.Target, error) {
	list, err := h.Service.List(ctx)
	if err != nil {
		return nil, err
	}
	targets := make([]runner.Target, len(list))
	for i, host := range list {
		targets[i] = runner.Target{ID: "ssh:" + strconv.FormatInt(host.ID, 10), Name: host.Name, Kind: "ssh"}
	}
	return targets, nil
}

// Run runs cmd through the host's login shell; a Dir that cannot be
// entered stops the script before it starts (exit status 1).
// Cancelling closes the connection, which ends the remote command.
func (h SSHHosts) Run(ctx context.Context, target string, cmd runner.Command, out io.Writer) (int, error) {
	_, idText, _ := strings.Cut(target, ":")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return -1, fmt.Errorf("invalid target %q", target)
	}
	conn, err := h.Service.Connect(ctx, id)
	if err != nil {
		return -1, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	session, err := conn.NewSession()
	if err != nil {
		return -1, err
	}
	defer session.Close()
	// The session copies stdout and stderr in separate goroutines.
	w := &lockedWriter{w: out}
	session.Stdout, session.Stderr = w, w
	script := cmd.Script
	if cmd.Dir != "" {
		script = "cd " + shellQuote(cmd.Dir) + " || exit 1\n" + script
	}
	err = session.Run(script)
	var exit *ssh.ExitError
	switch {
	case ctx.Err() != nil:
		return -1, ctx.Err()
	case errors.As(err, &exit):
		return exit.ExitStatus(), nil
	case err != nil:
		return -1, err
	}
	return 0, nil
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
