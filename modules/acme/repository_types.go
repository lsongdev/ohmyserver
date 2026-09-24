package acme

// StoredCertificate is a persistence record. Private key material must never
// be serialized into an API response.
type StoredCertificate struct {
	ID            int64
	OrderID       int64
	Domains       string
	CSRPem        string `json:"-"`
	PrivateKeyPEM string `json:"-"`
	CertPEM       string `json:"-"`
	Status        string
	CreatedAt     string
	UpdatedAt     string
}

type CertificateSummary struct {
	ID        int64
	Domains   string
	CertPEM   string
	Status    string
	CreatedAt string
	UpdatedAt string
}

type TLSMaterial struct {
	CertPEM       string `json:"-"`
	PrivateKeyPEM string `json:"-"`
	Status        string
}
