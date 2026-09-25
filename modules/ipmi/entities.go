package ipmi

import (
	"context"
	"time"
)

type Server struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	BMCIP       string    `json:"bmc_ip"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	Password    string    `json:"-"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type ServerDTO struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	BMCIP         string    `json:"bmc_ip"`
	Port          int       `json:"port"`
	Username      string    `json:"username"`
	HasCredential bool      `json:"has_credential"`
	Status        string    `json:"status"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type CreateRequest struct {
	Name        string `json:"name"`
	BMCIP       string `json:"bmc_ip"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Description string `json:"description"`
}
type UpdateRequest struct {
	Name        string  `json:"name"`
	BMCIP       string  `json:"bmc_ip"`
	Port        int     `json:"port"`
	Username    string  `json:"username"`
	Password    *string `json:"password"`
	Description string  `json:"description"`
}
type PowerRequest struct {
	Action string `json:"action"`
}
type HostInfo struct {
	DeviceID          uint8  `json:"device_id"`
	DeviceRevision    uint8  `json:"device_revision"`
	FirmwareRevision  string `json:"firmware_revision"`
	ManufacturerID    string `json:"manufacturer_id"`
	ProductID         uint16 `json:"product_id"`
	AuxFirmwareRev    string `json:"aux_firmware_rev"`
	PowerState        string `json:"power_state"`
	LastPowerEvent    string `json:"last_power_event"`
	ChassisIntrusion  string `json:"chassis_intrusion"`
	FrontPanelLockout string `json:"front_panel_lockout"`
	DriveFault        string `json:"drive_fault"`
	CoolingFault      string `json:"cooling_fault"`
}
type FRU struct {
	ID                  uint8  `json:"id"`
	BoardManufacturer   string `json:"board_manufacturer,omitempty"`
	BoardProductName    string `json:"board_product_name,omitempty"`
	BoardSerial         string `json:"board_serial,omitempty"`
	BoardPartNumber     string `json:"board_part_number,omitempty"`
	ProductManufacturer string `json:"product_manufacturer,omitempty"`
	ProductName         string `json:"product_name,omitempty"`
	ProductSerial       string `json:"product_serial,omitempty"`
	ProductPartNumber   string `json:"product_part_number,omitempty"`
	ProductVersion      string `json:"product_version,omitempty"`
	ChassisPartNumber   string `json:"chassis_part_number,omitempty"`
	ChassisSerial       string `json:"chassis_serial,omitempty"`
}
type Inventory struct {
	Host            *HostInfo `json:"host,omitempty"`
	FRUs            []FRU     `json:"frus"`
	ConnectionError string    `json:"connection_error,omitempty"`
}
type Service interface {
	ListIPMIServers(context.Context) ([]ServerDTO, error)
	CreateIPMIServerAPI(context.Context, CreateRequest) (*ServerDTO, error)
	ReadIPMIServer(context.Context, int64) (*ServerDTO, error)
	UpdateIPMIServerAPI(context.Context, int64, UpdateRequest) (*ServerDTO, error)
	RemoveIPMIServerAPI(context.Context, int64) error
	TestIPMIConnection(context.Context, int64) error
	IPMIInventory(context.Context, int64) (*Inventory, error)
	ControlIPMIPower(context.Context, int64, string) error
}
