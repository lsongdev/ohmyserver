package dnspod

import (
	"context"
	"testing"

	types "github.com/lsongdev/ohmyserver/modules/dns/providers/types"
)

func TestProviderRejectsCancelledContextBeforeVendorRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := &DNSProvider{}
	checks := []struct {
		name string
		call func() error
	}{
		{"list zones", func() error { _, err := provider.ListZones(ctx); return err }},
		{"list records", func() error { _, err := provider.ListRecords(ctx, "zone"); return err }},
		{"add record", func() error { return provider.AddRecord(ctx, "zone", &types.Record{}) }},
		{"update record", func() error { return provider.UpdateRecord(ctx, "zone", &types.Record{}) }},
		{"delete record", func() error { return provider.DeleteRecord(ctx, "zone", "record") }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); err != context.Canceled {
				t.Fatalf("got error %v, want context.Canceled", err)
			}
		})
	}
}
