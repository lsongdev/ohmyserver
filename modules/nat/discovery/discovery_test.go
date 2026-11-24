package discovery

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/modules/nat/stun"
)

func TestBehaviorScenarios(t *testing.T) {
	for _, tc := range []struct {
		name, mapping, filtering, nat string
		unsupported, controlsFail     bool
	}{
		{name: "full cone", mapping: "endpoint_independent", filtering: "endpoint_independent", nat: "full_cone_like"},
		{name: "address filter", mapping: "endpoint_independent", filtering: "address_dependent", nat: "restricted_cone_like"},
		{name: "port filter", mapping: "endpoint_independent", filtering: "address_port_dependent", nat: "port_restricted_cone_like"},
		{name: "address mapping", mapping: "address_dependent", filtering: "unknown", nat: "symmetric_like"},
		{name: "port mapping", mapping: "address_port_dependent", filtering: "unknown", nat: "symmetric_like"},
		{name: "unsupported", mapping: "unknown", filtering: "unknown", nat: "unknown", unsupported: true},
		{name: "unreachable alternate", mapping: "endpoint_independent", filtering: "unknown", nat: "unknown", controlsFail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := New()
			base := netip.MustParseAddrPort("192.0.2.1:3478")
			other := netip.MustParseAddrPort("192.0.2.2:3479")
			d.Resolve = func(context.Context, string) ([]netip.Addr, error) { return []netip.Addr{base.Addr()}, nil }
			var sockets []net.PacketConn
			d.Listen = func(ctx context.Context, _ string) (net.PacketConn, error) {
				c, e := net.ListenPacket("udp4", "127.0.0.1:0")
				sockets = append(sockets, c)
				return c, e
			}
			touched := map[net.PacketConn]map[netip.AddrPort]bool{}
			d.Exchange = func(ctx context.Context, c net.PacketConn, target, expected netip.AddrPort, flags uint32, _ time.Duration) (stun.Response, error) {
				if touched[c] == nil {
					touched[c] = map[netip.AddrPort]bool{}
				}
				if flags != 0 {
					// Prior traffic to an alternate endpoint would invalidate the test.
					if touched[c][expected] {
						t.Error("filtering socket already contacted alternate endpoint")
					}
					if flags == 6 && tc.filtering != "endpoint_independent" || flags == 2 && tc.filtering != "address_dependent" {
						return stun.Response{}, context.DeadlineExceeded
					}
				} else {
					if len(sockets) > 2 && c == sockets[2] && target != base && tc.controlsFail {
						return stun.Response{}, context.DeadlineExceeded
					}
					touched[c][target] = true
				}
				port := uint16(50000)
				if target.Addr() != base.Addr() {
					if tc.mapping == "address_dependent" {
						port++
					}
					if tc.mapping == "address_port_dependent" {
						port += target.Port()
					}
				}
				if len(sockets) > 2 && c == sockets[2] {
					port += 100
				}
				r := stun.Response{Mapped: netip.AddrPortFrom(netip.MustParseAddr("203.0.113.1"), port), Source: expected, Origin: target}
				if !tc.unsupported {
					r.Other = other
				}
				return r, nil
			}
			result := d.Run(context.Background(), Config{Servers: []string{"test:3478"}}, nil)
			if result.Mapping != tc.mapping || result.Filtering != tc.filtering || result.NATType != tc.nat {
				t.Fatalf("result: %+v", result)
			}
			if !tc.unsupported && len(sockets) != 3 {
				t.Fatalf("expected separate filtering socket, got %d", len(sockets))
			}
		})
	}
}

func TestTimeoutIsUnknown(t *testing.T) {
	d := New()
	d.Resolve = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("192.0.2.1")}, nil
	}
	d.Exchange = func(context.Context, net.PacketConn, netip.AddrPort, netip.AddrPort, uint32, time.Duration) (stun.Response, error) {
		return stun.Response{}, context.DeadlineExceeded
	}
	r := d.Run(context.Background(), Config{Servers: []string{"test:3478"}}, nil)
	if r.UDP || r.Mapping != "unknown" || r.Filtering != "unknown" || r.NATType != "unknown" {
		t.Fatalf("timeout classified: %+v", r)
	}
}

func TestValidate(t *testing.T) {
	for _, cfg := range []Config{{}, {Servers: []string{"host:0"}}, {Servers: []string{"[::1]:3478"}}, {Servers: []string{"host:3478junk"}}, {Servers: []string{"host:3478"}, LocalAddress: "::1"}} {
		if len(Validate(&cfg)) == 0 {
			t.Fatalf("accepted %+v", cfg)
		}
	}
	c := Config{Servers: []string{"  stun.example.com:3478 "}, LocalAddress: "192.168.1.2"}
	if len(Validate(&c)) != 0 || c.Servers[0] != "stun.example.com:3478" {
		t.Fatal(c)
	}
}

// Regression: a Fake-IP binding session must not be reused to verify the
// advertised real server; transparent gateways may rewrite its response source.
func TestFakeIPBehaviorUsesFreshSocket(t *testing.T) {
	d := New()
	fake := netip.MustParseAddr("198.18.0.1")
	origin := netip.MustParseAddrPort("192.0.2.1:3478")
	other := netip.MustParseAddrPort("192.0.2.2:3479")
	d.Resolve = func(context.Context, string) ([]netip.Addr, error) { return []netip.Addr{fake}, nil }
	fakeSocket := map[net.PacketConn]bool{}
	d.Exchange = func(_ context.Context, c net.PacketConn, target, expected netip.AddrPort, flags uint32, _ time.Duration) (stun.Response, error) {
		if target.Addr() == fake {
			fakeSocket[c] = true
		} else if fakeSocket[c] {
			t.Error("real behavior probe reused Fake-IP socket")
			return stun.Response{}, context.DeadlineExceeded
		}
		return stun.Response{Source: expected, Origin: origin, Other: other, Mapped: netip.MustParseAddrPort("203.0.113.1:50000")}, nil
	}
	r := d.Run(context.Background(), Config{Servers: []string{"test:3478"}}, nil)
	if r.NATType != "full_cone_like" {
		t.Fatalf("%+v", r)
	}
}
