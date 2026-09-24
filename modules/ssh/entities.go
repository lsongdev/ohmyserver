package ssh

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrNotFound = errors.New("SSH server not found")
var ErrInvalid = errors.New("invalid SSH server")
var ErrConnection = errors.New("SSH connection failed")
var ErrDisabled = errors.New("SSH server is disabled")

type Profile struct {
	ID                                         int64
	Name, Host                                 string
	Port                                       int
	Username, AuthType                         string
	Password, PrivateKey, HostKey, Description string
	Enabled                                    bool
	CreatedAt, UpdatedAt                       time.Time
}
type Stats struct {
	Connected     bool      `json:"connected"`
	ConnectedAt   time.Time `json:"connected_at,omitempty"`
	ClientCount   int       `json:"client_count"`
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryPercent float64   `json:"memory_percent"`
	MemoryUsage   uint64    `json:"memory_usage"`
	MemoryTotal   uint64    `json:"memory_total"`
	NetworkRX     uint64    `json:"network_rx"`
	NetworkTX     uint64    `json:"network_tx"`
	LoadAverage   []float64 `json:"load_average"`
}
type Metrics struct {
	Host                                string
	CPUUsage                            float64
	CPUCores                            int
	MemoryTotal, MemoryUsed, MemoryFree uint64
	MemoryUsage                         float64
	DiskTotal, DiskUsed, DiskFree       uint64
	DiskUsage                           float64
	NetworkReceived, NetworkSent        uint64
	Uptime                              uint64
	Load1, Load5, Load15                float64
}
type Server struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	AuthType      string `json:"auth_type"`
	HostKey       string `json:"host_key,omitempty"`
	Description   string `json:"description,omitempty"`
	Enabled       bool   `json:"enabled"`
	HasCredential bool   `json:"has_credential"`
}
type Input struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	AuthType    string `json:"auth_type"`
	Password    string `json:"password"`
	PrivateKey  string `json:"private_key"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
}
type Service interface {
	ListSSHProfiles(context.Context) ([]Server, error)
	GetSSHProfile(context.Context, int64) (Server, error)
	SaveSSHProfile(context.Context, int64, Input) (Server, error)
	DeleteSSHProfile(context.Context, int64) error
	TestSSHProfile(context.Context, Input) error
	SSHStats(context.Context, int64) (Stats, error)
	SSHMetrics(context.Context, int64) (Metrics, error)
}

func Validate(in *Input) error {
	in.Name, in.Host, in.Username = strings.TrimSpace(in.Name), strings.TrimSpace(in.Host), strings.TrimSpace(in.Username)
	if in.Name == "" || in.Host == "" || in.Username == "" {
		return ErrInvalid
	}
	if in.Port == 0 {
		in.Port = 22
	}
	if in.Port < 1 || in.Port > 65535 {
		return ErrInvalid
	}
	if in.AuthType != "password" && in.AuthType != "key" {
		return ErrInvalid
	}
	if in.AuthType == "password" && strings.TrimSpace(in.Password) == "" {
		return ErrInvalid
	}
	if in.AuthType == "key" && strings.TrimSpace(in.PrivateKey) == "" {
		return ErrInvalid
	}
	return nil
}
