package system

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// splitTerse splits one line of nmcli -t output on its unescaped colons and
// unescapes the fields: a colon or backslash inside a value is written \: or \\.
func splitTerse(line string) []string {
	var fields []string
	var cur strings.Builder
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case c == '\\' && i+1 < len(line):
			i++
			cur.WriteByte(line[i])
		case c == ':':
			fields = append(fields, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	return append(fields, cur.String())
}

const scanFields = "IN-USE,SSID,SIGNAL,CHAN,FREQ,SECURITY"

// parseScan reads a scan, one network per name: a name heard from several
// access points is shown once, at its strongest. Hidden networks have no
// name to show.
func parseScan(text string) []WifiNetwork {
	best := map[string]WifiNetwork{}
	for _, line := range strings.Split(text, "\n") {
		f := splitTerse(line)
		if len(f) != 6 || f[1] == "" {
			continue
		}
		signal, _ := strconv.Atoi(f[2])
		channel, _ := strconv.Atoi(f[3])
		mhz, _ := strconv.Atoi(strings.Fields(f[4] + " ")[0])
		n := WifiNetwork{SSID: f[1], Signal: signal, Channel: channel, Band: wifiBand(mhz), Security: f[5], Kind: wifiKind(f[5]), InUse: f[0] == "*"}
		if old, ok := best[n.SSID]; ok {
			n.InUse = n.InUse || old.InUse
			if old.Signal >= n.Signal {
				old.InUse = n.InUse
				n = old
			}
		}
		best[n.SSID] = n
	}
	list := make([]WifiNetwork, 0, len(best))
	for _, n := range best {
		list = append(list, n)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Signal != list[j].Signal {
			return list[i].Signal > list[j].Signal
		}
		return list[i].SSID < list[j].SSID
	})
	return list
}

func (n *NetworkManager) WifiDevices(ctx context.Context) ([]WifiDevice, error) {
	out, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-f", "DEVICE,TYPE,STATE", "device")
	if err != nil {
		return nil, err
	}
	active, _ := n.activeProfiles(ctx)
	saved, _ := n.WifiSaved(ctx)
	devices := []WifiDevice{}
	for _, line := range strings.Split(string(out), "\n") {
		f := splitTerse(line)
		if len(f) != 3 || f[1] != "wifi" {
			continue
		}
		d := WifiDevice{Name: f[0], State: f[2]}
		for _, s := range saved {
			if s.UUID == active[f[0]] {
				d.SSID, d.Profile = s.SSID, s.Name
			}
		}
		devices = append(devices, d)
	}
	return devices, nil
}

// activeProfiles maps each device to the UUID of the connection active on it.
func (n *NetworkManager) activeProfiles(ctx context.Context) (map[string]string, error) {
	out, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-f", "UUID,DEVICE", "connection", "show", "--active")
	if err != nil {
		return nil, err
	}
	active := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if f := splitTerse(line); len(f) == 2 && f[1] != "" {
			active[f[1]] = f[0]
		}
	}
	return active, nil
}

func (n *NetworkManager) WifiSaved(ctx context.Context) ([]WifiSaved, error) {
	out, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-f", "UUID,TYPE", "connection", "show")
	if err != nil {
		return nil, err
	}
	active, _ := n.activeProfiles(ctx)
	running := map[string]bool{}
	for _, uuid := range active {
		running[uuid] = true
	}
	saved := []WifiSaved{}
	for _, line := range strings.Split(string(out), "\n") {
		f := splitTerse(line)
		if len(f) != 2 || f[1] != "802-11-wireless" {
			continue
		}
		show, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-e", "no", "-f",
			"connection.id,connection.interface-name,connection.autoconnect,802-11-wireless.ssid,802-11-wireless-security.key-mgmt",
			"connection", "show", f[0])
		if err != nil {
			continue // removed meanwhile
		}
		v := parseTerse(string(show))
		saved = append(saved, WifiSaved{
			UUID: f[0], Name: v["connection.id"], SSID: clean(v["802-11-wireless.ssid"]), Device: clean(v["connection.interface-name"]),
			Kind: keyMgmtKind(clean(v["802-11-wireless-security.key-mgmt"])), Autoconnect: v["connection.autoconnect"] == "yes", Active: running[f[0]],
		})
	}
	sort.Slice(saved, func(i, j int) bool { return saved[i].SSID < saved[j].SSID })
	return saved, nil
}

func keyMgmtKind(m string) string {
	switch m {
	case "":
		return "open"
	case "wpa-psk", "sae":
		return m
	}
	return "unsupported"
}

func (n *NetworkManager) WifiScan(ctx context.Context, device string, rescan bool) ([]WifiNetwork, error) {
	mode := "no"
	if rescan {
		mode = "yes"
	}
	out, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-f", scanFields, "device", "wifi", "list", "ifname", device, "--rescan", mode)
	if err != nil {
		return nil, err
	}
	networks := parseScan(string(out))
	saved, _ := n.WifiSaved(ctx)
	for i := range networks {
		for _, s := range saved {
			if s.SSID == networks[i].SSID {
				networks[i].Saved = s.UUID
			}
		}
	}
	return networks, nil
}

