package system

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/lsongdev/smartctl-go/smartctl"
)

// Health summarises a disk's SMART report.
type Health struct {
	// Error says why there is no report: smartctl is missing, the device
	// does not support SMART (virtual disks, many USB bridges), …
	Error string `json:"error,omitempty"`
	// Asleep means the disk was in standby and was left alone; the other
	// fields then come from the last report, if any.
	Asleep       bool      `json:"asleep,omitempty"`
	Passed       *bool     `json:"passed,omitempty"` // the drive's own overall assessment
	Temperature  int64     `json:"temperature,omitempty"`
	PowerOnHours int64     `json:"power_on_hours,omitempty"`
	PowerCycles  int64     `json:"power_cycles,omitempty"`
	Wear         *int64    `json:"wear,omitempty"` // percent of rated endurance used (solid state)
	Warnings     []string  `json:"warnings,omitempty"`
	Testing      *int      `json:"testing,omitempty"` // a self-test is running, with this percent remaining
	CheckedAt    time.Time `json:"checked_at"`
}

// SmartAvailable reports whether smartctl is installed.
func SmartAvailable() bool {
	_, err := exec.LookPath("smartctl")
	return err == nil
}

// ErrNoSmartctl means smartmontools is not installed.
var ErrNoSmartctl = errors.New("smartctl is not installed (package smartmontools)")

// Report reads a disk's full SMART report. With wake false a disk in
// standby is left alone and smartctl.ErrAsleep returned.
func (s *Storage) Report(ctx context.Context, disk string, wake bool) (*smartctl.SmartInfo, error) {
	if !SmartAvailable() {
		return nil, ErrNoSmartctl
	}
	return smartctl.Collect(ctx, "/dev/"+disk, smartctl.Options{NoWake: !wake})
}

// SelfTest starts (short, long) or aborts a disk's self-test.
func (s *Storage) SelfTest(ctx context.Context, disk, kind string) error {
	if !SmartAvailable() {
		return ErrNoSmartctl
	}
	switch kind {
	case smartctl.TestShort, smartctl.TestLong, smartctl.TestAbort:
	default:
		return fmt.Errorf("%w self-test %q", ErrInvalid, kind)
	}
	return smartctl.SelfTest(ctx, "/dev/"+disk, kind, smartctl.Options{})
}

// Summarize turns a report into a Health.
func Summarize(info *smartctl.SmartInfo) Health {
	h := Health{CheckedAt: time.Now().UTC(), Temperature: info.Temperature.Current,
		PowerOnHours: info.PowerOnTime.Hours, PowerCycles: info.PowerCycleCount}
	passed := info.SmartStatus.Passed
	if info.SmartSupport.Available || info.SmartCtl.ExitStatus&smartctl.ExitFailing != 0 || info.NVMEVersion.String != "" {
		h.Passed = &passed
		if !passed {
			h.Warnings = append(h.Warnings, "The drive reports that it is failing: back up its data and replace it")
		}
	}
	if info.SmartSupport.Available && !info.SmartSupport.Enabled {
		h.Warnings = append(h.Warnings, "SMART is turned off on this drive")
	}
	attr := map[int]smartctl.ATASmartAttributesTableItem{}
	for _, a := range info.ATASmartAttributes.Table {
		attr[a.ID] = a
		if a.WhenFailed == "now" || a.WhenFailed == "FAILING_NOW" {
			h.Warnings = append(h.Warnings, fmt.Sprintf("%s is below its failure threshold", strings.ReplaceAll(a.Name, "_", " ")))
		}
	}
	counted := []struct {
		id   int
		what string
	}{
		{5, "reallocated sectors"}, {197, "sectors pending reallocation"}, {198, "uncorrectable sectors"},
		{187, "uncorrectable errors reported"}, {199, "interface CRC errors (often a loose or bad cable)"},
	}
	for _, c := range counted {
		if a, ok := attr[c.id]; ok && a.Raw.Value > 0 {
			h.Warnings = append(h.Warnings, fmt.Sprintf("%d %s", a.Raw.Value, c.what))
		}
	}
	// Solid-state wear, as the drive reports its remaining life (100 is new).
	for _, id := range []int{231, 233, 177, 202} {
		if a, ok := attr[id]; ok && a.Value > 0 && a.Value <= 100 && (id != 202 || strings.Contains(a.Name, "Percent")) {
			used := 100 - a.Value
			h.Wear = &used
			break
		}
	}
	if nvme := info.NVMESmartHealthInformationLog; info.NVMEVersion.String != "" {
		used := nvme.PercentageUsed
		h.Wear = &used
		h.PowerOnHours, h.PowerCycles = nvme.PowerOnHours, nvme.PowerCycles
		if h.Temperature == 0 {
			h.Temperature = nvme.Temperature
		}
		if nvme.CriticalWarning != 0 {
			h.Warnings = append(h.Warnings, fmt.Sprintf("NVMe critical warning 0x%02x", nvme.CriticalWarning))
		}
		if nvme.MediaErrors > 0 {
			h.Warnings = append(h.Warnings, fmt.Sprintf("%d media errors", nvme.MediaErrors))
		}
		if nvme.AvailableSpareThreshold > 0 && nvme.AvailableSpare <= nvme.AvailableSpareThreshold {
			h.Warnings = append(h.Warnings, fmt.Sprintf("Spare capacity is down to %d%%", nvme.AvailableSpare))
		}
	}
	if h.Wear != nil && *h.Wear >= 90 {
		h.Warnings = append(h.Warnings, fmt.Sprintf("%d%% of the rated write endurance is used", *h.Wear))
	}
	if h.Temperature >= 60 {
		h.Warnings = append(h.Warnings, fmt.Sprintf("Running hot at %d °C", h.Temperature))
	}
	if st := info.ATASmartData.SelfTest.Status; st.Value>>4 == 0xf {
		remaining := st.RemainingPercent
		h.Testing = &remaining
	}
	return h
}

// lines splits command output into its non-empty lines.
func lines(out []byte) []string {
	var result []string
	for line := range strings.SplitSeq(string(out), "\n") {
		if line = strings.TrimRight(line, "\r"); strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}
