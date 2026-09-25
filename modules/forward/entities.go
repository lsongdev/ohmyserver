package forward

import (
	"context"
	"time"
)

type Listener struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Protocol    string    `json:"protocol"`
	ListenPort  int       `json:"listen_port"`
	ForwardHost string    `json:"forward_host"`
	ForwardPort int       `json:"forward_port"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Repository interface {
	Initialize(context.Context) error
	List(context.Context) ([]Listener, error)
	Get(context.Context, int64) (*Listener, error)
	ByPort(context.Context, int) (*Listener, error)
	Create(context.Context, *Listener) error
	Update(context.Context, Listener) error
	SetStatus(context.Context, int64, string) error
	Delete(context.Context, int64) error
}
type Service interface {
	CreateForwardListener(context.Context, *Listener) error
	UpdateForwardListener(context.Context, Listener) error
	DeleteForwardListener(context.Context, int64) error
	ForwardListenerAction(context.Context, int64, string) error
}
