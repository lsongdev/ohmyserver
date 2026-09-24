//go:build windows

package terminal

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
)

// Local opens cmd.exe on Windows. Windows pipes do not expose resize, but keep
// the same session contract so browser terminals remain functional.
type Local struct {
	Shell string
}

func (b Local) Open(ctx context.Context, _, _ uint16) (Session, error) {
	shell := b.Shell
	if shell == "" {
		shell = os.Getenv("COMSPEC")
	}
	if shell == "" {
		shell = "cmd.exe"
	}

	cmd := exec.CommandContext(ctx, shell)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, writer := io.Pipe()
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = output.Close()
		_ = writer.Close()
		return nil, err
	}
	return &localSession{
		cmd: cmd, stdin: stdin, output: output, writer: writer,
	}, nil
}

type localSession struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	output    *io.PipeReader
	writer    *io.PipeWriter
	closeOnce sync.Once
}

func (s *localSession) Input() io.WriteCloser    { return s.stdin }
func (s *localSession) Output() io.Reader        { return s.output }
func (s *localSession) Resize(_, _ uint16) error { return nil }
func (s *localSession) Wait() error {
	err := s.cmd.Wait()
	_ = s.writer.CloseWithError(err)
	return err
}

func (s *localSession) Close() error {
	s.closeOnce.Do(func() {
		_ = s.stdin.Close()
		_ = s.output.Close()
		_ = s.writer.Close()
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
	})
	return nil
}
