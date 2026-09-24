package host

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"os/signal"
	"syscall"

	"github.com/lsongdev/ohmyserver/config"
)

// Run loads configuration, assembles the host and its mounted modules, then
// serves the application until the process receives a termination signal.
func Run() error {
	initDefaultWeb := flag.Bool("init-default-web", false, "initialize the database and ensure a default :80 Worker host")
	flag.Parse()

	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}

	server, err := NewServer(cfg)
	if err != nil {
		return err
	}
	if *initDefaultWeb {
		seedErr := server.SeedDefaultWebWorker()
		return errors.Join(seedErr, server.Close())
	}

	router, err := NewHTTPRouterAtBase(server, cfg.BasePath)
	if err != nil {
		return errors.Join(err, server.Close())
	}
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return errors.Join(err, server.Close())
	}

	slog.Info("server starting", "addr", cfg.Listen)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	serveErr := serveHTTP(ctx, listener, loggingMiddleware(router))
	stop()
	return errors.Join(serveErr, server.Close())
}
