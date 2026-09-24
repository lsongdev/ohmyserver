package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	projectsweb "github.com/lsongdev/ohmyserver/modules/projects/web"
	_ "modernc.org/sqlite"
)

type dockerRuntime struct{ binary string }

func (d dockerRuntime) InspectContainer(ctx context.Context, name string) (projectsweb.ContainerState, error) {
	output, err := exec.CommandContext(ctx, d.binary, "inspect", name).Output()
	if err != nil {
		return projectsweb.ContainerState{}, err
	}
	var items []struct {
		ID    string `json:"Id"`
		State struct {
			Running   bool `json:"Running"`
			Dead      bool `json:"Dead"`
			OOMKilled bool `json:"OOMKilled"`
			ExitCode  int  `json:"ExitCode"`
		} `json:"State"`
	}
	if err := json.Unmarshal(output, &items); err != nil || len(items) == 0 {
		return projectsweb.ContainerState{}, errors.New("docker inspect returned no container state")
	}
	state := items[0].State
	return projectsweb.ContainerState{ID: items[0].ID, Running: state.Running, Dead: state.Dead, OOMKilled: state.OOMKilled, ExitCode: state.ExitCode}, nil
}
func (d dockerRuntime) BuildImageFromDirContext(ctx context.Context, name, dir string, onLog func(string)) error {
	cmd := exec.CommandContext(ctx, d.binary, "build", "--tag", name, dir)
	return d.stream(cmd, onLog)
}
func (d dockerRuntime) RemoveContainer(ctx context.Context, name string) error {
	return d.run(ctx, "rm", "--force", name)
}
func (d dockerRuntime) CreateContainer(ctx context.Context, image, name string, command []string) (string, error) {
	args := []string{"create", "--name", name, "--publish-all", image}
	args = append(args, command...)
	output, err := exec.CommandContext(ctx, d.binary, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker create: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}
func (d dockerRuntime) StartContainer(ctx context.Context, name string) error {
	return d.run(ctx, "start", name)
}
func (d dockerRuntime) run(ctx context.Context, args ...string) error {
	output, err := exec.CommandContext(ctx, d.binary, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %w: %s", args[0], err, strings.TrimSpace(string(output)))
	}
	return nil
}
func (d dockerRuntime) stream(cmd *exec.Cmd, onLog func(string)) error {
	reader, writer := io.Pipe()
	cmd.Stdout, cmd.Stderr = writer, writer
	if err := cmd.Start(); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return err
	}
	linesDone := make(chan struct{})
	go func() {
		defer close(linesDone)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			onLog(scanner.Text())
		}
	}()
	err := cmd.Wait()
	_ = writer.Close()
	<-linesDone
	_ = reader.Close()
	return err
}

func main() {
	if len(os.Args) < 3 || os.Args[1] != "web" || os.Args[2] != "serve" {
		log.Fatal("usage: projects web serve [--listen address] [--db path] [--workspace path] [--docker binary]")
	}
	flags := flag.NewFlagSet("projects web serve", flag.ExitOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	dbPath := flags.String("db", "projects.db", "SQLite database path")
	workspace := flags.String("workspace", "./workspace", "project checkout workspace")
	dockerBinary := flags.String("docker", "docker", "Docker CLI binary")
	_ = flags.Parse(os.Args[3:])
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*workspace, 0755); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	repo := projectsweb.NewRepository(db)
	if err := repo.Initialize(ctx); err != nil {
		log.Fatal(err)
	}
	if err := repo.RecoverInterruptedDeployments(ctx); err != nil {
		log.Fatal(err)
	}
	manager := projectsweb.NewDeploymentManager(ctx, repo, dockerRuntime{binary: *dockerBinary}, *workspace)
	defer manager.Close()
	module := projectsweb.New(repo, manager)
	srv := &http.Server{Addr: *listen, Handler: module.StandaloneHandler()}
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	log.Printf("Projects module listening on http://%s", *listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
