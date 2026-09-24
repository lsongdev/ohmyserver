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

	forwardweb "github.com/lsongdev/ohmyserver/modules/forward/web"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "web" || os.Args[2] != "serve" {
		log.Fatal("usage: forward web serve [--listen address] [--db path]")
	}
	flags := flag.NewFlagSet("forward web serve", flag.ExitOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	database := flags.String("db", "forward.db", "SQLite database path")
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
	repo := forwardweb.NewRepository(db)
	if err = repo.Initialize(context.Background()); err != nil {
		log.Fatal(err)
	}
	service := forwardweb.NewLocalService(repo)
	if err = service.Start(context.Background()); err != nil {
		log.Fatal(err)
	}
	defer service.Close()
	srv := &http.Server{Addr: *listen, Handler: forwardweb.New(repo, service).StandaloneHandler()}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	log.Printf("forward module listening on http://%s", *listen)
	if err = srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
