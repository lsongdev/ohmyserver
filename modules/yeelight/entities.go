package yeelight

import (
	"context"
	"time"
)

type Device struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Status    string    `json:"status"`
	LastSeen  time.Time `json:"last_seen,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Input struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
}
type State struct {
	Power     string `json:"power"`
	Bright    int    `json:"bright"`
	CT        int    `json:"ct"`
	RGB       int    `json:"rgb"`
	Hue       int    `json:"hue"`
	Sat       int    `json:"sat"`
	ColorMode int    `json:"color_mode"`
	Name      string `json:"name"`
}
type FoundDevice struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	Name string `json:"name"`
}
type Control struct {
	Action string `json:"action"`
	Value  int    `json:"value"`
	Color  string `json:"color"`
}
type Repository interface {
	Initialize(context.Context) error
	Create(context.Context, *Device) error
	List(context.Context) ([]Device, error)
	Get(context.Context, int64) (*Device, error)
	Update(context.Context, *Device) error
	Delete(context.Context, int64) error
	SetStatus(context.Context, int64, bool) error
}
type Service interface {
	ListYeelight(context.Context) ([]Device, error)
	CreateYeelight(context.Context, Input) (Device, error)
	GetYeelight(context.Context, int64) (Device, error)
	UpdateYeelight(context.Context, int64, Input) (Device, error)
	DeleteYeelight(context.Context, int64) error
	YeelightState(context.Context, int64) (State, error)
	TestYeelight(context.Context, int64) error
	ControlYeelight(context.Context, int64, string, int, string) (State, error)
	DiscoverYeelight(context.Context) ([]FoundDevice, error)
}
