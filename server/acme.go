package server

import (
	"archive/zip"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lsongdev/acme-go/acme"
)

// ACMEProvider represents an ACME certificate provider
type ACMEProvider struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`          // e.g., "Let's Encrypt"
	DirectoryURL string `json:"directory_url"` // ACME directory URL
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// ACMEAccount represents an ACME account
type ACMEAccount struct {
	ID           int64  `json:"id"`
	ProviderID   int64  `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	AccountURL   string `json:"account_url"`
	Email        string `json:"email"`
	Status       string `json:"status"` // valid, deactivated, revoked
	KeyPem       string `json:"-"`      // Private key PEM (do not expose in JSON)
	Thumbprint   string `json:"thumbprint"`
	AgreedTerms  bool   `json:"agreed_terms"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// ACMEOrder represents an ACME order
type ACMEOrder struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"account_id"`
	OrderURL  string `json:"order_url"`
	Domains   string `json:"domains"` // comma-separated domains
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Identifier represents a domain identifier
type Identifier struct {
	Type  string `json:"type"`  // dns
	Value string `json:"value"` // domain name
}

// ACMEAuthorization represents an ACME authorization
type ACMEAuthorization struct {
	ID         int64           `json:"id"`
	OrderID    int64           `json:"order_id"`
	AuthzURL   string          `json:"authz_url"`
	Status     string          `json:"status"` // pending, valid, invalid
	Identifier Identifier      `json:"identifier"`
	Expires    time.Time       `json:"expires"`
	Challenges []ACMEChallenge `json:"challenges"`
	CreatedAt  string          `json:"created_at"`
}

// ACMEChallenge represents an ACME challenge
type ACMEChallenge struct {
	ID              int64     `json:"id"`
	AuthorizationID int64     `json:"authorization_id"`
	ChallengeURL    string    `json:"challenge_url"`
	Type            string    `json:"type"`   // dns-01, http-01
	Status          string    `json:"status"` // pending, processing, valid, invalid
	Token           string    `json:"token"`
	KeyAuth         string    `json:"key_auth"`
	Validated       time.Time `json:"validated"`
	Error           string    `json:"error"`
	CreatedAt       string    `json:"created_at"`
}

// Certificate represents a certificate (CSR + issued certificate)
type Certificate struct {
	ID            int64  `json:"id"`
	OrderID       int64  `json:"order_id"` // Associated ACME order ID
	Domains       string `json:"domains"`  // Comma-separated domains (for display and order creation)
	CSRPem        string `json:"csr_pem"`  // CSR in PEM format
	PrivateKeyPem string `json:"-"`        // Private key (do not expose)
	CertPem       string `json:"cert_pem"` // Issued certificate PEM
	Status        string `json:"status"`   // pending, processing, active, expired, revoked
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// IsExpired checks if the certificate is expired
func (c *Certificate) IsExpired() bool {
	if c.CertPem == "" {
		return false
	}
	block, _ := pem.Decode([]byte(c.CertPem))
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	return cert.NotAfter.Before(time.Now())
}

// DaysUntilExpiry returns the number of days until the certificate expires
func (c *Certificate) DaysUntilExpiry() int {
	if c.CertPem == "" {
		return -1
	}
	block, _ := pem.Decode([]byte(c.CertPem))
	if block == nil {
		return -1
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return -1
	}
	duration := cert.NotAfter.Sub(time.Now())
	return int(duration.Hours() / 24)
}

// GetCommonName extracts the common name from CSR or certificate
func (c *Certificate) GetCommonName() string {
	// Try certificate first
	if c.CertPem != "" {
		block, _ := pem.Decode([]byte(c.CertPem))
		if block != nil {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil && cert.Subject.CommonName != "" {
				return cert.Subject.CommonName
			}
		}
	}
	// Fall back to CSR
	if c.CSRPem != "" {
		block, _ := pem.Decode([]byte(c.CSRPem))
		if block != nil {
			csr, err := x509.ParseCertificateRequest(block.Bytes)
			if err == nil && csr.Subject.CommonName != "" {
				return csr.Subject.CommonName
			}
		}
	}
	return ""
}

// GetDNSNames extracts DNS names from CSR or certificate
func (c *Certificate) GetDNSNames() []string {
	// Try certificate first
	if c.CertPem != "" {
		block, _ := pem.Decode([]byte(c.CertPem))
		if block != nil {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil && len(cert.DNSNames) > 0 {
				return cert.DNSNames
			}
		}
	}
	// Fall back to CSR
	if c.CSRPem != "" {
		block, _ := pem.Decode([]byte(c.CSRPem))
		if block != nil {
			csr, err := x509.ParseCertificateRequest(block.Bytes)
			if err == nil && len(csr.DNSNames) > 0 {
				return csr.DNSNames
			}
		}
	}
	return nil
}

func (s *Server) CertificatesView(w http.ResponseWriter, r *http.Request) {
	certs, err := s.GetAllCertificates()
	if err != nil {
		s.Error(w, err)
		return
	}

	s.Render(w, "acme/certificates", H{
		"certificates": certs,
	})
}

func (s *Server) CertificateDetailView(w http.ResponseWriter, r *http.Request) {
	certIDStr := r.PathValue("id")
	certID, err := strconv.ParseInt(certIDStr, 10, 64)
	if err != nil {
		s.Error(w, &Error{Message: "Invalid certificate ID"})
		return
	}

	cert, err := s.GetCertificate(certID)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get certificate: %w", err))
		return
	}

	// Parse domains for display
	var domainList []string
	if cert.Domains != "" {
		domainList = strings.Split(cert.Domains, ",")
		for i, d := range domainList {
			domainList[i] = strings.TrimSpace(d)
		}
	}

	s.Render(w, "acme/certificate_detail", H{
		"certificate": cert,
		"domains":     domainList,
	})
}

