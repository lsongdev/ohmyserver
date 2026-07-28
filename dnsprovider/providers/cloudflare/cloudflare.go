package cloudflare

import (
	"context"

	cloudflareapi "github.com/cloudflare/cloudflare-go"
	"github.com/lsongdev/ohmyserver/dnsprovider/types"
)

type DNSProvider struct {
	client *cloudflareapi.API
}

func NewDNSProvider(config types.Config) (*DNSProvider, error) {
	var (
		client *cloudflareapi.API
		err    error
	)
	if token := config["token"]; token != "" {
		client, err = cloudflareapi.NewWithAPIToken(token)
	} else {
		client, err = cloudflareapi.New(config["apiKey"], config["email"])
	}
	if err != nil {
		return nil, err
	}
	return &DNSProvider{client: client}, nil
}

func (p *DNSProvider) ListZones() ([]types.Zone, error) {
	items, err := p.client.ListZones(context.Background())
	if err != nil {
		return nil, err
	}
	zones := make([]types.Zone, 0, len(items))
	for _, item := range items {
		zones = append(zones, types.Zone{
			ID: item.ID, Name: item.Name, CreatedAt: item.CreatedOn.Format("2006-01-02 15:04:05"),
		})
	}
	return zones, nil
}

func (p *DNSProvider) ListRecords(zoneID string) ([]types.Record, error) {
	items, _, err := p.client.ListDNSRecords(
		context.Background(),
		cloudflareapi.ZoneIdentifier(zoneID),
		cloudflareapi.ListDNSRecordsParams{},
	)
	if err != nil {
		return nil, err
	}
	records := make([]types.Record, 0, len(items))
	for _, item := range items {
		records = append(records, types.Record{
			ID: item.ID, Name: item.Name, Type: item.Type, Value: item.Content,
			TTL: item.TTL, UpdateAt: item.ModifiedOn.Format("2006-01-02 15:04:05"),
		})
	}
	return records, nil
}

func (p *DNSProvider) AddRecord(zoneID string, record *types.Record) error {
	_, err := p.client.CreateDNSRecord(
		context.Background(),
		cloudflareapi.ZoneIdentifier(zoneID),
		cloudflareapi.CreateDNSRecordParams{
			Type: record.Type, Name: record.Name, Content: record.Value,
			TTL: record.TTL, Comment: record.Comment,
		},
	)
	return err
}

func (p *DNSProvider) UpdateRecord(zoneID string, record *types.Record) error {
	_, err := p.client.UpdateDNSRecord(
		context.Background(),
		cloudflareapi.ZoneIdentifier(zoneID),
		cloudflareapi.UpdateDNSRecordParams{
			ID: record.ID, Type: record.Type, Name: record.Name,
			Content: record.Value, TTL: record.TTL, Comment: &record.Comment,
		},
	)
	return err
}

func (p *DNSProvider) DeleteRecord(zoneID, recordID string) error {
	return p.client.DeleteDNSRecord(context.Background(), cloudflareapi.ZoneIdentifier(zoneID), recordID)
}
