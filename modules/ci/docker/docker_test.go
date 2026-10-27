package docker

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// client returns a client for the local daemon, or skips the test.
func client(t *testing.T) *Client {
	t.Helper()
	c, err := FromEnv()
	if err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		t.Skip("no Docker daemon:", err)
	}
	return c
}

func TestRun(t *testing.T) {
	c := client(t)
	ctx := context.Background()
	if ok, _ := c.ImageExists(ctx, "alpine:3"); !ok {
		var log bytes.Buffer
		if err := c.Pull(ctx, "alpine:3", &log); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := c.ImageExists(ctx, "alpine:3"); !ok || err != nil {
		t.Fatalf("image missing after pull: %v", err)
	}
	if ok, _ := c.ImageExists(ctx, "ci-go-test/no-such-image:1"); ok {
		t.Fatal("missing image reported present")
	}

	volume := "ci-go-test-" + strings.ReplaceAll(t.Name(), "/", "-")
	defer c.RemoveVolume(ctx, volume)
	mount := []Mount{{Type: "volume", Source: volume, Target: "/work"}}
	var out bytes.Buffer
	code, err := c.Run(ctx, Container{Image: "alpine:3", Cmd: []string{"sh", "-c", "echo hi > /work/f; echo out; echo err >&2; exit 3"}, Mounts: mount}, &out)
	if err != nil || code != 3 || !strings.Contains(out.String(), "out") || !strings.Contains(out.String(), "err") {
		t.Fatalf("code %d err %v out %q", code, err, out.String())
	}
	out.Reset()
	// The volume keeps files between containers.
	if code, err := c.Run(ctx, Container{Image: "alpine:3", Cmd: []string{"cat", "/work/f"}, Mounts: mount, Env: []string{"A=1"}}, &out); err != nil || code != 0 || strings.TrimSpace(out.String()) != "hi" {
		t.Fatalf("second container: code %d err %v out %q", code, err, out.String())
	}

	// Cancelling kills the container.
	cctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.Run(cctx, Container{Image: "alpine:3", Cmd: []string{"sleep", "60"}}, &out); err == nil || time.Since(start) > 15*time.Second {
		t.Fatalf("cancel: %v after %v", err, time.Since(start))
	}
	if _, err := c.Run(ctx, Container{Image: "ci-go-test/no-such-image:1"}, &out); !NotFound(err) {
		t.Fatalf("missing image: %v", err)
	}
}

func TestRemoveLabeled(t *testing.T) {
	c := client(t)
	ctx := context.Background()
	label := map[string]string{"ci-go.test": t.Name()}
	if err := c.CreateVolume(ctx, "ci-go-test-labeled", label); err != nil {
		t.Fatal(err)
	}
	id, err := c.Create(ctx, Container{Image: "alpine:3", Cmd: []string{"true"}, Labels: label})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveLabeled(ctx, "ci-go.test="+t.Name()); err != nil {
		t.Fatal(err)
	}
	if err := c.call(ctx, "GET", "/containers/"+id+"/json", nil, nil, nil); !NotFound(err) {
		t.Fatalf("container left: %v", err)
	}
	if err := c.call(ctx, "GET", "/volumes/ci-go-test-labeled", nil, nil, nil); !NotFound(err) {
		t.Fatalf("volume left: %v", err)
	}
}