func (s *Server) CertificateDownloadHandler(w http.ResponseWriter, r *http.Request) {
	certIDStr := r.PathValue("id")
	certID, err := strconv.ParseInt(certIDStr, 10, 64)
	if err != nil {
		s.Error(w, &Error{Message: "Invalid certificate ID"})
		return
	}

	cert, err := s.GetCertificate(certID)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get certificate: %w", err))
		return
	}

	domain := strings.Split(cert.Domains, ",")[0]
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	files := map[string]string{
		fmt.Sprintf("%s.cert.pem", domain): cert.CertPem,
		fmt.Sprintf("%s.key.pem", domain):  cert.PrivateKeyPem,
		fmt.Sprintf("%s.csr.pem", domain):  cert.CSRPem,
	}

	for name, content := range files {
		if content == "" {
			continue
		}
		f, err := zw.Create(name)
		if err != nil {
			s.Error(w, fmt.Errorf("failed to create zip entry: %w", err))
			return
		}
		if _, err := f.Write([]byte(content)); err != nil {
			s.Error(w, fmt.Errorf("failed to write zip entry: %w", err))
			return
		}
	}

	if err := zw.Close(); err != nil {
		s.Error(w, fmt.Errorf("failed to close zip: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"certificate-%s.zip\"", domain))
	w.Write(buf.Bytes())
}

