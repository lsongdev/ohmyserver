// Command nat-go serves NAT detection independently of OhMyServer.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lsongdev/ohmyserver/modules/nat/web"
)

const usage = `usage: nat-go [flags]

Serves NAT detection. Runs IPv4 UDP NAT detection from this machine. No root privileges are needed. Set NAT_PASSWORD to require HTTP basic authentication
(any user name).

Flags:
`

func main() {
	flag.Usage = func() { fmt.Fprint(flag.CommandLine.Output(), usage); flag.PrintDefaults() }
	listen := flag.String("listen", "127.0.0.1:8080", "address for the UI and API")
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	if err := run(*listen, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(listen string, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	network := web.New(ctx, web.Options{})
	defer network.Close()

	password := os.Getenv("NAT_PASSWORD")
	if password == "" && !isLoopback(listen) {
		return fmt.Errorf("set NAT_PASSWORD before listening on a non-loopback address")
	}
	srv := &http.Server{Addr: listen, Handler: network.Standalone(password), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	logger.Info("nat-go listening", "url", "http://"+listen)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	// Requests are short: give them a moment, then close.
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		return srv.Close()
	}
	return nil
}

func isLoopback(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return host == "localhost" || ip != nil && ip.IsLoopback()
}
