package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"runner/server/store/envstore"
)

type EnvironmentService struct {
	store envstore.Store
}

func NewEnvironmentService(store envstore.Store) *EnvironmentService {
	return &EnvironmentService{store: store}
}

func (s *EnvironmentService) List(ctx context.Context) ([]*envstore.EnvironmentRecord, error) {
	items, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*envstore.EnvironmentRecord, 0, len(items))
	for i := range items {
		item := items[i]
		out = append(out, &item)
	}
	return out, nil
}

func (s *EnvironmentService) Get(ctx context.Context, name string) (*envstore.EnvironmentRecord, error) {
	name, err := validateEnvironmentName(name)
	if err != nil {
		return nil, err
	}
	item, err := s.store.Get(ctx, name)
	if err != nil {
		if errors.Is(err, envstore.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (s *EnvironmentService) Create(ctx context.Context, record *envstore.EnvironmentRecord) error {
	if record == nil {
		return fmt.Errorf("%w: empty environment record", ErrInvalid)
	}
	name, err := validateEnvironmentName(record.Name)
	if err != nil {
		return err
	}
	record.Name = name
	if _, err := s.store.Create(ctx, *record); err != nil {
		if errors.Is(err, envstore.ErrExists) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (s *EnvironmentService) AddVar(ctx context.Context, name string, envVar envstore.EnvVar) error {
	name, err := validateEnvironmentName(name)
	if err != nil {
		return err
	}
	envVar, err = normalizeEnvVar(envVar)
	if err != nil {
		return err
	}
	if _, err := s.store.AddVar(ctx, name, envVar); err != nil {
		if errors.Is(err, envstore.ErrNotFound) {
			return ErrNotFound
		}
		if errors.Is(err, envstore.ErrVarExist) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (s *EnvironmentService) UpdateVar(ctx context.Context, name, key string, envVar envstore.EnvVar) error {
	name, err := validateEnvironmentName(name)
	if err != nil {
		return err
	}
	key, err = validateEnvironmentKey(key)
	if err != nil {
		return err
	}
	envVar, err = normalizeEnvVar(envVar)
	if err != nil {
		return err
	}
	if envVar.Key != key {
		return fmt.Errorf("%w: variable key mismatch", ErrInvalid)
	}
	if _, err := s.store.UpdateVar(ctx, name, key, envVar); err != nil {
		if errors.Is(err, envstore.ErrNotFound) || errors.Is(err, envstore.ErrVarMiss) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *EnvironmentService) DeleteVar(ctx context.Context, name, key string) error {
	name, err := validateEnvironmentName(name)
	if err != nil {
		return err
	}
	key, err = validateEnvironmentKey(key)
	if err != nil {
		return err
	}
	if _, err := s.store.DeleteVar(ctx, name, key); err != nil {
		if errors.Is(err, envstore.ErrNotFound) || errors.Is(err, envstore.ErrVarMiss) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func validateEnvironmentName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", fmt.Errorf("%w: environment name is required", ErrInvalid)
	case strings.Contains(name, "/"), strings.Contains(name, "\\"), strings.Contains(name, ".."):
		return "", fmt.Errorf("%w: invalid environment name", ErrInvalid)
	default:
		return name, nil
	}
}

func validateEnvironmentKey(key string) (string, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	switch {
	case key == "":
		return "", fmt.Errorf("%w: variable key is required", ErrInvalid)
	case strings.Contains(key, " "), strings.Contains(key, "/"), strings.Contains(key, "\\"):
		return "", fmt.Errorf("%w: invalid variable key", ErrInvalid)
	default:
		return key, nil
	}
}

func normalizeEnvVar(envVar envstore.EnvVar) (envstore.EnvVar, error) {
	key, err := validateEnvironmentKey(envVar.Key)
	if err != nil {
		return envstore.EnvVar{}, err
	}
	envVar.Key = key
	envVar.Description = strings.TrimSpace(envVar.Description)
	return envVar, nil
}
