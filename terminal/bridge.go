package terminal

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 50 * time.Second
)

type frame struct {
	messageType int
	data        []byte
}

type bridgeResult struct {
	source string
	err    error
}

// Bridge forwards one WebSocket connection to one interactive session.
// It guarantees that exactly one goroutine writes WebSocket messages.
func Bridge(ctx context.Context, conn *websocket.Conn, session Session) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var closeOnce sync.Once
	closeSession := func() {
		closeOnce.Do(func() {
			_ = session.Input().Close()
			_ = session.Close()
		})
	}
	defer closeSession()
	defer conn.Close()

	frames := make(chan frame, 32)
	results := make(chan bridgeResult, 4)
	writerDone := make(chan struct{})

	go writePump(ctx, conn, frames, results, writerDone)
	go readPump(ctx, conn, session, results)
	go outputPump(ctx, session.Output(), frames, results)
	go func() {
		results <- bridgeResult{source: "wait", err: session.Wait()}
	}()

	result := <-results
	if result.source == "wait" {
		// Wait means the process exited, but its output pipe can still contain
		// final bytes. Let outputPump drain them before closing the transport.
		for {
			result = <-results
			if result.source == "output" {
				result.err = nil
				break
			}
			if result.source == "read" || result.source == "write" {
				break
			}
		}
	}
	closeSession()
	cancel()
	_ = conn.Close()
	<-writerDone

	if isNormalClose(result.err) {
		return nil
	}
	return result.err
}

func readPump(ctx context.Context, conn *websocket.Conn, session Session, results chan<- bridgeResult) {
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			results <- bridgeResult{source: "read", err: err}
			return
		}

		switch messageType {
		case websocket.BinaryMessage:
			if _, err := session.Input().Write(data); err != nil {
				results <- bridgeResult{source: "read", err: err}
				return
			}
		case websocket.TextMessage:
			message, err := parseClientMessage(data)
			if err != nil {
				results <- bridgeResult{source: "read", err: err}
				return
			}
			if err := session.Resize(message.Cols, message.Rows); err != nil {
				results <- bridgeResult{source: "read", err: err}
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func outputPump(ctx context.Context, output io.Reader, frames chan<- frame, results chan<- bridgeResult) {
	buf := make([]byte, 32*1024)
	for {
		n, err := output.Read(buf)
		if n > 0 {
			data := append([]byte(nil), buf[:n]...)
			select {
			case frames <- frame{messageType: websocket.BinaryMessage, data: data}:
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			results <- bridgeResult{source: "output", err: err}
			return
		}
	}
}

func writePump(
	ctx context.Context,
	conn *websocket.Conn,
	frames <-chan frame,
	results chan<- bridgeResult,
	done chan<- struct{},
) {
	defer close(done)
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case item := <-frames:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(item.messageType, item.data); err != nil {
				results <- bridgeResult{source: "write", err: err}
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				results <- bridgeResult{source: "write", err: err}
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func isNormalClose(err error) bool {
	return err == nil ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, context.Canceled) ||
		websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway)
}

// ErrorMessage encodes an error control message for failures before Bridge starts.
func ErrorMessage(err error) []byte {
	return encodeServerMessage(serverMessage{Type: "error", Message: err.Error()})
}
