// Command smbmgr serves the Samba manager on its own: the UI and API edit
// smb.conf and Samba users, so it usually runs as root. A panel shell
// can mount the same module (web.New) beside others instead.
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

	"github.com/lsongdev/ohmyserver/modules/smbmgr/web"
)

const usage = `usage: smbmgr [flags]

Serves the Samba manager. Changes are made as the user running smbmgr, which
is usually root. Set SMBMGR_PASSWORD to require HTTP basic authentication
(any user name).

Flags:
`

func main() {
	flag.Usage = func() { fmt.Fprint(flag.CommandLine.Output(), usage); flag.PrintDefaults() }
	listen := flag.String("listen", "127.0.0.1:8080", "address for the UI and API")
	config := flag.String("config", "", "smb.conf to manage (default: the one smbd reads)")
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	if err := run(*listen, *config, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(listen, config string, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	samba := web.New(web.Options{Config: config, Logger: logger})

	password := os.Getenv("SMBMGR_PASSWORD")
	if password == "" && !isLoopback(listen) {
		logger.Warn("serving without authentication on a non-loopback address; set SMBMGR_PASSWORD")
	}
	srv := &http.Server{Addr: listen, Handler: samba.Standalone(password), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	logger.Info("smbmgr listening", "url", "http://"+listen)
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
