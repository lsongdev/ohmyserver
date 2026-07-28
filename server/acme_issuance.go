package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	acmeapi "github.com/lsongdev/acme-go/acme"
	"github.com/lsongdev/ohmyserver/dnsprovider/types"
)

type ACMEIssuance struct {
	ID            int64
	CertificateID int64
	AccountID     int64
	OrderID       int64
	Domains       string
	ChallengeType string
	DNSTarget     string
	Status        string
	Step          string
	Error         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (s *Server) initACMEIssuanceSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS acme_issuances (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			certificate_id INTEGER NOT NULL,
			account_id INTEGER NOT NULL,
			order_id INTEGER NOT NULL DEFAULT 0,
			domains TEXT NOT NULL,
			challenge_type TEXT NOT NULL,
			dns_target TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'queued',
			step TEXT NOT NULL DEFAULT 'queued',
			error TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (certificate_id) REFERENCES certificates(id) ON DELETE CASCADE,
			FOREIGN KEY (account_id) REFERENCES acme_accounts(id)
		);
		CREATE INDEX IF NOT EXISTS idx_acme_issuances_certificate ON acme_issuances(certificate_id);
		CREATE INDEX IF NOT EXISTS idx_acme_issuances_status ON acme_issuances(status);
	`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		UPDATE acme_issuances
		SET status = 'failed', step = 'interrupted',
		    error = 'OhMyServer stopped before this issuance completed',
		    updated_at = CURRENT_TIMESTAMP
		WHERE status IN ('queued', 'running')
	`)
	return err
}

func (s *Server) createACMEIssuance(item *ACMEIssuance) error {
	now := time.Now()
	result, err := s.db.Exec(`
		INSERT INTO acme_issuances
			(certificate_id, account_id, domains, challenge_type, dns_target,
			 status, step, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'queued', 'queued', ?, ?)
	`, item.CertificateID, item.AccountID, item.Domains, item.ChallengeType, item.DNSTarget, now, now)
	if err != nil {
		return err
	}
	item.ID, err = result.LastInsertId()
	item.Status, item.Step, item.CreatedAt, item.UpdatedAt = "queued", "queued", now, now
	return err
}

func (s *Server) getACMEIssuance(id int64) (*ACMEIssuance, error) {
	var item ACMEIssuance
	var created, updated string
	err := s.db.QueryRow(`
		SELECT id, certificate_id, account_id, order_id, domains, challenge_type,
		       dns_target, status, step, error, created_at, updated_at
		FROM acme_issuances WHERE id = ?
	`, id).Scan(&item.ID, &item.CertificateID, &item.AccountID, &item.OrderID,
		&item.Domains, &item.ChallengeType, &item.DNSTarget, &item.Status,
		&item.Step, &item.Error, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return &item, nil
}

func (s *Server) updateACMEIssuance(id int64, status, step string, orderID int64, errValue error) {
	message := ""
	if errValue != nil {
		message = errValue.Error()
	}
	_, _ = s.db.Exec(`
		UPDATE acme_issuances
		SET status = ?, step = ?, order_id = CASE WHEN ? > 0 THEN ? ELSE order_id END,
		    error = ?, updated_at = ?
		WHERE id = ?
	`, status, step, orderID, orderID, message, time.Now(), id)
}

func normalizeCertificateDomains(value string) ([]string, error) {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	seen := make(map[string]struct{}, len(parts))
	domains := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(part), "."))
		base := strings.TrimPrefix(name, "*.")
		if base == "" || !strings.Contains(base, ".") || strings.ContainsAny(base, "/:@") {
			return nil, fmt.Errorf("invalid domain name %q", part)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		domains = append(domains, name)
	}
	if len(domains) == 0 {
		return nil, errors.New("at least one domain is required")
	}
	return domains, nil
}

func createAutomaticCSR(domains []string) (*Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: domains[0]},
		DNSNames: domains,
	}, key)
	if err != nil {
		return nil, err
	}
	return &Certificate{
		Domains: strings.Join(domains, ","),
		PrivateKeyPem: string(pem.EncodeToMemory(&pem.Block{
			Type: "EC PRIVATE KEY", Bytes: keyDER,
		})),
		CSRPem: string(pem.EncodeToMemory(&pem.Block{
			Type: "CERTIFICATE REQUEST", Bytes: csrDER,
		})),
	}, nil
}

