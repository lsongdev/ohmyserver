package ntp

import (
	"context"
	"time"
)

type Settings struct {
	Enabled     bool      `json:"enabled"`
	ListenAddr  string    `json:"listen_addr"`
	Stratum     int       `json:"stratum"`
	ReferenceID string    `json:"reference_id"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Repository interface {
	Initialize(context.Context) error
	Get(context.Context) (*Settings, error)
	Update(context.Context, Settings) error
}
type Service interface {
	UpdateNTPConfiguration(context.Context, Settings) error
	NTPRuntimeStatus(context.Context) (any, error)
	NTPAction(context.Context, string) error
}