func (s *Server) CreateCSRView(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodGet {
		s.Render(w, "acme/create_csr", H{})
		return
	}

	// Parse form
	err := r.ParseForm()
	if err != nil {
		s.Error(w, fmt.Errorf("failed to parse form: %w", err))
		return
	}

	commonName := r.FormValue("common_name")
	sanDomainsStr := r.FormValue("san_domains")
	keyType := r.FormValue("key_type")
	keySizeStr := r.FormValue("key_size")
	organization := r.FormValue("organization")
	organizationalUnit := r.FormValue("organizational_unit")
	country := r.FormValue("country")
	state := r.FormValue("state")
	locality := r.FormValue("locality")

	// Validate required fields
	if commonName == "" {
		s.Error(w, &Error{Message: "Common Name is required"})
		return
	}

	if keyType == "" {
		keyType = "RSA"
	}

	// Parse key size
	keySize := 2048
	if keySizeStr != "" {
		keySize, err = strconv.Atoi(keySizeStr)
		if err != nil {
			s.Error(w, &Error{Message: "Invalid key size"})
			return
		}
	}

	// Generate private key
	var privateKey interface{}
	switch keyType {
	case "RSA":
		if keySize != 2048 && keySize != 3072 && keySize != 4096 {
			keySize = 2048
		}
		privateKey, err = rsa.GenerateKey(rand.Reader, keySize)
		if err != nil {
			s.Error(w, fmt.Errorf("failed to generate RSA key: %w", err))
			return
		}
	case "ECDSA":
		var curve elliptic.Curve
		switch keySize {
		case 256:
			curve = elliptic.P256()
		case 384:
			curve = elliptic.P384()
		case 521:
			curve = elliptic.P521()
		default:
			curve = elliptic.P256()
			keySize = 256
		}
		privateKey, err = ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			s.Error(w, fmt.Errorf("failed to generate ECDSA key: %w", err))
			return
		}
	default:
		s.Error(w, &Error{Message: "Invalid key type. Must be RSA or ECDSA"})
		return
	}

	// Encode private key to PEM
	var privateKeyPem string
	switch key := privateKey.(type) {
	case *rsa.PrivateKey:
		privateKeyPem = string(pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		}))
	case *ecdsa.PrivateKey:
		keyBytes, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			s.Error(w, fmt.Errorf("failed to marshal EC private key: %w", err))
			return
		}
		privateKeyPem = string(pem.EncodeToMemory(&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: keyBytes,
		}))
	}

	// Build certificate subject
	subject := pkix.Name{
		CommonName:         commonName,
		Organization:       splitCommaSeparated(organization),
		OrganizationalUnit: splitCommaSeparated(organizationalUnit),
		Country:            splitCommaSeparated(country),
		Province:           splitCommaSeparated(state),
		Locality:           splitCommaSeparated(locality),
	}

	// Create CSR template
	csrTemplate := &x509.CertificateRequest{
		Subject: subject,
	}

	// Add SANs if provided
	if sanDomainsStr != "" {
		sanDomains := splitNewlineSeparated(sanDomainsStr)
		for _, domain := range sanDomains {
			domain = strings.TrimSpace(domain)
			if domain != "" {
				csrTemplate.DNSNames = append(csrTemplate.DNSNames, domain)
			}
		}
	}

	// Generate CSR
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, privateKey)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to create CSR: %w", err))
		return
	}

	// Encode CSR to PEM
	csrPem := string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	}))

	// Combine common name and SANs into domains list
	allDomains := append([]string{commonName}, csrTemplate.DNSNames...)
	domains := strings.Join(allDomains, ",")

	// Save certificate (CSR) to database
	_, err = s.CreateCertificate(
		domains,
		csrPem,
		privateKeyPem,
	)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to create certificate: %w", err))
		return
	}

	// Redirect to certificates list
	http.Redirect(w, r, "/acme/certificates", http.StatusSeeOther)
}

// Helper function to split comma-separated values
func splitCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// Helper function to split newline-separated values
func splitNewlineSeparated(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

// ACMEProvidersView handles the ACME providers list page
func (s *Server) ACMEProvidersView(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodPost {
		s.ACMEProviderCreate(w, r)
		return
	}

	providers, err := s.acme_get_providers()
	if err != nil {
		s.Error(w, err)
		return
	}

	s.Render(w, "acme/providers", H{
		"providers": providers,
	})
}

// ACMEProviderNewView handles the new provider page
func (s *Server) ACMEProviderNewView(w http.ResponseWriter, r *http.Request) {
	s.Render(w, "acme/provider_new", H{})
}

// ACMEProviderCreate handles creating a new provider
func (s *Server) ACMEProviderCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		s.Error(w, err)
		return
	}

	name := r.FormValue("name")
	directoryURL := r.FormValue("directory_url")

	if name == "" || directoryURL == "" {
		s.Error(w, &Error{Message: "Name and directory URL are required"})
		return
	}
	_, err = s.acme_create_provider(name, directoryURL)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to create ACME provider: %w", err))
		return
	}
	http.Redirect(w, r, "/acme/providers", http.StatusSeeOther)
}

