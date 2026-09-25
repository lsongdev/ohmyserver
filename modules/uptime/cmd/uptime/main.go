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

	uptime "github.com/lsongdev/ohmyserver/modules/uptime"
	uptimeweb "github.com/lsongdev/ohmyserver/modules/uptime/web"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "web" || os.Args[2] != "serve" {
		log.Fatal("usage: uptime web serve [--listen address] [--db path]")
	}
	flags := flag.NewFlagSet("uptime web serve", flag.ExitOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	database := flags.String("db", "uptime.db", "SQLite database path")
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
	repo := uptimeweb.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		log.Fatal(err)
	}
	module := uptimeweb.New(repo, uptime.NewChecker(repo))
	server := &http.Server{Addr: *listen, Handler: module.StandaloneHandler()}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); _ = server.Shutdown(context.Background()) }()
	log.Printf("uptime listening on http://%s", *listen)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
