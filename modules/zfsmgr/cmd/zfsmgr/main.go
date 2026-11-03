// Command zfsmgr serves the ZFS snapshot manager on its own: it runs zfs, so
// it usually runs as root, and applies retention policies for as long as it
// runs. A panel shell can mount the same module (web.New) beside others
// instead.
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

	"github.com/lsongdev/ohmyserver/modules/zfsmgr/web"
)

const usage = `usage: zfsmgr [flags]

Serves the ZFS snapshot manager. Changes are made as the user running zfsmgr,
which is usually root. Set ZFSMGR_PASSWORD to require HTTP basic authentication
(any user name).

Flags:
`

func main() {
	flag.Usage = func() { fmt.Fprint(flag.CommandLine.Output(), usage); flag.PrintDefaults() }
	listen := flag.String("listen", "127.0.0.1:8080", "address for the UI and API")
	loops := flag.Bool("test-loop-disks", false, "for trying things out: new pools may only use loop devices, and no schedules run")
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	if err := run(*listen, *loops, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(listen string, loops bool, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	module := web.New(web.Options{Logger: logger, TestLoopDisks: loops})
	defer module.Close()

	password := os.Getenv("ZFSMGR_PASSWORD")
	if password == "" && !isLoopback(listen) {
		logger.Warn("serving without authentication on a non-loopback address; set ZFSMGR_PASSWORD")
	}
	srv := &http.Server{Addr: listen, Handler: module.Standalone(password), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	logger.Info("zfsmgr listening", "url", "http://"+listen)
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
