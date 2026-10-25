// Package runner defines where job commands run. cron-go ships Local,
// which runs commands on this machine; a host program adds others (for
// example SSH hosts) by implementing Runner.
package runner

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// Target is a place a job can run. IDs are "<kind>" or "<kind>:<name>",
// e.g. "local" or "ssh:3", and are stored with the job.
type Target struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Kind returns the kind part of a target ID.
func Kind(target string) string {
	kind, _, _ := strings.Cut(target, ":")
	return kind
}

// Command is what to run.
type Command struct {
	Script string // run by a POSIX shell
	Dir    string // working directory; empty for the default
}

// Runner runs commands on the targets of one kind.
type Runner interface {
	Kind() string
	// Targets lists the targets this runner offers now.
	Targets(ctx context.Context) ([]Target, error)
	// Run runs cmd on target, writing its combined output to out, until it
	// exits or ctx is done. It returns the exit code; err reports a
	// failure to run at all (or a cancellation).
	Run(ctx context.Context, target string, cmd Command, out io.Writer) (exitCode int, err error)
}

// Local runs commands on this machine, as the user running the program.
type Local struct{}

func (Local) Kind() string { return "local" }

func (Local) Targets(context.Context) ([]Target, error) {
	name, _ := os.Hostname()
	if name == "" {
		name = "本机"
	}
	return []Target{{ID: "local", Name: name, Kind: "local"}}, nil
}

func (Local) Run(ctx context.Context, _ string, cmd Command, out io.Writer) (int, error) {
	shell, flag := "/bin/sh", "-c"
	if runtime.GOOS == "windows" {
		shell, flag = "cmd", "/C"
	}
	c := exec.CommandContext(ctx, shell, flag, cmd.Script)
	c.Dir, c.Stdout, c.Stderr = cmd.Dir, out, out
	setProcessGroup(c)
	// Cancellation kills the whole process group, so children started by
	// the script do not outlive it.
	c.Cancel = func() error { return killGroup(c) }
	c.WaitDelay = 5 * time.Second
	err := c.Run()
	var exit *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return -1, ctx.Err()
	case errors.As(err, &exit):
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal()), nil
		}
		return exit.ExitCode(), nil
	case err != nil:
		return -1, err
	}
	return 0, nil
}
