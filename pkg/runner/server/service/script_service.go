package service

import (
	"context"
	"fmt"
	"strings"

	"runner/scriptstore"
	"runner/workflow"
)

type ScriptService struct {
	store scriptstore.Store
}

func NewScriptService(store scriptstore.Store) *ScriptService {
	return &ScriptService{store: store}
}

func (s *ScriptService) List(ctx context.Context, filter ScriptFilter) ([]*ScriptRecord, error) {
	items, err := s.store.List(ctx, scriptstore.Filter{
		Language: filter.Language,
		Tag:      filter.Tag,
		Limit:    filter.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*ScriptRecord, 0, len(items))
	for _, item := range items {
		record := toScriptRecord(item)
		out = append(out, &record)
	}
	return out, nil
}

func (s *ScriptService) Get(ctx context.Context, name string) (*ScriptRecord, error) {
	item, err := s.store.Get(ctx, name)
	if err != nil {
		if err == scriptstore.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	record := toScriptRecord(item)
	return &record, nil
}

func (s *ScriptService) Create(ctx context.Context, record *ScriptRecord) error {
	if record == nil {
		return fmt.Errorf("%w: empty script record", ErrInvalid)
	}
	_, err := s.store.Create(ctx, scriptstore.Script{
		Name:        strings.TrimSpace(record.Name),
		Language:    strings.TrimSpace(record.Language),
		Description: record.Description,
		Tags:        append([]string{}, record.Tags...),
		Content:     record.Content,
	})
	if err != nil {
		switch err {
		case scriptstore.ErrExists:
			return ErrAlreadyExists
		default:
			return fmt.Errorf("%w: %v", ErrInvalid, err)
		}
	}
	return nil
}

func (s *ScriptService) Update(ctx context.Context, name string, record *ScriptRecord) error {
	if record == nil {
		return fmt.Errorf("%w: empty script record", ErrInvalid)
	}
	_, err := s.store.Update(ctx, name, scriptstore.Script{
		Language:    strings.TrimSpace(record.Language),
		Description: record.Description,
		Tags:        append([]string{}, record.Tags...),
		Content:     record.Content,
	})
	if err != nil {
		switch err {
		case scriptstore.ErrNotFound:
			return ErrNotFound
		default:
			return fmt.Errorf("%w: %v", ErrInvalid, err)
		}
	}
	return nil
}

func (s *ScriptService) Delete(ctx context.Context, name string) error {
	err := s.store.Delete(ctx, name)
	if err == scriptstore.ErrNotFound {
		return ErrNotFound
	}
	return err
}

func (s *ScriptService) Render(ctx context.Context, name string, vars map[string]any) (string, error) {
	item, err := s.store.Get(ctx, name)
	if err != nil {
		if err == scriptstore.ErrNotFound {
			return "", ErrNotFound
		}
		return "", err
	}
	return workflow.RenderString(item.Content, vars), nil
}

func toScriptRecord(item scriptstore.Script) ScriptRecord {
	return ScriptRecord{
		Name:        item.Name,
		Language:    item.Language,
		Description: item.Description,
		Tags:        append([]string{}, item.Tags...),
		Content:     item.Content,
		Version:     item.Version,
		Checksum:    item.Checksum,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}
