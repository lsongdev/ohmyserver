package acme

import (
	"crypto/x509"
	"encoding/pem"
	"reflect"
	"testing"
)

func TestNormalizeDomains(t *testing.T) {
	got, err := NormalizeDomains("Example.COM\nwww.example.com,*.example.net\nexample.com.")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"example.com", "www.example.com", "*.example.net"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeDomains()=%v, want %v", got, want)
	}
	if _, err := NormalizeDomains("localhost"); err == nil {
		t.Fatal("NormalizeDomains accepted a single-label hostname")
	}
}

func TestGenerateCSRIncludesRequestedNames(t *testing.T) {
	material, err := GenerateCSR([]string{"example.test", "www.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(material.CSRPem))
	if block == nil {
		t.Fatal("GenerateCSR returned invalid PEM")
	}
	request, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := request.CheckSignature(); err != nil {
		t.Fatalf("CSR signature is invalid: %v", err)
	}
	if !reflect.DeepEqual(request.DNSNames, []string{"example.test", "www.example.test"}) {
		t.Fatalf("CSR DNSNames=%v", request.DNSNames)
	}
	if material.PrivateKeyPEM == "" {
		t.Fatal("GenerateCSR returned no private key")
	}
}

func TestDNSRelativeName(t *testing.T) {
	name, fqdn, err := DNSRelativeName("api.example.com", "example.com")
	if err != nil || name != "_acme-challenge.api" || fqdn != "_acme-challenge.api.example.com" {
		t.Fatalf("DNSRelativeName()=(%q,%q,%v)", name, fqdn, err)
	}
	name, fqdn, err = DNSRelativeName("*.example.com", "example.com")
	if err != nil || name != "_acme-challenge" || fqdn != "_acme-challenge.example.com" {
		t.Fatalf("wildcard DNSRelativeName()=(%q,%q,%v)", name, fqdn, err)
	}
	if _, _, err := DNSRelativeName("example.net", "example.com"); err == nil {
		t.Fatal("DNSRelativeName accepted an identifier outside the selected zone")
	}
}
