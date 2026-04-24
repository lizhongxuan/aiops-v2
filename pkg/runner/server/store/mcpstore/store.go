package mcpstore

import "context"

type Store interface {
	List(ctx context.Context) ([]ServerRecord, error)
	Get(ctx context.Context, id string) (ServerRecord, error)
	Create(ctx context.Context, record ServerRecord) (ServerRecord, error)
	Update(ctx context.Context, id string, record ServerRecord) (ServerRecord, error)
	Delete(ctx context.Context, id string) error
}
