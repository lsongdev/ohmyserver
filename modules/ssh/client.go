package ssh

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	xssh "golang.org/x/crypto/ssh"
)

// Client owns one authenticated SSH transport. Host key verification follows
// the profile's TOFU pin; callers persist ObservedHostKey after first connect.
type Client struct {
	client          *xssh.Client
	ObservedHostKey string
}

type TerminalSession interface {
	Input() io.WriteCloser
	Output() io.Reader
	Resize(cols, rows uint16) error
	Wait() error
	Close() error
}

func Connect(ctx context.Context, profile Profile) (*Client, error) {
	if profile.Port < 1 {
		profile.Port = 22
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	observed := ""
	cfg := &xssh.ClientConfig{User: profile.Username, Auth: []xssh.AuthMethod{}, HostKeyCallback: func(_ string, _ net.Addr, key xssh.PublicKey) error {
		observed = xssh.FingerprintSHA256(key)
		if profile.HostKey != "" && profile.HostKey != observed {
			return fmt.Errorf("SSH host key changed: expected %s, got %s", profile.HostKey, observed)
		}
		return nil
	}}
	switch profile.AuthType {
	case "password":
		cfg.Auth = append(cfg.Auth, xssh.Password(profile.Password))
	case "key":
		signer, err := xssh.ParsePrivateKey([]byte(profile.PrivateKey))
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		cfg.Auth = append(cfg.Auth, xssh.PublicKeys(signer))
	default:
		return nil, fmt.Errorf("unsupported SSH authentication type %q", profile.AuthType)
	}
	addr := net.JoinHostPort(profile.Host, strconv.Itoa(profile.Port))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	cc, ch, req, err := xssh.NewClientConn(conn, addr, cfg)
	stop()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	if err = conn.SetDeadline(time.Time{}); err != nil {
		_ = cc.Close()
		return nil, fmt.Errorf("clear SSH connection deadline: %w", err)
	}
	return &Client{client: xssh.NewClient(cc, ch, req), ObservedHostKey: observed}, nil
}
func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}
func (c *Client) Run(ctx context.Context, command string) ([]byte, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("SSH client is closed")
	}
	session, err := c.client.NewSession()
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = session.Close() })
	defer stop()
	defer session.Close()
	return session.CombinedOutput(command)
}
func (c *Client) OpenTerminal(cols, rows uint16) (TerminalSession, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("SSH client is closed")
	}
	session, err := c.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("create SSH session: %w", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("create SSH stdin: %w", err)
	}
	out, writer := io.Pipe()
	session.Stdout = writer
	session.Stderr = writer
	modes := xssh.TerminalModes{xssh.ECHO: 1, xssh.TTY_OP_ISPEED: 14400, xssh.TTY_OP_OSPEED: 14400}
	if err = session.RequestPty("xterm-256color", int(rows), int(cols), modes); err != nil {
		_ = out.Close()
		_ = writer.Close()
		_ = session.Close()
		return nil, fmt.Errorf("request SSH PTY: %w", err)
	}
	if err = session.Shell(); err != nil {
		_ = out.Close()
		_ = writer.Close()
		_ = session.Close()
		return nil, fmt.Errorf("start SSH shell: %w", err)
	}
	return &terminalSession{client: c, session: session, stdin: stdin, output: out, writer: writer}, nil
}

type terminalSession struct {
	client  *Client
	session *xssh.Session
	stdin   io.WriteCloser
	output  *io.PipeReader
	writer  *io.PipeWriter
	once    sync.Once
}

func (s *terminalSession) Input() io.WriteCloser { return s.stdin }
func (s *terminalSession) Output() io.Reader     { return s.output }
func (s *terminalSession) Resize(cols, rows uint16) error {
	return s.session.WindowChange(int(rows), int(cols))
}
func (s *terminalSession) Wait() error {
	err := s.session.Wait()
	_ = s.writer.CloseWithError(err)
	return err
}
func (s *terminalSession) Close() error {
	var err error
	s.once.Do(func() {
		_ = s.stdin.Close()
		_ = s.output.Close()
		_ = s.writer.Close()
		_ = s.session.Close()
		err = s.client.Close()
	})
	return err
}
