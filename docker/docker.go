// Package docker is the small part of the Docker Engine API that ci-go
// needs: pulling images, running containers and following their output,
// and removing what a build created. It finds the daemon the way the
// docker CLI does.
package docker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// apiVersion is the Engine API spoken: Docker 25 and later.
const apiVersion = "v1.44"

// Client talks to one daemon.
type Client struct {
	host string
	http *http.Client
}

// FromEnv connects to the daemon the docker CLI would use: DOCKER_HOST,
// then the current docker context, then /var/run/docker.sock.
func FromEnv() (*Client, error) {
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		host = contextHost()
	}
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}
	return New(host)
}

// New connects to a daemon at a unix:// or tcp:// address.
func New(host string) (*Client, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("docker: invalid host %q: %w", host, err)
	}
	network, addr := "unix", u.Path
	switch u.Scheme {
	case "unix":
	case "tcp":
		network, addr = "tcp", u.Host
	default:
		return nil, fmt.Errorf("docker: unsupported host %q", host)
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &Client{host: host, http: &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
		MaxIdleConnsPerHost: 8,
	}}}, nil
}

// Host returns the daemon address.
func (c *Client) Host() string { return c.host }

// contextHost returns the docker endpoint of the current docker context,
// or "" for the default one.
func contextHost() string {
	dir := os.Getenv("DOCKER_CONFIG")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".docker")
	}
	name := os.Getenv("DOCKER_CONTEXT")
	if name == "" {
		var config struct {
			CurrentContext string `json:"currentContext"`
		}
		data, _ := os.ReadFile(filepath.Join(dir, "config.json"))
		json.Unmarshal(data, &config)
		name = config.CurrentContext
	}
	if name == "" || name == "default" {
		return ""
	}
	sum := sha256.Sum256([]byte(name))
	data, err := os.ReadFile(filepath.Join(dir, "contexts", "meta", hex.EncodeToString(sum[:]), "meta.json"))
	if err != nil {
		return ""
	}
	var meta struct {
		Endpoints map[string]struct{ Host string }
	}
	json.Unmarshal(data, &meta)
	return meta.Endpoints["docker"].Host
}

// Error is an error response from the daemon.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("docker: %s (HTTP %d)", e.Message, e.Status) }

// NotFound reports whether err is a 404 from the daemon.
func NotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
	}
	u := "http://docker/" + apiVersion + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		var e struct{ Message string }
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if json.Unmarshal(data, &e) != nil || e.Message == "" {
			e.Message = strings.TrimSpace(string(data))
		}
		return nil, &Error{Status: resp.StatusCode, Message: e.Message}
	}
	return resp, nil
}

