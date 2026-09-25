package web

import (
	"context"
	"fmt"
	"time"

	esxi "github.com/lsongdev/ohmyserver/modules/esxi"
)

// Service owns ESXi persistence and short-lived VMware API sessions.
type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }
func toDTO(v *Server) esxi.DTO {
	return esxi.DTO{ID: v.ID, Name: v.Name, Host: v.Host, Username: v.Username, Port: v.Port, Insecure: v.Insecure, HasCredential: v.Password != "", CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func (s *Service) ListESXi(ctx context.Context) ([]esxi.DTO, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]esxi.DTO, 0, len(items))
	for i := range items {
		out = append(out, toDTO(&items[i]))
	}
	return out, nil
}
func (s *Service) CreateESXi(ctx context.Context, in esxi.Input) (esxi.DTO, error) {
	secret := ""
	if in.Password != nil {
		secret = *in.Password
	}
	v := Server{Name: in.Name, Host: in.Host, Username: in.Username, Password: secret, Port: in.Port, Insecure: in.Insecure}
	if err := s.repo.Create(ctx, &v); err != nil {
		return esxi.DTO{}, err
	}
	return toDTO(&v), nil
}
func (s *Service) GetESXi(ctx context.Context, id int64) (esxi.DTO, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return esxi.DTO{}, err
	}
	return toDTO(v), nil
}
func (s *Service) UpdateESXi(ctx context.Context, id int64, in esxi.Input) (esxi.DTO, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return esxi.DTO{}, err
	}
	if in.Password != nil && *in.Password != "" {
		v.Password = *in.Password
	}
	v.Name, v.Host, v.Username, v.Port, v.Insecure = in.Name, in.Host, in.Username, in.Port, in.Insecure
	if err = s.repo.Update(ctx, v); err != nil {
		return esxi.DTO{}, err
	}
	return toDTO(v), nil
}
func (s *Service) DeleteESXi(ctx context.Context, id int64) error { return s.repo.Delete(ctx, id) }
func (s *Service) withClient(ctx context.Context, id int64, fn func(*esxi.Client) error) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	client := &esxi.Client{Name: v.Name, Host: v.Host, Username: v.Username, Password: v.Password, Port: v.Port, Insecure: v.Insecure}
	if err = client.Connect(ctx); err != nil {
		return err
	}
	defer client.Disconnect(ctx)
	return fn(client)
}
func (s *Service) TestESXi(ctx context.Context, id int64) error {
	return s.withClient(ctx, id, func(c *esxi.Client) error { return c.TestConnection(ctx) })
}
func (s *Service) ESXiInventory(ctx context.Context, id int64) (esxi.Inventory, error) {
	var result esxi.Inventory
	err := s.withClient(ctx, id, func(c *esxi.Client) error {
		vms, err := c.GetVMs(ctx)
		if err != nil {
			return err
		}
		hosts, err := c.GetHosts(ctx)
		if err != nil {
			return err
		}
		result = esxi.Inventory{VMs: vms, Hosts: hosts}
		return nil
	})
	if err != nil {
		return esxi.Inventory{}, fmt.Errorf("inventory: %w", err)
	}
	return result, nil
}
