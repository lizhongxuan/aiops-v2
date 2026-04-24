package scriptstore

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("script not found")
	ErrExists   = errors.New("script already exists")
)

type Filter struct {
	Language string
	Tag      string
	Limit    int
}

type Store interface {
	List(ctx context.Context, filter Filter) ([]Script, error)
	Get(ctx context.Context, name string) (Script, error)
	Create(ctx context.Context, script Script) (Script, error)
	Update(ctx context.Context, name string, script Script) (Script, error)
	Delete(ctx context.Context, name string) error
}