func (c *Client) call(ctx context.Context, method, path string, query url.Values, body, out any) error {
	resp, err := c.do(ctx, method, path, query, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Ping checks that the daemon answers.
func (c *Client) Ping(ctx context.Context) error {
	return c.call(ctx, "GET", "/_ping", nil, nil, nil)
}

// ImageExists reports whether an image is present locally.
func (c *Client) ImageExists(ctx context.Context, image string) (bool, error) {
	err := c.call(ctx, "GET", "/images/"+image+"/json", nil, nil, nil)
	if NotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// Pull pulls an image, writing one line per progress message to out.
func (c *Client) Pull(ctx context.Context, image string, out io.Writer) error {
	name, tag := image, "latest"
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") {
		name, tag = image[:i], image[i+1:]
	}
	if i := strings.Index(name, "@"); i >= 0 { // a digest
		name, tag = image, ""
	}
	q := url.Values{"fromImage": {name}}
	if tag != "" {
		q.Set("tag", tag)
	}
	resp, err := c.do(ctx, "POST", "/images/create", q, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	for {
		var msg struct {
			Status, Progress, ID string
			Error                string
		}
		if err := dec.Decode(&msg); err == io.EOF {
			return nil
		} else if err != nil {
			return fmt.Errorf("docker: pull %s: %w", image, err)
		}
		if msg.Error != "" {
			return fmt.Errorf("docker: pull %s: %s", image, msg.Error)
		}
		// Per-layer progress is noise in a build log: keep the outcomes.
		switch msg.Status {
		case "", "Downloading", "Extracting", "Waiting", "Verifying Checksum", "Pulling fs layer":
			continue
		}
		if msg.Progress == "" {
			if msg.ID != "" {
				fmt.Fprintf(out, "%s: %s\n", msg.ID, msg.Status)
			} else {
				fmt.Fprintln(out, msg.Status)
			}
		}
	}
}

// Mount attaches a named volume (created on first use) or a host path.
type Mount struct {
	Type   string `json:"Type"`   // volume | bind
	Source string `json:"Source"` // volume name or host path
	Target string `json:"Target"`
}

// Container describes a container to create.
type Container struct {
	Name       string
	Image      string
	Entrypoint []string
	Cmd        []string
	Env        []string
	WorkingDir string
	Mounts     []Mount
	Labels     map[string]string
	Memory     int64 // bytes; 0 is unlimited
	NanoCPUs   int64 // 1e9 per CPU; 0 is unlimited
}

// Create creates a container with a TTY, so its output is one plain stream.
func (c *Client) Create(ctx context.Context, spec Container) (string, error) {
	body := map[string]any{
		"Image": spec.Image, "Entrypoint": spec.Entrypoint, "Cmd": spec.Cmd, "Env": spec.Env,
		"WorkingDir": spec.WorkingDir, "Labels": spec.Labels, "Tty": true,
		"HostConfig": map[string]any{"Mounts": spec.Mounts, "Memory": spec.Memory, "NanoCpus": spec.NanoCPUs},
	}
	var q url.Values
	if spec.Name != "" {
		q = url.Values{"name": {spec.Name}}
	}
	var out struct{ ID string }
	if err := c.call(ctx, "POST", "/containers/create", q, body, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// Start starts a created container.
func (c *Client) Start(ctx context.Context, id string) error {
	return c.call(ctx, "POST", "/containers/"+id+"/start", nil, nil, nil)
}

// Logs follows a container's output from its start until it exits.
func (c *Client) Logs(ctx context.Context, id string) (io.ReadCloser, error) {
	resp, err := c.do(ctx, "GET", "/containers/"+id+"/logs", url.Values{"follow": {"1"}, "stdout": {"1"}, "stderr": {"1"}}, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// Wait waits for a container to exit and returns its exit code.
func (c *Client) Wait(ctx context.Context, id string) (int, error) {
	var out struct {
		StatusCode int
		Error      *struct{ Message string }
	}
	if err := c.call(ctx, "POST", "/containers/"+id+"/wait", nil, nil, &out); err != nil {
		return -1, err
	}
	if out.Error != nil && out.Error.Message != "" {
		return -1, fmt.Errorf("docker: %s", out.Error.Message)
	}
	return out.StatusCode, nil
}

// Remove force-removes a container; removing a missing one succeeds.
func (c *Client) Remove(ctx context.Context, id string) error {
	err := c.call(ctx, "DELETE", "/containers/"+id, url.Values{"force": {"1"}}, nil, nil)
	if NotFound(err) {
		return nil
	}
	return err
}

// RemoveVolume removes a volume; removing a missing one succeeds.
func (c *Client) RemoveVolume(ctx context.Context, name string) error {
	err := c.call(ctx, "DELETE", "/volumes/"+name, url.Values{"force": {"1"}}, nil, nil)
	if NotFound(err) {
		return nil
	}
	return err
}

// Run creates and starts a container, copies its output to out until it
// exits, removes it, and returns its exit code. Cancelling ctx kills the
// container.
func (c *Client) Run(ctx context.Context, spec Container, out io.Writer) (int, error) {
	id, err := c.Create(ctx, spec)
	if err != nil {
		return -1, err
	}
	defer c.Remove(context.WithoutCancel(ctx), id)
	if err := c.Start(ctx, id); err != nil {
		return -1, err
	}
	logs, err := c.Logs(ctx, id)
	if err != nil {
		return -1, err
	}
	copied := make(chan struct{})
	go func() {
		io.Copy(out, logs)
		close(copied)
	}()
	code, err := c.Wait(ctx, id)
	if ctx.Err() != nil {
		logs.Close()
		return -1, ctx.Err()
	}
	<-copied // the log stream ends when the container does
	logs.Close()
	return code, err
}

// CreateVolume creates a named volume with labels.
func (c *Client) CreateVolume(ctx context.Context, name string, labels map[string]string) error {
	return c.call(ctx, "POST", "/volumes/create", nil, map[string]any{"Name": name, "Labels": labels}, nil)
}

func labelFilter(label string) url.Values {
	f, _ := json.Marshal(map[string][]string{"label": {label}})
	return url.Values{"filters": {string(f)}}
}

// RemoveLabeled removes every container and volume carrying label (a key,
// or key=value), e.g. what a crashed process left behind.
func (c *Client) RemoveLabeled(ctx context.Context, label string) error {
	var containers []struct{ ID string }
	q := labelFilter(label)
	q.Set("all", "1")
	if err := c.call(ctx, "GET", "/containers/json", q, nil, &containers); err != nil {
		return err
	}
	var errs []error
	for _, ct := range containers {
		errs = append(errs, c.Remove(ctx, ct.ID))
	}
	var volumes struct{ Volumes []struct{ Name string } }
	if err := c.call(ctx, "GET", "/volumes", labelFilter(label), nil, &volumes); err != nil {
		return errors.Join(append(errs, err)...)
	}
	for _, v := range volumes.Volumes {
		errs = append(errs, c.RemoveVolume(ctx, v.Name))
	}
	return errors.Join(errs...)
}
