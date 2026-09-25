package dhcp

import "time"

type Settings struct {
	Enabled          bool      `json:"enabled"`
	ListenAddr       string    `json:"listen_addr"`
	ServerIP         string    `json:"server_ip"`
	SubnetMask       string    `json:"subnet_mask"`
	PoolStart        string    `json:"pool_start"`
	PoolEnd          string    `json:"pool_end"`
	Router           string    `json:"router"`
	DNSServers       string    `json:"dns_servers"`
	NTPServers       string    `json:"ntp_servers"`
	DomainName       string    `json:"domain_name"`
	BroadcastAddress string    `json:"broadcast_address"`
	LeaseSeconds     int       `json:"lease_seconds"`
	RenewalSeconds   int       `json:"renewal_seconds"`
	RebindingSeconds int       `json:"rebinding_seconds"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type StaticLease struct {
	ID          int64     `json:"id"`
	MAC         string    `json:"mac"`
	IP          string    `json:"ip"`
	Hostname    string    `json:"hostname"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Lease struct {
	ID        int64     `json:"id"`
	MAC       string    `json:"mac"`
	IP        string    `json:"ip"`
	Hostname  string    `json:"hostname"`
	State     string    `json:"state"`
	ExpiresAt time.Time `json:"expires_at"`
	LastSeen  time.Time `json:"last_seen"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
