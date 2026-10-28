package dns

import (
	"context"
	"fmt"

	acmeapi "github.com/lsongdev/acme-go/api"
)

// ChallengeSource publishes DNS-01 records for the names it covers: dns-go's
// own zones (Challenges), or a DNS provider account's (the domains module).
type ChallengeSource interface {
	acmeapi.DNSProvider
	acmeapi.DNSScope
}

// Sources hands each DNS-01 record to the first source covering its name,
// so one certificate may span zones served here and zones hosted at a
// provider. Names no source covers stay manual.
type Sources []ChallengeSource

var (
	_ acmeapi.DNSProvider = Sources{}
	_ acmeapi.DNSScope    = Sources{}
)

func (s Sources) source(fqdn string) ChallengeSource {
	for _, src := range s {
		if src.Covers(fqdn) {
			return src
		}
	}
	return nil
}

// Covers reports whether some source can publish fqdn.
func (s Sources) Covers(fqdn string) bool { return s.source(fqdn) != nil }

func (s Sources) Present(ctx context.Context, fqdn, value string) error {
	src := s.source(fqdn)
	if src == nil {
		return fmt.Errorf("no DNS source covers %s", fqdn)
	}
	return src.Present(ctx, fqdn, value)
}

func (s Sources) CleanUp(ctx context.Context, fqdn, value string) error {
	src := s.source(fqdn)
	if src == nil {
		return nil
	}
	return src.CleanUp(ctx, fqdn, value)
}
