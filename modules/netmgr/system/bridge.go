// Bridges join interfaces into one network, as a switch does. They are
// created and changed through NetworkManager only.
package system

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// maxLinkName is the longest interface name Linux accepts.
const maxLinkName = 15

// Bridge is a software switch that joins interfaces into one network.
type Bridge struct {
	Name    string   `json:"name"`
	STP     bool     `json:"stp"`
	Ports   []string `json:"ports"`
	Profile string   `json:"profile"` // the connection that holds it
	Version string   `json:"version"` // changes when the bridge or its ports do
}

// BridgeConfig is what a bridge is made from. Its addresses are the
// interface's own settings, edited like any other interface's.
type BridgeConfig struct {
	STP   bool     `json:"stp"`
	Ports []string `json:"ports"`
}

// BridgeBackend is a network manager that can make bridges.
type BridgeBackend interface {
	Bridges(ctx context.Context) ([]Bridge, error)
	// Bridge returns nil when there is no such bridge.
	Bridge(ctx context.Context, name string) (*Bridge, error)
	// PrepareBridge captures what ApplyBridge will change when it creates or
	// changes the bridge and its ports.
	PrepareBridge(ctx context.Context, name string, c BridgeConfig) (Undo, error)
	ApplyBridge(ctx context.Context, name string, c BridgeConfig) error
	// PrepareDeleteBridge captures a bridge and its ports before DeleteBridge
	// removes the bridge and frees the ports.
	PrepareDeleteBridge(ctx context.Context, name string) (Undo, error)
	DeleteBridge(ctx context.Context, name string) error
}

// dockerBridge matches the names Docker gives the bridges of its networks
// (br- and twelve hex digits). Other br- names, such as br-lan, are ours.
var dockerBridge = regexp.MustCompile(`^br-[0-9a-f]{12}$`)

// ValidBridgeName reports whether name can be a bridge this module makes,
// and says why not. Names reach commands and file names.
func ValidBridgeName(name string) error {
	switch {
	case !ValidName(name):
		return fmt.Errorf("%q is not a valid interface name", name)
	case len(name) > maxLinkName:
		return fmt.Errorf("%q is longer than %d characters, which Linux does not allow", name, maxLinkName)
	}
	if reason := EditableReason(Link{Name: name, Kind: "bridge"}); reason != "" {
		return fmt.Errorf("%q looks like a bridge made by other software; choose another name, such as br-lan", name)
	}
	return nil
}

// Normalize checks a bridge's ports and returns them trimmed, without
// repeats, in order.
func (c BridgeConfig) Normalize() (BridgeConfig, error) {
	out := BridgeConfig{STP: c.STP, Ports: []string{}}
	for _, p := range c.Ports {
		p = strings.TrimSpace(p)
		if !ValidName(p) {
			return BridgeConfig{}, &ValidationError{Fields: map[string]string{"ports": fmt.Sprintf("%q is not an interface name", p)}}
		}
		out.Ports = append(out.Ports, p)
	}
	sort.Strings(out.Ports)
	out.Ports = slices.Compact(out.Ports)
	return out, nil
}

// BridgeProblems checks a bridge against the interfaces that exist. A
// bridge is only made from Ethernet ports nothing else has claimed, and
// its name must not belong to an interface this module did not make.
func BridgeProblems(name string, current *Bridge, c BridgeConfig, links []Link) map[string]string {
	fields := map[string]string{}
	if err := ValidBridgeName(name); err != nil {
		fields["name"] = err.Error()
	}
	byName := map[string]Link{}
	for _, l := range links {
		byName[l.Name] = l
	}
	if _, taken := byName[name]; taken && current == nil && fields["name"] == "" {
		fields["name"] = name + " is already an interface that is not a bridge made here"
	}
	for _, p := range c.Ports {
		l, ok := byName[p]
		switch {
		case !ok:
			fields["ports"] = p + " does not exist"
		case l.Kind != "ethernet":
			fields["ports"] = p + " is not an Ethernet interface"
		case l.Master != "" && l.Master != name:
			fields["ports"] = p + " is already part of " + l.Master
		case l.Master == "" && EditableReason(l) != "":
			fields["ports"] = p + ": " + EditableReason(l)
		}
	}
	return fields
}
