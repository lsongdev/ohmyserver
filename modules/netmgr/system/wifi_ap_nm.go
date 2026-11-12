package system

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
)

const apFields = "802-11-wireless.ssid,802-11-wireless.band,802-11-wireless.channel,802-11-wireless.hidden,connection.master,ipv4.addresses"

// WifiAP reads the hotspot this module made for a radio. A hotspot made some
// other way is not read: it is not this module's to change.
func (n *NetworkManager) WifiAP(ctx context.Context, device string) (*WifiAP, error) {
	uuid := wifiAPUUID(device)
	psk, managed := savedSecret(n.profilePath(uuid), "wifi-security", "psk")
	if !managed {
		return nil, nil
	}
	out, _, err := n.Run.Run(ctx, "", "nmcli", "-t", "-e", "no", "-f", apFields, "connection", "show", uuid)
	if err != nil {
		if isUnknownConnection(err) || strings.Contains(err.Error(), "no such connection") {
			return nil, nil
		}
		return nil, err
	}
	v := parseTerse(string(out))
	channel, _ := strconv.Atoi(clean(v["802-11-wireless.channel"]))
	active, _ := n.activeProfiles(ctx)
	return &WifiAP{
		UUID: uuid, Device: device, SSID: clean(v["802-11-wireless.ssid"]), Band: clean(v["802-11-wireless.band"]), Channel: channel,
		Hidden: v["802-11-wireless.hidden"] == "yes", Address: strings.TrimSpace(strings.Split(clean(v["ipv4.addresses"]), ",")[0]),
		Bridge: clean(v["connection.master"]), HasPassword: psk != "", Active: active[device] == uuid,
	}, nil
}

// PrepareWifiAP refuses a hotspot with no password, or in a bridge that does
// not exist, and captures what SaveWifiAP replaces. The undo lists the
// network the radio has joined first and the hotspot last, so a revert, which
// walks backwards, deals with the hotspot before it brings that network back.
func (n *NetworkManager) PrepareWifiAP(ctx context.Context, device string, r WifiAPRequest) (Undo, error) {
	existing, err := n.WifiAP(ctx, device)
	if err != nil {
		return Undo{}, err
	}
	uuid := wifiAPUUID(device)
	path := n.profilePath(uuid)
	if saved, _ := savedSecret(path, "wifi-security", "psk"); r.Password == "" && saved == "" {
		return Undo{}, &ValidationError{Fields: map[string]string{"password": "A hotspot needs a password"}}
	}
	if r.Bridge != "" {
		if b, err := n.Bridge(ctx, r.Bridge); err != nil {
			return Undo{}, err
		} else if b == nil {
			return Undo{}, &ValidationError{Fields: map[string]string{"bridge": r.Bridge + " is not a bridge made here"}}
		}
	}
	undo := Undo{Backend: n.Name(), Link: device}
	if active, _ := n.activeProfiles(ctx); active[device] != "" && active[device] != uuid {
		undo.Profiles = append(undo.Profiles, ProfileUndo{UUID: active[device], Device: device})
	}
	if existing == nil {
		undo.Profiles = append(undo.Profiles, ProfileUndo{UUID: uuid, Name: uuid, Device: device, Created: true, File: path})
		return undo, nil
	}
	backup := n.profileBackup(uuid)
	if err := copyFile(path, backup, 0o600); err != nil {
		return Undo{}, err
	}
	undo.Cleanup = []string{backup}
	if !existing.Active {
		// The edit starts it, so a revert stops it before the file is put back.
		undo.Profiles = append(undo.Profiles, ProfileUndo{UUID: uuid, Down: true})
	}
	undo.Profiles = append(undo.Profiles, ProfileUndo{UUID: uuid, Device: device, File: path, Backup: backup, Reup: existing.Active})
	return undo, nil
}

// SaveWifiAP writes the hotspot and starts it, which ends the radio's use as a
// client. An empty password keeps the one saved.
func (n *NetworkManager) SaveWifiAP(ctx context.Context, device string, r WifiAPRequest) error {
	uuid := wifiAPUUID(device)
	path := n.profilePath(uuid)
	if r.Password == "" {
		r.Password, _ = savedSecret(path, "wifi-security", "psk")
	}
	if err := writeAtomic(path, []byte(wifiAPKeyfile(uuid, device, r)), 0o600); err != nil {
		return err
	}
	if err := n.nmcli(ctx, "connection", "load", path); err != nil {
		return err
	}
	return n.redial(ctx, uuid)
}

// StopWifiAP ends the hotspot and keeps it from starting at boot, so the
// radio goes back to the networks it knows.
func (n *NetworkManager) StopWifiAP(ctx context.Context, device string) error {
	uuid := wifiAPUUID(device)
	if err := n.nmcli(ctx, "connection", "down", uuid); err != nil && !isUnknownConnection(err) && !strings.Contains(err.Error(), "not an active") {
		return err
	}
	err := n.nmcli(ctx, "connection", "modify", uuid, "connection.autoconnect", "no")
	if err != nil && !errors.Is(err, os.ErrNotExist) && !isUnknownConnection(err) {
		return err
	}
	return nil
}
