package skillstore

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("skill not found")
	ErrExists   = errors.New("skill already exists")
)

type Store interface {
	List(ctx context.Context) ([]SkillRecord, error)
	Get(ctx context.Context, name string) (SkillRecord, error)
	Create(ctx context.Context, record SkillRecord) (SkillRecord, error)
	Update(ctx context.Context, name string, record SkillRecord) (SkillRecord, error)
	Delete(ctx context.Context, name string) error
}
