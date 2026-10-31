package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lsongdev/ohmyserver/modules/smbmgr/system"
)

// run accepts any configuration except one containing "maybe".
type run struct{}

func (run) Run(ctx context.Context, stdin, name string, args ...string) ([]byte, []byte, error) {
	if name == "testparm" {
		data, _ := os.ReadFile(args[len(args)-1])
		if strings.Contains(string(data), "maybe") {
			return nil, []byte("set_variable_helper(maybe): value is not boolean!\n"), &system.ExitError{Command: "testparm", Code: 1}
		}
	}
	return nil, nil, nil
}

func newServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	config := filepath.Join(dir, "smb.conf")
	os.WriteFile(config, []byte("[global]\n   workgroup = WORKGROUP\n\n[data]\n   path = "+dir+"\n   read only = no\n"), 0o644)
	yes := true
	srv := httptest.NewServer(New(Options{Runner: run{}, Config: config, Installed: &yes}).Handler())
	t.Cleanup(srv.Close)
	return srv, config
}

func call(t *testing.T, srv *httptest.Server, method, path string, body any, out any) int {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = strings.NewReader(string(data))
	}
	req, _ := http.NewRequest(method, srv.URL+path, r)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		var envelope struct{ Data json.RawMessage }
		json.NewDecoder(res.Body).Decode(&envelope)
		json.Unmarshal(envelope.Data, out)
	}
	return res.StatusCode
}

func TestShares(t *testing.T) {
	srv, config := newServer(t)
	var list Shares
	if call(t, srv, "GET", "/shares", nil, &list); len(list.Shares) != 1 {
		t.Fatalf("list: %+v", list)
	}
	if sh := list.Shares[0]; sh.Name != "data" || sh.ReadOnly || !sh.Browseable || sh.PathMissing {
		t.Errorf("data: %+v", sh)
	}

	add := map[string]any{"version": list.Version, "name": "media", "params": []map[string]string{{"key": "path", "value": "/nonexistent/media"}, {"key": "guest ok", "value": "yes"}}}
	var saved struct{ Version string }
	if code := call(t, srv, "POST", "/shares", add, &saved); code != 200 {
		t.Fatalf("add: %d", code)
	}
	// The list's version is stale now.
	if code := call(t, srv, "POST", "/shares", map[string]any{"version": list.Version, "name": "other"}, nil); code != 409 {
		t.Errorf("stale add: %d", code)
	}
	var media Section
	call(t, srv, "GET", "/shares/MEDIA", nil, &media)
	if media.Share == nil || !media.Share.Guest || !media.Share.ReadOnly || !media.Share.PathMissing {
		t.Errorf("media: %+v", media.Share)
	}

	bad := map[string]any{"version": media.Version, "params": []map[string]string{{"key": "read only", "value": "maybe"}}}
	if code := call(t, srv, "PUT", "/shares/media", bad, nil); code != 422 {
		t.Errorf("rejected by testparm: %d", code)
	}
	rename := map[string]any{"version": media.Version, "name": "films", "params": media.Params}
	if code := call(t, srv, "PUT", "/shares/media", rename, &saved); code != 200 {
		t.Errorf("rename: %d", code)
	}
	if code := call(t, srv, "DELETE", "/shares/films?version="+saved.Version, nil, nil); code != 200 {
		t.Errorf("delete: %d", code)
	}
	if code := call(t, srv, "GET", "/shares/global", nil, nil); code != 404 {
		t.Errorf("[global] as a share: %d", code)
	}
	data, _ := os.ReadFile(config)
	if strings.Contains(string(data), "films") || strings.Contains(string(data), "media") {
		t.Errorf("left behind:\n%s", data)
	}
}

func TestUnsupported(t *testing.T) {
	no := false
	srv := httptest.NewServer(New(Options{Runner: run{}, Config: "/nonexistent", Installed: &no}).Handler())
	defer srv.Close()
	var info Info
	if code := call(t, srv, "GET", "/info", nil, &info); code != 200 || info.Installed {
		t.Errorf("info: %d %+v", code, info)
	}
	if code := call(t, srv, "GET", "/shares", nil, nil); code != 503 {
		t.Errorf("shares: %d", code)
	}
}

func TestAddUserNeedsSystemAccount(t *testing.T) {
	dir := t.TempDir()
	passwd := filepath.Join(dir, "passwd")
	os.WriteFile(passwd, []byte("lsong:x:1000:1000::/home/lsong:/bin/bash\n"), 0o644)
	yes := true
	srv := httptest.NewServer(New(Options{Runner: run{}, Config: filepath.Join(dir, "smb.conf"), Passwd: passwd, Installed: &yes}).Handler())
	defer srv.Close()
	if code := call(t, srv, "POST", "/users", map[string]string{"name": "root", "password": "x"}, nil); code != 400 {
		t.Errorf("root: %d", code)
	}
	if code := call(t, srv, "POST", "/users", map[string]string{"name": "lsong", "password": "x"}, nil); code != 200 {
		t.Errorf("lsong: %d", code)
	}
}
