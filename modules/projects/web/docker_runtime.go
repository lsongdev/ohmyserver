package web

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// DockerRuntime implements project deployment operations through the Docker CLI.
// Keeping this adapter in web lets the standalone command remain a thin entry point.
type DockerRuntime struct{ Binary string }

func (d DockerRuntime) InspectContainer(ctx context.Context, name string) (ContainerState, error) {
	output, err := exec.CommandContext(ctx, d.Binary, "inspect", name).Output()
	if err != nil {
		return ContainerState{}, err
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
		return ContainerState{}, errors.New("docker inspect returned no container state")
	}
	state := items[0].State
	return ContainerState{ID: items[0].ID, Running: state.Running, Dead: state.Dead, OOMKilled: state.OOMKilled, ExitCode: state.ExitCode}, nil
}
func (d DockerRuntime) BuildImageFromDirContext(ctx context.Context, name, dir string, onLog func(string)) error {
	return d.stream(exec.CommandContext(ctx, d.Binary, "build", "--tag", name, dir), onLog)
}
func (d DockerRuntime) RemoveContainer(ctx context.Context, name string) error {
	return d.run(ctx, "rm", "--force", name)
}
func (d DockerRuntime) CreateContainer(ctx context.Context, image, name string, command []string) (string, error) {
	args := []string{"create", "--name", name, "--publish-all", image}
	args = append(args, command...)
	output, err := exec.CommandContext(ctx, d.Binary, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker create: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}
func (d DockerRuntime) StartContainer(ctx context.Context, name string) error {
	return d.run(ctx, "start", name)
}
func (d DockerRuntime) run(ctx context.Context, args ...string) error {
	output, err := exec.CommandContext(ctx, d.Binary, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %w: %s", args[0], err, strings.TrimSpace(string(output)))
	}
	return nil
}
func (d DockerRuntime) stream(cmd *exec.Cmd, onLog func(string)) error {
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
