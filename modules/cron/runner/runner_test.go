//go:build unix

package runner

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestLocal(t *testing.T) {
	var out bytes.Buffer
	code, err := Local{}.Run(context.Background(), "local", Command{Script: "echo out; echo err >&2; pwd; exit 3", Dir: "/"}, &out)
	if err != nil || code != 3 || out.String() != "out\nerr\n/\n" {
		t.Fatalf("code %d err %v out %q", code, err, out.String())
	}
	// Cancelling kills the script and the processes it started.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	code, err = Local{}.Run(ctx, "local", Command{Script: "sleep 30 & sleep 30; wait"}, &out)
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("cancel: code %d err %v after %v", code, err, time.Since(start))
	}
	if Kind("ssh:3") != "ssh" || Kind("local") != "local" {
		t.Fatal("Kind")
	}
}
