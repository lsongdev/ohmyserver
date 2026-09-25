package acme

import "context"

// Repository is the persistence capability consumed by the ACME engine. Its
// SQLite implementation lives in the web package with the rest of the web
// service's storage implementation.
type Repository interface {
	Create(context.Context, string, string, string) (*StoredCertificate, error)
	Get(context.Context, int64) (*StoredCertificate, error)
	List(context.Context) ([]StoredCertificate, error)
	Update(context.Context, int64, int64, string, string) error
	UpdateStatus(context.Context, int64, string) error
	Delete(context.Context, int64) error
	ListACMECertificates(context.Context) ([]Certificate, error)
	GetProvider(context.Context, int64) (*Provider, error)
	ListProviders(context.Context) ([]Provider, error)
	CreateProvider(context.Context, string, string) (int64, error)
	DeleteProvider(context.Context, int64) error
	GetAccount(context.Context, int64) (*Account, error)
	CreateAccount(context.Context, int64, string, string, string, string, bool) (*Account, error)
	DeleteAccount(context.Context, int64) error
	ListAccounts(context.Context) ([]Account, error)
	ListOrders(context.Context) ([]Order, error)
	ListIssuances(context.Context) ([]Issuance, error)
	GetIssuance(context.Context, int64) (*Issuance, error)
	CreateIssuance(context.Context, *Issuance) error
	CreateRenewalIssuance(context.Context, *Issuance) error
	LatestSuccessfulIssuance(context.Context, int64) (*Issuance, error)
	CreateOrder(context.Context, int64, string, string) (*Order, error)
	UpdateIssuance(context.Context, int64, string, string, int64, error) error
}

// Service is the application capability required by the ACME web adapter.
// The app provides DNS challenge integration without sharing DNS storage.
type Service interface {
	ListACMECertificates(context.Context) ([]Certificate, error)
	ListProviders(context.Context) ([]Provider, error)
	ListAccounts(context.Context) ([]Account, error)
	ListOrders(context.Context) ([]Order, error)
	ListIssuances(context.Context) ([]Issuance, error)
	GetIssuance(context.Context, int64) (*Issuance, error)
	ListDNSChallengeTargets(context.Context) ([]DNSChallengeTarget, []string, error)
	StartIssuance(context.Context, IssuanceRequest) (*Issuance, error)
	RenewCertificate(context.Context, int64, RenewalRequest) (*Issuance, error)
	CreateProvider(context.Context, string, string) (int64, error)
	DeleteProvider(context.Context, int64) error
	RegisterAccount(context.Context, int64, string, bool) (*Account, error)
	DeleteAccount(context.Context, int64) error
	GenerateCSR(context.Context, string) (*Certificate, error)
	CertificateArchive(context.Context, int64) ([]byte, error)
	GetCertificateDetails(context.Context, int64) (*Certificate, error)
}

// RenewalRequest selects the account and challenge capability to reuse when
// replacing an existing certificate.
type RenewalRequest struct {
	AccountID     int64  `json:"account_id"`
	ChallengeType string `json:"challenge_type"`
	DNSTarget     string `json:"dns_target,omitempty"`
}
