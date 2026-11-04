package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Runner runs commands. Exec runs them on this host; tests substitute
// recorded output.
type Runner interface {
	// Run runs a command with stdin as its standard input and returns its
	// standard output and error. A non-zero exit is an *ExitError,
	// alongside whatever was printed.
	Run(ctx context.Context, stdin string, name string, args ...string) (stdout, stderr []byte, err error)
}

// ExitError is a command that ran and failed.
type ExitError struct {
	Command string
	Code    int
	Output  string // standard error, or else standard output
}

func (e *ExitError) Error() string {
	if e.Output != "" {
		return fmt.Sprintf("%s: %s", e.Command, e.Output)
	}
	return fmt.Sprintf("%s: exit status %d", e.Command, e.Code)
}

// Exec runs commands on this host with LC_ALL=C.
type Exec struct{}

func (Exec) Run(ctx context.Context, stdin string, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), stderr.Bytes(), nil
	}
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return nil, nil, err // not started, e.g. not installed
	}
	output := strings.TrimSpace(stderr.String())
	if output == "" {
		output = strings.TrimSpace(stdout.String())
	}
	return stdout.Bytes(), stderr.Bytes(), &ExitError{Command: name, Code: exit.ExitCode(), Output: output}
}

// Stream runs a command on this host, writing its output to w as it comes.
// Cancelling ctx kills it.
func Stream(ctx context.Context, w io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	cmd.Stdout, cmd.Stderr = w, w
	return cmd.Run()
}
