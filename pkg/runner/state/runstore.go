package state

import (
	"context"
	"errors"
)

var (
	ErrRunNotFound = errors.New("run state not found")
	ErrRunExists   = errors.New("run state already exists")
)

type ListFilter struct {
	Status string
	Limit  int
}

type RunStateStore interface {
	CreateRun(ctx context.Context, run RunState) error
	UpdateRun(ctx context.Context, run RunState) error
	GetRun(ctx context.Context, runID string) (RunState, error)
	ListRuns(ctx context.Context, filter ListFilter) ([]RunState, error)
	MarkInterruptedRunning(ctx context.Context, reason string) (int, error)
}
