package system

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeCleansAndSorts(t *testing.T) {
	c, err := Config{Enabled: true, WAN: []string{" eth1", "ppp*", "eth1"}, Allow: []Port{{"udp", 51820}, {"tcp", 443}, {"tcp", 22}, {"tcp", 22}}}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.WAN, []string{"eth1", "ppp*"}) {
		t.Errorf("wan = %v", c.WAN)
	}
	if !reflect.DeepEqual(c.Allow, []Port{{"tcp", 22}, {"tcp", 443}, {"udp", 51820}}) {
		t.Errorf("allow = %v", c.Allow)
	}
}

// Names and numbers end up in a script handed to nft, so nothing but an
// interface name and a port may get there.
func TestNormalizeRefusesWhatCouldReachTheScript(t *testing.T) {
	cases := map[string]struct {
		c     Config
		field string
	}{
		"quote":            {Config{Enabled: true, WAN: []string{`eth1" drop`}}, "wan"},
		"newline":          {Config{Enabled: true, WAN: []string{"eth1\n}"}}, "wan"},
		"semicolon":        {Config{Enabled: true, WAN: []string{"eth1;"}}, "wan"},
		"too long":         {Config{Enabled: true, WAN: []string{"abcdefghijklmnop"}}, "wan"},
		"leading dash":     {Config{Enabled: true, WAN: []string{"-x"}}, "wan"},
		"star in middle":   {Config{Enabled: true, WAN: []string{"e*th"}}, "wan"},
		"loopback":         {Config{Enabled: true, WAN: []string{"lo"}}, "wan"},
		"loopback by star": {Config{Enabled: true, WAN: []string{"l*"}}, "wan"},
		"no wan":           {Config{Enabled: true}, "wan"},
		"protocol":         {Config{WAN: []string{"eth1"}, Allow: []Port{{"tcp; drop", 22}}}, "allow"},
		"port zero":        {Config{WAN: []string{"eth1"}, Allow: []Port{{"tcp", 0}}}, "allow"},
		"port too big":     {Config{WAN: []string{"eth1"}, Allow: []Port{{"udp", 65536}}}, "allow"},
	}
	for name, c := range cases {
		_, err := c.c.Normalize()
		v, ok := err.(*ValidationError)
		if !ok || v.Fields[c.field] == "" {
			t.Errorf("%s: %v", name, err)
		}
	}
	many := Config{WAN: []string{"eth1"}}
	for i := 1; i <= maxAllowed+1; i++ {
		many.Allow = append(many.Allow, Port{"tcp", i})
	}
	if _, err := many.Normalize(); err == nil {
		t.Error("too many ports accepted")
	}
}

// A disabled configuration may have no interface: it removes the table.
func TestDisabledNeedsNothing(t *testing.T) {
	if _, err := (Config{}).Normalize(); err != nil {
		t.Error(err)
	}
}

func TestScriptRestrictsOnlyTheWAN(t *testing.T) {
	c := Config{Enabled: true, WAN: []string{"eth1", "ppp*"}, Masquerade: true, AllowPing: true, Allow: []Port{{"tcp", 22}, {"tcp", 443}, {"udp", 51820}}}
	s := c.Script()
	for _, want := range []string{
		"table inet ohmyserver\ndelete table inet ohmyserver\ntable inet ohmyserver {",
		`iifname "eth1" jump wan_in`, `iifname "ppp*" jump wan_in`,
		`iifname "eth1" ct state new drop`,
		`oifname "eth1" meta nfproto ipv4 masquerade`, `oifname "ppp*" meta nfproto ipv4 masquerade`,
		"tcp dport { 22, 443 } accept", "udp dport { 51820 } accept",
		"icmp type echo-request accept", "ct state established,related accept", "ct state invalid drop",
		"udp sport 67 udp dport 68 accept", "nd-router-advert",
		"hook input priority 0; policy accept;", "hook forward priority 0; policy accept;",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script lacks %q:\n%s", want, s)
		}
	}
	// Nothing is dropped except through the WAN chain and the WAN forward rule.
	if strings.Contains(s, "policy drop") {
		t.Errorf("a default drop policy would reach every interface:\n%s", s)
	}
}

func TestScriptOmitsWhatIsNotAsked(t *testing.T) {
	s := Config{Enabled: true, WAN: []string{"eth1"}}.Script()
	for _, no := range []string{"masquerade", "postrouting", "echo-request accept", "dport {"} {
		if strings.Contains(s, no) {
			t.Errorf("script has %q:\n%s", no, s)
		}
	}
	// Echo requests are not accepted, but errors and discovery still are.
	if !strings.Contains(s, "icmpv6 type { destination-unreachable") {
		t.Errorf("IPv6 would break:\n%s", s)
	}
}

func TestDisabledScriptOnlyRemovesTheTable(t *testing.T) {
	s := Config{WAN: []string{"eth1"}, Masquerade: true}.Script()
	if s != "table inet ohmyserver\ndelete table inet ohmyserver\n" {
		t.Errorf("script = %q", s)
	}
}

func TestForwardingIsNeededOnlyForMasquerade(t *testing.T) {
	if (Config{Enabled: true}).Forwarding() || (Config{Masquerade: true}).Forwarding() || !(Config{Enabled: true, Masquerade: true}).Forwarding() {
		t.Error("forwarding is decided by enabled and masquerade together")
	}
}
