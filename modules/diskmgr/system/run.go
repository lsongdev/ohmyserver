package system

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Runner runs commands. Exec runs them on this host; tests substitute
// recorded output.
type Runner interface {
	// Output runs a command and returns its standard output. A non-zero
	// exit is an *ExitError, alongside whatever was printed.
	Output(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
	// Stream runs a command, sending each line of its interleaved standard
	// output and error to out.
	Stream(ctx context.Context, env []string, out func(string), name string, args ...string) error
}

// ExitError is a command that ran and failed.
type ExitError struct {
	Command string
	Code    int
	Output  string // standard error, or the last lines of output
}

func (e *ExitError) Error() string {
	if e.Output != "" {
		return fmt.Sprintf("%s: %s", e.Command, e.Output)
	}
	return fmt.Sprintf("%s: exit status %d", e.Command, e.Code)
}

// Exec runs commands on this host with LC_ALL=C.
type Exec struct{}

func command(ctx context.Context, env []string, name string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(append(os.Environ(), "LC_ALL=C"), env...)
	return cmd
}

func (Exec) Output(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := command(ctx, env, name, args)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	return out, wrapExit(ctx, name, args, err, stderr.String())
}

func (Exec) Stream(ctx context.Context, env []string, out func(string), name string, args ...string) error {
	cmd := command(ctx, env, name, args)
	reader, writer := io.Pipe()
	cmd.Stdout, cmd.Stderr = writer, writer
	var tail []string
	var wg sync.WaitGroup
	wg.Go(func() {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64<<10), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if tail = append(tail, line); len(tail) > 5 {
				tail = tail[1:]
			}
			out(line)
		}
		io.Copy(io.Discard, reader) // never block the command on a full pipe
	})
	err := cmd.Run()
	writer.Close()
	wg.Wait()
	return wrapExit(ctx, name, args, err, strings.Join(tail, "\n"))
}

func wrapExit(ctx context.Context, name string, args []string, err error, output string) error {
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
	sub := name
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub += " " + args[0]
	}
	return &ExitError{Command: sub, Code: exit.ExitCode(), Output: strings.TrimSpace(output)}
}
