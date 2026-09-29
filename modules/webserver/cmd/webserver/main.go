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

	webserverweb "github.com/lsongdev/ohmyserver/modules/webserver/web"
	"github.com/lsongdev/ohmyserver/modules/webserver/web/repository"
	_ "modernc.org/sqlite"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) < 2 || args[0] != "web" || args[1] != "serve" {
		return fmt.Errorf("usage: webserver web serve [--listen address] [--db path]")
	}
	flags := flag.NewFlagSet("webserver web serve", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "management HTTP listen address")
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locate user config directory: %w", err)
	}
	dbPath := flags.String("db", filepath.Join(configDir, "webserver", "webserver.db"), "SQLite database path")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o700); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		return fmt.Errorf("open Web Server database: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return fmt.Errorf("connect to Web Server database: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	repo := repository.NewRepository(db)
	if err := repo.Initialize(ctx); err != nil {
		return fmt.Errorf("initialize Web Server module: %w", err)
	}
	runtime := webserverweb.NewRuntime(ctx, repo, nil, nil)
	if err := runtime.Apply(ctx); err != nil {
		return fmt.Errorf("start configured web listeners: %w", err)
	}
	defer runtime.Close()
	service := webserverweb.NewLocalService(repo, runtime, nil, nil)
	server := &http.Server{Addr: *listen, Handler: webserverweb.New(service).Handler(), ReadHeaderTimeout: 5 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	log.Printf("Web Server module listening on http://%s (database %s)", *listen, *dbPath)
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
