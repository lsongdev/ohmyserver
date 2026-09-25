package docker

import (
	"context"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// TerminalSession is the PTY capability the application composition layer can
// inject into a terminal module without either feature importing the other.
type TerminalSession interface {
	Input() io.WriteCloser
	Output() io.Reader
	Resize(cols, rows uint16) error
	Wait() error
	Close() error
}

type terminalSession struct {
	client   *DockerClient
	execID   string
	response types.HijackedResponse
	ctx      context.Context
	cancel   context.CancelFunc
}

func (c *DockerClient) OpenTerminal(ctx context.Context, containerID string, cols, rows uint16) (TerminalSession, error) {
	exec, err := c.Client.ContainerExecCreate(ctx, containerID, types.ExecConfig{
		Cmd: []string{"/bin/sh"}, Tty: true, AttachStdin: true, AttachStdout: true, AttachStderr: true,
	})
	if err != nil {
		return nil, err
	}
	response, err := c.Client.ContainerExecAttach(ctx, exec.ID, types.ExecStartCheck{Tty: true})
	if err != nil {
		return nil, err
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	session := &terminalSession{client: c, execID: exec.ID, response: response, ctx: sessionCtx, cancel: cancel}
	if err := session.Resize(cols, rows); err != nil {
		_ = session.Close()
		return nil, err
	}
	return session, nil
}

func (s *terminalSession) Input() io.WriteCloser { return s.response.Conn }
func (s *terminalSession) Output() io.Reader     { return s.response.Reader }
func (s *terminalSession) Resize(cols, rows uint16) error {
	return s.client.Client.ContainerExecResize(s.ctx, s.execID, container.ResizeOptions{Width: uint(cols), Height: uint(rows)})
}
func (s *terminalSession) Wait() error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case <-ticker.C:
			info, err := s.client.Client.ContainerExecInspect(s.ctx, s.execID)
			if err != nil {
				return err
			}
			if !info.Running {
				return nil
			}
		}
	}
}
func (s *terminalSession) Close() error {
	s.cancel()
	s.response.Close()
	return nil
}
