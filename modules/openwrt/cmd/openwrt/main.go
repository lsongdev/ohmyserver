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

	openwrtweb "github.com/lsongdev/ohmyserver/modules/openwrt/web"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "web" || os.Args[2] != "serve" {
		log.Fatal("usage: openwrt web serve [--listen address] [--db path]")
	}
	flags := flag.NewFlagSet("openwrt web serve", flag.ExitOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	dbPath := flags.String("db", "openwrt.db", "SQLite database path")
	_ = flags.Parse(os.Args[3:])

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0755); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	repo := openwrtweb.NewRepository(db)
	if err := repo.Initialize(ctx); err != nil {
		log.Fatal(err)
	}
	module := openwrtweb.New(repo, openwrtweb.NewLocalService(repo))
	srv := &http.Server{Addr: *listen, Handler: module.Handler()}
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	log.Printf("OpenWrt module listening on http://%s", *listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
