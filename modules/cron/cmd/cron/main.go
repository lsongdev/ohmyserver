package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	cronweb "github.com/lsongdev/ohmyserver/modules/cron/web"
	_ "modernc.org/sqlite"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) < 2 || args[0] != "web" || args[1] != "serve" {
		return fmt.Errorf("usage: cron web serve [--listen address] [--db path]")
	}
	flags := flag.NewFlagSet("cron web serve", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	defaultDB, err := defaultDatabasePath()
	if err != nil {
		return err
	}
	dbPath := flags.String("db", defaultDB, "SQLite database path")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o700); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		return fmt.Errorf("open cron database: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return fmt.Errorf("connect to cron database: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	module := cronweb.NewLocalModule(ctx, db, nil)
	if err := module.Start(ctx); err != nil {
		return fmt.Errorf("start cron engine: %w", err)
	}
	defer module.Close()
	server := &http.Server{Addr: *listen, Handler: module.Handler(), ReadHeaderTimeout: 5 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	log.Printf("Cron module listening on http://%s (database %s)", *listen, *dbPath)
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func defaultDatabasePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(configDir, "cron", "cron.db"), nil
}
