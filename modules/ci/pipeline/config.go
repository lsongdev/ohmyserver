// Package pipeline reads a repository's .ci.yml and runs it: each step in
// a container of its own, sharing the checked-out workspace.
//
// The file is a subset of Woodpecker's format:
//
//	when:                     # optional: run only for these
//	  branch: [main, release/*]
//	  event: [push, manual]
//	steps:                    # a list, or a map of name to step (in order)
//	  - name: test
//	    image: golang:1.26
//	    commands: [go test ./...]
//	    environment: {CGO_ENABLED: "0"}
//	    secrets: [TOKEN]      # repository secrets passed as variables
//	    cache: [/root/go/pkg/mod]
//	    pull: true            # pull the image even if present
//	    when: {branch: main}  # optional, per step
//	  - name: image
//	    build: {context: ., dockerfile: Dockerfile, tag: "app:latest", args: {V: "1"}}
package pipeline

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is a parsed .ci.yml.
type Config struct {
	When  When   `yaml:"when"`
	Steps []Step `yaml:"-"`
}

// When restricts when a pipeline or step runs; empty lists allow all.
type When struct {
	Branch list `yaml:"branch"` // glob patterns, e.g. release/*
	Event  list `yaml:"event"`  // push, manual
}

// Match reports whether event on branch is allowed.
func (w When) Match(event, branch string) bool {
	if len(w.Event) > 0 && !contains(w.Event, event) {
		return false
	}
	if len(w.Branch) == 0 {
		return true
	}
	for _, pattern := range w.Branch {
		if ok, _ := path.Match(pattern, branch); ok {
			return true
		}
	}
	return false
}

func contains(l []string, s string) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
}

// Step is one step: commands run in an image, or an image build.
type Step struct {
	Name        string            `yaml:"name"`
	Image       string            `yaml:"image"`
	Commands    list              `yaml:"commands"`
	Environment map[string]string `yaml:"environment"`
	Secrets     list              `yaml:"secrets"`
	Cache       list              `yaml:"cache"`
	Pull        bool              `yaml:"pull"`
	Build       *Build            `yaml:"build"`
	When        When              `yaml:"when"`
}

// Build builds an image from the workspace.
type Build struct {
	Context    string            `yaml:"context"`    // relative to the workspace; "." by default
	Dockerfile string            `yaml:"dockerfile"` // relative to the workspace; "Dockerfile" in the context by default
	Tag        list              `yaml:"tag"`
	Args       map[string]string `yaml:"args"`
}

// list accepts a scalar or a sequence.
type list []string

func (l *list) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*l = list{n.Value}
		return nil
	}
	var s []string
	if err := n.Decode(&s); err != nil {
		return err
	}
	*l = s
	return nil
}

// Parse reads a .ci.yml.
func Parse(data []byte) (*Config, error) {
	var raw struct {
		When  When      `yaml:"when"`
		Steps yaml.Node `yaml:"steps"`
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	c := &Config{When: raw.When}
	switch raw.Steps.Kind {
	case yaml.SequenceNode:
		if err := raw.Steps.Decode(&c.Steps); err != nil {
			return nil, fmt.Errorf("steps: %w", err)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(raw.Steps.Content); i += 2 {
			var s Step
			if err := raw.Steps.Content[i+1].Decode(&s); err != nil {
				return nil, fmt.Errorf("steps.%s: %w", raw.Steps.Content[i].Value, err)
			}
			if s.Name == "" {
				s.Name = raw.Steps.Content[i].Value
			}
			c.Steps = append(c.Steps, s)
		}
	case 0:
		return nil, errors.New("no steps")
	default:
		return nil, errors.New("steps must be a list or a map")
	}
	if len(c.Steps) == 0 {
		return nil, errors.New("no steps")
	}
	names := map[string]bool{"clone": true}
	for i := range c.Steps {
		s := &c.Steps[i]
		s.Name = strings.TrimSpace(s.Name)
		switch {
		case s.Name == "":
			return nil, fmt.Errorf("steps[%d]: name required", i)
		case names[s.Name]:
			return nil, fmt.Errorf("step %q: duplicate or reserved name", s.Name)
		case s.Build != nil && (s.Image != "" || len(s.Commands) > 0):
			return nil, fmt.Errorf("step %q: build cannot have an image or commands", s.Name)
		case s.Build == nil && s.Image == "":
			return nil, fmt.Errorf("step %q: image required", s.Name)
		case s.Build == nil && len(s.Commands) == 0:
			return nil, fmt.Errorf("step %q: commands required", s.Name)
		case s.Build != nil && len(s.Build.Tag) == 0:
			return nil, fmt.Errorf("step %q: build needs a tag", s.Name)
		}
		names[s.Name] = true
		for _, p := range s.Cache {
			if !path.IsAbs(p) {
				return nil, fmt.Errorf("step %q: cache path %q must be absolute", s.Name, p)
			}
		}
		if s.Build != nil {
			if s.Build.Context == "" {
				s.Build.Context = "."
			}
			for _, p := range []string{s.Build.Context, s.Build.Dockerfile} {
				if path.IsAbs(p) || strings.HasPrefix(path.Clean(p), "..") {
					return nil, fmt.Errorf("step %q: %q must stay inside the workspace", s.Name, p)
				}
			}
		}
	}
	return c, nil
}

// Script is the shell script of a step's commands: it stops at the first
// failing command and echoes each command before running it.
func (s Step) Script() string {
	var b strings.Builder
	b.WriteString("set -e\n")
	for _, cmd := range s.Commands {
		fmt.Fprintf(&b, "echo %s\n%s\n", shellQuote("+ "+cmd), cmd)
	}
	return b.String()
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
