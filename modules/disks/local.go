package disks

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/lsongdev/ohmyserver/modules/disks/smartctl"
	"github.com/shirou/gopsutil/v3/disk"
)

// DiskInfo represents disk information
type DiskInfo struct {
	Device     string  `json:"device"`
	MountPoint string  `json:"mount_point"`
	FSType     string  `json:"fs_type"`
	TotalGB    float64 `json:"total_gb"`
	UsedGB     float64 `json:"used_gb"`
	FreeGB     float64 `json:"free_gb"`
	UsePercent float64 `json:"use_percent"`
}

// Disk represents a disk with SMART health information
type Disk struct {
	Device     string              `json:"device"`
	ModelName  string              `json:"model_name"`
	Serial     string              `json:"serial"`
	SmartInfo  *smartctl.SmartInfo `json:"smart_info,omitempty"`
	SmartError string              `json:"smart_error,omitempty"`
	Healthy    bool                `json:"healthy"`
}

// GetDiskUsage retrieves disk usage information from the system
func GetDiskUsage() ([]DiskInfo, error) {
	// Get all partitions (skip pseudo filesystems)
	partitions, err := disk.Partitions(false)
	if err != nil {
		return nil, fmt.Errorf("failed to get partitions: %w", err)
	}

	var disks []DiskInfo
	for _, p := range partitions {
		// Skip pseudo filesystems
		if p.Fstype == "tmpfs" || p.Fstype == "devtmpfs" ||
			p.Fstype == "proc" || p.Fstype == "sysfs" ||
			p.Fstype == "cgroup" || p.Fstype == "cgroup2" ||
			p.Fstype == "devpts" || p.Fstype == "mqueue" ||
			p.Fstype == "hugetlbfs" || p.Fstype == "debugfs" ||
			p.Fstype == "tracefs" || p.Fstype == "fusectl" ||
			p.Fstype == "securityfs" || p.Fstype == "pstore" ||
			p.Fstype == "bpf" || p.Fstype == "autofs" ||
			p.Fstype == "configfs" || p.Fstype == "ramfs" {
			continue
		}

		usage, err := disk.Usage(p.Mountpoint)
		if err != nil {
			log.Printf("Warning: failed to get usage for %s: %v", p.Mountpoint, err)
			continue
		}

		diskInfo := DiskInfo{
			Device:     p.Device,
			MountPoint: p.Mountpoint,
			FSType:     p.Fstype,
			TotalGB:    float64(usage.Total) / 1e9,
			UsedGB:     float64(usage.Used) / 1e9,
			FreeGB:     float64(usage.Free) / 1e9,
			UsePercent: usage.UsedPercent,
		}
		disks = append(disks, diskInfo)
	}

	return disks, nil
}

// GetDiskList retrieves a list of all disks/devices
func GetDiskList() ([]string, error) {
	if binary, err := exec.LookPath("lsblk"); err == nil {
		output, err := exec.Command(binary, "-dn", "-o", "NAME").Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			disks := make([]string, 0, len(lines))
			for _, line := range lines {
				if line = strings.TrimSpace(line); line != "" {
					disks = append(disks, "/dev/"+line)
				}
			}
			return disks, nil
		}
	}
	// gopsutil provides a portable fallback for hosts without lsblk. These may
	// be partitions rather than physical devices; smartctl may still report
	// detailed device support when queried.
	partitions, err := disk.Partitions(false)
	if err != nil {
		return nil, fmt.Errorf("list block devices (lsblk and partition enumeration failed): %w", err)
	}
	seen := make(map[string]struct{}, len(partitions))
	devices := make([]string, 0, len(partitions))
	for _, partition := range partitions {
		device := strings.TrimSpace(partition.Device)
		if device == "" {
			continue
		}
		if _, exists := seen[device]; exists {
			continue
		}
		seen[device] = struct{}{}
		devices = append(devices, device)
	}
	return devices, nil
}

// GetDiskHealth retrieves SMART health information for a specific disk
func GetDiskHealth(ctx context.Context, device string) (*Disk, error) {
	smartInfo, err := smartctl.Check(ctx, device)
	if err != nil {
		return &Disk{
			Device:     device,
			SmartError: fmt.Sprintf("smartctl command failed: %v", err),
		}, nil
	}

	return buildDiskFromSmartInfo(device, smartInfo), nil
}