// wifiUUID names the connection made for a network. It comes from the name,
// so the undo prepared before the connection exists can find it afterwards.
func wifiUUID(ssid string) string {
	sum := sha256.Sum256([]byte("ohmyserver-wifi\x00" + ssid))
	b := sum[:16]
	b[6], b[8] = b[6]&0x0f|0x50, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func savedBySSID(saved []WifiSaved, ssid string) *WifiSaved {
	for i := range saved {
		if saved[i].SSID == ssid {
			return &saved[i]
		}
	}
	return nil
}

// keyfileValue escapes a value for a NetworkManager connection file, which
// reads \\ and \s, and drops spaces at either end.
func keyfileValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	if strings.HasPrefix(s, " ") {
		s = `\s` + s[1:]
	}
	if strings.HasSuffix(s, " ") {
		s = s[:len(s)-1] + `\s`
	}
	return s
}

func wifiKeyfile(uuid string, r WifiRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[connection]\nid=%s\nuuid=%s\ntype=wifi\nautoconnect=true\n\n", keyfileValue(r.SSID), uuid)
	fmt.Fprintf(&b, "[wifi]\nmode=infrastructure\nssid=%s\n", keyfileValue(r.SSID))
	if r.Hidden {
		b.WriteString("hidden=true\n")
	}
	if r.Security != "open" {
		fmt.Fprintf(&b, "\n[wifi-security]\nkey-mgmt=%s\npsk=%s\n", r.Security, keyfileValue(r.Password))
	}
	b.WriteString("\n[ipv4]\nmethod=auto\n\n[ipv6]\nmethod=auto\n")
	return b.String()
}

func (n *NetworkManager) connectionDir() string {
	if n.ConnDir != "" {
		return n.ConnDir
	}
	return "/etc/NetworkManager/system-connections"
}

// PrepareWifi refuses what would silently change a saved network, and
// captures the connection now active on the device so a revert can bring it
// back after deleting the one it made.
func (n *NetworkManager) PrepareWifi(ctx context.Context, device string, r WifiRequest) (Undo, error) {
	saved, err := n.WifiSaved(ctx)
	if err != nil {
		return Undo{}, err
	}
	existing := savedBySSID(saved, r.SSID)
	switch {
	case existing != nil && r.Password != "":
		return Undo{}, &ValidationError{Fields: map[string]string{"password": r.SSID + " is saved already; forget it first to give it a new password"}}
	case existing == nil && r.Security != "open" && r.Password == "":
		return Undo{}, &ValidationError{Fields: map[string]string{"password": "A password is needed"}}
	}
	undo := Undo{Backend: n.Name(), Link: device}
	target := wifiUUID(r.SSID)
	if existing != nil {
		target = existing.UUID
	}
	if active, _ := n.activeProfiles(ctx); active[device] != "" && active[device] != target {
		undo.Profiles = append(undo.Profiles, ProfileUndo{UUID: active[device], Device: device})
	}
	if existing == nil {
		undo.Profiles = append(undo.Profiles, ProfileUndo{Name: target, Device: device, Created: true})
	}
	return undo, nil
}

// ConnectWifi joins the network, making its connection first when it is new.
// The password goes into a file only root can read, then NetworkManager
// loads it; it is on no command line.
func (n *NetworkManager) ConnectWifi(ctx context.Context, device string, r WifiRequest) error {
	saved, err := n.WifiSaved(ctx)
	if err != nil {
		return err
	}
	uuid := wifiUUID(r.SSID)
	if existing := savedBySSID(saved, r.SSID); existing != nil {
		uuid = existing.UUID
	} else if err := n.createWifi(ctx, uuid, r); err != nil {
		return err
	}
	// Bounded, so a network that never answers fails here, where the trial
	// can undo it, and not after nmcli's own, longer wait.
	return n.nmcli(ctx, "--wait", "30", "connection", "up", uuid, "ifname", device)
}

func (n *NetworkManager) createWifi(ctx context.Context, uuid string, r WifiRequest) error {
	path := filepath.Join(n.connectionDir(), "ohmyserver-"+uuid+".nmconnection")
	if err := writeAtomic(path, []byte(wifiKeyfile(uuid, r)), 0o600); err != nil {
		return err
	}
	if err := n.nmcli(ctx, "connection", "load", path); err != nil {
		os.Remove(path)
		return err
	}
	// A name NetworkManager reads differently from how it was written (a
	// keyfile reads some as lists of numbers) would join the wrong network.
	out, _, err := n.Run.Run(ctx, "", "nmcli", "-g", "802-11-wireless.ssid", "-e", "no", "connection", "show", uuid)
	if err != nil || strings.TrimSuffix(string(out), "\n") != r.SSID {
		n.nmcli(ctx, "connection", "delete", uuid)
		os.Remove(path)
		return fmt.Errorf("NetworkManager did not read the network name %q as written", r.SSID)
	}
	return nil
}

func (n *NetworkManager) ForgetWifi(ctx context.Context, uuid string) error {
	return n.nmcli(ctx, "connection", "delete", "uuid", uuid)
}
