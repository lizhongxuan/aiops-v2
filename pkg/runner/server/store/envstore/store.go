package envstore

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("environment not found")
	ErrExists   = errors.New("environment already exists")
	ErrVarExist = errors.New("environment variable already exists")
	ErrVarMiss  = errors.New("environment variable not found")
)

type Store interface {
	List(ctx context.Context) ([]EnvironmentRecord, error)
	Get(ctx context.Context, name string) (EnvironmentRecord, error)
	Create(ctx context.Context, record EnvironmentRecord) (EnvironmentRecord, error)
	AddVar(ctx context.Context, name string, envVar EnvVar) (EnvironmentRecord, error)
	UpdateVar(ctx context.Context, name, key string, envVar EnvVar) (EnvironmentRecord, error)
	DeleteVar(ctx context.Context, name, key string) (EnvironmentRecord, error)
}
