package widgets

import (
	"context"
	"time"
)

// Repository is the storage contract consumed by the widgets web module.
type Repository interface {
	List(context.Context, bool) ([]Widget, error)
	Get(context.Context, int64) (*Widget, error)
	Create(context.Context, *Widget) error
	Update(context.Context, Widget) error
	Delete(context.Context, int64) error
}

type Widget struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Config    string    `json:"config"`
	SortOrder int       `json:"sort_order"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
