package server

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"github.com/lsongdev/ohmyserver/smartctl"
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

// initDisksSchema creates the disk monitoring settings table
func (s *Server) initDisksSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS disk_settings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		key TEXT NOT NULL UNIQUE,
		value TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_disk_settings_key ON disk_settings(key);
	`
	_, err := s.db.Exec(query)
	return err
}

// GetDiskUsage retrieves disk usage information from the system
func (s *Server) GetDiskUsage() ([]DiskInfo, error) {
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
func (s *Server) GetDiskList() ([]string, error) {
	cmd := exec.Command("lsblk", "-dn", "-o", "NAME")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute lsblk command: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var disks []string
	for _, line := range lines {
		if line != "" {
			disks = append(disks, "/dev/"+line)
		}
	}

	return disks, nil
}

// GetDiskHealth retrieves SMART health information for a specific disk
func (s *Server) GetDiskHealth(device string) (*Disk, error) {
	smartInfo, err := smartctl.Check(device)
	if err != nil {
		return &Disk{
			Device:     device,
			SmartError: fmt.Sprintf("smartctl command failed: %v", err),
		}, nil
	}

	return s.buildDiskFromSmartInfo(device, smartInfo), nil
}

// buildDiskFromSmartInfo constructs a Disk struct from SMART info
func (s *Server) buildDiskFromSmartInfo(device string, smartInfo *smartctl.SmartInfo) *Disk {
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

// DisksView renders the disk list page
func (s *Server) DisksView(w http.ResponseWriter, r *http.Request) {
	diskUsage, err := s.GetDiskUsage()
	if err != nil {
		log.Printf("Error fetching disk usage: %v", err)
		s.Error(w, err)
		return
	}

	s.Render(w, "disks/index", H{
		"disks": diskUsage,
	})
}

// DiskDetailHandler handles viewing disk SMART details
func (s *Server) DiskDetailHandler(w http.ResponseWriter, r *http.Request) {
	device := r.URL.Query().Get("device")
	if device == "" {
		http.Error(w, "Device parameter is required", http.StatusBadRequest)
		return
	}

	disk, err := s.GetDiskHealth(device)
	if err != nil {
		log.Printf("Error fetching disk health for %s: %v", device, err)
		s.Error(w, err)
		return
	}

	s.Render(w, "disks/detail", H{
		"disk": disk,
	})
}

// DiskTestHandler tests SMART connection to a disk
func (s *Server) DiskTestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	device := r.FormValue("device")
	if device == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Device parameter is required"))
		return
	}

	disk, err := s.GetDiskHealth(device)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed to get disk health: " + err.Error()))
		return
	}

	if disk.SmartError != "" {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("SMART error: " + disk.SmartError))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("SMART data retrieved successfully"))
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

func init() {
	RegisterInitSchemaHook((*Server).initDisksSchema)
}
