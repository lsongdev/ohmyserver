//go:build !windows

package terminal

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// Local opens the current user's shell in a real PTY.
type Local struct {
	Shell string
}

func (b Local) Open(_ context.Context, cols, rows uint16) (Session, error) {
	shell := b.Shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, err
	}
	return &localSession{cmd: cmd, tty: tty}, nil
}

type localSession struct {
	cmd       *exec.Cmd
	tty       *os.File
	closeOnce sync.Once
}

func (s *localSession) Input() io.WriteCloser { return s.tty }
func (s *localSession) Output() io.Reader     { return s.tty }

func (s *localSession) Resize(cols, rows uint16) error {
	return pty.Setsize(s.tty, &pty.Winsize{Cols: cols, Rows: rows})
}

func (s *localSession) Wait() error { return s.cmd.Wait() }

func (s *localSession) Close() error {
	var err error
	s.closeOnce.Do(func() {
		err = s.tty.Close()
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
	})
	return err
}
