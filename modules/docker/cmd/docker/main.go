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

	docker "github.com/lsongdev/ohmyserver/modules/docker"
	dockerweb "github.com/lsongdev/ohmyserver/modules/docker/web"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "web" || os.Args[2] != "serve" {
		log.Fatal("usage: docker web serve [--listen address] [--db path] [--apps-dir path] [--compose-dir path]")
	}
	flags := flag.NewFlagSet("docker web serve", flag.ExitOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	dbPath := flags.String("db", "docker.db", "SQLite database path")
	appsDir := flags.String("apps-dir", "./apps-catalog", "App catalog checkout directory")
	composeDir := flags.String("compose-dir", "./compose", "Compose project directory")
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
	repo := dockerweb.NewRepository(db)
	if err := repo.InitializeAppCatalog(ctx); err != nil {
		log.Fatal(err)
	}
	client, err := docker.NewDockerClient("")
	if err != nil {
		log.Fatal(err)
	}
	service := docker.NewLocalService(ctx, client, repo, *appsDir, *composeDir)
	srv := &http.Server{Addr: *listen, Handler: dockerweb.New(service).Handler()}
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	log.Printf("Docker module listening on http://%s", *listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	if err := service.Close(); err != nil {
		log.Printf("close Docker client: %v", err)
	}
}
