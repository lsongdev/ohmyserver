//go:build !windows

package terminal

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestLocalShellUsesPTY(t *testing.T) {
	session, err := (Local{Shell: "/bin/sh"}).Open(context.Background(), 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	if _, err := session.Input().Write([]byte("printf 'terminal-ready\\n'\nexit\n")); err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		var output []byte
		for {
			n, err := session.Output().Read(buf)
			output = append(output, buf[:n]...)
			if bytes.Contains(output, []byte("terminal-ready")) {
				result <- nil
				return
			}
			if err != nil {
				result <- err
				return
			}
		}
	}()

	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for shell output")
	}
}
