package server

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeCertificateDomains(t *testing.T) {
	domains, err := normalizeCertificateDomains("Example.COM\nwww.example.com,*.example.net\nexample.com.")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(domains, ",")
	want := "example.com,www.example.com,*.example.net"
	if got != want {
		t.Fatalf("domains = %q, want %q", got, want)
	}
	if _, err := normalizeCertificateDomains("localhost"); err == nil {
		t.Fatal("localhost should not be accepted as a public certificate name")
	}
}

func TestDNSRelativeName(t *testing.T) {
	name, fqdn, err := dnsRelativeName("api.example.com", "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if name != "_acme-challenge.api" || fqdn != "_acme-challenge.api.example.com" {
		t.Fatalf("name = %q, fqdn = %q", name, fqdn)
	}
	name, _, err = dnsRelativeName("*.example.com", "example.com")
	if err != nil || name != "_acme-challenge" {
		t.Fatalf("wildcard relative name = %q, err = %v", name, err)
	}
	if _, _, err := dnsRelativeName("example.net", "example.com"); err == nil {
		t.Fatal("domain outside selected zone should fail")
	}
}

func TestLocalDNS01PresentationAndCleanup(t *testing.T) {
	server := newDNSTestServer(t)
	zone, err := server.CreateDNSZone("example.com", "", true)
	if err != nil {
		t.Fatal(err)
	}
	cleanup, err := server.presentDNS01(fmt.Sprintf("local:%d", zone.ID), "www.example.com", "proof")
	if err != nil {
		t.Fatal(err)
	}
	records, err := server.GetDNSRecordsByZone(zone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Name != "_acme-challenge.www" ||
		records[0].Type != "TXT" || records[0].Value != "proof" {
		t.Fatalf("records = %#v", records)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	records, err = server.GetDNSRecordsByZone(zone.ID)
	if err != nil || len(records) != 0 {
		t.Fatalf("records after cleanup = %#v, err = %v", records, err)
	}
}

func TestACMENewCertificateTemplate(t *testing.T) {
	server := &Server{}
	response := httptest.NewRecorder()
	server.Render(response, "acme/certificate_new", H{
		"accounts":   []ACMEAccount{{ID: 1, ProviderName: "Test CA", Email: "ops@example.com", Status: "valid"}},
		"dnsTargets": []DNSChallengeTarget{{Value: "local:1", Label: "Local DNS — example.com"}},
		"hostname":   "example.com", "selectedAccountID": int64(0),
		"challengeType": "http-01", "dnsTarget": "",
	})
	if response.Code != 200 {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `type="`) ||
		!strings.Contains(response.Body.String(), `name="account_id"`) {
		t.Fatal("certificate form did not render expected typed inputs")
	}
}
