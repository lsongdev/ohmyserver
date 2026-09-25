package dnspod

import (
	"context"
	"strconv"

	types "github.com/lsongdev/ohmyserver/modules/dns/providers/types"
	dnspodapi "github.com/nrdcg/dnspod-go"
)

type DNSProvider struct {
	client *dnspodapi.Client
}

func NewDNSProvider(config types.Config) (*DNSProvider, error) {
	return &DNSProvider{client: dnspodapi.NewClient(dnspodapi.CommonParams{
		LoginToken: config["token"],
		Format:     "json",
	})}, nil
}

func (p *DNSProvider) ListZones(ctx context.Context) ([]types.Zone, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items, _, err := p.client.Domains.List()
	if err != nil {
		return nil, err
	}
	zones := make([]types.Zone, 0, len(items))
	for _, item := range items {
		zones = append(zones, types.Zone{ID: item.ID.String(), Name: item.Name, CreatedAt: item.CreatedOn})
	}
	return zones, nil
}

func (p *DNSProvider) zoneName(zoneID string) (string, error) {
	id, err := strconv.Atoi(zoneID)
	if err != nil {
		return "", err
	}
	item, _, err := p.client.Domains.Get(id)
	if err != nil {
		return "", err
	}
	return item.Name, nil
}

func (p *DNSProvider) ListRecords(ctx context.Context, zoneID string) ([]types.Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items, _, err := p.client.Records.List(zoneID, "")
	if err != nil {
		return nil, err
	}
	records := make([]types.Record, 0, len(items))
	for _, item := range items {
		ttl, _ := strconv.Atoi(item.TTL)
		records = append(records, types.Record{
			ID: item.ID, Name: item.Name, Type: item.Type, Value: item.Value,
			TTL: ttl, Comment: item.Remark, UpdateAt: item.UpdateOn,
		})
	}
	return records, nil
}

func (p *DNSProvider) AddRecord(ctx context.Context, zoneID string, record *types.Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name, err := p.zoneName(zoneID)
	if err != nil {
		return err
	}
	_, _, err = p.client.Records.Create(name, dnspodapi.Record{
		Name: record.Name, Type: record.Type, Value: record.Value,
	})
	return err
}

func (p *DNSProvider) UpdateRecord(ctx context.Context, zoneID string, record *types.Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name, err := p.zoneName(zoneID)
	if err != nil {
		return err
	}
	_, _, err = p.client.Records.Update(name, record.ID, dnspodapi.Record{
		Name: record.Name, Type: record.Type, Value: record.Value, Remark: record.Comment,
	})
	return err
}

func (p *DNSProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name, err := p.zoneName(zoneID)
	if err != nil {
		return err
	}
	_, err = p.client.Records.Delete(name, recordID)
	return err
}
