package acme

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// CSRMaterial contains the private issuance material needed to complete an
// ACME order. Callers must keep PrivateKeyPEM secret.
type CSRMaterial struct {
	Domains       string
	PrivateKeyPEM string
	CSRPem        string
}

// NormalizeDomains validates and normalizes a comma, newline, or whitespace
// separated list of DNS names, preserving the first occurrence of each name.
func NormalizeDomains(value string) ([]string, error) {
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

// GenerateCSR creates a P-256 key and CSR for the requested DNS names.
func GenerateCSR(domains []string) (*CSRMaterial, error) {
	if len(domains) == 0 {
		return nil, errors.New("at least one domain is required")
	}
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
	return &CSRMaterial{
		Domains:       strings.Join(domains, ","),
		PrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})),
		CSRPem:        string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})),
	}, nil
}

// DNSRelativeName computes the relative TXT name and FQDN for an ACME
// _acme-challenge record inside zoneName.
func DNSRelativeName(identifier, zoneName string) (relative, fqdn string, err error) {
	identifier = strings.TrimPrefix(strings.ToLower(strings.TrimSuffix(identifier, ".")), "*.")
	zoneName = strings.ToLower(strings.TrimSuffix(zoneName, "."))
	fqdn = "_acme-challenge." + identifier
	if identifier != zoneName && !strings.HasSuffix(identifier, "."+zoneName) {
		return "", "", fmt.Errorf("%s is not inside selected zone %s", identifier, zoneName)
	}
	relative = strings.TrimSuffix(fqdn, "."+zoneName)
	if relative == fqdn {
		return "", "", fmt.Errorf("cannot make %s relative to %s", fqdn, zoneName)
	}
	return relative, fqdn, nil
}

// VerifyIssuedCertificate checks that the issued certificate matches its key
// and contains every requested DNS name.
func VerifyIssuedCertificate(certPEM, keyPEM string, domains []string) error {
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
