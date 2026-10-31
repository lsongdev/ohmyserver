package system

import (
	"errors"
	"strings"
	"testing"
)

// The NAS's smb.conf, with a comment and a continuation line added.
const nas = `# Managed by hand
[global]
   workgroup = WORKGROUP
   server string = files-go NAS
   hosts allow = 192.168.8.0/24 \
                 127.0.0.1
   map to guest = never

[data]
   ; the whole pool
   path = /data
   read only = no
   valid users = lsong
`

func TestParseRoundTrip(t *testing.T) {
	c := Parse(nas)
	if got := c.String(); got != nas {
		t.Fatalf("round trip changed the file:\n%s", got)
	}
	if v, _ := c.Global().Get("Hosts Allow"); v != "192.168.8.0/24 127.0.0.1" {
		t.Errorf("hosts allow = %q", v)
	}
	if n := len(c.Shares()); n != 1 || c.Shares()[0].Name != "data" {
		t.Errorf("shares = %v", c.Shares())
	}
}

func TestSetParamsKeepsPlaceAndComments(t *testing.T) {
	c := Parse(nas)
	data := c.Section("DATA")
	err := data.SetParams([]Param{
		{Key: "path", Value: "/data"},
		{Key: "read only", Value: "yes"},
		{Key: "browseable", Value: "no"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `[data]
   ; the whole pool
   path = /data
   read only = yes
   browseable = no
`
	if got := c.String(); !strings.HasSuffix(got, want) {
		t.Fatalf("got\n%s\nwant suffix\n%s", got, want)
	}
	// The other section is untouched, continuation line included.
	if !strings.Contains(c.String(), "192.168.8.0/24 \\\n                 127.0.0.1\n") {
		t.Error("[global] changed")
	}
}

func TestAddRenameRemove(t *testing.T) {
	c := Parse(nas)
	if _, err := c.AddShare("Media", []Param{{Key: "path", Value: "/data/media"}, {Key: "guest ok", Value: "yes"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(c.String(), "   valid users = lsong\n\n[Media]\n   path = /data/media\n   guest ok = yes\n") {
		t.Fatalf("added:\n%s", c.String())
	}
	if _, err := c.AddShare("media", nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("duplicate name (other case): %v", err)
	}
	if err := c.Rename(c.Section("media"), "data"); !errors.Is(err, ErrInvalid) {
		t.Errorf("rename onto another share: %v", err)
	}
	if err := c.Rename(c.Section("media"), "Films"); err != nil || c.Section("films") == nil {
		t.Errorf("rename: %v", err)
	}
	if !c.RemoveShare("films") || c.Section("films") != nil {
		t.Error("remove")
	}
	if c.String() != nas {
		t.Errorf("add then remove changed the file:\n%s", c.String())
	}
	if c.RemoveShare("global") {
		t.Error("removed [global]")
	}
}

func TestValidation(t *testing.T) {
	for _, name := range []string{"", " x", "a/b", "global", "GLOBAL", "a]b", strings.Repeat("x", 81)} {
		if ShareName(name) == nil {
			t.Errorf("share name %q accepted", name)
		}
	}
	for _, name := range []string{"data", "My Files", "ipc$x", "媒体"} {
		if err := ShareName(name); err != nil {
			t.Errorf("share name %q: %v", name, err)
		}
	}
	s := &Section{Name: "x"}
	for _, p := range [][]Param{
		{{Key: "path", Value: "/a\n[global]"}},
		{{Key: "x = y", Value: "1"}},
		{{Key: "read only", Value: "no"}, {Key: "ReadOnly", Value: "yes"}},
	} {
		if s.SetParams(p) == nil {
			t.Errorf("params %v accepted", p)
		}
	}
	if err := s.SetParams([]Param{{Key: "vfs objects", Value: "fruit streams_xattr"}, {Key: "fruit:metadata", Value: "stream"}}); err != nil {
		t.Error(err)
	}
}

func TestGlobalCreated(t *testing.T) {
	c := Parse("[a]\n path = /a\n")
	if err := c.Global().SetParams([]Param{{Key: "workgroup", Value: "HOME"}}); err != nil {
		t.Fatal(err)
	}
	if got := c.String(); got != "[global]\n   workgroup = HOME\n\n[a]\n path = /a\n" {
		t.Fatalf("got %q", got)
	}
}
