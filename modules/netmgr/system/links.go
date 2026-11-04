// Package system reads this machine's network state from iproute2 and
// changes interface settings through whichever network manager runs here
// (NetworkManager or systemd-networkd). It has no state of its own.
package system

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// ErrNotFound is an interface this machine does not have.
var ErrNotFound = errors.New("no such interface")

// Installed reports whether this is Linux with iproute2; everything reads
// through it.
func Installed() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := exec.LookPath("ip")
	return err == nil
}

var linkName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$`)

// ValidName reports whether name can be an interface name. Names reach
// commands and file names, so anything else is refused before that.
func ValidName(name string) bool { return linkName.MatchString(name) && !strings.Contains(name, "..") }

// Link is a network interface.
type Link struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"` // ethernet, bridge, vlan, wireguard, …
	State     string    `json:"state"`
	Up        bool      `json:"up"` // administratively up
	Carrier   bool      `json:"carrier"`
	MAC       string    `json:"mac,omitempty"`
	MTU       int       `json:"mtu"`
	SpeedMbps int       `json:"speed_mbps,omitempty"`
	Master    string    `json:"master,omitempty"`
	Addresses []Address `json:"addresses"`
	RXBytes   uint64    `json:"rx_bytes"`
	TXBytes   uint64    `json:"tx_bytes"`
	RXPackets uint64    `json:"rx_packets"`
	TXPackets uint64    `json:"tx_packets"`
	RXErrors  uint64    `json:"rx_errors"`
	TXErrors  uint64    `json:"tx_errors"`
	RXDropped uint64    `json:"rx_dropped"`
	TXDropped uint64    `json:"tx_dropped"`
	Uplink    bool      `json:"uplink"` // a default route leaves through it
}

// Address is an address assigned to an interface.
type Address struct {
	Family    string `json:"family"` // inet or inet6
	Address   string `json:"address"`
	Prefix    int    `json:"prefix"`
	Scope     string `json:"scope"`
	Dynamic   bool   `json:"dynamic,omitempty"`
	ValidLife int64  `json:"valid_seconds,omitempty"` // only when it expires
}

type ipLink struct {
	Name      string   `json:"ifname"`
	Flags     []string `json:"flags"`
	MTU       int      `json:"mtu"`
	OperState string   `json:"operstate"`
	LinkType  string   `json:"link_type"`
	Address   string   `json:"address"`
	Master    string   `json:"master"`
	LinkInfo  struct {
		Kind string `json:"info_kind"`
	} `json:"linkinfo"`
	AddrInfo []struct {
		Family    string `json:"family"`
		Local     string `json:"local"`
		Prefixlen int    `json:"prefixlen"`
		Scope     string `json:"scope"`
		Dynamic   bool   `json:"dynamic"`
		ValidLife int64  `json:"valid_life_time"`
	} `json:"addr_info"`
	Stats struct {
		RX ipCounters `json:"rx"`
		TX ipCounters `json:"tx"`
	} `json:"stats64"`
}

type ipCounters struct {
	Bytes   uint64 `json:"bytes"`
	Packets uint64 `json:"packets"`
	Errors  uint64 `json:"errors"`
	Dropped uint64 `json:"dropped"`
}

// foreverLife is the kernel's "does not expire".
const foreverLife = 4294967295

// Links lists the interfaces, with addresses and counters.
func Links(ctx context.Context, run Runner) ([]Link, error) {
	out, _, err := run.Run(ctx, "", "ip", "-j", "-d", "-s", "addr", "show")
	if err != nil {
		return nil, err
	}
	links, err := parseLinks(out)
	if err != nil {
		return nil, err
	}
	routes, err := Routes(ctx, run)
	if err != nil {
		return nil, err
	}
	uplinks := map[string]bool{}
	for _, r := range routes {
		if r.Destination == "default" && r.Device != "" {
			uplinks[r.Device] = true
		}
	}
	for i := range links {
		links[i].Uplink = uplinks[links[i].Name]
		links[i].SpeedMbps = linkSpeed(links[i].Name)
	}
	return links, nil
}

// FindLink returns one interface, or ErrNotFound.
func FindLink(ctx context.Context, run Runner, name string) (*Link, error) {
	if !ValidName(name) {
		return nil, ErrNotFound
	}
	links, err := Links(ctx, run)
	if err != nil {
		return nil, err
	}
	for i := range links {
		if links[i].Name == name {
			return &links[i], nil
		}
	}
	return nil, ErrNotFound
}

