package web

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	yeelight "github.com/lsongdev/ohmyserver/modules/yeelight"
	yeelightlib "github.com/lsongdev/yeelight-go/yeelight"
)

type LocalService struct{ repo *Repository }

func NewLocalService(repo *Repository) *LocalService                       { return &LocalService{repo: repo} }
func (s *LocalService) ListYeelight(ctx context.Context) ([]Device, error) { return s.repo.List(ctx) }
func (s *LocalService) CreateYeelight(ctx context.Context, in Input) (Device, error) {
	v := Device{Name: in.Name, Host: in.Host, Port: in.Port}
	if err := s.repo.Create(ctx, &v); err != nil {
		return Device{}, err
	}
	return v, nil
}
func (s *LocalService) GetYeelight(ctx context.Context, id int64) (Device, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return Device{}, err
	}
	return *v, nil
}
func (s *LocalService) UpdateYeelight(ctx context.Context, id int64, in Input) (Device, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return Device{}, err
	}
	v.Name, v.Host, v.Port = in.Name, in.Host, in.Port
	if err = s.repo.Update(ctx, v); err != nil {
		return Device{}, err
	}
	return *v, nil
}
func (s *LocalService) DeleteYeelight(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
func deviceClient(v *Device) *yeelightlib.Yeelight {
	return yeelightlib.New(&yeelightlib.Config{Host: v.Host, Port: v.Port, Timeout: 3 * time.Second})
}
func readState(ctx context.Context, client *yeelightlib.Yeelight) (State, error) {
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	result, err := client.GetProp("power", "bright", "ct", "rgb", "hue", "sat", "color_mode", "name")
	if err != nil {
		return State{}, fmt.Errorf("read Yeelight state: %w", err)
	}
	if len(result.Result) < 8 {
		return State{}, fmt.Errorf("Yeelight returned incomplete state")
	}
	values := make([]string, 8)
	for i, v := range result.Result {
		values[i], _ = v.(string)
	}
	out := State{Power: values[0]}
	out.Bright, _ = strconv.Atoi(values[1])
	out.CT, _ = strconv.Atoi(values[2])
	if n, e := strconv.ParseInt(values[3], 10, 64); e == nil {
		out.RGB = int(n)
	}
	out.Hue, _ = strconv.Atoi(values[4])
	out.Sat, _ = strconv.Atoi(values[5])
	out.ColorMode, _ = strconv.Atoi(values[6])
	out.Name = values[7]
	return out, nil
}
func (s *LocalService) YeelightState(ctx context.Context, id int64) (State, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return State{}, err
	}
	state, err := readState(ctx, deviceClient(v))
	_ = s.repo.SetStatus(ctx, id, err == nil)
	return state, err
}
func (s *LocalService) TestYeelight(ctx context.Context, id int64) error {
	_, err := s.YeelightState(ctx, id)
	return err
}
func (s *LocalService) ControlYeelight(ctx context.Context, id int64, action string, value int, color string) (State, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return State{}, err
	}
	if err = ctx.Err(); err != nil {
		return State{}, err
	}
	client := deviceClient(v)
	effect := &yeelightlib.Effect{Effect: "smooth", Duration: 500}
	var result *yeelightlib.CommandResult
	switch action {
	case "on":
		result, err = client.SetPower("on", effect, yeelightlib.Normal)
	case "off":
		result, err = client.SetPower("off", effect, yeelightlib.Normal)
	case "toggle":
		result, err = client.Toggle()
	case "brightness":
		result, err = client.SetBright(value, effect)
	case "color_temperature":
		result, err = client.SetCT(value, effect)
	case "color":
		rgb, parseErr := strconv.ParseInt(strings.TrimPrefix(color, "#"), 16, 32)
		if parseErr != nil {
			return State{}, fmt.Errorf("invalid RGB color: %w", parseErr)
		}
		result, err = client.SetRGB(int(rgb), effect)
	}
	if err != nil {
		return State{}, err
	}
	if result != nil && result.Error != nil {
		return State{}, fmt.Errorf("Yeelight error %d: %s", result.Error.Code, result.Error.Message)
	}
	return s.YeelightState(ctx, id)
}
func (s *LocalService) DiscoverYeelight(ctx context.Context) ([]FoundDevice, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lights, err := yeelightlib.Discover()
	if err != nil {
		return nil, err
	}
	out := make([]FoundDevice, 0, len(lights))
	for _, light := range lights {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out = append(out, FoundDevice{Host: light.GetHost(), Port: light.GetPort(), Name: light.GetHost()})
	}
	return out, nil
}

var _ yeelight.Service = (*LocalService)(nil)
