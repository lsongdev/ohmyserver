package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	dhcpweb "github.com/lsongdev/ohmyserver/modules/dhcp/web"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "web" || os.Args[2] != "serve" {
		log.Fatal("usage: dhcp web serve [--listen address] [--db path]")
	}
	flags := flag.NewFlagSet("dhcp web serve", flag.ExitOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	dbPath := flags.String("db", "dhcp.db", "SQLite database path")
	_ = flags.Parse(os.Args[3:])
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	module := dhcpweb.NewLocalModule(ctx, db, nil)
	if err := module.Initialize(ctx); err != nil {
		log.Fatal(err)
	}
	if err := module.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer module.Close()
	srv := &http.Server{Addr: *listen, Handler: module.StandaloneHandler()}
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	log.Printf("DHCP module listening on http://%s", *listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
