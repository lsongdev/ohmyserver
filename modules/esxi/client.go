package esxi

import (
	"context"
	"fmt"
	"net/url"
	"sync"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
)

// Client is a short-lived runtime adapter; persistent configuration belongs to modules/esxi.
type Client struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Username  string `json:"username"`
	Password  string `json:"-"`
	Port      int    `json:"port"`
	Insecure  bool   `json:"insecure"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	client    *govmomi.Client
	clientMu  sync.RWMutex
}
type VM struct {
	Name       string `json:"name"`
	GuestOS    string `json:"guest_os"`
	PowerState string `json:"power_state"`
	CPU        int32  `json:"cpu"`
	Memory     int64  `json:"memory"`
	IPAddress  string `json:"ip_address"`
	Host       string `json:"host"`
	Path       string `json:"path"`
}
type Host struct {
	Name        string  `json:"name"`
	ProductName string  `json:"product_name"`
	Version     string  `json:"version"`
	CPU         int32   `json:"cpu"`
	CPUUsage    float64 `json:"cpu_usage"`
	Memory      int64   `json:"memory"`
	MemoryUsage int64   `json:"memory_usage"`
}

func (e *Client) Connect(ctx context.Context) error {
	e.clientMu.Lock()
	defer e.clientMu.Unlock()
	u := &url.URL{Scheme: "https", Host: fmt.Sprintf("%s:%d", e.Host, e.Port), Path: "/sdk"}
	u.User = url.UserPassword(e.Username, e.Password)
	client, err := govmomi.NewClient(ctx, u, e.Insecure)
	if err != nil {
		return err
	}
	e.client = client
	return nil
}
func (e *Client) Disconnect(ctx context.Context) error {
	e.clientMu.Lock()
	defer e.clientMu.Unlock()
	if e.client != nil {
		err := e.client.Logout(ctx)
		e.client = nil
		return err
	}
	return nil
}
func (e *Client) GetClient() *govmomi.Client {
	e.clientMu.RLock()
	defer e.clientMu.RUnlock()
	return e.client
}
func (e *Client) GetVimClient() *vim25.Client {
	e.clientMu.RLock()
	defer e.clientMu.RUnlock()
	if e.client == nil {
		return nil
	}
	return e.client.Client
}
func (e *Client) GetVMs(ctx context.Context) ([]VM, error) {
	client := e.GetVimClient()
	if client == nil {
		return nil, fmt.Errorf("not connected to ESXi server")
	}
	manager := view.NewManager(client)
	container, err := manager.CreateContainerView(ctx, client.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
	if err != nil {
		return nil, err
	}
	defer container.Destroy(ctx)
	var vms []mo.VirtualMachine
	if err = container.Retrieve(ctx, []string{"VirtualMachine"}, []string{"summary", "runtime", "config", "guest"}, &vms); err != nil {
		return nil, err
	}
	result := make([]VM, 0, len(vms))
	for _, vm := range vms {
		result = append(result, VM{Name: vm.Summary.Config.Name, GuestOS: vm.Summary.Config.GuestFullName, PowerState: string(vm.Summary.Runtime.PowerState), CPU: vm.Summary.Config.NumCpu, Memory: int64(vm.Summary.Config.MemorySizeMB), IPAddress: vm.Guest.IpAddress, Host: vm.Summary.Runtime.Host.Reference().Value, Path: vm.Summary.Config.VmPathName})
	}
	return result, nil
}
func (e *Client) GetHosts(ctx context.Context) ([]Host, error) {
	client := e.GetVimClient()
	if client == nil {
		return nil, fmt.Errorf("not connected to ESXi server")
	}
	manager := view.NewManager(client)
	container, err := manager.CreateContainerView(ctx, client.ServiceContent.RootFolder, []string{"HostSystem"}, true)
	if err != nil {
		return nil, err
	}
	defer container.Destroy(ctx)
	var hosts []mo.HostSystem
	if err = container.Retrieve(ctx, []string{"HostSystem"}, []string{"summary", "hardware", "runtime"}, &hosts); err != nil {
		return nil, err
	}
	result := make([]Host, 0, len(hosts))
	for _, host := range hosts {
		result = append(result, Host{Name: host.Summary.Config.Name, ProductName: host.Summary.Config.Product.Name, Version: host.Summary.Config.Product.Version, CPU: int32(host.Summary.Hardware.NumCpuThreads), CPUUsage: float64(host.Summary.QuickStats.OverallCpuUsage), Memory: host.Summary.Hardware.MemorySize / 1024 / 1024, MemoryUsage: int64(host.Summary.QuickStats.OverallMemoryUsage)})
	}
	return result, nil
}
func (e *Client) TestConnection(context.Context) error {
	client := e.GetVimClient()
	if client == nil {
		return fmt.Errorf("not connected")
	}
	if client.ServiceContent.RootFolder.Value == "" {
		return fmt.Errorf("unable to retrieve service content")
	}
	return nil
}
