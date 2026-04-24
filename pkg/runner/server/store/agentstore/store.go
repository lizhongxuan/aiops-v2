package agentstore

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("agent not found")
	ErrExists   = errors.New("agent already exists")
)

type Store interface {
	List(ctx context.Context, filter Filter) ([]AgentRecord, error)
	Get(ctx context.Context, id string) (AgentRecord, error)
	Create(ctx context.Context, record AgentRecord) (AgentRecord, error)
	Update(ctx context.Context, id string, record AgentRecord) (AgentRecord, error)
	Delete(ctx context.Context, id string) error
	Heartbeat(ctx context.Context, id string, beat Heartbeat) (AgentRecord, error)
}
