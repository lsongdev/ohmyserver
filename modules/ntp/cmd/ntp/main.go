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

	ntpweb "github.com/lsongdev/ohmyserver/modules/ntp/web"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "web" || os.Args[2] != "serve" {
		log.Fatal("usage: ntp web serve [--listen address] [--db path]")
	}
	flags := flag.NewFlagSet("ntp web serve", flag.ExitOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	database := flags.String("db", "ntp.db", "SQLite database path")
	_ = flags.Parse(os.Args[3:])
	if dir := filepath.Dir(*database); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatal(err)
		}
	}
	db, err := sql.Open("sqlite", *database)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	module := ntpweb.NewLocalModule(db, nil)
	if err = module.Initialize(ctx); err != nil {
		log.Fatal(err)
	}
	defer module.Close()
	srv := &http.Server{Addr: *listen, Handler: module.StandaloneHandler()}
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	log.Printf("NTP module UI listening on http://%s", *listen)
	if err = srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
