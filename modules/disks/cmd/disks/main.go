package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	disks "github.com/lsongdev/ohmyserver/modules/disks"
	disksweb "github.com/lsongdev/ohmyserver/modules/disks/web"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "web" || os.Args[2] != "serve" {
		log.Fatal("usage: disks web serve [--listen address]")
	}
	flags := flag.NewFlagSet("disks web serve", flag.ExitOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	_ = flags.Parse(os.Args[3:])
	srv := &http.Server{Addr: *listen, Handler: disksweb.New(disks.NewLocalService()).Handler()}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	log.Printf("disks listening on http://%s", *listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
