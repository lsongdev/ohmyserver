// Command diskmgr serves the disk module on its own: the UI and API show
// capacity, RAID arrays and SMART health, and start self-tests; smartctl
// needs root. A panel shell can mount the same module (web.New) beside
// others instead.
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

	"github.com/lsongdev/ohmyserver/diskmgr/web"
)

const usage = `usage: diskmgr [flags]

Serves the disk module. Reading SMART data needs root, so diskmgr
usually runs as root. Set DISKMGR_PASSWORD to require HTTP basic authentication
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
	disks := web.New(web.Options{Logger: logger})

	password := os.Getenv("DISKMGR_PASSWORD")
	if password == "" && !isLoopback(listen) {
		logger.Warn("serving without authentication on a non-loopback address; set DISKMGR_PASSWORD")
	}
	srv := &http.Server{Addr: listen, Handler: disks.Standalone(password), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	logger.Info("diskmgr listening", "url", "http://"+listen)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	// Give requests a moment to finish, then close.
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
