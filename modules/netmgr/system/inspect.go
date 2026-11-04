package system

import (
	"context"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Port is a socket waiting for connections or datagrams.
type Port struct {
	Protocol string `json:"protocol"` // tcp or udp
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Process  string `json:"process,omitempty"`
	PID      int    `json:"pid,omitempty"`
}

// Ports lists the listening sockets. Processes need root to be named.
func Ports(ctx context.Context, run Runner) ([]Port, error) {
	out, _, err := run.Run(ctx, "", "ss", "-H", "-t", "-u", "-l", "-n", "-p")
	if err != nil {
		return nil, err
	}
	return parsePorts(string(out)), nil
}

var ssUser = regexp.MustCompile(`users:\(\("([^"]*)",pid=(\d+)`)

func parsePorts(text string) []Port {
	ports := []Port{}
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		address, port, ok := splitLocal(f[4])
		if !ok {
			continue
		}
		p := Port{Protocol: f[0], Address: address, Port: port}
		if m := ssUser.FindStringSubmatch(line); m != nil {
			p.Process = m[1]
			p.PID, _ = strconv.Atoi(m[2])
		}
		ports = append(ports, p)
	}
	sort.SliceStable(ports, func(i, j int) bool {
		if ports[i].Port != ports[j].Port {
			return ports[i].Port < ports[j].Port
		}
		if ports[i].Protocol != ports[j].Protocol {
			return ports[i].Protocol < ports[j].Protocol
		}
		return ports[i].Address < ports[j].Address
	})
	return ports
}

// splitLocal splits ss's "address:port", such as 0.0.0.0:22, [::]:22,
// *:22 or 127.0.0.53%lo:53.
func splitLocal(s string) (string, int, bool) {
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return "", 0, false
	}
	port, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return "", 0, false
	}
	return strings.Trim(s[:i], "[]"), port, true
}

// Resolvers is who answers this machine's DNS queries.
type Resolvers struct {
	Servers []string            `json:"servers"` // from /etc/resolv.conf
	Search  []string            `json:"search"`
	Stub    bool                `json:"stub"`             // resolv.conf points at a local stub
	Global  []string            `json:"global,omitempty"` // systemd-resolved's own
	Links   map[string][]string `json:"links,omitempty"`  // systemd-resolved's, per interface
}

// ReadResolvers reads resolv.conf, and systemd-resolved's view where it runs.
func ReadResolvers(ctx context.Context, run Runner, resolvConf string) Resolvers {
	r := Resolvers{Servers: []string{}, Search: []string{}}
	if data, err := os.ReadFile(resolvConf); err == nil {
		r.Servers, r.Search = parseResolvConf(string(data))
	}
	for _, s := range r.Servers {
		if strings.HasPrefix(s, "127.") || s == "::1" {
			r.Stub = true
		}
	}
	if out, _, err := run.Run(ctx, "", "resolvectl", "dns"); err == nil {
		r.Global, r.Links = parseResolvectl(string(out))
	}
	return r
}

func parseResolvConf(text string) (servers, search []string) {
	servers, search = []string{}, []string{}
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "nameserver":
			servers = append(servers, f[1])
		case "search", "domain":
			// "." is what systemd-resolved writes for "no search domains".
			for _, domain := range f[1:] {
				if domain != "." {
					search = append(search, domain)
				}
			}
		}
	}
	return servers, search
}

var resolvectlLink = regexp.MustCompile(`^Link \d+ \(([^)]+)\):\s*(.*)$`)

func parseResolvectl(text string) ([]string, map[string][]string) {
	var global []string
	links := map[string][]string{}
	for _, line := range strings.Split(text, "\n") {
		if m := resolvectlLink.FindStringSubmatch(line); m != nil {
			if servers := strings.Fields(m[2]); len(servers) > 0 {
				links[m[1]] = servers
			}
		} else if rest, ok := strings.CutPrefix(line, "Global:"); ok {
			global = strings.Fields(rest)
		}
	}
	return global, links
}
