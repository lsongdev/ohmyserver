package web

import (
	"context"
	"fmt"
	"strings"
	"time"

	idraclib "github.com/lsongdev/idrac-go/idrac"
	idrac "github.com/lsongdev/ohmyserver/modules/idrac"
)

type LocalService struct{ repo *Repository }

func NewLocalService(repo *Repository) *LocalService { return &LocalService{repo: repo} }
func toDTO(v *Server) idrac.DTO {
	return idrac.DTO{ID: v.ID, Name: v.Name, Host: v.Host, Port: v.Port, Username: v.Username, Version: v.Version, Status: v.Status, Description: v.Description, HasCredential: v.Password != "", CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func (s *LocalService) ListIDRAC(ctx context.Context) ([]idrac.DTO, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]idrac.DTO, 0, len(items))
	for i := range items {
		out = append(out, toDTO(&items[i]))
	}
	return out, nil
}
func (s *LocalService) CreateIDRAC(ctx context.Context, in idrac.Input) (idrac.DTO, error) {
	password := ""
	if in.Password != nil {
		password = *in.Password
	}
	v := Server{Name: in.Name, Host: in.Host, Port: in.Port, Username: in.Username, Password: password, Version: in.Version, Description: in.Description}
	if err := s.repo.Create(ctx, &v); err != nil {
		return idrac.DTO{}, err
	}
	return toDTO(&v), nil
}
func (s *LocalService) GetIDRAC(ctx context.Context, id int64) (idrac.DTO, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return idrac.DTO{}, err
	}
	return toDTO(v), nil
}
func (s *LocalService) UpdateIDRAC(ctx context.Context, id int64, in idrac.Input) (idrac.DTO, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return idrac.DTO{}, err
	}
	if in.Password != nil && *in.Password != "" {
		v.Password = *in.Password
	}
	v.Name, v.Host, v.Port, v.Username, v.Version, v.Description = in.Name, in.Host, in.Port, in.Username, in.Version, in.Description
	if err = s.repo.Update(ctx, v); err != nil {
		return idrac.DTO{}, err
	}
	return toDTO(v), nil
}
func (s *LocalService) DeleteIDRAC(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
func (s *LocalService) withClient(ctx context.Context, id int64, fn func(*idrac.Client) error) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	client := &idrac.Client{Host: v.Host, Username: v.Username, Password: v.Password}
	if err = client.Connect(ctx); err != nil {
		return err
	}
	defer client.Disconnect(ctx)
	return fn(client)
}
func (s *LocalService) TestIDRAC(ctx context.Context, id int64) error {
	return s.withClient(ctx, id, func(c *idrac.Client) error { return c.TestConnection(ctx) })
}
func (s *LocalService) ReadIDRACInventory(ctx context.Context, id int64) (idrac.Inventory, error) {
	var result idrac.Inventory
	err := s.withClient(ctx, id, func(c *idrac.Client) error {
		system, err := c.GetSystemInfo(ctx)
		if err != nil {
			return err
		}
		power, err := c.GetPowerState(ctx)
		if err != nil {
			return err
		}
		sensors, err := c.GetHardwareStatus(ctx)
		if err != nil {
			return err
		}
		result = idrac.Inventory{System: system, PowerState: power, Sensors: sensors}
		return nil
	})
	return result, err
}
func (s *LocalService) ControlIDRACPower(ctx context.Context, id int64, action string) error {
	var state idraclib.PowerState
	switch strings.ToLower(action) {
	case "on":
		state = idraclib.PowerOn
	case "off":
		state = idraclib.PowerOff
	case "graceful":
		state = idraclib.PowerGracefulShutdown
	case "restart":
		state = idraclib.PowerRestart
	case "pushbutton":
		state = idraclib.PowerPushPowerButton
	case "nmi":
		state = idraclib.PowerNmi
	default:
		return fmt.Errorf("unsupported power action %q", action)
	}
	return s.withClient(ctx, id, func(c *idrac.Client) error { return c.ControlPower(ctx, state) })
}

var _ idrac.Service = (*LocalService)(nil)
