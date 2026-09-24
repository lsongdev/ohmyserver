module github.com/lsongdev/ohmyserver/modules/terminal

go 1.26.1

require (
	github.com/gorilla/websocket v1.5.3
	github.com/lsongdev/ohmyserver/pkg/terminal v0.0.0
)

require github.com/creack/pty v1.1.24 // indirect

replace github.com/lsongdev/ohmyserver/pkg/terminal => ../../pkg/terminal
