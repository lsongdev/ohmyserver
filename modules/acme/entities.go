package acme

import (
	"context"
	"strings"
	"time"
)

func splitDomains(value string) []string {
	parts := strings.Split(value, ",")
	domains := make([]string, 0, len(parts))
	for _, part := range parts {
		if domain := strings.TrimSpace(part); domain != "" {
			domains = append(domains, domain)
		}
	}
	return domains
}

type Certificate struct {
	ID              int64     `json:"id"`
	Domains         []string  `json:"domains"`
	CSRPEM          string    `json:"csr_pem,omitempty"`
	CertPEM         string    `json:"cert_pem,omitempty"`
	CommonName      string    `json:"common_name"`
	DNSNames        []string  `json:"dns_names"`
	Status          string    `json:"status"`
	ExpiresAt       time.Time `json:"expires_at,omitzero"`
	DaysUntilExpiry int       `json:"days_until_expiry"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
}

// DNSChallengeTarget describes a supported DNS-01 target exposed by another
// module through the narrow capability injected by the application.
type DNSChallengeTarget struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	ZoneName string `json:"zone_name"`
}

type DNSChallenge interface {
	Targets(context.Context) ([]DNSChallengeTarget, []string, error)
	Present(context.Context, DNSChallengeTarget, string, string) (func() error, error)
}

// CertificateUpdated is called after a renewed certificate is persisted so
// the host can refresh consumers such as an HTTPS listener.
type CertificateUpdated func(context.Context, int64) error

type Provider struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	DirectoryURL   string `json:"directory_url"`
	TermsOfService string `json:"terms_of_service"`
	Website        string `json:"website"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type Account struct {
	ID           int64  `json:"id"`
	ProviderID   int64  `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	AccountURL   string `json:"account_url"`
	Email        string `json:"email"`
	Status       string `json:"status"`
	KeyPem       string `json:"-"`
	Thumbprint   string `json:"thumbprint"`
	AgreedTerms  bool   `json:"agreed_terms"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}
