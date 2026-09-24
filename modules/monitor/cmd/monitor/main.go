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
	"time"

	monitor "github.com/lsongdev/ohmyserver/modules/monitor"
	monitorweb "github.com/lsongdev/ohmyserver/modules/monitor/web"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "web" || os.Args[2] != "serve" {
		log.Fatal("usage: monitor web serve [--listen address] [--db path]")
	}
	flags := flag.NewFlagSet("monitor web serve", flag.ExitOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	database := flags.String("db", "monitor.db", "SQLite database path")
	_ = flags.Parse(os.Args[3:])
	if dir := filepath.Dir(*database); dir != "." {
		if e := os.MkdirAll(dir, 0755); e != nil {
			log.Fatal(e)
		}
	}
	db, e := sql.Open("sqlite", *database)
	if e != nil {
		log.Fatal(e)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	repo := monitorweb.NewRepository(db)
	if e = repo.Initialize(context.Background()); e != nil {
		log.Fatal(e)
	}
	srv := &http.Server{Addr: *listen, Handler: monitorweb.New(repo, monitor.LocalCollector{}).StandaloneHandler()}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	collector := monitorweb.NewCollectionService(ctx, repo, monitor.LocalCollector{}, nil, 3*time.Second)
	if e := collector.Start(); e != nil {
		log.Fatal(e)
	}
	defer func() {
		stop()
		_ = collector.Close()
	}()
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	log.Printf("Monitor module listening on http://%s", *listen)
	if e = srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}
