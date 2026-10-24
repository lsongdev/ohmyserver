// Package gateway connects the yabber gateway to the other modules. Each
// adapter implements an interface the consuming module defines, so the
// modules never import one another:
//
//   - yabber.CertificateSource and yabber.HTTP01Store from the acme-go
//     certificate manager: the gateway serves certificates acme-go issues
//     and answers its HTTP-01 challenges on port 80.
//   - docker-go's apps.Exposer: installed apps get a gateway site for
//     their domain, plus a certificate when an ACME account is available.
package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"slices"
	"time"

	acmeapi "github.com/lsongdev/acme-go/api"
	"github.com/lsongdev/docker-go/apps"
	"github.com/lsongdev/yabber"
	yabberapi "github.com/lsongdev/yabber/api"
)

// Certificates serves the certificate manager's active certificates.
func Certificates(certs *acmeapi.Service) yabber.CertificateSource {
	return yabber.CertificateSourceFunc(func() ([]tls.Certificate, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return certs.TLSCertificates(ctx)
	})
}

// Challenges answers HTTP-01 challenges for issuances the certificate
// manager is running, on the gateway's plain HTTP listener.
func Challenges(certs *acmeapi.Service) yabber.HTTP01Store { return challenges{certs} }

type challenges struct{ certs *acmeapi.Service }

func (c challenges) GetHTTP01Token(_, token string) (string, bool) {
	return c.certs.KeyAuthorization(token)
}

// sitePrefix names the gateway sites the exposer owns, so it never touches
// sites an operator configured.
const sitePrefix = "app-"

// AppExposer publishes docker-go apps through the gateway.
type AppExposer struct {
	Gateway *yabberapi.API   // changes go through the management API's lock and are saved
	Certs   *acmeapi.Service // optional: requests certificates for exposed domains
}

var _ apps.Exposer = (*AppExposer)(nil)

// Expose routes the app's domain to its published port on this host and
// requests a certificate for it if none covers it yet.
func (e *AppExposer) Expose(ctx context.Context, app apps.Exposure) error {
	log := app.Log
	if log == nil {
		log = func(string) {}
	}
	name := sitePrefix + app.ID
	err := e.Gateway.Update(func(c *yabber.Config) error {
		c.Sites = slices.DeleteFunc(c.Sites, func(s yabber.Site) bool { return s.Name == name })
		c.Sites = append(c.Sites, yabber.Site{
			Name:    name,
			Hosts:   []string{app.Domain},
			Handler: yabber.Handler{Proxy: &yabber.Proxy{Upstream: fmt.Sprintf("http://127.0.0.1:%d", app.Port)}},
		})
		return nil
	})
	if err != nil {
		return fmt.Errorf("gateway: publish %s: %w", app.Domain, err)
	}
	log(fmt.Sprintf("Gateway routes %s to port %d", app.Domain, app.Port))
	if e.Certs != nil {
		e.ensureCertificate(ctx, app.Domain, log)
	}
	return nil
}

// Unexpose removes the app's gateway site. Its certificate is kept; the
// certificate manager shows it and it can be deleted there.
func (e *AppExposer) Unexpose(ctx context.Context, id string) error {
	name := sitePrefix + id
	return e.Gateway.Update(func(c *yabber.Config) error {
		c.Sites = slices.DeleteFunc(c.Sites, func(s yabber.Site) bool { return s.Name == name })
		return nil
	})
}

// ensureCertificate starts an issuance for domain unless a certificate
// already covers it. Failing to get one does not fail publishing: the site
// works over HTTP until a certificate exists.
func (e *AppExposer) ensureCertificate(ctx context.Context, domain string, log func(string)) {
	certificates, err := e.Certs.ListCertificates(ctx)
	if err != nil {
		log("Certificate check failed: " + err.Error())
		return
	}
	for _, c := range certificates {
		if c.Status != acmeapi.CertificateRevoked && c.Status != acmeapi.CertificateFailed && slices.Contains(c.Domains, domain) {
			log("Certificate for " + domain + " is managed by the certificate manager")
			return
		}
	}
	accounts, err := e.Certs.ListAccounts(ctx)
	if err != nil {
		log("Certificate check failed: " + err.Error())
		return
	}
	i := slices.IndexFunc(accounts, func(a acmeapi.Account) bool { return a.Status == acmeapi.AccountValid })
	if i < 0 {
		log("No ACME account is registered, so " + domain + " is served without HTTPS; register one in the certificate manager")
		return
	}
	_, issuance, err := e.Certs.CreateCertificate(ctx, acmeapi.CreateCertificateRequest{
		Domains: []string{domain}, AccountID: accounts[i].ID, ChallengeType: "http-01", AutoRenew: true,
	})
	if err != nil {
		log("Requesting a certificate failed: " + err.Error())
		return
	}
	log(fmt.Sprintf("Requested a certificate for %s (issuance #%d); HTTPS starts once it is issued", domain, issuance.ID))
}
