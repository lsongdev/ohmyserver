package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/lsongdev/ohmyserver/internal/shell"
	"github.com/lsongdev/ohmyserver/modules/cellular/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	listen := flag.String("listen", "127.0.0.1:10181", "HTTP listen address")
	port := flag.String("port", "", "AT serial device (default: discover Quectel modem)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	password := os.Getenv("CELLULAR_PASSWORD")
	cellular := web.New(web.Options{Port: *port, Authenticated: password != ""})
	defer cellular.Close()
	opts := shell.Options{Title: "Cellular", Home: "cellular"}
	if password != "" {
		opts.Auth = &shell.Auth{Secret: password}
	}
	host, err := shell.New(opts, cellular)
	if err != nil {
		return err
	}
	return shell.ListenAndServe(ctx, *listen, host.Handler())
}
