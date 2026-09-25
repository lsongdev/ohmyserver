package host

import (
	"context"
	"errors"
	"fmt"
	"testing"

	acmemodule "github.com/lsongdev/ohmyserver/modules/acme"
)

func TestLocalDNS01PresentationAndCleanup(t *testing.T) {
	server := newDNSTestServer(t)
	zone, err := server.DNSRepository().CreateZone(context.Background(), "example.com", "", true)
	if err != nil {
		t.Fatal(err)
	}
	name, _, err := acmemodule.DNSRelativeName("www.example.com", zone.Name)
	if err != nil {
		t.Fatal(err)
	}
	cleanup, err := server.dnsLocalService(context.Background()).PresentTXT(context.Background(), fmt.Sprintf("local:%d", zone.ID), name, "proof")
	if err != nil {
		t.Fatal(err)
	}
	records, err := server.DNSRepository().RecordsByZone(context.Background(), zone.ID)
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
	records, err = server.DNSRepository().RecordsByZone(context.Background(), zone.ID)
	if err != nil || len(records) != 0 {
		t.Fatalf("records after cleanup = %#v, err = %v", records, err)
	}
}

func TestLocalDNS01PresentationHonorsCancellation(t *testing.T) {
	server := newDNSTestServer(t)
	zone, err := server.DNSRepository().CreateZone(context.Background(), "example.com", "", true)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	name, _, err := acmemodule.DNSRelativeName("www.example.com", zone.Name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.dnsLocalService(ctx).PresentTXT(ctx, fmt.Sprintf("local:%d", zone.ID), name, "proof"); !errors.Is(err, context.Canceled) {
		t.Fatalf("presentation error = %v, want context canceled", err)
	}
	records, err := server.DNSRepository().RecordsByZone(context.Background(), zone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("cancelled presentation wrote records: %#v", records)
	}
}

func TestDNSChallengeTargetsHonorsCancellation(t *testing.T) {
	server := newDNSTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := server.dnsLocalService(ctx).ChallengeTargets(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("challenge targets error = %v, want context canceled", err)
	}
}
