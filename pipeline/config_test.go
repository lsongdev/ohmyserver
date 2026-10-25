package pipeline

import (
	"os/exec"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	c, err := Parse([]byte(`
when:
  branch: [main, release/*]
steps:
  test:
    image: golang:1.26
    commands:
      - go vet ./...
      - go test ./...
    cache: /root/go/pkg/mod
    secrets: TOKEN
  image:
    build:
      tag: app:latest
      args: {VERSION: "1"}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Steps) != 2 || c.Steps[0].Name != "test" || c.Steps[1].Name != "image" {
		t.Fatalf("steps %+v", c.Steps)
	}
	s := c.Steps[0]
	if len(s.Commands) != 2 || s.Cache[0] != "/root/go/pkg/mod" || s.Secrets[0] != "TOKEN" {
		t.Fatalf("test step %+v", s)
	}
	if b := c.Steps[1].Build; b.Context != "." || b.Tag[0] != "app:latest" || b.Args["VERSION"] != "1" {
		t.Fatalf("build %+v", b)
	}
	for _, m := range []struct {
		event, branch string
		want          bool
	}{{"push", "main", true}, {"push", "release/1.2", true}, {"push", "dev", false}, {"manual", "main", true}} {
		if c.When.Match(m.event, m.branch) != m.want {
			t.Errorf("match %s %s", m.event, m.branch)
		}
	}
	if (When{Event: list{"push"}}).Match("manual", "main") {
		t.Error("event filter ignored")
	}

	// The list form is accepted too.
	if c, err := Parse([]byte("steps:\n  - name: a\n    image: alpine\n    commands: [echo a]\n")); err != nil || c.Steps[0].Name != "a" {
		t.Fatalf("list form: %+v %v", c, err)
	}
}

func TestParseErrors(t *testing.T) {
	for name, src := range map[string]string{
		"not yaml":         "steps: [",
		"no steps":         "when: {branch: main}",
		"empty steps":      "steps: []",
		"no image":         "steps: {a: {commands: [x]}}",
		"no commands":      "steps: {a: {image: alpine}}",
		"build and image":  "steps: {a: {image: alpine, build: {tag: x}}}",
		"build no tag":     "steps: {a: {build: {context: .}}}",
		"reserved name":    "steps: {clone: {image: alpine, commands: [x]}}",
		"duplicate":        "steps: [{name: a, image: x, commands: [y]}, {name: a, image: x, commands: [y]}]",
		"relative cache":   "steps: {a: {image: alpine, commands: [x], cache: [go/mod]}}",
		"context escapes":  "steps: {a: {build: {tag: x, context: ../other}}}",
		"absolute context": "steps: {a: {build: {tag: x, context: /etc}}}",
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The script echoes each command, quoting it safely, and stops at the
// first failure.
func TestScript(t *testing.T) {
	s := Step{Commands: list{`echo "it's"`, "false", "echo never"}}
	out, err := exec.Command("/bin/sh", "-c", s.Script()).CombinedOutput()
	if err == nil || string(out) != "+ echo \"it's\"\nit's\n+ false\n" {
		t.Fatalf("out %q err %v", out, err)
	}
	if !strings.HasPrefix(s.Script(), "set -e\n") {
		t.Fatal("no set -e")
	}
}
