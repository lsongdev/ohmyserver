// Package dns connects the dns-go server to the other modules. Each adapter
// implements an interface the consuming module defines, so the modules
// never import one another:
//
//   - acme-go's api.DNSProvider (and api.DNSScope): DNS-01 challenges for
//     names in a zone dns-go serves authoritatively are published as TXT
//     records automatically; other names stay manual.
//   - dns-go's certificate source: DoT and DoH listeners with "tls: true"
//     serve the certificates acme-go issues.
//   - gateway.Records: a docker-go app's domain gets an address record when
//     a zone dns-go manages contains it.
package dns

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"sync"
	"time"

	acmeapi "github.com/lsongdev/acme-go/api"
	dnsapi "github.com/lsongdev/dns-go/api"
	"github.com/lsongdev/dns-go/config"
)

// Challenges publishes acme-go's DNS-01 records in dns-go's zones.
type Challenges struct{ Manager *dnsapi.Manager }

var (
	_ acmeapi.DNSProvider = Challenges{}
	_ acmeapi.DNSScope    = Challenges{}
)

// Covers reports whether dns-go can answer the CA for fqdn: it must serve
// the zone authoritatively. A forwarding resolver's zones are local
// overrides the CA never sees (and it only answers private networks).
func (c Challenges) Covers(fqdn string) bool {
	if c.Manager.Settings().Mode != config.ModeAuthoritative {
		return false
	}
	_, ok := c.Manager.ZoneFor(fqdn)
	return ok
}

func (c Challenges) Present(ctx context.Context, fqdn, value string) error {
	return c.Manager.AddRecord(ctx, challengeRecord(fqdn, value))
}

func (c Challenges) CleanUp(ctx context.Context, fqdn, value string) error {
	return c.Manager.RemoveRecord(ctx, challengeRecord(fqdn, value))
}

func challengeRecord(fqdn, value string) dnsapi.Record {
	return dnsapi.Record{Name: fqdn, TTL: 60, Type: "TXT", Data: strconv.Quote(value)}
}

// Certificates serves acme-go's active certificates to dns-go's TLS
// listeners, choosing by SNI. The list is re-read at most once a minute.
func Certificates(certs *acmeapi.Service) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	c := &certificateCache{certs: certs}
	return c.get
}

type certificateCache struct {
	certs *acmeapi.Service

	mu     sync.Mutex
	loaded time.Time
	list   []tls.Certificate
}

func (c *certificateCache) get(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	list, err := c.load()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if hello.SupportsCertificate(&list[i]) == nil {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("no certificate from the certificate manager covers %q", hello.ServerName)
}

// load returns the cached list, re-reading it when stale. It does not use
// the handshake's context: the list serves every connection.
func (c *certificateCache) load() ([]tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.list != nil && time.Since(c.loaded) < time.Minute {
		return c.list, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	list, err := c.certs.TLSCertificates(ctx)
	if err != nil {
		return nil, err
	}
	c.list, c.loaded = list, time.Now()
	return list, nil
}

// AppRecords gives app domains an address record in dns-go: the domain
// resolves to Address (the host apps are published on) when a zone dns-go
// manages contains it. It never touches records it did not add: a name
// that already has other address records is left alone.
type AppRecords struct {
	Manager *dnsapi.Manager
	Address string // the -host value; records are added only when it is an IP
}

// Publish adds domain's address record, if a zone contains domain.
func (a *AppRecords) Publish(ctx context.Context, domain string, log func(string)) {
	origin, ok := a.Manager.ZoneFor(domain)
	if !ok {
		return
	}
	record, ok := a.record(domain)
	if !ok {
		log(fmt.Sprintf("DNS zone %s contains %s, but %q is not an IP address to point it at; set -host", origin, domain, a.Address))
		return
	}
	if a.Manager.Settings().Mode == config.ModeAuthoritative && !publicAddr(record.Data) {
		log(fmt.Sprintf("DNS zone %s is served publicly; not publishing the private address %s for %s", origin, record.Data, domain))
		return
	}
	existing, err := a.Manager.Lookup(domain)
	if err != nil {
		log("DNS lookup failed: " + err.Error())
		return
	}
	for _, r := range existing {
		if r.Type == record.Type && r.Data == record.Data {
			return // already published
		}
		if r.Type == "A" || r.Type == "AAAA" || r.Type == "CNAME" {
			log(fmt.Sprintf("DNS zone %s already has a %s record for %s; left unchanged", origin, r.Type, domain))
			return
		}
	}
	if err := a.Manager.AddRecord(ctx, record); err != nil {
		log("Adding the DNS record failed: " + err.Error())
		return
	}
	log(fmt.Sprintf("DNS: %s → %s (zone %s)", domain, record.Data, origin))
}

// Unpublish removes the record Publish would add for domain.
func (a *AppRecords) Unpublish(ctx context.Context, domain string) error {
	record, ok := a.record(domain)
	if !ok {
		return nil
	}
	err := a.Manager.RemoveRecord(ctx, record)
	if errors.Is(err, dnsapi.ErrNoZone) {
		return nil
	}
	return err
}

func (a *AppRecords) record(domain string) (dnsapi.Record, bool) {
	addr, err := netip.ParseAddr(a.Address)
	if err != nil {
		return dnsapi.Record{}, false
	}
	typ := "A"
	if addr.Is6() && !addr.Is4In6() {
		typ = "AAAA"
	}
	return dnsapi.Record{Name: domain, TTL: 300, Type: typ, Data: addr.Unmap().String()}, true
}

func publicAddr(s string) bool {
	addr, err := netip.ParseAddr(s)
	return err == nil && addr.IsGlobalUnicast() && !addr.IsPrivate() &&
		!slices.ContainsFunc(cgnat, func(p netip.Prefix) bool { return p.Contains(addr) })
}

var cgnat = []netip.Prefix{netip.MustParsePrefix("100.64.0.0/10")}
