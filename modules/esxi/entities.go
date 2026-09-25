package esxi

import (
	"context"
	"time"
)

type DTO struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Host          string    `json:"host"`
	Username      string    `json:"username"`
	Port          int       `json:"port"`
	Insecure      bool      `json:"insecure"`
	HasCredential bool      `json:"has_credential"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type Input struct {
	Name     string  `json:"name"`
	Host     string  `json:"host"`
	Username string  `json:"username"`
	Password *string `json:"password"`
	Port     int     `json:"port"`
	Insecure bool    `json:"insecure"`
}
type Inventory struct {
	VMs   any `json:"vms"`
	Hosts any `json:"hosts"`
}
type Service interface {
	ListESXi(context.Context) ([]DTO, error)
	CreateESXi(context.Context, Input) (DTO, error)
	GetESXi(context.Context, int64) (DTO, error)
	UpdateESXi(context.Context, int64, Input) (DTO, error)
	DeleteESXi(context.Context, int64) error
	TestESXi(context.Context, int64) error
	ESXiInventory(context.Context, int64) (Inventory, error)
}
