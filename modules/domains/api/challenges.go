package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lsongdev/ohmyserver/modules/domains/provider"
)

// Challenges publishes ACME DNS-01 TXT records in the accounts' zones. It
// has the methods of acme-go's DNSProvider and DNSScope without importing
// acme-go: a panel connects the two.
type Challenges struct{ Service *Service }

// challengeTTL is short: the record lives for one validation.
const challengeTTL = 120

// lookupTimeout bounds refreshing the zone lists when a name is checked.
const lookupTimeout = 20 * time.Second

// find returns the zone holding fqdn: the longest matching zone of any
// account.
func (c Challenges) find(ctx context.Context, fqdn string) (ZoneView, bool) {
	zones, err := c.Service.Zones(ctx, false)
	if err != nil {
		return ZoneView{}, false
	}
	var best ZoneView
	for _, z := range zones {
		if provider.InZone(fqdn, z.Name) && len(z.Name) > len(best.Name) {
			best = z
		}
	}
	return best, best.Name != ""
}

// Covers reports whether an account holds the zone of fqdn.
func (c Challenges) Covers(fqdn string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout)
	defer cancel()
	_, ok := c.find(ctx, fqdn)
	return ok
}

// Present adds the TXT record, unless an identical one is there already.
func (c Challenges) Present(ctx context.Context, fqdn, value string) error {
	z, p, err := c.open(ctx, fqdn)
	if err != nil {
		return err
	}
	existing, err := p.Records(ctx, z.ID, provider.Filter{Type: "TXT", Name: fqdn})
	if err != nil {
		return err
	}
	for _, r := range existing {
		if unquote(r.Content) == value {
			return nil
		}
	}
	_, err = p.CreateRecord(ctx, z.ID, provider.Record{Type: "TXT", Name: fqdn, Content: value, TTL: challengeTTL,
		Comment: "ACME DNS-01 challenge, removed after validation"})
	if err == nil {
		c.Service.log.Info("domains: DNS-01 record published", "name", fqdn, "zone", z.Name, "account", z.AccountName)
	}
	return err
}

// CleanUp removes the TXT records with that value.
func (c Challenges) CleanUp(ctx context.Context, fqdn, value string) error {
	z, p, err := c.open(ctx, fqdn)
	if err != nil {
		return err
	}
	existing, err := p.Records(ctx, z.ID, provider.Filter{Type: "TXT", Name: fqdn})
	if err != nil {
		return err
	}
	for _, r := range existing {
		if unquote(r.Content) == value {
			if err := p.DeleteRecord(ctx, z.ID, r.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c Challenges) open(ctx context.Context, fqdn string) (ZoneView, provider.Provider, error) {
	z, ok := c.find(ctx, fqdn)
	if !ok {
		return z, nil, fmt.Errorf("domains: no account holds the zone of %s", fqdn)
	}
	_, p, err := c.Service.open(ctx, z.Account)
	return z, p, err
}

// unquote strips the quotes some providers return around TXT content.
func unquote(s string) string {
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		return s[1 : len(s)-1]
	}
	return s
}
