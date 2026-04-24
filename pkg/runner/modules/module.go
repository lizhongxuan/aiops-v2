package modules

import (
	"context"
	"io"

	"runner/workflow"
)

type Request struct {
	Step   workflow.Step
	Host   workflow.HostSpec
	Vars   map[string]any
	Stdout io.Writer
	Stderr io.Writer
}

type Result struct {
	Changed bool
	Diff    map[string]any
	Output  map[string]any
}

type Module interface {
	Check(ctx context.Context, req Request) (Result, error)
	Apply(ctx context.Context, req Request) (Result, error)
	Rollback(ctx context.Context, req Request) (Result, error)
}
