package ipmi

import (
	"context"
	"fmt"
	ipmi "github.com/bougou/go-ipmi"
	"log"
)

// Client represents an IPMI/BMC server
type Client struct {
	BMCIP    string
	Port     int
	Username string
	Password string
	client   *ipmi.Client
}

// Connect establishes connection to IPMI device
func (s *Client) Connect(ctx context.Context) error {
	client, err := ipmi.NewClient(s.BMCIP, s.Port, s.Username, s.Password)
	if err != nil {
		return fmt.Errorf("failed to create IPMI client: %w", err)
	}

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to IPMI: %w", err)
	}

	s.client = client
	return nil
}

// Disconnect closes connection to IPMI device
func (s *Client) Disconnect(ctx context.Context) error {
	if s.client != nil {
		// go-ipmi doesn't have explicit disconnect, client will be garbage collected
		s.client = nil
	}
	return nil
}

// GetHostInfo retrieves host/BMC information
func (s *Client) GetHostInfo(ctx context.Context) (*HostInfo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to IPMI device")
	}

	host := &HostInfo{}

	// Get device ID
	deviceID, err := s.client.GetDeviceID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get device ID: %w", err)
	}

	host.DeviceID = deviceID.DeviceID
	host.DeviceRevision = deviceID.DeviceRevision
	host.FirmwareRevision = deviceID.FirmwareVersionStr()
	host.ManufacturerID = fmt.Sprintf("%d", deviceID.ManufacturerID)
	host.ProductID = deviceID.ProductID
	if len(deviceID.AuxiliaryFirmwareRevision) == 4 {
		host.AuxFirmwareRev = fmt.Sprintf("%02x.%02x.%02x.%02x",
			deviceID.AuxiliaryFirmwareRevision[0],
			deviceID.AuxiliaryFirmwareRevision[1],
			deviceID.AuxiliaryFirmwareRevision[2],
			deviceID.AuxiliaryFirmwareRevision[3])
	}

	// Get chassis status
	chassisStatus, err := s.client.GetChassisStatus(ctx)
	if err != nil {
		// Non-critical error, continue with partial info
		log.Printf("Warning: failed to get chassis status: %v", err)
	} else {
		// Power state
		if chassisStatus.PowerIsOn {
			host.PowerState = "on"
		} else {
			host.PowerState = "off"
		}

		// Last power event
		if chassisStatus.LastPowerOnByCommand {
			host.LastPowerEvent = "power-on-by-command"
		} else if chassisStatus.LastPowerDownByPowerFault {
			host.LastPowerEvent = "power-fault"
		} else if chassisStatus.LastPowerDownByPowerInterlockActivated {
			host.LastPowerEvent = "interlock-activated"
		} else if chassisStatus.LastPowerDownByPowerOverload {
			host.LastPowerEvent = "overload"
		} else if chassisStatus.ACFailed {
			host.LastPowerEvent = "ac-failed"
		} else {
			host.LastPowerEvent = "unknown"
		}

		// Chassis intrusion
		if chassisStatus.ChassisIntrusionActive {
			host.ChassisIntrusion = "active"
		} else {
			host.ChassisIntrusion = "inactive"
		}

		// Front panel lockout
		if chassisStatus.FrontPanelLockoutActive {
			host.FrontPanelLockout = "active"
		} else {
			host.FrontPanelLockout = "inactive"
		}

		// Drive fault
		if chassisStatus.DriveFault {
			host.DriveFault = "detected"
		} else {
			host.DriveFault = "none"
		}

		// Cooling fault
		if chassisStatus.CollingFanFault {
			host.CoolingFault = "detected"
		} else {
			host.CoolingFault = "none"
		}
	}

	return host, nil
}

// GetFRUInfo retrieves FRU (Field Replaceable Unit) information
func (s *Client) GetFRUInfo(ctx context.Context) ([]*ipmi.FRU, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not connected to IPMI device")
	}

	frus, err := s.client.GetFRUs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get FRU data: %w", err)
	}

	return frus, nil
}

// TestConnection tests the connection to IPMI device
func (s *Client) TestConnection(ctx context.Context) error {
	if s.client == nil {
		return fmt.Errorf("not connected")
	}

	_, err := s.client.GetDeviceID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get device info: %w", err)
	}

	return nil
}

// ControlPower controls the power state of the host
func (s *Client) ControlPower(ctx context.Context, action string) error {
	if s.client == nil {
		return fmt.Errorf("not connected to IPMI device")
	}

	var err error
	switch action {
	case "on":
		_, err = s.client.ChassisControl(ctx, ipmi.ChassisControlPowerUp)
	case "off":
		_, err = s.client.ChassisControl(ctx, ipmi.ChassisControlPowerDown)
	case "cycle":
		_, err = s.client.ChassisControl(ctx, ipmi.ChassisControlPowerCycle)
	case "reset":
		_, err = s.client.ChassisControl(ctx, ipmi.ChassisControlHardReset)
	default:
		return fmt.Errorf("unknown power action: %s", action)
	}

	return err
}
