package runnerembed

import (
	"context"
	"path/filepath"

	runnerapp "runner/server/app"
	runnerservice "runner/server/service"
)

type Options struct {
	DataDir                    string
	WorkflowReferenceGuardMode runnerservice.WorkflowReferenceGuardMode
}

type Runtime = runnerapp.Runtime

func NewRuntime(ctx context.Context, opts Options) (*Runtime, error) {
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = ".data"
	}
	return runnerapp.NewRuntime(ctx, runnerapp.RuntimeOptions{
		Config:                     ConfigFromDataDir(filepath.Clean(dataDir)),
		WorkflowReferenceGuardMode: opts.WorkflowReferenceGuardMode,
	})
}
