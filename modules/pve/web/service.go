package web

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	pvemodule "github.com/lsongdev/ohmyserver/modules/pve"
	pvelib "github.com/lsongdev/ohmyserver/modules/pve/pve"
)

type LocalService struct{ repo *Repository }

func NewLocalService(repo *Repository) *LocalService { return &LocalService{repo: repo} }
func dto(v *Server) pvemodule.DTO {
	return pvemodule.DTO{ID: v.ID, Name: v.Name, Host: v.Host, Port: v.Port, Username: v.Username, Insecure: v.Insecure, HasCredential: v.Password != "", CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func (s *LocalService) ListPVE(ctx context.Context) ([]pvemodule.DTO, error) {
	items, e := s.repo.List(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]pvemodule.DTO, 0, len(items))
	for i := range items {
		out = append(out, dto(&items[i]))
	}
	return out, nil
}
func (s *LocalService) CreatePVE(ctx context.Context, in pvemodule.Input) (pvemodule.DTO, error) {
	secret := ""
	if in.Password != nil {
		secret = *in.Password
	}
	v := Server{Name: in.Name, Host: in.Host, Username: in.Username, Password: secret, Port: in.Port, Insecure: in.Insecure}
	if e := ctx.Err(); e != nil {
		return pvemodule.DTO{}, e
	}
	if e := s.repo.Create(ctx, &v); e != nil {
		return pvemodule.DTO{}, e
	}
	return dto(&v), nil
}
func (s *LocalService) GetPVE(ctx context.Context, id int64) (pvemodule.DTO, error) {
	v, e := s.repo.Get(ctx, id)
	if e != nil {
		return pvemodule.DTO{}, e
	}
	return dto(v), nil
}
func (s *LocalService) UpdatePVE(ctx context.Context, id int64, in pvemodule.Input) (pvemodule.DTO, error) {
	v, e := s.repo.Get(ctx, id)
	if e != nil {
		return pvemodule.DTO{}, e
	}
	if in.Password != nil && *in.Password != "" {
		v.Password = *in.Password
	}
	v.Name, v.Host, v.Username, v.Port, v.Insecure = in.Name, in.Host, in.Username, in.Port, in.Insecure
	if e = s.repo.Update(ctx, v); e != nil {
		return pvemodule.DTO{}, e
	}
	return dto(v), nil
}
func (s *LocalService) DeletePVE(ctx context.Context, id int64) error { return s.repo.Delete(ctx, id) }
func (s *LocalService) withClient(parent context.Context, id int64, fn func(*pvelib.Client) error) error {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	if e := ctx.Err(); e != nil {
		return e
	}
	v, e := s.repo.Get(ctx, id)
	if e != nil {
		return e
	}
	c := pvelib.NewClient(ctx, fmt.Sprintf("https://%s:%d", v.Host, v.Port), v.Username, v.Password, v.Insecure)
	if e = c.Login(); e != nil {
		return fmt.Errorf("PVE login: %w", e)
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	return fn(c)
}
func (s *LocalService) TestPVE(ctx context.Context, id int64) error {
	return s.withClient(ctx, id, func(c *pvelib.Client) error { _, e := c.GetNodes(); return e })
}
func (s *LocalService) PVEInventory(ctx context.Context, id int64) (pvemodule.Inventory, error) {
	var result pvemodule.Inventory
	e := s.withClient(ctx, id, func(c *pvelib.Client) error {
		nodes, e := c.GetNodes()
		if e != nil {
			return e
		}
		result.Nodes = make([]pvemodule.Node, 0, len(nodes))
		for _, node := range nodes {
			vms, e := c.GetVMs(node.Node)
			if e != nil {
				return fmt.Errorf("node %s: %w", node.Node, e)
			}
			templates, e := c.GetTemplates(node.Node)
			if e != nil {
				return fmt.Errorf("templates on node %s: %w", node.Node, e)
			}
			n := pvemodule.Node{Node: node.Node, Status: node.Status, CPU: node.CPU, MaxCPU: node.MaxCPU, Mem: node.Mem, MaxMem: node.MaxMem, Uptime: node.Uptime, VMs: make([]pvemodule.VM, 0, len(vms)), Templates: make([]pvemodule.VM, 0, len(templates))}
			for _, v := range vms {
				n.VMs = append(n.VMs, toVM(v))
			}
			for _, v := range templates {
				n.Templates = append(n.Templates, toVM(v))
			}
			result.Nodes = append(result.Nodes, n)
		}
		return nil
	})
	return result, e
}
func toVM(v pvelib.VMInfo) pvemodule.VM {
	return pvemodule.VM{VMID: v.VMID, Name: v.Name, Status: v.Status, MaxMem: v.MaxMem, MaxDisk: v.MaxDisk, Uptime: v.Uptime, CPUs: v.CPUs, Template: v.Template}
}
func (s *LocalService) CreatePVEVM(ctx context.Context, id int64, in pvemodule.CreateVMInput) (pvemodule.CreateVMResult, error) {
	var result pvemodule.CreateVMResult
	e := s.withClient(ctx, id, func(c *pvelib.Client) error {
		newID, e := c.GetNextVMID()
		if e != nil {
			return e
		}
		task, e := c.CloneVM(in.Node, in.TemplateID, pvelib.CloneVMParams{NewID: newID, Name: in.Name, Full: 1})
		if e != nil {
			return e
		}
		cores := in.Cores
		if cores < 1 {
			cores = 2
		}
		sockets := in.Sockets
		if sockets < 1 {
			sockets = 1
		}
		memory := in.MemoryMB
		if memory < 1 {
			memory = 2048
		}
		if e = c.UpdateVMConfig(in.Node, newID, map[string]string{"cores": strconv.Itoa(cores), "sockets": strconv.Itoa(sockets), "memory": strconv.Itoa(memory)}); e != nil {
			return fmt.Errorf("VM cloned as %d but configuration failed: %w", newID, e)
		}
		if in.DiskGB > 0 {
			cfg, e := c.GetVMConfig(in.Node, newID)
			if e != nil {
				return e
			}
			for _, disk := range []string{"scsi0", "scsi1", "virtio0", "virtio1", "sata0", "sata1"} {
				value, ok := cfg[disk].(string)
				if !ok {
					continue
				}
				parts := strings.Split(value, ":")
				size := strings.TrimSuffix(parts[len(parts)-1], "G")
				current, err := strconv.Atoi(size)
				if err != nil || in.DiskGB <= current {
					break
				}
				if e = c.ResizeDiskVM(in.Node, newID, disk, in.DiskGB-current); e != nil {
					return fmt.Errorf("VM cloned as %d but disk resize failed: %w", newID, e)
				}
				break
			}
		}
		if in.Start {
			if e = c.StartVM(in.Node, newID); e != nil {
				return fmt.Errorf("VM cloned as %d but start failed: %w", newID, e)
			}
		}
		result = pvemodule.CreateVMResult{VMID: newID, Task: task}
		return nil
	})
	return result, e
}

var _ pvemodule.Service = (*LocalService)(nil)
