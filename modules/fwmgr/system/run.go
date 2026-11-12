package system

import (
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

// locale keeps command output in English and parseable, and in UTF-8: under
// plain C, nmcli prints every non-ASCII character of a network name as "?",
// which would show the wrong name and make a join compare the wrong one. A
// system without C.UTF-8 falls back to C, as before.
const locale = "LC_ALL=C.UTF-8"

// Exec runs commands on this host in the locale above.
type Exec struct{}

func (Exec) Run(ctx context.Context, stdin string, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), locale)
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
