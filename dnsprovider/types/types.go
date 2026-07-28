package types

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
	ListZones() ([]Zone, error)
	ListRecords(zoneID string) ([]Record, error)
	AddRecord(zoneID string, record *Record) error
	UpdateRecord(zoneID string, record *Record) error
	DeleteRecord(zoneID, recordID string) error
}

type Config = map[string]string
