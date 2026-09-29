package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type apiTestService struct {
	providerName, directoryURL, email string
	agreed                            bool
	deletedProvider, deletedAccount   int64
	renewedID                         int64
}

func (*apiTestService) ListACMECertificates(context.Context) ([]Certificate, error) { return nil, nil }
func (*apiTestService) ListProviders(context.Context) ([]Provider, error)           { return nil, nil }
func (*apiTestService) ListAccounts(context.Context) ([]Account, error)             { return nil, nil }
func (*apiTestService) ListOrders(context.Context) ([]Order, error)                 { return nil, nil }
func (*apiTestService) ListIssuances(context.Context) ([]Issuance, error)           { return nil, nil }
func (*apiTestService) GetIssuance(context.Context, int64) (*Issuance, error)       { return nil, nil }
func (*apiTestService) ListDNSChallengeTargets(context.Context) ([]DNSChallengeTarget, []string, error) {
	return nil, nil, nil
}
func (*apiTestService) StartIssuance(context.Context, IssuanceRequest) (*Issuance, error) {
	return nil, nil
}
func (s *apiTestService) RenewCertificate(_ context.Context, id int64, _ RenewalRequest) (*Issuance, error) {
	s.renewedID = id
	return &Issuance{ID: 9, CertificateID: id, Status: "queued"}, nil
}
func (s *apiTestService) CreateProvider(_ context.Context, name, url string) (int64, error) {
	s.providerName, s.directoryURL = name, url
	return 7, nil
}
func (s *apiTestService) DeleteProvider(_ context.Context, id int64) error {
	s.deletedProvider = id
	return nil
}
func (s *apiTestService) RegisterAccount(_ context.Context, id int64, email string, agreed bool) (*Account, error) {
	s.email, s.agreed = email, agreed
	return &Account{ID: id, Email: email, KeyPem: "private"}, nil
}
func (s *apiTestService) DeleteAccount(_ context.Context, id int64) error {
	s.deletedAccount = id
	return nil
}
func (*apiTestService) GenerateCSR(context.Context, string) (*Certificate, error) { return nil, nil }
func (*apiTestService) CertificateArchive(context.Context, int64) ([]byte, error) {
	return []byte("archive"), nil
}
func (*apiTestService) GetCertificateDetails(context.Context, int64) (*Certificate, error) {
	return &Certificate{ID: 1, Domains: []string{"example.test"}, CSRPEM: "csr", CertPEM: "certificate"}, nil
}

func TestProviderCreateRequiresJSONAndReturnsCreated(t *testing.T) {
	service := &apiTestService{}
	module := New(service)
	r := httptest.NewRequest(http.MethodPost, "/providers", strings.NewReader(`{"name":"test","directory_url":"https://acme.test/directory"}`))
	r.RequestURI = "/providers"
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	module.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusCreated || service.providerName != "test" || service.directoryURL != "https://acme.test/directory" {
		t.Fatalf("status=%d service=%+v body=%s", w.Code, service, w.Body.String())
	}
	if w.Header().Get("Location") != "/providers" {
		t.Fatalf("unexpected Location: %q", w.Header().Get("Location"))
	}
}

func TestRegisterAccountNeverSerializesPrivateKey(t *testing.T) {
	service := &apiTestService{}
	module := New(service)
	r := httptest.NewRequest(http.MethodPost, "/providers/4/accounts", strings.NewReader(`{"email":"admin@example.test","agreed_terms":true}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	module.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || service.email != "admin@example.test" || !service.agreed {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "private") || !strings.Contains(w.Body.String(), `"email":"admin@example.test"`) {
		t.Fatalf("unexpected account response: %s", w.Body.String())
	}
}

func TestCertificateDetailsExposePEMButNotPrivateKey(t *testing.T) {
	module := New(&apiTestService{})
	response := httptest.NewRecorder()
	module.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/certificates/1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "certificate") || !strings.Contains(response.Body.String(), "csr") || strings.Contains(response.Body.String(), "private") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRenewCertificateValidatesAndDelegates(t *testing.T) {
	service := &apiTestService{}
	module := New(service)
	request := httptest.NewRequest(http.MethodPost, "/certificates/17/renew", strings.NewReader(`{"account_id":3,"challenge_type":"dns-01","dns_target":"local:example.test"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	module.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || service.renewedID != 17 || !strings.Contains(response.Body.String(), `"id":9`) {
		t.Fatalf("status=%d renewed=%d body=%s", response.Code, service.renewedID, response.Body.String())
	}

	badRequest := httptest.NewRequest(http.MethodPost, "/certificates/17/renew", strings.NewReader(`{"account_id":3,"challenge_type":"dns-01","unknown":true}`))
	badRequest.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	module.Handler().ServeHTTP(response, badRequest)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown renewal field status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestWriteRoutesRejectUnknownFieldsAndWrongContentType(t *testing.T) {
	module := New(&apiTestService{})
	for _, test := range []struct{ contentType, body string }{{"text/plain", `{"name":"x","directory_url":"https://example.test"}`}, {"application/json", `{"name":"x","directory_url":"https://example.test","private_key":"leak"}`}} {
		r := httptest.NewRequest(http.MethodPost, "/providers", strings.NewReader(test.body))
		r.Header.Set("Content-Type", test.contentType)
		w := httptest.NewRecorder()
		module.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("Content-Type=%q status=%d body=%s", test.contentType, w.Code, w.Body.String())
		}
	}
}

func TestHandlerServesManifestAndEmbeddedAssets(t *testing.T) {
	module := New(&apiTestService{})
	for _, path := range []string{"/manifest.json", "/assets/index.js", "/assets/pages/providers.js"} {
		response := httptest.NewRecorder()
		module.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Errorf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}

func TestHandlerServesPageAndMountedAPIAssets(t *testing.T) {
	module := New(&apiTestService{})
	page := httptest.NewRecorder()
	module.Handler().ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `name="api-base" content="."`) {
		t.Fatalf("GET /: status=%d body=%q", page.Code, page.Body.String())
	}
	for _, path := range []string{"/manifest.json", "/assets/index.js"} {
		response := httptest.NewRecorder()
		module.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Errorf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}