func (s *Server) ACMECertificateNewHandler(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.GetACMEAccounts()
	if err != nil {
		s.Error(w, err)
		return
	}
	targets, warnings, err := s.DNSChallengeTargets()
	if err != nil {
		s.Error(w, err)
		return
	}
	data := H{
		"accounts": accounts, "dnsTargets": targets, "warnings": warnings,
		"hostname":          strings.TrimSpace(r.URL.Query().Get("hostname")),
		"selectedAccountID": int64(0), "challengeType": "http-01", "dnsTarget": "",
	}
	if r.Method == http.MethodGet {
		s.Render(w, "acme/certificate_new", data)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.Error(w, err)
		return
	}
	data["hostname"] = r.FormValue("domains")
	accountID, err := strconv.ParseInt(r.FormValue("account_id"), 10, 64)
	if err == nil {
		data["selectedAccountID"] = accountID
	}
	data["challengeType"] = r.FormValue("challenge_type")
	data["dnsTarget"] = r.FormValue("dns_target")
	if err != nil {
		data["error"] = "Select an ACME account."
		s.Render(w, "acme/certificate_new", data)
		return
	}
	account, err := s.GetACMEAccount(accountID)
	if err != nil || account.Status != "valid" {
		data["error"] = "The selected ACME account is not valid."
		s.Render(w, "acme/certificate_new", data)
		return
	}
	domains, err := normalizeCertificateDomains(r.FormValue("domains"))
	challengeType := r.FormValue("challenge_type")
	dnsTarget := r.FormValue("dns_target")
	if err == nil && challengeType != "http-01" && challengeType != "dns-01" {
		err = errors.New("select HTTP-01 or DNS-01")
	}
	if err == nil && challengeType == "http-01" {
		for _, domain := range domains {
			if strings.HasPrefix(domain, "*.") {
				err = errors.New("wildcard domains require DNS-01")
				break
			}
		}
	}
	if err == nil && challengeType == "dns-01" && dnsTarget == "" {
		err = errors.New("select a local or hosted DNS zone")
	}
	if err == nil && challengeType == "dns-01" {
		var selected *DNSChallengeTarget
		for index := range targets {
			if targets[index].Value == dnsTarget {
				selected = &targets[index]
				break
			}
		}
		if selected == nil {
			err = errors.New("the selected DNS zone is not available")
		} else {
			for _, domain := range domains {
				if _, _, zoneErr := dnsRelativeName(domain, selected.ZoneName); zoneErr != nil {
					err = zoneErr
					break
				}
			}
		}
	}
	if err != nil {
		data["error"] = err.Error()
		s.Render(w, "acme/certificate_new", data)
		return
	}
	generated, err := createAutomaticCSR(domains)
	if err == nil {
		generated, err = s.CreateCertificate(generated.Domains, generated.CSRPem, generated.PrivateKeyPem)
	}
	if err != nil {
		s.Error(w, err)
		return
	}
	issuance := &ACMEIssuance{
		CertificateID: generated.ID, AccountID: accountID,
		Domains: generated.Domains, ChallengeType: challengeType, DNSTarget: dnsTarget,
	}
	if err := s.createACMEIssuance(issuance); err != nil {
		_ = s.DeleteCertificate(generated.ID)
		s.Error(w, err)
		return
	}
	s.acmeWorkers.Add(1)
	go func() {
		defer s.acmeWorkers.Done()
		s.runACMEIssuance(issuance.ID)
	}()
	http.Redirect(w, r, fmt.Sprintf("/acme/issuances/%d", issuance.ID), http.StatusSeeOther)
}

func (s *Server) ACMEIssuanceView(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}
	item, err := s.getACMEIssuance(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	account, _ := s.GetACMEAccount(item.AccountID)
	authorityName := ""
	if account != nil {
		if authority, authorityErr := s.GetACMEProvider(account.ProviderID); authorityErr == nil {
			authorityName = authority.Name
		}
	}
	s.Render(w, "acme/issuance", H{
		"issuance": item, "account": account, "authorityName": authorityName,
		"running": item.Status == "queued" || item.Status == "running",
	})
}