// ACMEProviderDeleteHandler handles deleting a provider
func (s *Server) ACMEProviderDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	err = s.DeleteACMEProvider(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/acme/providers", http.StatusSeeOther)
}

// ACMEAccountsView handles the ACME accounts list page for a provider
func (s *Server) ACMEAccountsView(w http.ResponseWriter, r *http.Request) {

	accounts, err := s.GetACMEAccounts()
	if err != nil {
		s.Error(w, err)
		return
	}

	s.Render(w, "acme/accounts", H{
		"accounts": accounts,
	})
}

// ACMEAccountNewView handles the new account page
func (s *Server) ACMERegisterView(w http.ResponseWriter, r *http.Request) {
	providerIdStr := r.PathValue("id")

	if providerIdStr == "" {
		s.Error(w, &Error{Message: "Provider ID is required"})
		return
	}

	var website, newAccount, termsOfService string

	providerId, err := strconv.ParseInt(providerIdStr, 10, 64)
	if err != nil {
		s.Error(w, fmt.Errorf("%w", err))
		return
	}

	provider, err := s.GetACMEProvider(providerId)
	if err != nil {
		s.Error(w, fmt.Errorf("%w", err))
		return
	}

	client, err := acme.NewClient(&acme.Config{
		DirectoryURL: provider.DirectoryURL,
	})
	directory, err := client.GetDirectory()
	if err != nil {
		s.Error(w, fmt.Errorf("%w", err))
		return
	}

	website = directory.Meta.Website
	newAccount = directory.NewAccount
	termsOfService = directory.Meta.TermsOfService

	if r.Method == http.MethodPost {
		email := r.FormValue("email")
		agreedTerms := r.FormValue("terms") == "on"

		if email == "" {
			s.Error(w, &Error{Message: "Email is required"})
			return
		}
		err = client.GenerateKey()
		if err != nil {
			s.Error(w, fmt.Errorf("failed to generate key: %w", err))
			return
		}
		keyPem, err := client.ExportKey()
		if err != nil {
			s.Error(w, fmt.Errorf("failed to export key: %w", err))
			return
		}
		thumbprint, err := client.GetThumbprint()
		if err != nil {
			s.Error(w, fmt.Errorf("failed to get thumbprint: %w", err))
			return
		}
		accountUrl, resp, err := client.Register(&acme.AccountRequest{
			TermsOfServiceAgreed: agreedTerms,
			Contact: []string{
				fmt.Sprintf("mailto:%s", email),
			},
		})
		if err != nil {
			s.Error(w, fmt.Errorf("failed to register ACME account: %w", err))
			return
		}
		log.Println(accountUrl, resp)
		_, err = s.CreateACMEAccount(providerId, accountUrl, email, keyPem, thumbprint, agreedTerms)
		http.Redirect(w, r, "/acme/accounts", http.StatusSeeOther)
		return
	}

	templateData := H{
		"provider":       provider,
		"newAccount":     newAccount,
		"website":        website,
		"termsOfService": termsOfService,
	}
	s.Render(w, "acme/register", templateData)
}

// ACMEAccountDeleteHandler handles deleting an account
func (s *Server) ACMEAccountDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	err = s.DeleteACMEAccount(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/acme/providers", http.StatusSeeOther)
}

// ACMEOrdersView handles the ACME orders list page for an account
func (s *Server) ACMEOrdersView(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodPost {
		s.ACMEOrderCreate(w, r)
		return
	}

	orders, err := s.GetACMEOrders()
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get ACME orders: %w", err))
		return
	}

	accounts, err := s.GetACMEAccounts()
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get ACME accounts: %w", err))
		return
	}

	// Build account lookup map
	accountMap := make(map[int64]*ACMEAccount)
	for i := range accounts {
		accountMap[accounts[i].ID] = &accounts[i]
	}

	s.Render(w, "acme/orders", H{
		"orders":   orders,
		"accounts": accountMap,
	})
}

