package acme

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	acmeapi "github.com/lsongdev/acme-go/acme"
)

// Engine runs ACME account, certificate, and issuance operations using the
// module-owned repository and explicitly injected DNS challenge capability.
type Engine struct {
	repository         Repository
	dns                DNSChallenge
	certificateUpdated CertificateUpdated
	ctx                context.Context
	cancel             context.CancelFunc
	workers            sync.WaitGroup
	lifecycleMu        sync.Mutex
	closed             bool
	tokenMu            sync.RWMutex
	tokens             map[string]string
}

func NewEngine(ctx context.Context, repository Repository, dns DNSChallenge, certificateUpdated CertificateUpdated) *Engine {
	ctx, cancel := context.WithCancel(ctx)
	return &Engine{repository: repository, dns: dns, certificateUpdated: certificateUpdated, ctx: ctx, cancel: cancel, tokens: make(map[string]string)}
}

// Close stops and waits for all in-flight certificate issuance workers.
func (e *Engine) Close() error {
	e.lifecycleMu.Lock()
	e.closed = true
	e.cancel()
	e.lifecycleMu.Unlock()
	e.workers.Wait()
	return nil
}

func (e *Engine) GetHTTP01Token(host, token string) (string, bool) {
	e.tokenMu.RLock()
	defer e.tokenMu.RUnlock()
	if normalized, _, err := net.SplitHostPort(host); err == nil {
		host = normalized
	}
	value, ok := e.tokens[strings.ToLower(strings.TrimSuffix(host, "."))+"\x00"+token]
	return value, ok
}

func (e *Engine) putHTTP01Token(host, token, value string) func() {
	key := strings.ToLower(strings.TrimSuffix(host, ".")) + "\x00" + token
	e.tokenMu.Lock()
	e.tokens[key] = value
	e.tokenMu.Unlock()
	return func() {
		e.tokenMu.Lock()
		delete(e.tokens, key)
		e.tokenMu.Unlock()
	}
}

func (e *Engine) getIssuance(ctx context.Context, id int64) (*Issuance, error) {
	return e.repository.GetIssuance(ctx, id)
}

func (e *Engine) updateIssuance(ctx context.Context, id int64, status, step string, orderID int64, errValue error) {
	_ = e.repository.UpdateIssuance(ctx, id, status, step, orderID, errValue)
}

// ACME API service methods compose the ACME-owned repository with application
// capabilities such as DNS challenges and the background issuance runner.
func (e *Engine) ListACMECertificates(ctx context.Context) ([]Certificate, error) {
	return e.repository.ListACMECertificates(ctx)
}
func (e *Engine) ListProviders(ctx context.Context) ([]Provider, error) {
	return e.repository.ListProviders(ctx)
}
func (e *Engine) CreateProvider(ctx context.Context, name, directoryURL string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return e.repository.CreateProvider(ctx, strings.TrimSpace(name), strings.TrimSpace(directoryURL))
}
func (e *Engine) DeleteProvider(ctx context.Context, id int64) error {
	return e.repository.DeleteProvider(ctx, id)
}
func (e *Engine) DeleteAccount(ctx context.Context, id int64) error {
	return e.repository.DeleteAccount(ctx, id)
}
func (e *Engine) GenerateCSR(ctx context.Context, value string) (*Certificate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	domains, err := NormalizeDomains(value)
	if err != nil {
		return nil, err
	}
	generated, err := GenerateCSR(domains)
	if err != nil {
		return nil, err
	}
	stored, err := e.repository.Create(ctx, generated.Domains, generated.CSRPem, generated.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}
	return &Certificate{ID: stored.ID, Domains: domains, CSRPEM: generated.CSRPem, Status: "pending", CreatedAt: stored.CreatedAt, UpdatedAt: stored.UpdatedAt}, nil
}

