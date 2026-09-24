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

	ipmiweb "github.com/lsongdev/ohmyserver/modules/ipmi/web"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "web" || os.Args[2] != "serve" {
		log.Fatal("usage: ipmi web serve [--listen address] [--db path]")
	}
	flags := flag.NewFlagSet("ipmi web serve", flag.ExitOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	database := flags.String("db", "ipmi.db", "SQLite database path")
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
	module := ipmiweb.NewLocalModule(db)
	if e = module.Initialize(context.Background()); e != nil {
		log.Fatal(e)
	}
	srv := &http.Server{Addr: *listen, Handler: module.StandaloneHandler()}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	log.Printf("IPMI module listening on http://%s", *listen)
	if e = srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}