// ACMEOrderNewView handles creating a new order
func (s *Server) ACMEOrderNewView(w http.ResponseWriter, r *http.Request) {
	// Get all active accounts
	accounts, err := s.GetACMEAccounts()
	if err != nil {
		s.Error(w, err)
		return
	}

	certs, err := s.GetAllCertificates()
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get certificates: %w", err))
		return
	}
	s.Render(w, "acme/order_new", H{
		"accounts":     accounts,
		"certificates": certs,
	})
}

// ACMEOrderCreate handles creating a new order
func (s *Server) ACMEOrderCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		s.Error(w, err)
		return
	}

	accountIDStr := r.FormValue("account_id")
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		s.Error(w, &Error{Message: "Invalid account ID"})
		return
	}

	domainsStr := r.FormValue("domains")
	if domainsStr == "" {
		s.Error(w, &Error{Message: "At least one domain is required"})
		return
	}

	// Parse domains (one per line)
	domains := strings.Split(strings.TrimSpace(domainsStr), "\n")
	var identifiers []acme.Identifier
	for _, domain := range domains {
		domain = strings.TrimSpace(domain)
		if domain != "" {
			identifiers = append(identifiers, acme.Identifier{
				Type:  "dns",
				Value: domain,
			})
		}
	}

	if len(identifiers) == 0 {
		s.Error(w, &Error{Message: "At least one valid domain is required"})
		return
	}

	// Get account
	account, err := s.GetACMEAccount(accountID)
	if err != nil {
		s.Error(w, err)
		return
	}

	// Get provider
	provider, err := s.GetACMEProvider(account.ProviderID)
	if err != nil {
		s.Error(w, err)
		return
	}

	// Import key
	client, err := acme.NewClient(&acme.Config{
		DirectoryURL: provider.DirectoryURL,
		AccountURL:   account.AccountURL,
		AccountKey:   account.KeyPem,
	})

	// Get directory
	directory, err := client.GetDirectory()
	if err != nil {
		s.Error(w, err)
		return
	}
	client.Directory = directory

	// Create order
	orderReq := &acme.OrderRequest{
		Identifiers: identifiers,
	}

	orderURL, orderResp, err := client.CreateOrder(orderReq)
	if err != nil {
		s.Error(w, err)
		return
	}

	// Build comma-separated domains string
	var domainList []string
	for _, id := range orderResp.Identifiers {
		domainList = append(domainList, id.Value)
	}
	domainsCSV := strings.Join(domainList, ",")

	// Save to database
	acmeOrder, err := s.CreateACMEOrder(
		accountID,
		orderURL,
		domainsCSV,
	)
	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/acme/orders/%d", acmeOrder.ID), http.StatusSeeOther)
}

