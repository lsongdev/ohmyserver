package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Networkd keeps settings in .network files. Only the keys the form owns
// are rewritten: comments, order and everything else in the file stay.
type Networkd struct {
	Run Runner
	Dir string // where this module writes, /etc/systemd/network
}

func (*Networkd) Name() string { return "systemd-networkd" }

// unit is a parsed .network file, one entry per section.
type unit struct{ sections []*section }

type section struct {
	name  string     // "" before the first header
	lines []unitLine // in file order
}

type unitLine struct {
	raw        string
	key, value string // empty for comments and blank lines
}

func parseUnit(text string) *unit {
	u := &unit{sections: []*section{{}}}
	cur := u.sections[0]
	for _, raw := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			cur = &section{name: line[1 : len(line)-1]}
			u.sections = append(u.sections, cur)
		case line == "" || line[0] == '#' || line[0] == ';' || !strings.Contains(line, "="):
			cur.lines = append(cur.lines, unitLine{raw: raw})
		default:
			key, value, _ := strings.Cut(line, "=")
			cur.lines = append(cur.lines, unitLine{raw: raw, key: strings.TrimSpace(key), value: strings.TrimSpace(value)})
		}
	}
	return u
}

func (u *unit) String() string {
	var b strings.Builder
	for i, s := range u.sections {
		if i == 0 && len(s.lines) == 0 {
			continue
		}
		if s.name != "" {
			fmt.Fprintf(&b, "[%s]\n", s.name)
		}
		for _, l := range s.lines {
			b.WriteString(l.raw)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (u *unit) named(name string) []*section {
	var found []*section
	for _, s := range u.sections {
		if s.name == name {
			found = append(found, s)
		}
	}
	return found
}

// values reads a key through every section of that name; as in systemd, an
// empty assignment clears what came before.
func (u *unit) values(sectionName, key string) []string {
	var values []string
	for _, s := range u.named(sectionName) {
		for _, l := range s.lines {
			if l.key != key {
				continue
			}
			if l.value == "" {
				values = nil
			} else {
				values = append(values, l.value)
			}
		}
	}
	return values
}

// set replaces a key's lines with one line per value, at the first one's
// place or else at the end of the section's settings.
func (s *section) set(key string, values []string) {
	var fresh []unitLine
	for _, v := range values {
		fresh = append(fresh, unitLine{raw: key + "=" + v, key: key, value: v})
	}
	at := -1
	kept := s.lines[:0:0]
	for _, l := range s.lines {
		if l.key == key {
			if at < 0 {
				at = len(kept)
			}
			continue
		}
		kept = append(kept, l)
	}
	if at < 0 {
		// After the last setting, not after trailing blank lines.
		at = len(kept)
		for at > 0 && kept[at-1].key == "" && strings.TrimSpace(kept[at-1].raw) == "" {
			at--
		}
	}
	s.lines = slices.Insert(kept, at, fresh...)
}

func (s *section) valuesOf(key string) []string {
	var values []string
	for _, l := range s.lines {
		if l.key == key {
			values = append(values, l.value)
		}
	}
	return values
}

// simpleRoute reports whether a [Route] section only says where and through
// which gateway, which the form can represent.
func simpleRoute(s *section) (destination, gateway string, ok bool) {
	for _, l := range s.lines {
		switch l.key {
		case "":
			if t := strings.TrimSpace(l.raw); t != "" {
				return "", "", false // a comment would be lost
			}
		case "Destination":
			destination = l.value
		case "Gateway":
			// _dhcp4 and the like have no place in the form.
			if _, err := netip.ParseAddr(l.value); err != nil {
				return "", "", false
			}
			gateway = l.value
		default:
			return "", "", false
		}
	}
	return destination, gateway, true
}

func splitFamily(addrs []string) (four, six string) {
	for _, s := range addrs {
		a, err := netip.ParseAddr(s)
		if err != nil {
			continue
		}
		if a.Is4() && four == "" {
			four = s
		} else if !a.Is4() && six == "" {
			six = s
		}
	}
	return four, six
}

// configOf reads the form's model out of a file, and lists what it keeps
// without showing.
func configOf(u *unit) (Config, []string) {
	c := Config{Addresses: []string{}, DNS: []string{}, Routes: []StaticRoute{}}
	switch strings.ToLower(lastOf(u.values("Network", "DHCP"))) {
	case "yes", "true", "1", "on", "both":
		c.DHCP4, c.DHCP6 = true, true
	case "ipv4":
		c.DHCP4 = true
	case "ipv6":
		c.DHCP6 = true
	}
	c.Addresses = append(c.Addresses, u.values("Network", "Address")...)
	var gateways []string
	gateways = append(gateways, u.values("Network", "Gateway")...)
	for _, v := range u.values("Network", "DNS") {
		c.DNS = append(c.DNS, strings.Fields(v)...)
	}
	c.MTU, _ = strconv.Atoi(lastOf(u.values("Link", "MTUBytes")))
	var preserved []string
	for _, s := range u.sections {
		switch s.name {
		case "", "Match":
		case "Route":
			destination, gateway, ok := simpleRoute(s)
			switch {
			case !ok:
				preserved = append(preserved, describeSection(s))
			case destination == "" || destination == "0.0.0.0/0" || destination == "::/0":
				gateways = append(gateways, gateway)
			default:
				c.Routes = append(c.Routes, StaticRoute{Destination: destination, Gateway: gateway})
			}
		case "Network", "Link":
			for _, l := range s.lines {
				if l.key == "" || ownedKey(s.name, l.key) {
					continue
				}
				preserved = append(preserved, "["+s.name+"] "+l.key+"="+l.value)
			}
		default:
			preserved = append(preserved, describeSection(s))
		}
	}
	c.Gateway4, c.Gateway6 = splitFamily(gateways)
	return c, preserved
}

func ownedKey(sectionName, key string) bool {
	if sectionName == "Link" {
		return key == "MTUBytes"
	}
	return slices.Contains([]string{"DHCP", "Address", "Gateway", "DNS"}, key)
}

func describeSection(s *section) string {
	var parts []string
	for _, l := range s.lines {
		if l.key != "" {
			parts = append(parts, l.key+"="+l.value)
		}
	}
	return "[" + s.name + "] " + strings.Join(parts, " ")
}

func lastOf(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

// applyConfig writes c into the file's text.
func applyConfig(u *unit, c Config) {
	network := firstOrNew(u, "Network")
	// Settings repeated in further [Network] sections would add to the new ones.
	for _, s := range u.named("Network")[1:] {
		for _, key := range []string{"DHCP", "Address", "Gateway", "DNS"} {
			s.set(key, nil)
		}
	}
	var dhcp []string
	switch {
	case c.DHCP4 && c.DHCP6:
		dhcp = []string{"yes"}
	case c.DHCP4:
		dhcp = []string{"ipv4"}
	case c.DHCP6:
		dhcp = []string{"ipv6"}
	case len(network.valuesOf("DHCP")) > 0:
		dhcp = []string{"no"}
	}
	network.set("DHCP", dhcp)
	network.set("Address", c.Addresses)
	var gateways []string
	for _, g := range []string{c.Gateway4, c.Gateway6} {
		if g != "" {
			gateways = append(gateways, g)
		}
	}
	network.set("Gateway", gateways)
	var dns []string
	if len(c.DNS) > 0 {
		dns = []string{strings.Join(c.DNS, " ")}
	}
	network.set("DNS", dns)

	if c.MTU > 0 {
		firstOrNew(u, "Link").set("MTUBytes", []string{strconv.Itoa(c.MTU)})
	} else {
		for _, s := range u.named("Link") {
			s.set("MTUBytes", nil)
		}
	}

	// The simple route sections are the form's; rich ones stay.
	u.sections = slices.DeleteFunc(u.sections, func(s *section) bool {
		if s.name != "Route" {
			return false
		}
		_, _, ok := simpleRoute(s)
		return ok
	})
	for _, r := range c.Routes {
		s := &section{name: "Route"}
		s.set("Destination", []string{r.Destination})
		if r.Gateway != "" {
			s.set("Gateway", []string{r.Gateway})
		}
		if last := u.sections[len(u.sections)-1]; len(last.lines) > 0 && strings.TrimSpace(last.lines[len(last.lines)-1].raw) != "" {
			last.lines = append(last.lines, unitLine{raw: ""})
		}
		u.sections = append(u.sections, s)
	}
	// Nothing left in [Link] but what it was created for.
	u.sections = slices.DeleteFunc(u.sections, func(s *section) bool {
		return s.name == "Link" && len(s.lines) == 0
	})
}

func firstOrNew(u *unit, name string) *section {
	if found := u.named(name); len(found) > 0 {
		return found[0]
	}
	if last := u.sections[len(u.sections)-1]; len(last.lines) > 0 && strings.TrimSpace(last.lines[len(last.lines)-1].raw) != "" {
		last.lines = append(last.lines, unitLine{raw: ""})
	}
	s := &section{name: name}
	u.sections = append(u.sections, s)
	return s
}

// matchReason says why a file cannot be edited for just this interface.
func matchReason(u *unit) string {
	matches := u.named("Match")
	if len(matches) == 0 {
		return "This file matches interfaces in a way this page cannot edit safely."
	}
	for _, s := range matches {
		for _, l := range s.lines {
			if l.key == "" {
				continue
			}
			if !slices.Contains([]string{"Name", "OriginalName", "MACAddress", "PermanentMACAddress"}, l.key) ||
				strings.ContainsAny(l.value, "*?[! \t") {
				return "This file's [Match] section (" + l.key + "=" + l.value + ") may cover other interfaces, so changing it here could change them too."
			}
		}
	}
	return ""
}

type networkdStatus struct {
	NetworkFile string
	DropIns     []string
}

func (n *Networkd) status(ctx context.Context, name string) (networkdStatus, error) {
	out, _, err := n.Run.Run(ctx, "", "networkctl", "--json=short", "status", name)
	if err != nil {
		return networkdStatus{}, err
	}
	var raw struct {
		NetworkFile        string   `json:"NetworkFile"`
		NetworkFileDropins []string `json:"NetworkFileDropins"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return networkdStatus{}, fmt.Errorf("reading networkctl's output: %w", err)
	}
	return networkdStatus{raw.NetworkFile, raw.NetworkFileDropins}, nil
}

// sharedWith names another interface configured by the same file.
func (n *Networkd) sharedWith(ctx context.Context, name, file string) string {
	out, _, err := n.Run.Run(ctx, "", "networkctl", "--json=short", "list")
	if err != nil {
		return ""
	}
	var raw struct {
		Interfaces []struct{ Name string }
	}
	if json.Unmarshal(out, &raw) != nil {
		return ""
	}
	for _, l := range raw.Interfaces {
		if l.Name == name {
			continue
		}
		if s, err := n.status(ctx, l.Name); err == nil && s.NetworkFile == file {
			return l.Name
		}
	}
	return ""
}

// target resolves the file a change is written to: the interface's own in
// /etc, a copy of a vendor file (which then takes its place), or a new one.
type target struct {
	file   string
	source string // read from, may differ from file
	exists bool
	reason string
}

func (n *Networkd) target(ctx context.Context, link Link) (target, error) {
	st, err := n.status(ctx, link.Name)
	if err != nil {
		return target{}, err
	}
	if st.NetworkFile == "" {
		return target{file: filepath.Join(n.Dir, "10-ohmyserver-"+link.Name+".network")}, nil
	}
	t := target{file: st.NetworkFile, source: st.NetworkFile, exists: true}
	switch {
	case strings.HasPrefix(st.NetworkFile, "/run/"):
		t.reason = "This interface's settings are generated at boot (" + st.NetworkFile + "), for example by netplan; change them at their source."
	case len(st.DropIns) > 0:
		t.reason = "Drop-in files also configure this interface (" + strings.Join(st.DropIns, ", ") + "); this page would not see their settings."
	case !strings.HasPrefix(st.NetworkFile, n.Dir+"/"):
		t.file = filepath.Join(n.Dir, filepath.Base(st.NetworkFile))
	}
	return t, nil
}

func (n *Networkd) load(t target) (string, error) {
	if t.source == "" {
		return "", nil
	}
	data, err := os.ReadFile(t.source)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func version(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}

func (n *Networkd) Read(ctx context.Context, link Link) (*Setting, error) {
	t, err := n.target(ctx, link)
	if err != nil {
		return nil, err
	}
	text, err := n.load(t)
	if err != nil {
		return nil, err
	}
	u := parseUnit(text)
	c, preserved := configOf(u)
	s := &Setting{Config: c, Version: version(text), Target: t.file, Exists: t.exists, Preserved: preserved, Reason: t.reason}
	if t.exists && s.Reason == "" {
		s.Reason = matchReason(u)
		for _, l := range u.named("Link") {
			for _, line := range l.lines {
				if line.key == "Unmanaged" && strings.EqualFold(line.value, "yes") {
					s.Reason = "This file tells systemd-networkd not to manage the interface."
				}
			}
		}
		if s.Reason == "" {
			if other := n.sharedWith(ctx, link.Name, t.source); other != "" {
				s.Reason = "This file also configures " + other + ", so changing it here would change that interface too."
			}
		}
	}
	return s, nil
}

func (n *Networkd) render(ctx context.Context, link Link, c Config) (t target, before, after string, err error) {
	t, err = n.target(ctx, link)
	if err != nil {
		return t, "", "", err
	}
	before, err = n.load(t)
	if err != nil {
		return t, "", "", err
	}
	u := parseUnit(before)
	if !t.exists {
		u = parseUnit("[Match]\nName=" + link.Name + "\n")
	}
	applyConfig(u, c)
	return t, before, u.String(), nil
}

func (n *Networkd) Plan(ctx context.Context, link Link, c Config) (*Plan, error) {
	t, before, after, err := n.render(ctx, link, c)
	if err != nil {
		return nil, err
	}
	return &Plan{Target: t.file, Before: before, After: after}, nil
}

func (n *Networkd) Prepare(ctx context.Context, link Link) (Undo, error) {
	t, err := n.target(ctx, link)
	if err != nil {
		return Undo{}, err
	}
	undo := Undo{Backend: n.Name(), Link: link.Name, MTU: link.MTU, File: t.file}
	// A vendor file copied to /etc is undone by removing the copy.
	data, err := os.ReadFile(t.file)
	switch {
	case err == nil:
		undo.Content, undo.Existed = data, true
	case !errors.Is(err, fs.ErrNotExist):
		return Undo{}, err
	}
	return undo, nil
}

func (n *Networkd) Apply(ctx context.Context, link Link, c Config) error {
	t, before, after, err := n.render(ctx, link, c)
	if err != nil {
		return err
	}
	if t.exists && t.source == t.file {
		if err := writeAtomic(t.file+".bak", []byte(before), 0o644); err != nil {
			return err
		}
	}
	if err := writeAtomic(t.file, []byte(after), 0o644); err != nil {
		return err
	}
	return n.reload(ctx, link.Name)
}

func (n *Networkd) Revert(ctx context.Context, undo Undo) error {
	if undo.Existed {
		if err := writeAtomic(undo.File, undo.Content, 0o644); err != nil {
			return err
		}
	} else if err := os.Remove(undo.File); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := n.reload(ctx, undo.Link); err != nil {
		return err
	}
	return restoreMTU(ctx, n.Run, undo)
}

// reload makes networkd read its files, and the one interface use them.
func (n *Networkd) reload(ctx context.Context, name string) error {
	if _, _, err := n.Run.Run(ctx, "", "networkctl", "reload"); err != nil {
		return err
	}
	_, _, err := n.Run.Run(ctx, "", "networkctl", "reconfigure", name)
	return err
}

func (n *Networkd) SetUp(ctx context.Context, name string, up bool) error {
	verb := "down"
	if up {
		verb = "up"
	}
	_, _, err := n.Run.Run(ctx, "", "networkctl", verb, name)
	return err
}

// writeAtomic replaces a file in one step, so a crash leaves the old or the
// new one and never half of either.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Hidden and without the .network suffix: networkd ignores it meanwhile.
	tmp, err := os.CreateTemp(dir, ".netmgr-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
