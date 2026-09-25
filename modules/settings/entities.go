package settings

import (
	"context"
	"time"
)

type Setting struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Value       string    `json:"value"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Repository interface {
	Initialize(context.Context) error
	Create(context.Context, string, string, string, string) (*Setting, error)
	List(context.Context) ([]Setting, error)
	Get(context.Context, int64) (*Setting, error)
	GetByName(context.Context, string) (*Setting, error)
	Update(context.Context, int64, string) error
	Delete(context.Context, int64) error
}
