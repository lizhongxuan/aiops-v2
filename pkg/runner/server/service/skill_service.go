package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"runner/server/store/skillstore"
)

type SkillService struct {
	store skillstore.Store
}

func NewSkillService(store skillstore.Store) *SkillService {
	return &SkillService{store: store}
}

func (s *SkillService) List(ctx context.Context) ([]*skillstore.SkillRecord, error) {
	items, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*skillstore.SkillRecord, 0, len(items))
	for i := range items {
		item := items[i]
		out = append(out, &item)
	}
	return out, nil
}

func (s *SkillService) Get(ctx context.Context, name string) (*skillstore.SkillRecord, error) {
	item, err := s.store.Get(ctx, strings.TrimSpace(name))
	if err != nil {
		if errors.Is(err, skillstore.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (s *SkillService) Create(ctx context.Context, record *skillstore.SkillRecord) error {
	if record == nil {
		return fmt.Errorf("%w: empty skill record", ErrInvalid)
	}
	if strings.TrimSpace(record.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if _, err := s.store.Create(ctx, *record); err != nil {
		if errors.Is(err, skillstore.ErrExists) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (s *SkillService) Update(ctx context.Context, name string, record *skillstore.SkillRecord) error {
	if record == nil {
		return fmt.Errorf("%w: empty skill record", ErrInvalid)
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if _, err := s.store.Update(ctx, name, *record); err != nil {
		if errors.Is(err, skillstore.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *SkillService) Delete(ctx context.Context, name string) error {
	if err := s.store.Delete(ctx, strings.TrimSpace(name)); err != nil {
		if errors.Is(err, skillstore.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}
