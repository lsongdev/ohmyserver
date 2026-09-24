package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	terminalweb "github.com/lsongdev/ohmyserver/modules/terminal/web"
	transport "github.com/lsongdev/ohmyserver/pkg/terminal"
)

type localTerminal struct{ shell string }

func (s localTerminal) OpenTerminal(ctx context.Context, kind, id string, cols, rows uint16) (transport.Session, error) {
	if kind != "local" || id != "" {
		return nil, errors.New("standalone terminal only provides the local shell")
	}
	return (transport.Local{Shell: s.shell}).Open(ctx, cols, rows)
}

func main() {
	if len(os.Args) < 3 || os.Args[1] != "web" || os.Args[2] != "serve" {
		log.Fatal("usage: terminal web serve [--listen address] [--shell path]")
	}
	flags := flag.NewFlagSet("terminal web serve", flag.ExitOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	shell := flags.String("shell", strings.TrimSpace(os.Getenv("SHELL")), "shell executable for local terminal")
	_ = flags.Parse(os.Args[3:])
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	module := terminalweb.New(localTerminal{shell: *shell})
	srv := &http.Server{Addr: *listen, Handler: module.StandaloneHandler()}
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	log.Printf("Terminal module listening on http://%s", *listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