func (e *Engine) CertificateArchive(ctx context.Context, id int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cert, err := e.repository.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, content := range map[string]string{"certificate.pem": cert.CertPEM, "private-key.pem": cert.PrivateKeyPEM, "request.csr.pem": cert.CSRPem} {
		if content == "" {
			continue
		}
		file, createErr := archive.Create(name)
		if createErr != nil {
			return nil, createErr
		}
		if _, writeErr := file.Write([]byte(content)); writeErr != nil {
			return nil, writeErr
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (e *Engine) GetCertificateDetails(ctx context.Context, id int64) (*Certificate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	item, err := e.repository.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &Certificate{ID: item.ID, Domains: splitDomains(item.Domains), CSRPEM: item.CSRPem, CertPEM: item.CertPEM, Status: item.Status, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}, nil
}
func (e *Engine) RegisterAccount(ctx context.Context, providerID int64, email string, agreedTerms bool) (*Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, errors.New("email is required")
	}
	provider, err := e.repository.GetProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	client, err := acmeapi.NewClient(&acmeapi.Config{DirectoryURL: provider.DirectoryURL})
	if err != nil {
		return nil, err
	}
	directory, err := client.GetDirectory()
	if err != nil {
		return nil, err
	}
	if directory.Meta.TermsOfService != "" && !agreedTerms {
		return nil, errors.New("terms of service must be accepted")
	}
	if err := client.GenerateKey(); err != nil {
		return nil, err
	}
	keyPEM, err := client.ExportKey()
	if err != nil {
		return nil, err
	}
	thumbprint, err := client.GetThumbprint()
	if err != nil {
		return nil, err
	}
	accountURL, _, err := client.Register(&acmeapi.AccountRequest{TermsOfServiceAgreed: agreedTerms, Contact: []string{"mailto:" + email}})
	if err != nil {
		return nil, err
	}
	return e.repository.CreateAccount(ctx, providerID, accountURL, email, keyPEM, thumbprint, agreedTerms)
}
func (e *Engine) ListAccounts(ctx context.Context) ([]Account, error) {
	return e.repository.ListAccounts(ctx)
}
func (e *Engine) ListOrders(ctx context.Context) ([]Order, error) {
	return e.repository.ListOrders(ctx)
}
func (e *Engine) ListIssuances(ctx context.Context) ([]Issuance, error) {
	return e.repository.ListIssuances(ctx)
}
func (e *Engine) GetIssuance(ctx context.Context, id int64) (*Issuance, error) {
	return e.repository.GetIssuance(ctx, id)
}
func (e *Engine) ListDNSChallengeTargets(ctx context.Context) ([]DNSChallengeTarget, []string, error) {
	if e.dns == nil {
		return nil, nil, errors.New("DNS challenge integration is unavailable")
	}
	targets, warnings, err := e.dns.Targets(ctx)
	return targets, warnings, err
}
func (e *Engine) StartIssuance(ctx context.Context, request IssuanceRequest) (*Issuance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := e.ctx.Err(); err != nil {
		return nil, errors.New("ACME engine is closed")
	}
	if request.AccountID < 1 {
		return nil, errors.New("select an ACME account")
	}
	account, err := e.repository.GetAccount(ctx, request.AccountID)
	if err != nil || account.Status != "valid" {
		return nil, errors.New("the selected ACME account is not valid")
	}
	domains, err := NormalizeDomains(request.Domains)
	if err != nil {
		return nil, err
	}
	if request.ChallengeType != "http-01" && request.ChallengeType != "dns-01" {
		return nil, errors.New("select HTTP-01 or DNS-01")
	}
	if request.ChallengeType == "http-01" {
		for _, domain := range domains {
			if strings.HasPrefix(domain, "*.") {
				return nil, errors.New("wildcard domains require DNS-01")
			}
		}
	}
	if request.ChallengeType == "dns-01" {
		if e.dns == nil {
			return nil, errors.New("DNS challenge integration is unavailable")
		}
		if request.DNSTarget == "" {
			return nil, errors.New("select a local or hosted DNS zone")
		}
		targets, _, targetErr := e.dns.Targets(ctx)
		if targetErr != nil {
			return nil, targetErr
		}
		var selected *DNSChallengeTarget
		for i := range targets {
			if targets[i].Value == request.DNSTarget {
				selected = &targets[i]
				break
			}
		}
		if selected == nil {
			return nil, errors.New("the selected DNS zone is not available")
		}
		for _, domain := range domains {
			if _, _, zoneErr := DNSRelativeName(domain, selected.ZoneName); zoneErr != nil {
				return nil, zoneErr
			}
		}
	}
	material, err := GenerateCSR(domains)
	if err != nil {
		return nil, err
	}
	e.lifecycleMu.Lock()
	if e.closed || e.ctx.Err() != nil {
		e.lifecycleMu.Unlock()
		return nil, errors.New("ACME engine is closed")
	}
	generated, err := e.repository.Create(ctx, material.Domains, material.CSRPem, material.PrivateKeyPEM)
	if err != nil {
		e.lifecycleMu.Unlock()
		return nil, err
	}
	item := &Issuance{CertificateID: generated.ID, AccountID: request.AccountID, Domains: generated.Domains, ChallengeType: request.ChallengeType, DNSTarget: request.DNSTarget}
	if err := e.repository.CreateIssuance(ctx, item); err != nil {
		_ = e.repository.Delete(ctx, generated.ID)
		e.lifecycleMu.Unlock()
		return nil, err
	}
	e.workers.Add(1)
	e.lifecycleMu.Unlock()
	go func(id int64) { defer e.workers.Done(); e.runIssuance(id) }(item.ID)
	return item, nil
}

// RenewCertificate starts a new ACME order for the domains and key belonging
// to an existing active certificate. The certificate remains usable if the
// renewal attempt fails.
func (e *Engine) RenewCertificate(ctx context.Context, certificateID int64, request RenewalRequest) (*Issuance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.AccountID < 1 {
		return nil, errors.New("select an ACME account")
	}
	if request.ChallengeType != "http-01" && request.ChallengeType != "dns-01" {
		return nil, errors.New("select HTTP-01 or DNS-01")
	}
	certificate, err := e.repository.Get(ctx, certificateID)
	if err != nil {
		return nil, err
	}
	if certificate.Status != "active" || certificate.CertPEM == "" {
		return nil, errors.New("only an active certificate can be renewed")
	}
	account, err := e.repository.GetAccount(ctx, request.AccountID)
	if err != nil || account.Status != "valid" {
		return nil, errors.New("the selected ACME account is not valid")
	}
	domains := splitDomains(certificate.Domains)
	if request.ChallengeType == "http-01" {
		for _, domain := range domains {
			if strings.HasPrefix(domain, "*.") {
				return nil, errors.New("wildcard domains require DNS-01")
			}
		}
	}
	if request.ChallengeType == "dns-01" {
		if e.dns == nil {
			return nil, errors.New("DNS challenge integration is unavailable")
		}
		if request.DNSTarget == "" {
			return nil, errors.New("select a local or hosted DNS zone")
		}
		targets, _, targetErr := e.dns.Targets(ctx)
		if targetErr != nil {
			return nil, targetErr
		}
		var selected *DNSChallengeTarget
		for i := range targets {
			if targets[i].Value == request.DNSTarget {
				selected = &targets[i]
				break
			}
		}
		if selected == nil {
			return nil, errors.New("the selected DNS zone is not available")
		}
		for _, domain := range domains {
			if _, _, zoneErr := DNSRelativeName(domain, selected.ZoneName); zoneErr != nil {
				return nil, zoneErr
			}
		}
	}
	issuance := &Issuance{CertificateID: certificateID, AccountID: request.AccountID, Domains: certificate.Domains, ChallengeType: request.ChallengeType, DNSTarget: request.DNSTarget}
	e.lifecycleMu.Lock()
	if e.closed || e.ctx.Err() != nil {
		e.lifecycleMu.Unlock()
		return nil, errors.New("ACME engine is closed")
	}
	if err := e.repository.CreateRenewalIssuance(ctx, issuance); err != nil {
		e.lifecycleMu.Unlock()
		return nil, err
	}
	e.workers.Add(1)
	e.lifecycleMu.Unlock()
	go func(id int64) { defer e.workers.Done(); e.runIssuance(id) }(issuance.ID)
	return issuance, nil
}

func (e *Engine) runIssuance(id int64) {
	ctx := e.ctx
	item, err := e.getIssuance(ctx, id)
	if err != nil {
		return
	}
	fail := func(step string, err error) {
		persistCtx := context.WithoutCancel(ctx)
		certificate, certificateErr := e.repository.Get(persistCtx, item.CertificateID)
		if certificateErr == nil && certificate.Status != "active" {
			_ = e.repository.UpdateStatus(persistCtx, item.CertificateID, "failed")
		}
		e.updateIssuance(persistCtx, id, "failed", step, item.OrderID, err)
	}
	e.updateIssuance(ctx, id, "running", "creating-order", 0, nil)
	account, err := e.repository.GetAccount(ctx, item.AccountID)
	if err != nil {
		fail("account", err)
		return
	}
	authority, err := e.repository.GetProvider(ctx, account.ProviderID)
	if err != nil {
		fail("authority", err)
		return
	}
	client, err := acmeapi.NewClient(&acmeapi.Config{
		DirectoryURL: authority.DirectoryURL, AccountURL: account.AccountURL, AccountKey: account.KeyPem,
	})
	if err == nil {
		err = client.ImportKey(account.KeyPem)
	}
	if err == nil {
		client.Directory, err = client.GetDirectory()
	}
	if err != nil {
		fail("client", err)
		return
	}
	domains := splitDomains(item.Domains)
	identifiers := make([]acmeapi.Identifier, 0, len(domains))
	for _, domain := range domains {
		identifiers = append(identifiers, acmeapi.Identifier{Type: "dns", Value: domain})
	}
	orderURL, orderResponse, err := client.CreateOrder(&acmeapi.OrderRequest{Identifiers: identifiers})
	if err != nil {
		fail("creating-order", err)
		return
	}
	order, err := e.repository.CreateOrder(ctx, item.AccountID, orderURL, item.Domains)
	if err != nil {
		fail("saving-order", err)
		return
	}
	item.OrderID = order.ID
	e.updateIssuance(ctx, id, "running", "authorizing", order.ID, nil)
	for index, authzURL := range orderResponse.Authorizations {
		authz, err := client.GetAuthorization(authzURL)
		if err != nil {
			fail("authorization", err)
			return
		}
		if authz.Status == acmeapi.AuthorizationStatusValid {
			continue
		}
		var selected *acmeapi.Challenge
		for challengeIndex := range authz.Challenges {
			if authz.Challenges[challengeIndex].Type == item.ChallengeType {
				selected = &authz.Challenges[challengeIndex]
				break
			}
		}
		if selected == nil {
			fail("challenge", fmt.Errorf("%s did not offer %s", authz.Identifier.Value, item.ChallengeType))
			return
		}
		e.updateIssuance(ctx, id, "running", fmt.Sprintf("presenting-%d-of-%d", index+1, len(orderResponse.Authorizations)), order.ID, nil)
		var cleanup func() error
		switch item.ChallengeType {
		case "http-01":
			file, err := client.HTTP01KeyAuthorization(selected.Token)
			if err == nil {
				remove := e.putHTTP01Token(authz.Identifier.Value, selected.Token, string(file.Content))
				cleanup = func() error { remove(); return nil }
			}
			if err != nil {
				fail("presenting-http-01", err)
				return
			}
		case "dns-01":
			record, err := client.DNS01KeyAuthorization(selected.Token)
			if err == nil {
				targets, _, targetErr := e.dns.Targets(ctx)
				if targetErr != nil {
					err = targetErr
				} else {
					var target *DNSChallengeTarget
					for targetIndex := range targets {
						if targets[targetIndex].Value == item.DNSTarget {
							target = &targets[targetIndex]
							break
						}
					}
					if target == nil {
						err = errors.New("the selected DNS zone is no longer available")
					} else {
						cleanup, err = e.dns.Present(ctx, *target, authz.Identifier.Value, record.Content)
					}
				}
			}
			if err != nil {
				fail("presenting-dns-01", err)
				return
			}
			e.updateIssuance(ctx, id, "running", "waiting-dns-propagation", order.ID, nil)
			recordName := "_acme-challenge." + strings.TrimPrefix(authz.Identifier.Value, "*.")
			err = waitForDNS01Propagation(ctx, recordName, record.Content, 2*time.Minute)
			if err != nil {
				if cleanup != nil {
					_ = cleanup()
				}
				fail("waiting-dns-propagation", err)
				return
			}
		}
		err = client.CompleteChallenge(selected.URL)
		if err == nil {
			err = waitForAuthorization(ctx, client, authzURL, 3*time.Minute)
		}
		if cleanup != nil {
			if cleanupErr := cleanup(); err == nil {
				err = cleanupErr
			}
		}
		if err != nil {
			fail("validating-"+item.ChallengeType, err)
			return
		}
	}
	e.updateIssuance(ctx, id, "running", "finalizing", order.ID, nil)
	readyOrder, err := waitForOrder(ctx, client, orderURL, acmeapi.OrderStatusReady, 2*time.Minute)
	if err != nil {
		fail("waiting-ready", err)
		return
	}
	certificate, err := e.repository.Get(ctx, item.CertificateID)
	if err != nil {
		fail("certificate", err)
		return
	}
	csrBlock, _ := pem.Decode([]byte(certificate.CSRPem))
	if csrBlock == nil {
		fail("finalizing", errors.New("invalid CSR PEM"))
		return
	}
	finalized, err := client.FinalizeOrder(readyOrder.Finalize, csrBlock.Bytes)
	if err != nil {
		fail("finalizing", err)
		return
	}
	if finalized.Certificate == "" {
		finalized, err = waitForOrder(ctx, client, orderURL, acmeapi.OrderStatusValid, 5*time.Minute)
	}
	if err != nil || finalized.Certificate == "" {
		if err == nil {
			err = errors.New("order became valid without a certificate URL")
		}
		fail("downloading", err)
		return
	}
	certificatePEM, err := client.GetCertificatePEM(finalized.Certificate)
	if err == nil {
		err = VerifyIssuedCertificate(certificatePEM, certificate.PrivateKeyPEM, domains)
	}
	if err != nil {
		fail("verifying", err)
		return
	}
	if err := e.repository.Update(ctx, item.CertificateID, order.ID, certificatePEM, "active"); err != nil {
		fail("saving-certificate", err)
		return
	}
	if e.certificateUpdated != nil {
		if err := e.certificateUpdated(ctx, item.CertificateID); err != nil {
			e.updateIssuance(ctx, id, "succeeded", "certificate-saved-runtime-refresh-failed", order.ID, err)
			return
		}
	}
	e.updateIssuance(ctx, id, "succeeded", "complete", order.ID, nil)
}

func waitForAuthorization(ctx context.Context, client *acmeapi.Client, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := client.GetAuthorization(url)
		if err != nil {
			return err
		}
		switch item.Status {
		case acmeapi.AuthorizationStatusValid:
			return nil
		case "invalid", "deactivated", "expired", "revoked":
			for _, challenge := range item.Challenges {
				if challenge.Status == acmeapi.ChallengeStatusInvalid && challenge.Error.Detail != "" {
					return fmt.Errorf("authorization became %s: %s", item.Status, challenge.Error.Detail)
				}
			}
			return fmt.Errorf("authorization became %s", item.Status)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return errors.New("authorization validation timed out")
}

func waitForDNS01Propagation(ctx context.Context, name, value string, timeout time.Duration) error {
	resolvers := []string{"1.1.1.1:53", "8.8.8.8:53"}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		for _, address := range resolvers {
			resolverAddress := address
			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
					dialer := net.Dialer{Timeout: 5 * time.Second}
					return dialer.DialContext(ctx, "udp", resolverAddress)
				},
			}
			records, err := resolver.LookupTXT(ctx, strings.TrimSuffix(name, "."))
			if err != nil {
				lastErr = err
				continue
			}
			for _, record := range records {
				if record == value {
					return nil
				}
			}
			lastErr = fmt.Errorf("TXT record is not visible through %s", resolverAddress)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("DNS TXT propagation timed out for %s: %w", name, lastErr)
	}
	return fmt.Errorf("DNS TXT propagation timed out for %s", name)
}

func waitForOrder(ctx context.Context, client *acmeapi.Client, url, wanted string, timeout time.Duration) (*acmeapi.OrderResponse, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		item, err := client.GetOrder(url)
		if err != nil {
			return nil, err
		}
		if item.Status == wanted || (wanted == acmeapi.OrderStatusReady && item.Status == acmeapi.OrderStatusValid) {
			return item, nil
		}
		if item.Status == acmeapi.OrderStatusInvalid {
			return nil, errors.New("ACME order became invalid")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, fmt.Errorf("waiting for ACME order status %s timed out", wanted)
}
