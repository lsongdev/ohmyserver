# terminal

`terminal` is a small transport layer for interactive sessions. It knows how
to move input, output, resize, and lifecycle events between a WebSocket and a
`Session`; it does not know about SSH profiles, databases, or Docker resources.

```go
type Session interface {
    Input() io.WriteCloser
    Output() io.Reader
    Resize(cols, rows uint16) error
    Wait() error
    Close() error
}
```

Backends own their resources:

- `Local` uses a PTY on Unix and a process-pipe fallback on Windows.
- `server/ssh.go` owns SSH authentication, host-key pinning, and SSH PTYs.
- `server/docker.go` owns Docker exec creation and resize operations.

The browser sends binary frames for stdin and JSON text frames for controls:

```json
{"type":"resize","cols":120,"rows":40}
```

Session output is returned in binary frames. A bridge uses one WebSocket writer
goroutine and closes the backend when either side exits.
