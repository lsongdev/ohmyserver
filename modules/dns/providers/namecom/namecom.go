package namecom

import (
	"context"
	"fmt"
	"strconv"

	types "github.com/lsongdev/ohmyserver/modules/dns/providers/types"
	"github.com/namedotcom/go/namecom"
)

type DNSProvider struct {
	client *namecom.NameCom
}

func NewDNSProvider(config types.Config) (*DNSProvider, error) {
	return &DNSProvider{client: namecom.New(config["user"], config["token"])}, nil
}

func (p *DNSProvider) ListZones(ctx context.Context) ([]types.Zone, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items, err := p.client.ListDomains(&namecom.ListDomainsRequest{})
	if err != nil {
		return nil, err
	}
	zones := make([]types.Zone, 0, len(items.Domains))
	for _, item := range items.Domains {
		zones = append(zones, types.Zone{ID: item.DomainName, Name: item.DomainName, CreatedAt: item.CreateDate})
	}
	return zones, nil
}

func (p *DNSProvider) ListRecords(ctx context.Context, zoneID string) ([]types.Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items, err := p.client.ListRecords(&namecom.ListRecordsRequest{DomainName: zoneID})
	if err != nil {
		return nil, err
	}
	records := make([]types.Record, 0, len(items.Records))
	for _, item := range items.Records {
		records = append(records, types.Record{
			ID: fmt.Sprintf("%d", item.ID), Name: item.Host, Type: item.Type,
			Value: item.Answer, TTL: int(item.TTL),
		})
	}
	return records, nil
}

func (p *DNSProvider) AddRecord(ctx context.Context, zoneID string, record *types.Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := p.client.CreateRecord(&namecom.Record{
		DomainName: zoneID, Type: record.Type, Host: record.Name,
		Answer: record.Value, TTL: uint32(record.TTL),
	})
	return err
}

func (p *DNSProvider) UpdateRecord(ctx context.Context, zoneID string, record *types.Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id, err := strconv.Atoi(record.ID)
	if err != nil {
		return err
	}
	_, err = p.client.UpdateRecord(&namecom.Record{
		DomainName: zoneID, ID: int32(id), Type: record.Type, Host: record.Name,
		Answer: record.Value, TTL: uint32(record.TTL),
	})
	return err
}

func (p *DNSProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id, err := strconv.Atoi(recordID)
	if err != nil {
		return err
	}
	_, err = p.client.DeleteRecord(&namecom.DeleteRecordRequest{DomainName: zoneID, ID: int32(id)})
	return err
}