// buildDiskFromSmartInfo constructs a Disk struct from SMART info
func buildDiskFromSmartInfo(device string, smartInfo *smartctl.SmartInfo) *Disk {
	disk := &Disk{
		Device:    device,
		ModelName: smartInfo.ModelName,
		Serial:    smartInfo.SerialNumber,
		SmartInfo: smartInfo,
	}

	// Determine health status
	disk.Healthy = true

	// Check if SMART is available and enabled
	if !smartInfo.SmartSupport.Available {
		disk.Healthy = false
		disk.SmartError = "SMART not available"
	} else if !smartInfo.SmartSupport.Enabled {
		disk.Healthy = false
		disk.SmartError = "SMART not enabled"
	}

	// Check SMART status
	if !smartInfo.SmartStatus.Passed {
		disk.Healthy = false
	}

	// Check for critical warnings (NVMe)
	if smartInfo.NVMESmartHealthInformationLog.CriticalWarning > 0 {
		disk.Healthy = false
	}

	// Check temperature
	if smartInfo.Temperature.Current > 70 {
		// Temperature warning (not necessarily unhealthy)
	}

	// Check for grown defect list (SCSI)
	if smartInfo.ScsiGrownDefectList > 0 {
		disk.Healthy = false
	}

	return disk
}

// GetFormattedTemperature returns formatted temperature string
func (d *Disk) GetFormattedTemperature() string {
	if d.SmartInfo == nil {
		return "N/A"
	}
	return fmt.Sprintf("%d°C", d.SmartInfo.Temperature.Current)
}

// GetPowerOnHours returns formatted power-on hours string
func (d *Disk) GetPowerOnHours() string {
	if d.SmartInfo == nil {
		return "N/A"
	}

	// Try ATA first
	if d.SmartInfo.PowerOnTime.Hours > 0 {
		hours := d.SmartInfo.PowerOnTime.Hours
		days := hours / 24
		years := days / 365
		if years > 0 {
			return fmt.Sprintf("%d years, %d days (%d hours)", years, days%365, hours)
		}
		return fmt.Sprintf("%d days (%d hours)", days, hours)
	}

	// Try NVMe
	if d.SmartInfo.NVMESmartHealthInformationLog.PowerOnHours > 0 {
		hours := d.SmartInfo.NVMESmartHealthInformationLog.PowerOnHours
		days := hours / 24
		years := days / 365
		if years > 0 {
			return fmt.Sprintf("%d years, %d days (%d hours)", years, days%365, hours)
		}
		return fmt.Sprintf("%d days (%d hours)", days, hours)
	}

	return "N/A"
}

// GetFormattedCapacity returns formatted capacity string
func (d *Disk) GetFormattedCapacity() string {
	if d.SmartInfo == nil {
		return "N/A"
	}

	bytes := d.SmartInfo.UserCapacity.Bytes
	if bytes == 0 {
		// Try NVMe namespaces
		for _, ns := range d.SmartInfo.NVMENamespaces {
			bytes += ns.Size.Bytes
		}
	}

	if bytes == 0 {
		return "N/A"
	}

	gb := float64(bytes) / 1e9
	if gb >= 1000 {
		return fmt.Sprintf("%.2f TB", gb/1000)
	}
	return fmt.Sprintf("%.2f GB", gb)
}

// GetSMARTAttributes returns ATA SMART attributes if available
func (d *Disk) GetSMARTAttributes() []smartctl.ATASmartAttributesTableItem {
	if d.SmartInfo == nil {
		return nil
	}
	return d.SmartInfo.ATASmartAttributes.Table
}

// GetNVMeHealthInfo returns NVMe health information if available
func (d *Disk) GetNVMeHealthInfo() string {
	if d.SmartInfo == nil {
		return ""
	}

	if d.SmartInfo.NVMESmartHealthInformationLog.CriticalWarning == 0 &&
		d.SmartInfo.NVMESmartHealthInformationLog.Temperature == 0 {
		return ""
	}

	var parts []string
	if d.SmartInfo.NVMESmartHealthInformationLog.CriticalWarning > 0 {
		parts = append(parts, fmt.Sprintf("Critical Warning: %d", d.SmartInfo.NVMESmartHealthInformationLog.CriticalWarning))
	}
	if d.SmartInfo.NVMESmartHealthInformationLog.AvailableSpare > 0 {
		parts = append(parts, fmt.Sprintf("Available Spare: %d%%", d.SmartInfo.NVMESmartHealthInformationLog.AvailableSpare))
	}
	if d.SmartInfo.NVMESmartHealthInformationLog.PercentageUsed > 0 {
		parts = append(parts, fmt.Sprintf("Percentage Used: %d%%", d.SmartInfo.NVMESmartHealthInformationLog.PercentageUsed))
	}

	return strings.Join(parts, ", ")
}

// GetSCSIErrorLog returns SCSI error log summary
func (d *Disk) GetSCSIErrorLog() string {
	if d.SmartInfo == nil {
		return ""
	}

	if d.SmartInfo.ScsiGrownDefectList == 0 {
		return ""
	}

	return fmt.Sprintf("Grown Defect List: %d", d.SmartInfo.ScsiGrownDefectList)
}
