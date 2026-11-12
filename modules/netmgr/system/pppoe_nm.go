package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const pppoeFields = "connection.id,connection.autoconnect,pppoe.parent,pppoe.username,pppoe.service,ppp.mtu,ipv4.route-metric"

// pppoePath is the connection file this module keeps for a connection, and
// pppoeBackup the hidden copy of it from before a change. NetworkManager
// ignores hidden files, so the copy is never read as a second connection.
func (n *NetworkManager) pppoePath(uuid string) string {
	return filepath.Join(n.connectionDir(), "ohmyserver-"+uuid+".nmconnection")
}

func (n *NetworkManager) pppoeBackup(uuid string) string {
	return filepath.Join(n.connectionDir(), ".ohmyserver-"+uuid+".prev")
}

// keyfileUnescape reads a value written by keyfileValue.
func keyfileUnescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			if s[i] == 's' {
				b.WriteByte(' ')
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// savedPassword reads the password out of a connection file this module
// wrote; ok is false when there is no such file.
func savedPassword(path string) (password string, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
		} else if v, found := strings.CutPrefix(line, "password="); found && section == "pppoe" {
			return keyfileUnescape(v), true
		}
	}
	return "", true
}

func copyFile(from, to string, perm os.FileMode) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return writeAtomic(to, data, perm)
}

func (n *NetworkManager) PPPoEList(ctx context.Context) ([]PPPoE, error) {
	out, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-f", "UUID,TYPE", "connection", "show")
	if err != nil {
		return nil, err
	}
	active, _ := n.activeProfiles(ctx)
	device := map[string]string{}
	for dev, uuid := range active {
		device[uuid] = dev
	}
	list := []PPPoE{}
	for _, line := range strings.Split(string(out), "\n") {
		f := splitTerse(line)
		if len(f) != 2 || f[1] != "pppoe" {
			continue
		}
		show, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-e", "no", "-f", pppoeFields, "connection", "show", f[0])
		if err != nil {
			continue // removed meanwhile
		}
		v := parseTerse(string(show))
		mtu, _ := strconv.Atoi(clean(v["ppp.mtu"]))
		metric, _ := strconv.Atoi(clean(v["ipv4.route-metric"]))
		password, managed := savedPassword(n.pppoePath(f[0]))
		list = append(list, PPPoE{
			UUID: f[0], Name: v["connection.id"], Parent: clean(v["pppoe.parent"]), Username: clean(v["pppoe.username"]),
			Service: clean(v["pppoe.service"]), MTU: mtu, Metric: max(metric, 0), HasPassword: managed && password != "",
			Autoconnect: v["connection.autoconnect"] == "yes", Device: device[f[0]], Active: device[f[0]] != "", Managed: managed,
		})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

// pppoeByName finds a connection by the name it is shown with.
func (n *NetworkManager) pppoeByName(ctx context.Context, name string) (*PPPoE, error) {
	list, err := n.PPPoEList(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Name == name {
			return &list[i], nil
		}
	}
	return nil, ErrNotFound
}

func needPassword() error {
	return &ValidationError{Fields: map[string]string{"password": "A password is needed"}}
}

// PreparePPPoE refuses what would take over a connection made some other
// way, and captures what SavePPPoE replaces: nothing for a new connection
// (a revert deletes it), a hidden copy of the file for an existing one.
func (n *NetworkManager) PreparePPPoE(ctx context.Context, r PPPoERequest) (Undo, error) {
	existing, err := n.pppoeByName(ctx, r.Name)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Undo{}, err
	}
	uuid := pppoeUUID(r.Name)
	path := n.pppoePath(uuid)
	undo := Undo{Backend: n.Name(), Link: r.Parent}
	switch {
	case existing != nil && !existing.Managed:
		return Undo{}, &ValidationError{Fields: map[string]string{"name": r.Name + " is a connection made some other way; choose another name"}}
	case existing == nil:
		if r.Password == "" {
			return Undo{}, needPassword()
		}
		undo.Profiles = []ProfileUndo{{UUID: uuid, Name: uuid, Device: r.Parent, Created: true, File: path}}
		return undo, nil
	}
	if saved, _ := savedPassword(path); r.Password == "" && saved == "" {
		return Undo{}, needPassword()
	}
	backup := n.pppoeBackup(uuid)
	if err := copyFile(path, backup, 0o600); err != nil {
		return Undo{}, err
	}
	undo.Profiles = []ProfileUndo{{UUID: uuid, Device: r.Parent, File: path, Backup: backup, Reup: existing.Active}}
	undo.Cleanup = []string{backup}
	return undo, nil
}

// SavePPPoE writes the connection and dials it. An empty password keeps the
// one saved. A failure here is undone by the revert Prepare made possible.
func (n *NetworkManager) SavePPPoE(ctx context.Context, r PPPoERequest) error {
	uuid := pppoeUUID(r.Name)
	path := n.pppoePath(uuid)
	if r.Password == "" {
		r.Password, _ = savedPassword(path)
	}
	if err := writeAtomic(path, []byte(pppoeKeyfile(uuid, r)), 0o600); err != nil {
		return err
	}
	if err := n.nmcli(ctx, "connection", "load", path); err != nil {
		return err
	}
	return n.redial(ctx, uuid)
}

// redial dials a connection that may be up. On a device, bringing a PPPoE
// connection up while it was up failed with "the device could not be readied"
// and left it down; hanging it up first worked, with no wait in between. The
// new session probably cannot take the interface the old one still holds.
func (n *NetworkManager) redial(ctx context.Context, uuid string) error {
	if err := n.nmcli(ctx, "connection", "down", uuid); err != nil && !isUnknownConnection(err) && !strings.Contains(err.Error(), "not an active") {
		return err
	}
	return n.nmcli(ctx, "--wait", "45", "connection", "up", uuid)
}

// PrepareDial notes that a revert is to hang the connection up again.
func (n *NetworkManager) PrepareDial(ctx context.Context, name string) (Undo, error) {
	p, err := n.pppoeByName(ctx, name)
	if err != nil {
		return Undo{}, err
	}
	undo := Undo{Backend: n.Name(), Link: p.Parent}
	if !p.Active {
		undo.Profiles = []ProfileUndo{{UUID: p.UUID, Down: true}}
	}
	return undo, nil
}

func (n *NetworkManager) Dial(ctx context.Context, name string) error {
	p, err := n.pppoeByName(ctx, name)
	if err != nil {
		return err
	}
	return n.nmcli(ctx, "--wait", "45", "connection", "up", p.UUID)
}

func (n *NetworkManager) Hangup(ctx context.Context, name string) error {
	p, err := n.pppoeByName(ctx, name)
	if err != nil {
		return err
	}
	return n.nmcli(ctx, "connection", "down", p.UUID)
}

// ForgetPPPoE deletes a connection and the files it kept, password included.
func (n *NetworkManager) ForgetPPPoE(ctx context.Context, name string) error {
	p, err := n.pppoeByName(ctx, name)
	if err != nil {
		return err
	}
	if err := n.nmcli(ctx, "connection", "delete", p.UUID); err != nil && !isUnknownConnection(err) {
		return err
	}
	for _, file := range []string{n.pppoePath(p.UUID), n.pppoeBackup(p.UUID)} {
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
