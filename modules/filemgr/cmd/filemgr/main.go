// Command filemgr serves the file manager on its own: the UI and API browse,
// view, edit, upload, download, move, copy and delete files under a root
// directory, as the user running it. A panel shell can mount the same
// module (web.New) beside others instead.
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

	"github.com/lsongdev/ohmyserver/modules/filemgr/web"
)

const usage = `usage: filemgr [flags]

Serves the file manager. Files are read and changed as the user running
filemgr. Set FILEMGR_PASSWORD to require HTTP basic authentication (any
user name).

Flags:
`

func main() {
	flag.Usage = func() { fmt.Fprint(flag.CommandLine.Output(), usage); flag.PrintDefaults() }
	listen := flag.String("listen", "127.0.0.1:8080", "address for the UI and API")
	root := flag.String("root", "/", "the directory shown as /")
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	if err := run(*listen, *root, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(listen, root string, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	files, err := web.New(web.Options{Logger: logger, Root: root})
	if err != nil {
		return err
	}
	password := os.Getenv("FILEMGR_PASSWORD")
	if password == "" && !isLoopback(listen) {
		logger.Warn("serving without authentication on a non-loopback address; set FILEMGR_PASSWORD")
	}
	srv := &http.Server{Addr: listen, Handler: files.Standalone(password), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	logger.Info("filemgr listening", "url", "http://"+listen, "root", root)
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
