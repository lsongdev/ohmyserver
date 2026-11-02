package system

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner runs commands. Exec runs them on this host; tests substitute
// recorded output.
type Runner interface {
	// Run runs a command and returns its standard output. A non-zero exit
	// is an *ExitError.
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	// Lines calls line for each line of a command's standard output, and
	// stops the command when line returns false, which is not an error.
	Lines(ctx context.Context, line func(string) bool, name string, args ...string) error
}

// ExitError is a command that ran and failed.
type ExitError struct {
	Command string
	Code    int
	Output  string // standard error, or else standard output
}

func (e *ExitError) Error() string {
	if e.Output != "" {
		return e.Output
	}
	return fmt.Sprintf("%s: exit status %d", e.Command, e.Code)
}

// Exec runs commands on this host with LC_ALL=C.
type Exec struct{}

func (Exec) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), exitError(ctx, name, err, stderr.String(), stdout.String())
}

func (Exec) Lines(ctx context.Context, line func(string) bool, name string, args ...string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	reader := bufio.NewReader(stdout)
	for {
		text, readErr := reader.ReadString('\n')
		if text != "" && !line(strings.TrimRight(text, "\n")) {
			cancel()
			cmd.Wait()
			return nil
		}
		if readErr != nil {
			break // EOF, or a broken pipe that Wait reports
		}
	}
	return exitError(ctx, name, cmd.Wait(), stderr.String(), "")
}

func exitError(ctx context.Context, name string, err error, stderr, stdout string) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return err // not started, e.g. not installed
	}
	output := strings.TrimSpace(stderr)
	if output == "" {
		output = strings.TrimSpace(stdout)
	}
	return &ExitError{Command: name, Code: exit.ExitCode(), Output: output}
}
