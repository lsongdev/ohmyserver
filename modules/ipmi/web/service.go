package web

import (
	"context"
	"time"

	ipmi "github.com/lsongdev/ohmyserver/modules/ipmi"
)

type LocalService struct{ repo *Repository }

func NewLocalService(repo *Repository) *LocalService { return &LocalService{repo: repo} }
func dto(v *Server) *ipmi.ServerDTO {
	return &ipmi.ServerDTO{ID: v.ID, Name: v.Name, BMCIP: v.BMCIP, Port: v.Port, Username: v.Username, HasCredential: v.Password != "", Status: v.Status, Description: v.Description, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func (s *LocalService) ListIPMIServers(ctx context.Context) ([]ipmi.ServerDTO, error) {
	rows, e := s.repo.List(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]ipmi.ServerDTO, 0, len(rows))
	for i := range rows {
		out = append(out, *dto(&rows[i]))
	}
	return out, nil
}
func (s *LocalService) CreateIPMIServerAPI(ctx context.Context, in ipmi.CreateRequest) (*ipmi.ServerDTO, error) {
	v := Server{Name: in.Name, BMCIP: in.BMCIP, Port: in.Port, Username: in.Username, Password: in.Password, Description: in.Description}
	if e := s.repo.Create(ctx, &v); e != nil {
		return nil, e
	}
	return dto(&v), nil
}
func (s *LocalService) ReadIPMIServer(ctx context.Context, id int64) (*ipmi.ServerDTO, error) {
	v, e := s.repo.Get(ctx, id)
	if e != nil {
		return nil, e
	}
	return dto(v), nil
}
func (s *LocalService) UpdateIPMIServerAPI(ctx context.Context, id int64, in ipmi.UpdateRequest) (*ipmi.ServerDTO, error) {
	v, e := s.repo.Get(ctx, id)
	if e != nil {
		return nil, e
	}
	v.Name, v.BMCIP, v.Port, v.Username, v.Description = in.Name, in.BMCIP, in.Port, in.Username, in.Description
	if in.Password != nil && *in.Password != "" {
		v.Password = *in.Password
	}
	if e = s.repo.Update(ctx, v); e != nil {
		return nil, e
	}
	return dto(v), nil
}
func (s *LocalService) RemoveIPMIServerAPI(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
func (s *LocalService) withClient(ctx context.Context, id int64, fn func(*ipmi.Client) error) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	v, e := s.repo.Get(ctx, id)
	if e != nil {
		return e
	}
	c := &ipmi.Client{BMCIP: v.BMCIP, Port: v.Port, Username: v.Username, Password: v.Password}
	if e = c.Connect(ctx); e != nil {
		return e
	}
	defer c.Disconnect(ctx)
	return fn(c)
}
func (s *LocalService) TestIPMIConnection(ctx context.Context, id int64) error {
	return s.withClient(ctx, id, func(c *ipmi.Client) error { return c.TestConnection(ctx) })
}
func (s *LocalService) IPMIInventory(ctx context.Context, id int64) (*ipmi.Inventory, error) {
	result := &ipmi.Inventory{FRUs: []ipmi.FRU{}}
	e := s.withClient(ctx, id, func(c *ipmi.Client) error {
		host, he := c.GetHostInfo(ctx)
		frus, fe := c.GetFRUInfo(ctx)
		if he == nil {
			result.Host = host
		} else {
			result.ConnectionError = he.Error()
		}
		if fe != nil {
			if result.ConnectionError == "" {
				result.ConnectionError = fe.Error()
			}
			return nil
		}
		for _, f := range frus {
			v := ipmi.FRU{ID: f.DeviceID()}
			if f.BoardInfoArea != nil {
				v.BoardManufacturer = string(f.BoardInfoArea.Manufacturer)
				v.BoardProductName = string(f.BoardInfoArea.ProductName)
				v.BoardSerial = string(f.BoardInfoArea.SerialNumber)
				v.BoardPartNumber = string(f.BoardInfoArea.PartNumber)
			}
			if f.ProductInfoArea != nil {
				v.ProductManufacturer = string(f.ProductInfoArea.Manufacturer)
				v.ProductName = string(f.ProductInfoArea.Name)
				v.ProductSerial = string(f.ProductInfoArea.SerialNumber)
				v.ProductPartNumber = string(f.ProductInfoArea.PartModel)
				v.ProductVersion = string(f.ProductInfoArea.Version)
			}
			if f.ChassisInfoArea != nil {
				v.ChassisPartNumber = string(f.ChassisInfoArea.PartNumber)
				v.ChassisSerial = string(f.ChassisInfoArea.SerialNumber)
			}
			result.FRUs = append(result.FRUs, v)
		}
		return nil
	})
	if e != nil {
		return result, e
	}
	return result, nil
}
func (s *LocalService) ControlIPMIPower(ctx context.Context, id int64, action string) error {
	return s.withClient(ctx, id, func(c *ipmi.Client) error { return c.ControlPower(ctx, action) })
}

var _ ipmi.Service = (*LocalService)(nil)
