package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	system "github.com/lsongdev/ohmyserver/modules/system"
	systemweb "github.com/lsongdev/ohmyserver/modules/system/web"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "web" || os.Args[2] != "serve" {
		log.Fatal("usage: system web serve [--listen address]")
	}
	flags := flag.NewFlagSet("system web serve", flag.ExitOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	_ = flags.Parse(os.Args[3:])
	module := systemweb.New(system.NewService(system.ServiceOptions{Info: system.Info{
		ProgramName: "OhMyServer", ProgramVersion: "standalone", ProgramDesc: "System information module",
		Listen:  *listen,
		Credits: []system.Credit{{Name: "Go", URL: "https://golang.org", License: "BSD"}},
	}}))
	srv := &http.Server{Addr: *listen, Handler: module.Handler()}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	log.Printf("system module listening on http://%s", *listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
