package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	files "github.com/lsongdev/ohmyserver/modules/files"
	filesweb "github.com/lsongdev/ohmyserver/modules/files/web"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) < 2 || args[0] != "web" || args[1] != "serve" {
		return fmt.Errorf("usage: files web serve [--listen address] [--root path]")
	}
	flags := flag.NewFlagSet("files web serve", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("locate current directory: %w", err)
	}
	rootPath := flags.String("root", root, "directory exposed by the files module")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	service, err := files.NewLocalFilesystemAt(*rootPath)
	if err != nil {
		return fmt.Errorf("open files root: %w", err)
	}
	defer service.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: *listen, Handler: filesweb.New(service).Handler(), ReadHeaderTimeout: 5 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	log.Printf("Files module listening on http://%s (root %s)", *listen, *rootPath)
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
