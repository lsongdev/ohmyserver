package shell

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

// testModule satisfies Module the way contract modules do: structurally.
type testModule struct {
	id     string
	assets fstest.MapFS
}

func (m testModule) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, m.id+" says hi") })
	mux.HandleFunc("POST /hooks/{name}", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "hook "+r.PathValue("name")) })
	return mux
}

func (m testModule) Assets() fs.FS { return m.assets }

func module(id string) testModule {
	return testModule{id: id, assets: fstest.MapFS{
		"manifest.json": {Data: []byte(`{"contract":1,"id":"` + id + `","title":"` + strings.ToUpper(id) + `","entry":"module.js","styles":["app.css"],"nav":[{"path":"/","title":"Home"}]}`)},
		"module.js":     {Data: []byte("export function mount(el, ctx) {}")},
		"app.css":       {Data: []byte("p{}")},
	}}
}

func get(t *testing.T, c *http.Client, u string) (int, string) {
	t.Helper()
	resp, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestMountsModulesByManifest(t *testing.T) {
	h, err := New(Options{Home: "acme", Titles: map[string]string{"acme": "证书"}}, module("docker"), module("acme"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()
	for path, want := range map[string]string{
		"/m/acme/api/hello":          "acme says hi",
		"/m/docker/ui/module.js":     "export function mount",
		"/m/docker/ui/manifest.json": `"contract":1`,
		"/_shell/shell.js":           "mount",
	} {
		if status, body := get(t, srv.Client(), srv.URL+path); status != 200 || !strings.Contains(body, want) {
			t.Errorf("%s: %d %.60q", path, status, body)
		}
	}
	_, body := get(t, srv.Client(), srv.URL+"/_panel.json")
	var panel struct {
		Home    string
		Modules []moduleInfo
	}
	json.Unmarshal([]byte(body), &panel)
	if panel.Home != "acme" || len(panel.Modules) != 2 || panel.Modules[1].Title != "证书" ||
		panel.Modules[0].API != "m/docker/api/" || panel.Modules[0].Entry != "module.js" {
		t.Fatalf("panel = %s", body)
	}
}

func TestMountsUnderAnotherID(t *testing.T) {
	h, err := New(Options{Home: "certificates", IDs: map[string]string{"acme": "certificates"}, Titles: map[string]string{"acme": "Certificates"}},
		module("docker"), module("acme"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()
	if status, body := get(t, srv.Client(), srv.URL+"/m/certificates/api/hello"); status != 200 || body != "acme says hi" {
		t.Errorf("renamed api: %d %q", status, body)
	}
	if status, _ := get(t, srv.Client(), srv.URL+"/m/acme/api/hello"); status == 200 {
		t.Error("still mounted under the manifest id")
	}
	_, body := get(t, srv.Client(), srv.URL+"/_panel.json")
	if !strings.Contains(body, `"id":"certificates"`) || !strings.Contains(body, `"title":"Certificates"`) {
		t.Errorf("panel = %s", body)
	}
	if _, err := New(Options{IDs: map[string]string{"acme": "docker"}}, module("docker"), module("acme")); err == nil {
		t.Error("an id clash was accepted")
	}
}

func TestIndexHasStrictCSP(t *testing.T) {
	h, _ := New(Options{Title: "<Panel>"}, module("docker"))
	w := httptest.NewRecorder()
	h.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	body := w.Body.String()
	if !strings.Contains(body, "&lt;Panel&gt;") || strings.Contains(body, "<script>") {
		t.Fatalf("index = %s", body)
	}
	// The policy must allow the import map as served, byte for byte.
	m := regexp.MustCompile(`<script type="importmap">([^<]*)</script>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no import map in %s", body)
	}
	sum := sha256.Sum256([]byte(m[1]))
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self' 'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") {
		t.Fatalf("csp = %s", csp)
	}
}

func TestManifestValidation(t *testing.T) {
	bad := func(edit func(fstest.MapFS)) testModule {
		m := module("x")
		edit(m.assets)
		return m
	}
	for name, m := range map[string]testModule{
		"no manifest": bad(func(a fstest.MapFS) { delete(a, "manifest.json") }),
		"old contract": bad(func(a fstest.MapFS) {
			a["manifest.json"].Data = []byte(`{"contract":2,"id":"x","title":"X","entry":"module.js"}`)
		}),
		"bad id": bad(func(a fstest.MapFS) {
			a["manifest.json"].Data = []byte(`{"contract":1,"id":"X","title":"X","entry":"module.js"}`)
		}),
		"missing entry": bad(func(a fstest.MapFS) { delete(a, "module.js") }),
		"missing style": bad(func(a fstest.MapFS) { delete(a, "app.css") }),
	} {
		if _, err := New(Options{}, m); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := New(Options{}, module("a"), module("a")); err == nil {
		t.Error("duplicate id accepted")
	}
	if _, err := New(Options{Home: "nope"}, module("a")); err == nil {
		t.Error("unknown home accepted")
	}
}

func TestAuth(t *testing.T) {
	h, _ := New(Options{Auth: &Auth{Secret: "s3cret"}}, module("docker"))
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	if status, _ := get(t, client, srv.URL+"/m/docker/api/hello"); status != 401 {
		t.Fatalf("anonymous api: %d", status)
	}
	if status, _ := get(t, client, srv.URL+"/_panel.json"); status != 401 {
		t.Fatalf("anonymous manifest: %d", status)
	}
	// The shell and module assets are public so the login page can load.
	if status, _ := get(t, client, srv.URL+"/_shell/shell.js"); status != 200 {
		t.Fatalf("shell: %d", status)
	}
	login := func(secret string) int {
		resp, err := client.Post(srv.URL+"/_auth/login", "application/json", strings.NewReader(`{"secret":"`+secret+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if status := login("wrong"); status != 401 {
		t.Fatalf("wrong secret: %d", status)
	}
	if status := login("s3cret"); status != 204 {
		t.Fatalf("login: %d", status)
	}
	if status, body := get(t, client, srv.URL+"/m/docker/api/hello"); status != 200 || body != "docker says hi" {
		t.Fatalf("after login: %d %q", status, body)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/m/docker/api/hello", nil)
	req.Header.Set("Authorization", "Bearer s3cret")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("bearer: %d", resp.StatusCode)
	}
	// A different secret invalidates existing sessions.
	other, _ := New(Options{Auth: &Auth{Secret: "changed"}}, module("docker"))
	srv2 := httptest.NewServer(other.Handler())
	defer srv2.Close()
	u, _ := url.Parse(srv.URL)
	req, _ = http.NewRequest("GET", srv2.URL+"/m/docker/api/hello", nil)
	for _, c := range jar.Cookies(u) {
		req.AddCookie(c)
	}
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("session signed with old secret accepted: %d", resp.StatusCode)
	}
}

// Paths a module declares public skip the sign-in; nothing else does.
func TestPublicPaths(t *testing.T) {
	m := module("ci")
	m.assets["manifest.json"].Data = []byte(`{"contract":1,"id":"ci","title":"CI","entry":"module.js","styles":["app.css"],"public":["/hooks/"]}`)
	h, err := New(Options{Auth: &Auth{Secret: "s3cret"}}, m)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()
	post := func(path string) int {
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if status := post("/m/ci/api/hooks/github"); status != 200 {
		t.Fatalf("public hook: %d", status)
	}
	if status, _ := get(t, srv.Client(), srv.URL+"/m/ci/api/hello"); status != 401 {
		t.Fatalf("private api: %d", status)
	}
	if status := post("/m/ci/api/hooks/../hello"); status == 200 {
		t.Fatal("public prefix escaped")
	}
	for _, bad := range []string{`"hooks/"`, `"/"`, `"/a/../b/"`, `"/{x}"`} {
		m.assets["manifest.json"].Data = []byte(`{"contract":1,"id":"ci","title":"CI","entry":"module.js","public":[` + bad + `]}`)
		if _, err := New(Options{}, m); err == nil {
			t.Errorf("public path %s accepted", bad)
		}
	}
}
