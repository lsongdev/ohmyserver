package pve

import (
	"context"
	"time"
)

type Server struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	Insecure  bool      `json:"insecure"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DTO struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	Username      string    `json:"username"`
	Insecure      bool      `json:"insecure"`
	HasCredential bool      `json:"has_credential"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type Input struct {
	Name     string  `json:"name"`
	Host     string  `json:"host"`
	Port     int     `json:"port"`
	Username string  `json:"username"`
	Password *string `json:"password"`
	Insecure bool    `json:"insecure"`
}
type VM struct {
	VMID     int    `json:"vmid"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	MaxMem   int64  `json:"maxmem"`
	MaxDisk  int64  `json:"maxdisk"`
	Uptime   int64  `json:"uptime"`
	CPUs     int    `json:"cpus"`
	Template int    `json:"template,omitempty"`
}
type Node struct {
	Node      string  `json:"node"`
	Status    string  `json:"status"`
	CPU       float64 `json:"cpu"`
	MaxCPU    int     `json:"maxcpu"`
	Mem       int64   `json:"mem"`
	MaxMem    int64   `json:"maxmem"`
	Uptime    int64   `json:"uptime"`
	VMs       []VM    `json:"vms"`
	Templates []VM    `json:"templates"`
}
type Inventory struct {
	Nodes []Node `json:"nodes"`
}
type CreateVMInput struct {
	Node       string `json:"node"`
	TemplateID int    `json:"template_id"`
	Name       string `json:"name"`
	Cores      int    `json:"cores"`
	Sockets    int    `json:"sockets"`
	MemoryMB   int    `json:"memory_mb"`
	DiskGB     int    `json:"disk_gb"`
	Start      bool   `json:"start"`
}
type CreateVMResult struct {
	VMID int    `json:"vmid"`
	Task string `json:"task,omitempty"`
}
type Service interface {
	ListPVE(context.Context) ([]DTO, error)
	CreatePVE(context.Context, Input) (DTO, error)
	GetPVE(context.Context, int64) (DTO, error)
	UpdatePVE(context.Context, int64, Input) (DTO, error)
	DeletePVE(context.Context, int64) error
	TestPVE(context.Context, int64) error
	PVEInventory(context.Context, int64) (Inventory, error)
	CreatePVEVM(context.Context, int64, CreateVMInput) (CreateVMResult, error)
}
