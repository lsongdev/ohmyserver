package disks

import (
	"context"
	"fmt"
	"strings"
)

type LocalService struct{}

func NewLocalService() *LocalService { return &LocalService{} }
func (*LocalService) DiskUsage(ctx context.Context) ([]Usage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items, err := GetDiskUsage()
	if err != nil {
		return nil, err
	}
	out := make([]Usage, 0, len(items))
	for _, v := range items {
		out = append(out, Usage{Device: v.Device, MountPoint: v.MountPoint, FSType: v.FSType, TotalGB: v.TotalGB, UsedGB: v.UsedGB, FreeGB: v.FreeGB, UsePercent: v.UsePercent})
	}
	return out, nil
}
func (*LocalService) DiskDevices(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return GetDiskList()
}
func (s *LocalService) DiskHealth(ctx context.Context, device string) (Health, error) {
	if err := ctx.Err(); err != nil {
		return Health{}, err
	}
	device = strings.TrimSpace(device)
	devices, err := GetDiskList()
	if err != nil {
		return Health{}, err
	}
	allowed := false
	for _, candidate := range devices {
		if candidate == device {
			allowed = true
			break
		}
	}
	if !allowed {
		return Health{}, fmt.Errorf("%w: %s is not a listed block device", ErrInvalidDevice, device)
	}
	d, err := GetDiskHealth(ctx, device)
	if err != nil {
		return Health{}, err
	}
	result := Health{Device: d.Device, ModelName: d.ModelName, Serial: d.Serial, SmartError: d.SmartError, Healthy: d.Healthy, SmartInfo: d.SmartInfo, Temperature: d.GetFormattedTemperature(), Capacity: d.GetFormattedCapacity(), PowerOnHours: d.GetPowerOnHours(), NVMeHealth: d.GetNVMeHealthInfo(), SCSIError: d.GetSCSIErrorLog()}
	for _, a := range d.GetSMARTAttributes() {
		result.Attributes = append(result.Attributes, Attribute{ID: a.ID, Name: a.Name, Value: a.Value, Worst: a.Worst, Threshold: a.Thresh, WhenFailed: a.WhenFailed, Raw: a.Raw.String})
	}
	return result, nil
}
