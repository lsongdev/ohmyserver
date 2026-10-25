// Command ci-go serves the CI on its own: builds run in Docker containers
// on the daemon the docker CLI would use, and the UI and API manage them.
// A panel shell can mount the same module (web.New) beside others instead.
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

	"github.com/lsongdev/ci-go/api"
	"github.com/lsongdev/ci-go/store"
	"github.com/lsongdev/ci-go/web"
)

const usage = `usage: ci-go [flags]

Serves the CI. Builds run repositories' code in containers, so add only
repositories you trust. Set CI_GO_PASSWORD to require HTTP basic
authentication (any user name).

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
	ci, err := web.NewWithStore(ctx, st, web.Options{Options: api.Options{Logger: logger}})
	if err != nil {
		st.Close()
		return err
	}
	defer ci.Close()

	password := os.Getenv("CI_GO_PASSWORD")
	if password == "" && !isLoopback(listen) {
		logger.Warn("serving without authentication on a non-loopback address; set CI_GO_PASSWORD")
	}
	srv := &http.Server{Addr: listen, Handler: ci.Standalone(password), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	logger.Info("ci-go listening", "url", "http://"+listen, "db", dbPath)
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
		return "ci.db"
	}
	return filepath.Join(dir, "ci-go", "ci.db")
}