// ACMEOrderDetailHandler handles the order detail page
func (s *Server) ACMEOrderDetailHandler(w http.ResponseWriter, r *http.Request) {
	orderIDStr := r.PathValue("id")
	orderID, err := strconv.ParseInt(orderIDStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	order, err := s.GetACMEOrder(orderID)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get ACME order: %w", err))
		return
	}

	account, err := s.GetACMEAccount(order.AccountID)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get ACME account: %w", err))
		return
	}

	provider, err := s.GetACMEProvider(account.ProviderID)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get ACME provider: %w", err))
		return
	}

	client, err := acme.NewClient(&acme.Config{
		DirectoryURL: provider.DirectoryURL,
		AccountURL:   account.AccountURL,
		AccountKey:   account.KeyPem,
	})
	if err != nil {
		s.Error(w, fmt.Errorf("failed to create ACME client: %w", err))
		return
	}

	// Import the account key so we can compute thumbprints
	err = client.ImportKey(account.KeyPem)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to import account key: %w", err))
		return
	}

	resp, err := client.GetOrder(order.OrderURL)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get order from ACME server: %w", err))
		return
	}

	var authorizations []acme.Authorization
	// Get authorizations
	for _, authzURL := range resp.Authorizations {
		authz, err := client.GetAuthorization(authzURL)
		if err != nil {
			continue
		}
		authorizations = append(authorizations, *authz)
	}

	certs, err := s.GetAllCertificates()
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get certificates: %w", err))
		return
	}

	// Compute key authorizations for challenges
	type ChallengeInfo struct {
		acme.Challenge
		FileName  string // For HTTP-01
		DNSRecord string // For DNS-01
		Content   string
	}

	type AuthorizationInfo struct {
		acme.Authorization
		Challenges []ChallengeInfo
	}

	var authInfos []AuthorizationInfo
	for _, authz := range authorizations {
		authInfo := AuthorizationInfo{
			Authorization: authz,
		}
		for _, challenge := range authz.Challenges {
			challengeInfo := ChallengeInfo{
				Challenge: challenge,
			}
			// Compute key authorization based on challenge type
			switch challenge.Type {
			case "http-01":
				if file, err := client.HTTP01KeyAuthorization(challenge.Token); err == nil {
					challengeInfo.Content = string(file.Content)
					challengeInfo.FileName = file.FileName
				}
			case "dns-01":
				if record, err := client.DNS01KeyAuthorization(challenge.Token); err == nil {
					challengeInfo.Content = record.Content
					challengeInfo.DNSRecord = record.Name
				}
			}
			authInfo.Challenges = append(authInfo.Challenges, challengeInfo)
		}
		authInfos = append(authInfos, authInfo)
	}

	// Parse domains into slice for template
	var domainList []string
	if order.Domains != "" {
		domainList = strings.Split(order.Domains, ",")
		for i, d := range domainList {
			domainList[i] = strings.TrimSpace(d)
		}
	}

	s.Render(w, "acme/order_detail", H{
		"order":          order,
		"details":        resp,
		"authorizations": authInfos,
		"certificates":   certs,
		"domains":        domainList,
	})
}

// ACMEChallengeComplete handles completing a challenge
func (s *Server) ACMEChallengeComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		s.Error(w, fmt.Errorf("failed to parse form: %w", err))
		return
	}

	challengeURL := r.FormValue("challenge_url")
	if challengeURL == "" {
		s.Error(w, &Error{Message: "Challenge URL is required"})
		return
	}

	// Get order to find account
	orderIDStr := r.FormValue("order_id")
	orderID, err := strconv.ParseInt(orderIDStr, 10, 64)
	if err != nil {
		s.Error(w, &Error{Message: "Invalid order ID"})
		return
	}

	order, err := s.GetACMEOrder(orderID)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get order: %w", err))
		return
	}

	// Get account and provider
	account, err := s.GetACMEAccount(order.AccountID)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get account: %w", err))
		return
	}

	provider, err := s.GetACMEProvider(account.ProviderID)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get provider: %w", err))
		return
	}

	// Create ACME client
	client, err := acme.NewClient(&acme.Config{
		DirectoryURL: provider.DirectoryURL,
		AccountURL:   account.AccountURL,
		AccountKey:   account.KeyPem,
	})

	err = client.ImportKey(account.KeyPem)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to import account key: %w", err))
		return
	}

	client.Directory, err = client.GetDirectory()
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get directory: %w", err))
		return
	}

	// Complete the challenge
	err = client.CompleteChallenge(challengeURL)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to complete challenge: %w", err))
		return
	}

	// log.Printf("Challenge completed successfully: %s", challengeURL)

	// Redirect back to order detail
	http.Redirect(w, r, fmt.Sprintf("/acme/orders/%d", orderID), http.StatusSeeOther)
}