func (s *Server) GetHTTP01Token(host, token string) (string, bool) {
	s.acmeHTTP01Mu.RLock()
	defer s.acmeHTTP01Mu.RUnlock()
	value, ok := s.acmeHTTP01[host+"\x00"+token]
	return value, ok
}

func (s *Server) putHTTP01Token(host, token, value string) func() {
	key := strings.ToLower(strings.TrimSuffix(host, ".")) + "\x00" + token
	s.acmeHTTP01Mu.Lock()
	s.acmeHTTP01[key] = value
	s.acmeHTTP01Mu.Unlock()
	return func() {
		s.acmeHTTP01Mu.Lock()
		delete(s.acmeHTTP01, key)
		s.acmeHTTP01Mu.Unlock()
	}
}

func (s *Server) runACMEIssuance(id int64) {
	ctx := s.acmeContext
	if ctx == nil {
		ctx = context.Background()
	}
	item, err := s.getACMEIssuance(id)
	if err != nil {
		return
	}
	fail := func(step string, err error) {
		_ = s.UpdateCertificateStatus(item.CertificateID, "failed")
		s.updateACMEIssuance(id, "failed", step, item.OrderID, err)
	}
	s.updateACMEIssuance(id, "running", "creating-order", 0, nil)
	account, err := s.GetACMEAccount(item.AccountID)
	if err != nil {
		fail("account", err)
		return
	}
	authority, err := s.GetACMEProvider(account.ProviderID)
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
	domains := splitCommaSeparated(item.Domains)
	identifiers := make([]acmeapi.Identifier, 0, len(domains))
	for _, domain := range domains {
		identifiers = append(identifiers, acmeapi.Identifier{Type: "dns", Value: domain})
	}
	orderURL, orderResponse, err := client.CreateOrder(&acmeapi.OrderRequest{Identifiers: identifiers})
	if err != nil {
		fail("creating-order", err)
		return
	}
	order, err := s.CreateACMEOrder(item.AccountID, orderURL, item.Domains)
	if err != nil {
		fail("saving-order", err)
		return
	}
	item.OrderID = order.ID
	s.updateACMEIssuance(id, "running", "authorizing", order.ID, nil)
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
		s.updateACMEIssuance(id, "running", fmt.Sprintf("presenting-%d-of-%d", index+1, len(orderResponse.Authorizations)), order.ID, nil)
		var cleanup func() error
		switch item.ChallengeType {
		case "http-01":
			file, err := client.HTTP01KeyAuthorization(selected.Token)
			if err == nil {
				remove := s.putHTTP01Token(authz.Identifier.Value, selected.Token, string(file.Content))
				cleanup = func() error { remove(); return nil }
			}
			if err != nil {
				fail("presenting-http-01", err)
				return
			}
		case "dns-01":
			record, err := client.DNS01KeyAuthorization(selected.Token)
			if err == nil {
				cleanup, err = s.presentDNS01(item.DNSTarget, authz.Identifier.Value, record.Content)
			}
			if err != nil {
				fail("presenting-dns-01", err)
				return
			}
			s.updateACMEIssuance(id, "running", "waiting-dns-propagation", order.ID, nil)
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
	s.updateACMEIssuance(id, "running", "finalizing", order.ID, nil)
	readyOrder, err := waitForOrder(ctx, client, orderURL, acmeapi.OrderStatusReady, 2*time.Minute)
	if err != nil {
		fail("waiting-ready", err)
		return
	}
	certificate, err := s.GetCertificate(item.CertificateID)
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
		err = verifyIssuedCertificate(certificatePEM, certificate.PrivateKeyPem, domains)
	}
	if err != nil {
		fail("verifying", err)
		return
	}
	if err := s.UpdateCertificate(item.CertificateID, order.ID, certificatePEM, "active"); err != nil {
		fail("saving-certificate", err)
		return
	}
	s.updateACMEIssuance(id, "succeeded", "complete", order.ID, nil)
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

func verifyIssuedCertificate(certPEM, keyPEM string, domains []string) error {
	pair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return fmt.Errorf("certificate and private key do not match: %w", err)
	}
	if len(pair.Certificate) == 0 {
		return errors.New("issued certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return err
	}
	for _, domain := range domains {
		if strings.HasPrefix(domain, "*.") {
			found := false
			for _, dnsName := range leaf.DNSNames {
				if strings.EqualFold(strings.TrimSuffix(dnsName, "."), domain) {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("issued certificate does not contain wildcard SAN %s", domain)
			}
			continue
		}
		if err := leaf.VerifyHostname(domain); err != nil {
			return fmt.Errorf("issued certificate does not cover %s: %w", domain, err)
		}
	}
	return nil
}

func dnsRelativeName(identifier, zoneName string) (string, string, error) {
	identifier = strings.TrimPrefix(strings.ToLower(strings.TrimSuffix(identifier, ".")), "*.")
	zoneName = strings.ToLower(strings.TrimSuffix(zoneName, "."))
	fqdn := "_acme-challenge." + identifier
	if identifier != zoneName && !strings.HasSuffix(identifier, "."+zoneName) {
		return "", "", fmt.Errorf("%s is not inside selected zone %s", identifier, zoneName)
	}
	relative := strings.TrimSuffix(fqdn, "."+zoneName)
	if relative == fqdn {
		return "", "", fmt.Errorf("cannot make %s relative to %s", fqdn, zoneName)
	}
	return relative, fqdn, nil
}

func (s *Server) presentDNS01(target, identifier, value string) (func() error, error) {
	if strings.HasPrefix(target, "local:") {
		zoneID, err := strconv.ParseInt(strings.TrimPrefix(target, "local:"), 10, 64)
		if err != nil {
			return nil, errors.New("invalid local DNS zone")
		}
		zone, err := s.GetDNSZoneByID(zoneID)
		if err != nil {
			return nil, err
		}
		name, _, err := dnsRelativeName(identifier, zone.Name)
		if err != nil {
			return nil, err
		}
		record := &DNSLocalRecord{
			ZoneID: zone.ID, Name: name, Type: "TXT", TTL: 60,
			Value: value, Enabled: true,
		}
		if err := s.saveDNSRecord(record); err != nil {
			return nil, err
		}
		if err := s.reloadDNS(); err != nil {
			_ = s.DeleteDNSLocalRecord(record.ID)
			return nil, err
		}
		return func() error {
			if err := s.DeleteDNSLocalRecord(record.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			return s.reloadDNS()
		}, nil
	}
	providerID, zoneID, err := parseProviderTarget(target)
	if err != nil {
		return nil, err
	}
	connection, err := s.GetDNSProvider(providerID)
	if err != nil {
		return nil, err
	}
	client, err := s.GetDNSProviderClient(providerID)
	if err != nil {
		return nil, err
	}
	zones, err := client.ListZones()
	if err != nil {
		return nil, err
	}
	zoneName := ""
	for _, zone := range zones {
		if zone.ID == zoneID {
			zoneName = zone.Name
			break
		}
	}
	if zoneName == "" {
		return nil, fmt.Errorf("hosted zone is no longer available from %s", connection.Name)
	}
	relative, fqdn, err := dnsRelativeName(identifier, zoneName)
	if err != nil {
		return nil, err
	}
	before, err := client.ListRecords(zoneID)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]struct{}, len(before))
	for _, record := range before {
		existing[record.ID] = struct{}{}
	}
	if err := client.AddRecord(zoneID, &types.Record{Type: "TXT", Name: relative, Value: value, TTL: 60, Comment: "OhMyServer ACME DNS-01"}); err != nil {
		return nil, err
	}
	after, err := client.ListRecords(zoneID)
	if err != nil {
		return nil, err
	}
	recordID := ""
	for _, record := range after {
		name := strings.ToLower(strings.TrimSuffix(record.Name, "."))
		if _, existed := existing[record.ID]; !existed && strings.EqualFold(record.Type, "TXT") &&
			(name == strings.ToLower(relative) || name == strings.ToLower(fqdn)) && record.Value == value {
			recordID = record.ID
			break
		}
	}
	if recordID == "" {
		return nil, errors.New("DNS provider created the TXT record but did not return its record ID")
	}
	return func() error { return client.DeleteRecord(zoneID, recordID) }, nil
}

func init() {
	RegisterInitSchemaHook((*Server).initACMEIssuanceSchema)
}
