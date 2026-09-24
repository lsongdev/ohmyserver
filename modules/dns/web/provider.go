package web

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lsongdev/ohmyserver/modules/dns/providers"
	"github.com/lsongdev/ohmyserver/modules/dns/providers/types"
)

// NewProviderClient resolves a persisted connection into its vendor client.
// It is shared by DNS's HTTP service and host-level ACME challenge adapter.
func NewProviderClient(ctx context.Context, repo *Repository, id int64) (types.IDNSProvider, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	connection, err := repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get DNS provider connection: %w", err)
	}
	var config types.Config
	if err := json.Unmarshal([]byte(connection.Config), &config); err != nil {
		return nil, fmt.Errorf("decode DNS provider configuration: %w", err)
	}
	return providers.NewDNSProviderByName(connection.Type, config)
}

func (m *Module) providerZones(ctx context.Context, id int64) ([]ProviderZone, error) {
	client, err := NewProviderClient(ctx, m.repo, id)
	if err != nil {
		return nil, err
	}
	zones, err := client.ListZones(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]ProviderZone, 0, len(zones))
	for _, zone := range zones {
		items = append(items, ProviderZone{ID: zone.ID, Name: zone.Name, CreatedAt: zone.CreatedAt})
	}
	return items, nil
}

func (m *Module) providerRecords(ctx context.Context, id int64, zoneID string) ([]ProviderRecord, error) {
	client, err := NewProviderClient(ctx, m.repo, id)
	if err != nil {
		return nil, err
	}
	records, err := client.ListRecords(ctx, zoneID)
	if err != nil {
		return nil, err
	}
	items := make([]ProviderRecord, 0, len(records))
	for _, record := range records {
		items = append(items, ProviderRecord{ID: record.ID, Type: record.Type, Name: record.Name, Value: record.Value, TTL: record.TTL, Comment: record.Comment, UpdatedAt: record.UpdateAt})
	}
	return items, nil
}

func (m *Module) saveProviderRecord(ctx context.Context, id int64, zoneID string, item ProviderRecord) (ProviderRecord, error) {
	if err := ctx.Err(); err != nil {
		return item, err
	}
	if strings.TrimSpace(item.Type) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Value) == "" {
		return item, fmt.Errorf("record type, name, and value are required")
	}
	if item.TTL <= 0 {
		item.TTL = 300
	}
	item.Type, item.Name = strings.ToUpper(strings.TrimSpace(item.Type)), strings.TrimSpace(item.Name)
	item.Value, item.Comment = strings.TrimSpace(item.Value), strings.TrimSpace(item.Comment)
	record := &types.Record{ID: item.ID, Type: item.Type, Name: item.Name, Value: item.Value, TTL: item.TTL, Comment: item.Comment}
	client, err := NewProviderClient(ctx, m.repo, id)
	if err != nil {
		return item, err
	}
	if item.ID == "" {
		err = client.AddRecord(ctx, zoneID, record)
	} else {
		err = client.UpdateRecord(ctx, zoneID, record)
	}
	if err != nil {
		return item, err
	}
	item.ID, item.UpdatedAt = record.ID, record.UpdateAt
	return item, nil
}

func (m *Module) deleteProviderRecord(ctx context.Context, id int64, zoneID, recordID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(recordID) == "" {
		return fmt.Errorf("record id is required")
	}
	client, err := NewProviderClient(ctx, m.repo, id)
	if err != nil {
		return err
	}
	return client.DeleteRecord(ctx, zoneID, recordID)
}
