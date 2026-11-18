// Package at manages a serial command session.
package at

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.bug.st/serial"
)

// Rejection means the modem returned a terminal error and the session is
// still synchronized.
type Rejection string

func (e Rejection) Error() string { return string(e) }

// Session is owned by a single caller. After a transport error or cancellation,
// close it before issuing another command, as a late reply may still arrive.
type Session struct {
	port    serial.Port
	pending string
}

func Open(path string) (*Session, error) {
	port, err := serial.Open(path, &serial.Mode{BaudRate: 115200})
	if err != nil {
		return nil, err
	}
	if err := port.SetReadTimeout(200 * time.Millisecond); err != nil {
		port.Close()
		return nil, err
	}
	if err := port.ResetInputBuffer(); err != nil {
		port.Close()
		return nil, err
	}
	return &Session{port: port}, nil
}

func (s *Session) Close() error { return s.port.Close() }

// Query reads a complete reply. It filters command echo and sends unrelated
// unsolicited lines to notify. For unprefixed replies use a matching function
// that recognizes their format.
func (s *Session) Query(ctx context.Context, command string, match func(string) bool, notify func(string)) ([]string, error) {
	if err := s.write(ctx, command+"\r"); err != nil {
		return nil, err
	}
	return s.reply(ctx, command, match, notify)
}

// Submit waits for a modem input prompt before writing a PDU and Ctrl-Z.
// The boolean reports whether submission may have reached the modem.
func (s *Session) Submit(ctx context.Context, command, pdu string) ([]string, bool, error) {
	if err := s.write(ctx, command+"\r"); err != nil {
		return nil, false, err
	}
	promptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		line, err := s.line(promptCtx)
		if err != nil {
			_, _ = s.port.Write([]byte{0x1b})
			return nil, false, err
		}
		if terminalError(line) {
			return nil, false, Rejection(line)
		}
		if line == ">" {
			break
		}
	}
	if err := ctx.Err(); err != nil {
		_, _ = s.port.Write([]byte{0x1b})
		return nil, false, err
	}
	if err := s.write(ctx, pdu+"\x1a"); err != nil {
		_, _ = s.port.Write([]byte{0x1b})
		return nil, true, err
	}
	lines, err := s.reply(ctx, "", func(line string) bool { return strings.HasPrefix(line, "+CMGS:") }, nil)
	return lines, true, err
}

func (s *Session) write(ctx context.Context, data string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	n, err := s.port.Write([]byte(data))
	if err == nil && n != len(data) {
		return errors.New("short serial write")
	}
	return err
}

func terminalError(line string) bool {
	return line == "ERROR" || strings.HasPrefix(line, "+CME ERROR:") || strings.HasPrefix(line, "+CMS ERROR:")
}

func (s *Session) reply(ctx context.Context, command string, match func(string) bool, notify func(string)) ([]string, error) {
	var reply []string
	for {
		line, err := s.line(ctx)
		if err != nil {
			return reply, err
		}
		switch {
		case line == "" || line == command:
		case line == "OK":
			return reply, nil
		case line == ">":
			_, _ = s.port.Write([]byte{0x1b})
			return reply, errors.New("Interactive input prompt cancelled")
		case line == "NO CARRIER" || line == "NO ANSWER" || line == "NO DIALTONE" || line == "BUSY":
			return reply, Rejection(line)
		case terminalError(line):
			return reply, Rejection(line)
		case match(line):
			if len(reply) >= 4096 {
				return nil, errors.New("too many lines in AT reply")
			}
			reply = append(reply, line)
		case notify != nil:
			notify(line)
		}
	}
}

func (s *Session) line(ctx context.Context) (string, error) {
	buf := make([]byte, 512)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if strings.HasPrefix(s.pending, ">") {
			s.pending = s.pending[1:]
			return ">", nil
		}
		if end := strings.IndexAny(s.pending, "\r\n"); end >= 0 {
			line := strings.TrimSpace(s.pending[:end])
			s.pending = s.pending[end+1:]
			return line, nil
		}
		if len(s.pending) >= 4096 {
			return "", fmt.Errorf("AT line exceeds 4096 bytes")
		}
		n, err := s.port.Read(buf)
		if err != nil {
			return "", err
		}
		s.pending += string(buf[:n])
	}
}
