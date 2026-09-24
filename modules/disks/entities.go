package disks

import (
	"context"
	"errors"
)

var ErrInvalidDevice = errors.New("invalid disk device")

type Usage struct {
	Device     string  `json:"device"`
	MountPoint string  `json:"mount_point"`
	FSType     string  `json:"fs_type"`
	TotalGB    float64 `json:"total_gb"`
	UsedGB     float64 `json:"used_gb"`
	FreeGB     float64 `json:"free_gb"`
	UsePercent float64 `json:"use_percent"`
}
type Attribute struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Value      int64  `json:"value"`
	Worst      int64  `json:"worst"`
	Threshold  int64  `json:"threshold"`
	WhenFailed string `json:"when_failed,omitempty"`
	Raw        string `json:"raw"`
}
type Health struct {
	Device       string      `json:"device"`
	ModelName    string      `json:"model_name,omitempty"`
	Serial       string      `json:"serial,omitempty"`
	SmartError   string      `json:"smart_error,omitempty"`
	Healthy      bool        `json:"healthy"`
	Temperature  string      `json:"temperature,omitempty"`
	Capacity     string      `json:"capacity,omitempty"`
	PowerOnHours string      `json:"power_on_hours,omitempty"`
	NVMeHealth   string      `json:"nvme_health,omitempty"`
	SCSIError    string      `json:"scsi_error,omitempty"`
	Attributes   []Attribute `json:"attributes,omitempty"`
	SmartInfo    any         `json:"smart_info,omitempty"`
}
type Service interface {
	DiskUsage(context.Context) ([]Usage, error)
	DiskDevices(context.Context) ([]string, error)
	DiskHealth(context.Context, string) (Health, error)
}