func parseLinks(data []byte) ([]Link, error) {
	var raw []ipLink
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("reading ip's output: %w", err)
	}
	links := make([]Link, 0, len(raw))
	for _, l := range raw {
		link := Link{
			Name: l.Name, Kind: linkKind(l), State: strings.ToLower(l.OperState), MTU: l.MTU,
			MAC: l.Address, Master: l.Master, Addresses: []Address{},
			Up: slices.Contains(l.Flags, "UP"), Carrier: slices.Contains(l.Flags, "LOWER_UP"),
			RXBytes: l.Stats.RX.Bytes, TXBytes: l.Stats.TX.Bytes,
			RXPackets: l.Stats.RX.Packets, TXPackets: l.Stats.TX.Packets,
			RXErrors: l.Stats.RX.Errors, TXErrors: l.Stats.TX.Errors,
			RXDropped: l.Stats.RX.Dropped, TXDropped: l.Stats.TX.Dropped,
		}
		for _, a := range l.AddrInfo {
			addr := Address{Family: a.Family, Address: a.Local, Prefix: a.Prefixlen, Scope: a.Scope, Dynamic: a.Dynamic}
			if a.ValidLife != foreverLife {
				addr.ValidLife = a.ValidLife
			}
			link.Addresses = append(link.Addresses, addr)
		}
		links = append(links, link)
	}
	// Loopback last, the rest by name: real interfaces are what people look for.
	sort.SliceStable(links, func(i, j int) bool {
		li, lj := links[i].Kind == "loopback", links[j].Kind == "loopback"
		if li != lj {
			return lj
		}
		return links[i].Name < links[j].Name
	})
	return links, nil
}

func linkKind(l ipLink) string {
	switch {
	case l.LinkType == "loopback":
		return "loopback"
	case l.LinkInfo.Kind != "":
		return l.LinkInfo.Kind
	case l.LinkType == "ether" && isWireless(l.Name):
		return "wifi"
	case l.LinkType == "ether":
		return "ethernet"
	}
	return l.LinkType
}

func isWireless(name string) bool {
	_, err := os.Stat(filepath.Join("/sys/class/net", name, "wireless"))
	return err == nil
}

// linkSpeed is the negotiated speed, when the driver knows it. Reading it
// from a link that is down fails or answers -1.
func linkSpeed(name string) int {
	data, err := os.ReadFile(filepath.Join("/sys/class/net", name, "speed"))
	if err != nil {
		return 0
	}
	speed, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || speed <= 0 {
		return 0
	}
	return speed
}

// EditableReason says why an interface's settings cannot be changed here,
// or "" when they can. Interfaces other software creates and addresses are
// left alone, since that software would overwrite the settings.
func EditableReason(l Link) string {
	switch l.Kind {
	case "loopback":
		return "The loopback interface has fixed settings."
	case "veth", "tun", "tap", "wireguard":
		return "This " + l.Kind + " interface is set up by the software that created it."
	}
	if l.Master != "" {
		return "This interface is part of " + l.Master + "; addresses belong there."
	}
	for _, prefix := range []string{"docker", "br-", "virbr", "veth", "cni", "flannel", "tailscale"} {
		if strings.HasPrefix(l.Name, prefix) {
			return "This interface is set up by the software that created it."
		}
	}
	return ""
}

// Route is an entry of the main routing table.
type Route struct {
	Family      string `json:"family"` // inet or inet6
	Destination string `json:"destination"`
	Gateway     string `json:"gateway,omitempty"`
	Device      string `json:"device,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	Scope       string `json:"scope,omitempty"`
	Source      string `json:"source,omitempty"`
	Metric      int    `json:"metric,omitempty"`
	Type        string `json:"type,omitempty"` // unreachable, blackhole, …; empty for unicast
}

// Routes reads the main tables of both families.
func Routes(ctx context.Context, run Runner) ([]Route, error) {
	var routes []Route
	for _, family := range []struct{ name, flag string }{{"inet", "-4"}, {"inet6", "-6"}} {
		out, _, err := run.Run(ctx, "", "ip", family.flag, "-j", "route", "show")
		if err != nil {
			return nil, err
		}
		parsed, err := parseRoutes(out, family.name)
		if err != nil {
			return nil, err
		}
		routes = append(routes, parsed...)
	}
	return routes, nil
}

func parseRoutes(data []byte, family string) ([]Route, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	var raw []struct {
		Type     string `json:"type"`
		Dst      string `json:"dst"`
		Gateway  string `json:"gateway"`
		Dev      string `json:"dev"`
		Protocol string `json:"protocol"`
		Scope    string `json:"scope"`
		Prefsrc  string `json:"prefsrc"`
		Metric   int    `json:"metric"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("reading ip's output: %w", err)
	}
	routes := make([]Route, 0, len(raw))
	for _, r := range raw {
		routes = append(routes, Route{
			Family: family, Destination: r.Dst, Gateway: r.Gateway, Device: r.Dev,
			Protocol: r.Protocol, Scope: r.Scope, Source: r.Prefsrc, Metric: r.Metric, Type: r.Type,
		})
	}
	return routes, nil
}

// DeviceTo is the interface traffic to address leaves through, "" when none.
func DeviceTo(ctx context.Context, run Runner, address string) string {
	out, _, err := run.Run(ctx, "", "ip", "-j", "route", "get", address)
	if err != nil {
		return ""
	}
	var raw []struct {
		Dev string `json:"dev"`
	}
	if json.Unmarshal(out, &raw) != nil || len(raw) == 0 {
		return ""
	}
	return raw[0].Dev
}
