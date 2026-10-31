package system

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrInvalid is a request the configuration cannot take: a bad share name,
// parameter or value.
var ErrInvalid = errors.New("invalid")

// Conf is a parsed smb.conf that writes back what it read: comments, blank
// lines, the order of sections and parameters, and parameters it does not
// know, so changing one share changes only that share's lines.
type Conf struct {
	Head     []line // before the first section
	Sections []*Section
}

// Section is [global], a share, or [homes] / [printers].
type Section struct {
	Name   string
	header string // the header line as read; empty for a new section
	lines  []line
}

// line is a parameter, or a comment or blank line when key is empty. raw
// is the text as read (continuation lines included) until it changes.
type line struct {
	key, value string
	raw        string
}

// Param is a parameter as the API exchanges it.
type Param struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Canon is how Samba compares parameter names: case and spaces do not
// matter ("Read Only" is "readonly").
func Canon(key string) string {
	return strings.ToLower(strings.Join(strings.Fields(key), ""))
}

// Parse reads smb.conf text. It never fails: lines it does not understand
// are kept as they are, as Samba itself skips them.
func Parse(text string) *Conf {
	c := &Conf{}
	physical := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if n := len(physical); n > 0 && physical[n-1] == "" {
		physical = physical[:n-1] // the final newline
	}
	var current *Section
	add := func(l line) {
		if current == nil {
			c.Head = append(c.Head, l)
		} else {
			current.lines = append(current.lines, l)
		}
	}
	for i := 0; i < len(physical); i++ {
		raw := physical[i]
		logical := strings.TrimSpace(raw)
		// A trailing backslash continues a parameter on the next line.
		for strings.HasSuffix(logical, `\`) && i+1 < len(physical) && !isComment(logical) {
			i++
			raw += "\n" + physical[i]
			logical = strings.TrimSpace(strings.TrimSuffix(logical, `\`)) + " " + strings.TrimSpace(physical[i])
		}
		switch {
		case logical == "" || isComment(logical):
			add(line{raw: raw})
		case strings.HasPrefix(logical, "["):
			name, _, _ := strings.Cut(logical[1:], "]")
			current = &Section{Name: strings.TrimSpace(name), header: raw}
			c.Sections = append(c.Sections, current)
		default:
			key, value, ok := strings.Cut(logical, "=")
			if !ok {
				add(line{raw: raw})
				continue
			}
			add(line{key: strings.TrimSpace(key), value: strings.TrimSpace(value), raw: raw})
		}
	}
	return c
}

func isComment(s string) bool { return strings.HasPrefix(s, "#") || strings.HasPrefix(s, ";") }

// String writes the configuration back.
func (c *Conf) String() string {
	var b strings.Builder
	for _, l := range c.Head {
		b.WriteString(l.text(defaultIndent) + "\n")
	}
	for _, s := range c.Sections {
		header := s.header
		if header == "" {
			header = "[" + s.Name + "]"
		}
		b.WriteString(header + "\n")
		indent := s.indent()
		for _, l := range s.lines {
			b.WriteString(l.text(indent) + "\n")
		}
	}
	return b.String()
}

const defaultIndent = "   "

func (l line) text(indent string) string {
	if l.raw != "" || l.key == "" {
		return l.raw
	}
	return indent + l.key + " = " + l.value
}

// indent is how the section's parameters are indented, so new ones line up.
func (s *Section) indent() string {
	for _, l := range s.lines {
		if l.key != "" && l.raw != "" {
			return l.raw[:len(l.raw)-len(strings.TrimLeft(l.raw, " \t"))]
		}
	}
	return defaultIndent
}

// Section finds a section by name; Samba ignores case in share names.
func (c *Conf) Section(name string) *Section {
	for _, s := range c.Sections {
		if strings.EqualFold(s.Name, name) {
			return s
		}
	}
	return nil
}

// Global returns [global], adding it at the top when it is missing.
func (c *Conf) Global() *Section {
	if s := c.Section("global"); s != nil {
		return s
	}
	s := &Section{Name: "global", lines: []line{{}}}
	c.Sections = append([]*Section{s}, c.Sections...)
	return s
}

// Shares returns every section but [global].
func (c *Conf) Shares() []*Section {
	var shares []*Section
	for _, s := range c.Sections {
		if !strings.EqualFold(s.Name, "global") {
			shares = append(shares, s)
		}
	}
	return shares
}

// AddShare appends a share after a blank line.
func (c *Conf) AddShare(name string, params []Param) (*Section, error) {
	if err := ShareName(name); err != nil {
		return nil, err
	}
	if c.Section(name) != nil {
		return nil, fmt.Errorf("%w: a share named %q exists", ErrInvalid, name)
	}
	if n := len(c.Sections); n > 0 {
		last := c.Sections[n-1]
		if k := len(last.lines); k == 0 || strings.TrimSpace(last.lines[k-1].raw) != "" || last.lines[k-1].key != "" {
			last.lines = append(last.lines, line{})
		}
	} else if k := len(c.Head); k > 0 && strings.TrimSpace(c.Head[k-1].raw) != "" {
		c.Head = append(c.Head, line{})
	}
	s := &Section{Name: name}
	c.Sections = append(c.Sections, s)
	return s, s.SetParams(params)
}

// RemoveShare removes a share and its lines.
func (c *Conf) RemoveShare(name string) bool {
	for i, s := range c.Sections {
		if strings.EqualFold(s.Name, name) && !strings.EqualFold(name, "global") {
			c.Sections = append(c.Sections[:i], c.Sections[i+1:]...)
			// The blank lines that separated the last section go with it.
			if i == len(c.Sections) && i > 0 {
				prev := c.Sections[i-1]
				for k := len(prev.lines); k > 0 && prev.lines[k-1].key == "" && strings.TrimSpace(prev.lines[k-1].raw) == ""; k-- {
					prev.lines = prev.lines[:k-1]
				}
			}
			return true
		}
	}
	return false
}

// Rename gives a share a new name, keeping its place and contents.
func (c *Conf) Rename(s *Section, name string) error {
	if s.Name == name {
		return nil
	}
	if err := ShareName(name); err != nil {
		return err
	}
	if other := c.Section(name); other != nil && other != s {
		return fmt.Errorf("%w: a share named %q exists", ErrInvalid, name)
	}
	s.Name, s.header = name, ""
	return nil
}

// Params returns the section's parameters in order.
func (s *Section) Params() []Param {
	params := []Param{}
	for _, l := range s.lines {
		if l.key != "" {
			params = append(params, Param{Key: l.key, Value: l.value})
		}
	}
	return params
}

// Get returns a parameter's value; the last one wins, as in Samba.
func (s *Section) Get(key string) (string, bool) {
	value, found := "", false
	for _, l := range s.lines {
		if l.key != "" && Canon(l.key) == Canon(key) {
			value, found = l.value, true
		}
	}
	return value, found
}

// SetParams makes params the section's parameters: existing ones keep
// their place (and their text when the value is unchanged), ones no longer
// listed go, and new ones follow the last parameter. Comments stay.
func (s *Section) SetParams(params []Param) error {
	want := map[string]int{}
	for i, p := range params {
		p.Key, p.Value = strings.Join(strings.Fields(p.Key), " "), strings.TrimSpace(p.Value)
		if err := checkParam(p); err != nil {
			return err
		}
		if _, dup := want[Canon(p.Key)]; dup {
			return fmt.Errorf("%w: %q is listed twice", ErrInvalid, p.Key)
		}
		want[Canon(p.Key)] = i
		params[i] = p
	}
	placed := map[int]bool{}
	kept := s.lines[:0:0]
	last := -1 // index in kept of the last parameter
	for _, l := range s.lines {
		if l.key == "" {
			kept = append(kept, l)
			continue
		}
		i, ok := want[Canon(l.key)]
		if !ok || placed[i] {
			continue
		}
		placed[i] = true
		if l.value != params[i].Value {
			l.value, l.raw = params[i].Value, ""
		}
		kept = append(kept, l)
		last = len(kept) - 1
	}
	var added []line
	for i, p := range params {
		if !placed[i] {
			added = append(added, line{key: p.Key, value: p.Value})
		}
	}
	at := last + 1
	if last < 0 {
		// After the comments that open the section.
		for at = 0; at < len(kept) && strings.TrimSpace(kept[at].raw) != ""; at++ {
		}
	}
	s.lines = append(kept[:at:at], append(added, kept[at:]...)...)
	return nil
}

var (
	shareName = regexp.MustCompile(`^[^\[\]\\/:*?"<>|+=;,%\x00-\x1f]{1,80}$`)
	paramKey  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 :._-]{0,99}$`)
)

// ShareName checks a share name: Windows clients cannot reach names with
// \ / : * ? " < > | and a few others, and [global] is not a share.
func ShareName(name string) error {
	switch {
	case name != strings.TrimSpace(name) || !shareName.MatchString(name):
		return fmt.Errorf("%w share name %q: up to 80 characters, without spaces at the ends or any of [ ] \\ / : * ? \" < > | + = ; , %%", ErrInvalid, name)
	case strings.EqualFold(name, "global"):
		return fmt.Errorf("%w share name: [global] holds the server settings", ErrInvalid)
	}
	return nil
}

func checkParam(p Param) error {
	if !paramKey.MatchString(p.Key) {
		return fmt.Errorf("%w parameter name %q", ErrInvalid, p.Key)
	}
	if strings.ContainsAny(p.Value, "\r\n\x00") {
		return fmt.Errorf("%w value for %q: one line only", ErrInvalid, p.Key)
	}
	return nil
}

// Bool reads a Samba boolean; ok is false for anything else.
func Bool(value string) (v, ok bool) {
	switch strings.ToLower(value) {
	case "yes", "true", "1", "on":
		return true, true
	case "no", "false", "0", "off":
		return false, true
	}
	return false, false
}
