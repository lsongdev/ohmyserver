package at

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.bug.st/serial"
)

type testPort struct {
	serial.Port
	source *strings.Reader
}

func (p testPort) Write(data []byte) (int, error) { return len(data), nil }

func (p testPort) Read(data []byte) (int, error) {
	if p.source.Len() == 0 {
		time.Sleep(time.Millisecond)
		return 0, nil
	}
	return p.source.Read(data[:1])
}

func TestQueryFrames(t *testing.T) {
	session := Session{port: testPort{source: strings.NewReader("\r\nAT+CPIN?\r\r\n+CMTI: \"SM\",1\r\n+CPIN: READY\r\nOK\r\nERROR\r\n")}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var unsolicited []string
	reply, err := session.Query(ctx, "AT+CPIN?", func(line string) bool {
		return strings.HasPrefix(line, "+CPIN:")
	}, func(line string) { unsolicited = append(unsolicited, line) })
	if err != nil || len(reply) != 1 || reply[0] != "+CPIN: READY" {
		t.Fatalf("reply=%q error=%v", reply, err)
	}
	if len(unsolicited) != 1 || !strings.HasPrefix(unsolicited[0], "+CMTI:") {
		t.Fatalf("unsolicited=%q", unsolicited)
	}
	_, err = session.Query(ctx, "AT", func(string) bool { return false }, nil)
	var rejection Rejection
	if !errors.As(err, &rejection) {
		t.Fatalf("expected rejection, got %v", err)
	}
}

func TestQueryDeadline(t *testing.T) {
	session := Session{port: testPort{source: strings.NewReader("+CPIN: READY\r\n")}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := session.Query(ctx, "AT+CPIN?", func(string) bool { return true }, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
}

type recordingPort struct {
	testPort
	writes []string
}

func (p *recordingPort) Write(data []byte) (int, error) {
	p.writes = append(p.writes, string(data))
	return len(data), nil
}

func TestSubmitPrompt(t *testing.T) {
	port := &recordingPort{testPort: testPort{source: strings.NewReader("\r\n+CMTI: \"SM\",2\r\n> \r\n+CMGS: 11\r\nOK\r\n")}}
	session := Session{port: port}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lines, submitted, err := session.Submit(ctx, "AT+CMGS=2", "001122")
	if err != nil || !submitted || len(lines) != 1 || lines[0] != "+CMGS: 11" {
		t.Fatalf("unexpected submit result: %q, %t, %v", lines, submitted, err)
	}
	if len(port.writes) != 2 || port.writes[0] != "AT+CMGS=2\r" || port.writes[1] != "001122\x1a" {
		t.Fatalf("writes: %q", port.writes)
	}
}

func TestSubmitRejectedBeforePrompt(t *testing.T) {
	port := &recordingPort{testPort: testPort{source: strings.NewReader("\r\n+CMS ERROR: 310\r\n")}}
	session := Session{port: port}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, submitted, err := session.Submit(ctx, "AT+CMGS=2", "001122")
	var rejection Rejection
	if submitted || !errors.As(err, &rejection) || len(port.writes) != 1 {
		t.Fatalf("submission=%t, error=%v, writes=%q", submitted, err, port.writes)
	}
}