// ACMEOrderFinalize handles finalizing an order with CSR
func (s *Server) ACMEOrderFinalize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		s.Error(w, fmt.Errorf("failed to parse form: %w", err))
		return
	}

	orderIDStr := r.PathValue("id")
	orderID, err := strconv.ParseInt(orderIDStr, 10, 64)
	if err != nil {
		s.Error(w, &Error{Message: "Invalid order ID"})
		return
	}

	certIDStr := r.FormValue("cert_id")
	if certIDStr == "" {
		s.Error(w, &Error{Message: "Certificate is required"})
		return
	}
	certID, err := strconv.ParseInt(certIDStr, 10, 64)
	if err != nil {
		s.Error(w, &Error{Message: "Invalid certificate ID"})
		return
	}

	// Get order
	order, err := s.GetACMEOrder(orderID)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get order: %w", err))
		return
	}

	// Get certificate
	cert, err := s.GetCertificate(certID)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get certificate: %w", err))
		return
	}

	// Get account and provider
	account, err := s.GetACMEAccount(order.AccountID)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get account: %w", err))
		return
	}

	provider, err := s.GetACMEProvider(account.ProviderID)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get provider: %w", err))
		return
	}

	// Create ACME client
	client, err := acme.NewClient(&acme.Config{
		DirectoryURL: provider.DirectoryURL,
		AccountURL:   account.AccountURL,
		AccountKey:   account.KeyPem,
	})
	if err != nil {
		s.Error(w, fmt.Errorf("failed to create ACME client: %w", err))
		return
	}

	resp, err := client.GetOrder(order.OrderURL)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to get order: %w", err))
		return
	}

	// Parse CSR PEM to get DER encoding
	block, _ := pem.Decode([]byte(cert.CSRPem))
	if block == nil {
		s.Error(w, &Error{Message: "Failed to decode CSR PEM"})
		return
	}

	// Finalize order with DER-encoded CSR
	_, err = client.FinalizeOrder(resp.Finalize, block.Bytes)
	if err != nil {
		s.Error(w, fmt.Errorf("failed to finalize order: %w", err))
		return
	}

	log.Printf("Order finalized, waiting for certificate issuance: order_id=%d, cert_id=%d", orderID, certID)

	// Update certificate status to processing
	err = s.UpdateCertificateStatus(certID, "processing")
	if err != nil {
		log.Printf("Warning: failed to update certificate status: %v", err)
	}

	// Poll for certificate issuance (max 5 minutes, check every 5 seconds)
	var certPEM string
	maxAttempts := 60 // 5 minutes with 5-second intervals
	for attempt := 0; attempt < maxAttempts; attempt++ {
		time.Sleep(5 * time.Second)

		// Check order status
		orderResp, err := client.GetOrder(order.OrderURL)
		if err != nil {
			log.Printf("Poll attempt %d: failed to get order: %v", attempt+1, err)
			continue
		}

		log.Printf("Poll attempt %d: order status = %s", attempt+1, orderResp.Status)

		if orderResp.Status == acme.OrderStatusValid {
			// Certificate is ready
			certURL := orderResp.Certificate
			if certURL == "" {
				log.Printf("Order is valid but no certificate URL")
				break
			}

			// Download certificate
			certPEM, err = client.GetCertificatePEM(certURL)
			if err != nil {
				log.Printf("Failed to download certificate: %v", err)
				break
			}

			log.Printf("Certificate downloaded successfully")
			break
		} else if orderResp.Status == acme.OrderStatusInvalid {
			log.Printf("Order is invalid: %s", orderResp.Status)
			break
		}
		// Continue polling for "processing" status
	}

	if certPEM == "" {
		s.Error(w, &Error{Message: "Failed to retrieve certificate after finalization. Please check order status manually."})
		return
	}

	// Parse certificate to get metadata
	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		s.Error(w, &Error{Message: "Failed to parse certificate PEM"})
		return
	}

	_, err = x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		log.Printf("Warning: failed to parse certificate: %v", err)
	}

	// Update certificate record with issued certificate
	err = s.UpdateCertificate(certID, orderID, certPEM, "active")
	if err != nil {
		s.Error(w, fmt.Errorf("failed to save certificate: %w", err))
		return
	}

	log.Printf("Certificate stored successfully: cert_id=%d", certID)
	if err := s.applyYabber(); err != nil {
		log.Printf("Warning: failed to reload Yabber after certificate update: %v", err)
	}

	// Redirect back to order detail
	http.Redirect(w, r, fmt.Sprintf("/acme/orders/%d", orderID), http.StatusSeeOther)
}
