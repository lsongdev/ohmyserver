package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	dnsweb "github.com/lsongdev/ohmyserver/modules/dns/web"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "web" || os.Args[2] != "serve" {
		log.Fatal("usage: dns web serve [--listen address] [--db path] [--list-dir path]")
	}
	flags := flag.NewFlagSet("dns web serve", flag.ExitOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	dbPath := flags.String("db", "dns.db", "SQLite database path")
	listDir := flags.String("list-dir", "./dns-lists", "subscription list directory")
	_ = flags.Parse(os.Args[3:])

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*listDir, 0755); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	repo := dnsweb.NewRepository(db)
	if err := repo.InitializeModule(ctx); err != nil {
		log.Fatal(err)
	}
	service := dnsweb.NewLocalService(ctx, repo, *listDir)
	defer func() {
		if err := service.Close(); err != nil {
			log.Printf("close DNS runtime: %v", err)
		}
	}()
	module := dnsweb.New(repo, service, *listDir)
	srv := &http.Server{Addr: *listen, Handler: module.Handler()}
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	log.Printf("DNS module listening on http://%s", *listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
