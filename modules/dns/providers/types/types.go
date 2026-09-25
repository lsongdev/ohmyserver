package types

import "context"

type Zone struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

type Record struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	TTL      int    `json:"ttl"`
	Comment  string `json:"comment"`
	UpdateAt string `json:"updateAt"`
}

type IDNSProvider interface {
	ListZones(ctx context.Context) ([]Zone, error)
	ListRecords(ctx context.Context, zoneID string) ([]Record, error)
	AddRecord(ctx context.Context, zoneID string, record *Record) error
	UpdateRecord(ctx context.Context, zoneID string, record *Record) error
	DeleteRecord(ctx context.Context, zoneID, recordID string) error
}

type Config = map[string]string
