package terminal

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestBridgeForwardsInputOutputAndResize(t *testing.T) {
	session := newTestSession()
	resize := make(chan [2]uint16, 1)
	session.resize = resize
	bridgeErr := make(chan error, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		bridgeErr <- Bridge(r.Context(), conn, session)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":132,"rows":43}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case size := <-resize:
		if size != [2]uint16{132, 43} {
			t.Fatalf("unexpected resize: %v", size)
		}
	case <-time.After(time.Second):
		t.Fatal("resize was not forwarded")
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("whoami\n")); err != nil {
		t.Fatal(err)
	}
	input := make([]byte, len("whoami\n"))
	if _, err := io.ReadFull(session.inputReader, input); err != nil {
		t.Fatal(err)
	}
	if string(input) != "whoami\n" {
		t.Fatalf("unexpected input %q", input)
	}

	if _, err := session.outputWriter.Write([]byte("root\r\n")); err != nil {
		t.Fatal(err)
	}
	messageType, output, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.BinaryMessage || string(output) != "root\r\n" {
		t.Fatalf("unexpected output type=%d data=%q", messageType, output)
	}

	if err := conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-bridgeErr:
		if err != nil {
			t.Fatalf("bridge: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("bridge did not close")
	}
}

type testSession struct {
	inputReader  *io.PipeReader
	inputWriter  *io.PipeWriter
	outputReader *io.PipeReader
	outputWriter *io.PipeWriter
	resize       chan [2]uint16
	done         chan struct{}
	closeOnce    sync.Once
}

func newTestSession() *testSession {
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	return &testSession{
		inputReader: inputReader, inputWriter: inputWriter,
		outputReader: outputReader, outputWriter: outputWriter,
		done: make(chan struct{}),
	}
}

func (s *testSession) Input() io.WriteCloser { return s.inputWriter }
func (s *testSession) Output() io.Reader     { return s.outputReader }

func (s *testSession) Resize(cols, rows uint16) error {
	s.resize <- [2]uint16{cols, rows}
	return nil
}

func (s *testSession) Wait() error {
	<-s.done
	return context.Canceled
}

func (s *testSession) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		_ = s.inputReader.Close()
		_ = s.inputWriter.Close()
		_ = s.outputReader.Close()
		_ = s.outputWriter.Close()
	})
	return nil
}
