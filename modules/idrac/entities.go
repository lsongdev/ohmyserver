package idrac

import (
	"context"
	"time"
)

type DTO struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	Username      string    `json:"username"`
	Version       string    `json:"version"`
	Status        string    `json:"status"`
	Description   string    `json:"description"`
	HasCredential bool      `json:"has_credential"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type Input struct {
	Name        string  `json:"name"`
	Host        string  `json:"host"`
	Port        int     `json:"port"`
	Username    string  `json:"username"`
	Password    *string `json:"password"`
	Version     string  `json:"version"`
	Description string  `json:"description"`
}
type Inventory struct {
	System     System   `json:"system,omitempty"`
	PowerState string   `json:"power_state,omitempty"`
	Sensors    []Sensor `json:"sensors"`
}
type System struct {
	Model            string  `json:"model"`
	AssetTag         string  `json:"asset_tag"`
	SerialNumber     string  `json:"serial_number"`
	HostName         string  `json:"host_name"`
	BiosVersion      string  `json:"bios_version"`
	MemoryGiB        float64 `json:"memory_gib"`
	ProcessorCount   int     `json:"processor_count"`
	ProcessorModel   string  `json:"processor_model"`
	ProcessorThreads int     `json:"processor_threads"`
	Manufacturer     string  `json:"manufacturer"`
	Name             string  `json:"name"`
}
type Sensor struct {
	Name         string  `json:"name"`
	Reading      float64 `json:"reading"`
	ReadingUnits string  `json:"reading_units"`
	State        string  `json:"state"`
	Health       string  `json:"health"`
}
type Service interface {
	ListIDRAC(context.Context) ([]DTO, error)
	CreateIDRAC(context.Context, Input) (DTO, error)
	GetIDRAC(context.Context, int64) (DTO, error)
	UpdateIDRAC(context.Context, int64, Input) (DTO, error)
	DeleteIDRAC(context.Context, int64) error
	TestIDRAC(context.Context, int64) error
	ReadIDRACInventory(context.Context, int64) (Inventory, error)
	ControlIDRACPower(context.Context, int64, string) error
}
