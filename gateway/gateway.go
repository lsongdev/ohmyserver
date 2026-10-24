// Package gateway connects the yabber gateway to the other modules. Each
// adapter implements an interface the consuming module defines, so the
// modules never import one another:
//
//   - yabber.CertificateSource and yabber.HTTP01Store from the acme-go
//     certificate manager: the gateway serves certificates acme-go issues
//     and answers its HTTP-01 challenges on port 80.
//   - docker-go's apps.Exposer: installed apps get a gateway site for
//     their domain, plus a certificate when an ACME account is available.
//     Until the certificate exists the site is served on plain HTTP only
//     (the gateway refuses TLS hosts without a certificate); Reconcile
//     moves it onto every listener once it is issued. With Records set,
//     the domain also gets an address record in DNS.
package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"slices"
	"strings"
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
	Server  *yabber.Server   // the gateway, for reading its configuration
	Certs   *acmeapi.Service // optional: requests certificates for exposed domains
	// Covered reports whether a usable certificate covers domain. Nil asks
	// Certs.
	Covered func(ctx context.Context, domain string) bool
	// Records, optional, publishes app domains in DNS.
	Records Records
}

// Records publishes the address of an app's domain in DNS (package dns
// implements it for dns-go). Publish reports what it did, or why it did
// nothing, through log; failing to publish never fails exposing the app.
type Records interface {
	Publish(ctx context.Context, domain string, log func(string))
	Unpublish(ctx context.Context, domain string) error
}

var _ apps.Exposer = (*AppExposer)(nil)

// Expose routes the app's domain to its published port on this host. A
// domain no certificate covers yet is served on the plain HTTP listeners
// only, and a certificate is requested for it.
func (e *AppExposer) Expose(ctx context.Context, app apps.Exposure) error {
	log := app.Log
	if log == nil {
		log = func(string) {}
	}
	name := sitePrefix + app.ID
	previous := e.siteHost(name)
	covered := e.covered(ctx, app.Domain)
	var restricted bool
	err := e.Gateway.Update(func(c *yabber.Config) error {
		c.Sites = slices.DeleteFunc(c.Sites, func(s yabber.Site) bool { return s.Name == name })
		site := yabber.Site{
			Name:    name,
			Hosts:   []string{app.Domain},
			Handler: yabber.Handler{Proxy: &yabber.Proxy{Upstream: fmt.Sprintf("http://127.0.0.1:%d", app.Port)}},
		}
		if !covered && hasTLS(c) {
			site.Listeners = plainListeners(c)
			if len(site.Listeners) == 0 {
				return fmt.Errorf("no plain HTTP listener can serve %s until its certificate is issued", app.Domain)
			}
			restricted = true
		}
		c.Sites = append(c.Sites, site)
		return nil
	})
	if err != nil {
		return fmt.Errorf("gateway: publish %s: %w", app.Domain, err)
	}
	if restricted {
		log(fmt.Sprintf("Gateway serves http://%s from port %d; HTTPS starts once its certificate is issued", app.Domain, app.Port))
	} else {
		log(fmt.Sprintf("Gateway serves %s from port %d", app.Domain, app.Port))
	}
	if e.Records != nil {
		if previous != "" && !strings.EqualFold(previous, app.Domain) {
			if err := e.Records.Unpublish(ctx, previous); err != nil {
				log("Removing the DNS record for " + previous + " failed: " + err.Error())
			}
		}
		e.Records.Publish(ctx, app.Domain, log)
	}
	if e.Certs != nil && !covered {
		e.ensureCertificate(ctx, app.Domain, log)
	}
	return nil
}

// siteHost returns the domain the named site serves, or "".
func (e *AppExposer) siteHost(name string) string {
	for _, site := range e.Server.Config().Sites {
		if site.Name == name && len(site.Hosts) > 0 {
			return site.Hosts[0]
		}
	}
	return ""
}

// Reconcile moves app sites that were waiting for a certificate onto every
// listener once one covers their domain, and returns how many it moved.
func (e *AppExposer) Reconcile(ctx context.Context) (int, error) {
	var ready []string
	for _, site := range e.Server.Config().Sites {
		if strings.HasPrefix(site.Name, sitePrefix) && len(site.Listeners) > 0 && len(site.Hosts) > 0 && e.covered(ctx, site.Hosts[0]) {
			ready = append(ready, site.Name)
		}
	}
	if len(ready) == 0 {
		return 0, nil
	}
	err := e.Gateway.Update(func(c *yabber.Config) error {
		for i := range c.Sites {
			if slices.Contains(ready, c.Sites[i].Name) {
				c.Sites[i].Listeners = nil
			}
		}
		return nil
	})
	return len(ready), err
}

// RunReconcile calls Reconcile every interval until ctx is done.
func (e *AppExposer) RunReconcile(ctx context.Context, interval time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if n, err := e.Reconcile(ctx); err != nil {
			logger.Error("moving app sites onto HTTPS failed", "err", err)
		} else if n > 0 {
			logger.Info("app sites now served over HTTPS", "count", n)
		}
	}
}

// covered reports whether an active, unexpired certificate covers domain.
func (e *AppExposer) covered(ctx context.Context, domain string) bool {
	if e.Covered != nil {
		return e.Covered(ctx, domain)
	}
	if e.Certs == nil {
		return false
	}
	certificates, err := e.Certs.ListCertificates(ctx)
	if err != nil {
		return false
	}
	now := time.Now()
	for _, c := range certificates {
		if c.Status == acmeapi.CertificateActive && now.Before(c.NotAfter) && slices.Contains(c.Domains, domain) {
			return true
		}
	}
	return false
}

// hasTLS reports whether the gateway may serve sites over TLS. Without
// explicit listeners yabber adds an HTTPS one when certificates exist.
func hasTLS(c *yabber.Config) bool {
	return len(c.Listeners) == 0 || slices.ContainsFunc(c.Listeners, func(l yabber.Listener) bool { return l.TLS })
}

// plainListeners returns the gateway's plain HTTP listeners.
func plainListeners(c *yabber.Config) []string {
	if len(c.Listeners) == 0 {
		return []string{"http"} // yabber's default listener
	}
	var names []string
	for _, l := range c.Listeners {
		if !l.TLS {
			names = append(names, l.Name)
		}
	}
	return names
}

// Unexpose removes the app's gateway site and its DNS record. Its
// certificate is kept; the certificate manager shows it and it can be
// deleted there.
func (e *AppExposer) Unexpose(ctx context.Context, id string) error {
	name := sitePrefix + id
	domain := e.siteHost(name)
	err := e.Gateway.Update(func(c *yabber.Config) error {
		c.Sites = slices.DeleteFunc(c.Sites, func(s yabber.Site) bool { return s.Name == name })
		return nil
	})
	if err != nil || e.Records == nil || domain == "" {
		return err
	}
	return e.Records.Unpublish(ctx, domain)
}

// ensureCertificate starts an issuance for domain unless one is already
// pending or valid. Failing to get one does not fail publishing: the site
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
