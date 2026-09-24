package host

import (
	"context"

	acmemodule "github.com/lsongdev/ohmyserver/modules/acme"
)

// acmeDNSChallengeAdapter joins the ACME and DNS module capabilities at the
// application boundary; neither module imports the other.
type acmeDNSChallengeAdapter struct{ server *Server }

func (a acmeDNSChallengeAdapter) Targets(ctx context.Context) ([]acmemodule.DNSChallengeTarget, []string, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	targets, warnings, err := a.server.dnsLocalService(ctx).ChallengeTargets(ctx)
	if err != nil {
		return nil, nil, err
	}
	result := make([]acmemodule.DNSChallengeTarget, 0, len(targets))
	for _, target := range targets {
		result = append(result, acmemodule.DNSChallengeTarget{Value: target.Value, Label: target.Label, Kind: target.Kind, ZoneName: target.ZoneName})
	}
	return result, warnings, ctx.Err()
}

func (a acmeDNSChallengeAdapter) Present(ctx context.Context, target acmemodule.DNSChallengeTarget, identifier, value string) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relative, _, err := acmemodule.DNSRelativeName(identifier, target.ZoneName)
	if err != nil {
		return nil, err
	}
	return a.server.dnsLocalService(ctx).PresentTXT(ctx, target.Value, relative, value)
}

// GetHTTP01Token adapts the ACME challenge store to the Web Server module's
// TLS challenge capability at the application boundary.
func (s *Server) GetHTTP01Token(host, token string) (string, bool) {
	return s.acmeEngine.GetHTTP01Token(host, token)
}
