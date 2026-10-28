// Command domains serves the domains module on its own: DNS provider
// accounts (Cloudflare, …) and the zones and records they hold. A panel
// shell can mount the same module (web.New) beside others instead.
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
	"path/filepath"
	"syscall"
	"time"

	"github.com/lsongdev/ohmyserver/modules/domains/store"
	"github.com/lsongdev/ohmyserver/modules/domains/web"
)

const usage = `usage: domains [flags]

Manages the zones and records of DNS provider accounts. Set
DOMAINS_PASSWORD to require HTTP basic authentication (any user name).

Flags:
`

func main() {
	flag.Usage = func() { fmt.Fprint(flag.CommandLine.Output(), usage); flag.PrintDefaults() }
	listen := flag.String("listen", "127.0.0.1:8080", "address for the UI and API")
	dbPath := flag.String("db", defaultDatabasePath(), "SQLite database path")
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	if err := run(*listen, *dbPath, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(listen, dbPath string, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return err
	}
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	module := web.NewWithStore(st, web.Options{Logger: logger})
	defer module.Close()

	password := os.Getenv("DOMAINS_PASSWORD")
	if password == "" && !isLoopback(listen) {
		logger.Warn("serving without authentication on a non-loopback address; set DOMAINS_PASSWORD")
	}
	srv := &http.Server{Addr: listen, Handler: module.Standalone(password), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	logger.Info("domains listening", "url", "http://"+listen, "db", dbPath)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

func isLoopback(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return host == "localhost" || ip != nil && ip.IsLoopback()
}

func defaultDatabasePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "domains.db"
	}
	return filepath.Join(dir, "domains", "domains.db")
}
