package idrac

import (
	"context"
	"fmt"

	idraclib "github.com/lsongdev/idrac-go/idrac"
)

type Client struct {
	Host     string
	Username string
	Password string
	client   *idraclib.Client
}

func (c *Client) Connect(context.Context) error {
	client, err := idraclib.NewClient(&idraclib.Config{Host: c.Host, Username: c.Username, Password: c.Password})
	if err != nil {
		return fmt.Errorf("create iDRAC client: %w", err)
	}
	c.client = client
	return nil
}
func (c *Client) Disconnect(context.Context) error { c.client = nil; return nil }
func (c *Client) TestConnection(context.Context) error {
	if c.client == nil {
		return fmt.Errorf("not connected to iDRAC")
	}
	if _, err := c.client.GetSystemInfo(); err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	return nil
}
func (c *Client) GetSystemInfo(context.Context) (System, error) {
	if c.client == nil {
		return System{}, fmt.Errorf("not connected to iDRAC")
	}
	v, err := c.client.GetSystemInfo()
	if err != nil {
		return System{}, fmt.Errorf("read iDRAC system info: %w", err)
	}
	return System{Model: v.Model, AssetTag: v.AssetTag, SerialNumber: v.SerialNumber, HostName: v.HostName, BiosVersion: v.BiosVersion, MemoryGiB: v.MemoryGiB, ProcessorCount: v.ProcessorSummary.Count, ProcessorModel: v.ProcessorSummary.Model, ProcessorThreads: v.ProcessorSummary.LogicalCount, Manufacturer: v.Manufacturer, Name: v.Name}, nil
}
func (c *Client) GetPowerState(context.Context) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("not connected to iDRAC")
	}
	return c.client.GetPowerState()
}
func (c *Client) GetHardwareStatus(context.Context) ([]Sensor, error) {
	if c.client == nil {
		return nil, fmt.Errorf("not connected to iDRAC")
	}
	items, err := c.client.GetHardwareStatus()
	if err != nil {
		return nil, fmt.Errorf("read iDRAC hardware status: %w", err)
	}
	out := make([]Sensor, 0, len(items))
	for _, v := range items {
		out = append(out, Sensor{Name: v.Name, Reading: v.Reading, ReadingUnits: v.ReadingUnits, State: v.State, Health: v.Health})
	}
	return out, nil
}
func (c *Client) ControlPower(_ context.Context, state idraclib.PowerState) error {
	if c.client == nil {
		return fmt.Errorf("not connected to iDRAC")
	}
	return c.client.SetPowerState(state)
}
