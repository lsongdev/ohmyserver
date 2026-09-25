package pkgmgr

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

// Executor runs commands. Implementations can provide privilege separation,
// remote execution, logging, or deterministic tests.
type Executor interface {
	Run(context.Context, Command) (Result, error)
}

// ExecExecutor executes a command directly on the local host.
type ExecExecutor struct{}

func (ExecExecutor) Run(ctx context.Context, command Command) (Result, error) {
	started := time.Now()
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Env = append(os.Environ(), command.Env...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := Result{
		Command:  cloneCommand(command),
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
		Duration: time.Since(started),
	}
	if err == nil {
		return result, nil
	}
	result.ExitCode = -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}
	return result, &CommandError{Result: result, Err: err}
}

// PrefixExecutor wraps commands with another executable. It is useful for
// applying a narrowly configured privilege broker to mutation commands.
type PrefixExecutor struct {
	Name  string
	Args  []string
	Inner Executor
}

func (e PrefixExecutor) Run(ctx context.Context, command Command) (Result, error) {
	if e.Name == "" {
		return Result{}, errors.New("pkgmgr: prefix executable is empty")
	}
	inner := e.Inner
	if inner == nil {
		inner = ExecExecutor{}
	}
	args := append([]string{}, e.Args...)
	args = append(args, command.Name)
	args = append(args, command.Args...)
	return inner.Run(ctx, Command{Name: e.Name, Args: args, Env: command.Env})
}

// Sudo returns a non-interactive sudo executor. It should normally be used only
// as Options.MutatingExecutor, leaving read-only queries unprivileged.
func Sudo(inner Executor) Executor {
	return PrefixExecutor{Name: "sudo", Args: []string{"-n"}, Inner: inner}
}

func cloneCommand(command Command) Command {
	return Command{
		Name: command.Name,
		Args: append([]string(nil), command.Args...),
		Env:  append([]string(nil), command.Env...),
	}
}
