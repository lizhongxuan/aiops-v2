package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"runner/logging"
	"runner/scheduler"
	"runner/state"
	"runner/workflow"
)

type RunOptions struct {
	RunID       string
	Store       state.RunStateStore
	Notifier    state.RunStateNotifier
	NotifyRetry int
	NotifyDelay time.Duration
}

type runTracker struct {
	mu          sync.Mutex
	store       state.RunStateStore
	notifier    state.RunStateNotifier
	notifyRetry int
	notifyDelay time.Duration
	run         state.RunState
	started     bool
}

func newRunTracker(wf workflow.Workflow, opts RunOptions, fallbackStore state.RunStateStore) (*runTracker, error) {
	runID := strings.TrimSpace(opts.RunID)
	if runID == "" {
		runID = state.NewRunID()
	}
	if err := state.ValidateRunID(runID); err != nil {
		return nil, err
	}
	store := opts.Store
	if store == nil {
		store = fallbackStore
	}
	if store == nil {
		return nil, fmt.Errorf("run state store is nil")
	}
	notifyDelay := opts.NotifyDelay
	if notifyDelay <= 0 {
		notifyDelay = 300 * time.Millisecond
	}

	now := time.Now().UTC()
	tracker := &runTracker{
		store:       store,
		notifier:    opts.Notifier,
		notifyRetry: opts.NotifyRetry,
		notifyDelay: notifyDelay,
		run: state.RunState{
			RunID:           runID,
			WorkflowName:    strings.TrimSpace(wf.Name),
			WorkflowVersion: strings.TrimSpace(wf.Version),
			Status:          state.RunStatusQueued,
			Version:         1,
			StartedAt:       now,
			UpdatedAt:       now,
			Steps:           []state.StepState{},
		},
	}
	return tracker, nil
}

func (t *runTracker) Start(ctx context.Context) error {
	t.mu.Lock()
	run := state.CloneRunState(t.run)
	t.mu.Unlock()

	if err := t.store.CreateRun(ctx, run); err != nil {
		return err
	}
	t.mu.Lock()
	t.started = true
	t.mu.Unlock()

	if err := t.transitionRun(ctx, state.RunStatusRunning, "", ""); err != nil {
		return err
	}
	return nil
}

func (t *runTracker) Finish(ctx context.Context, status, message string, runErr error) error {
	if strings.TrimSpace(status) == "" {
		status = state.RunStatusSuccess
	}
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}
	return t.transitionRun(ctx, status, message, errText)
}

func (t *runTracker) RunID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.run.RunID
}

func (t *runTracker) Snapshot() state.RunState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return state.CloneRunState(t.run)
}

func (t *runTracker) StepStart(step workflow.Step, targets []workflow.HostSpec) {
	t.mu.Lock()
	now := time.Now().UTC()
	t.run.UpsertStepStart(step.Name, now)
	t.run.UpdatedAt = now
	t.run.Version++
	run := state.CloneRunState(t.run)
	t.mu.Unlock()

	if err := t.store.UpdateRun(context.Background(), run); err != nil {
		logging.L().Warn("run tracker persist step start failed",
			zap.String("run_id", run.RunID),
			zap.String("step", step.Name),
			zap.Error(err),
		)
	}

	t.notifyAsync(state.RunStateCallback{
		RunID:        run.RunID,
		WorkflowName: run.WorkflowName,
		Status:       run.Status,
		Step:         step.Name,
		Timestamp:    now,
		Version:      run.Version,
	})
}

func (t *runTracker) StepFinish(step workflow.Step, status string) {
	if strings.TrimSpace(status) == "" {
		status = state.RunStatusSuccess
	}

	t.mu.Lock()
	now := time.Now().UTC()
	stepStatus := strings.ToLower(strings.TrimSpace(status))
	t.run.UpsertStepFinish(step.Name, stepStatus, "", now)
	t.run.UpdatedAt = now
	t.run.Version++
	run := state.CloneRunState(t.run)
	t.mu.Unlock()

	if err := t.store.UpdateRun(context.Background(), run); err != nil {
		logging.L().Warn("run tracker persist step finish failed",
			zap.String("run_id", run.RunID),
			zap.String("step", step.Name),
			zap.Error(err),
		)
	}

	t.notifyAsync(state.RunStateCallback{
		RunID:        run.RunID,
		WorkflowName: run.WorkflowName,
		Status:       run.Status,
		Step:         step.Name,
		Timestamp:    now,
		Version:      run.Version,
	})
}

func (t *runTracker) HostResult(step workflow.Step, host workflow.HostSpec, result scheduler.Result) {
	t.mu.Lock()
	now := time.Now().UTC()
	hostResult := state.HostResult{
		Host:      host.Name,
		Status:    strings.TrimSpace(result.Status),
		Message:   strings.TrimSpace(result.Error),
		Output:    copyMap(result.Output),
		StartedAt: now,
	}
	if strings.TrimSpace(result.Status) != state.RunStatusRunning {
		hostResult.FinishedAt = now
	}
	t.run.UpsertHostResult(step.Name, hostResult)
	t.run.Args = mergeRunArgs(t.run.Args, result.Output)
	if strings.EqualFold(hostResult.Status, state.RunStatusFailed) && hostResult.Message != "" {
		t.run.LastError = hostResult.Message
	}
	t.run.UpdatedAt = now
	t.run.Version++
	run := state.CloneRunState(t.run)
	t.mu.Unlock()

	if err := t.store.UpdateRun(context.Background(), run); err != nil {
		logging.L().Warn("run tracker persist host result failed",
			zap.String("run_id", run.RunID),
			zap.String("step", step.Name),
			zap.String("host", host.Name),
			zap.Error(err),
		)
	}

	t.notifyAsync(state.RunStateCallback{
		RunID:        run.RunID,
		WorkflowName: run.WorkflowName,
		Status:       run.Status,
		Step:         step.Name,
		Host:         host.Name,
		Timestamp:    now,
		Error:        hostResult.Message,
		Version:      run.Version,
	})
}

func (t *runTracker) transitionRun(ctx context.Context, status, message, runErr string) error {
	now := time.Now().UTC()
	nextStatus := strings.TrimSpace(strings.ToLower(status))
	if nextStatus == "" {
		nextStatus = state.RunStatusSuccess
	}
	nextMessage := strings.TrimSpace(message)
	nextError := strings.TrimSpace(runErr)
	if nextMessage == "" && nextError != "" {
		nextMessage = nextError
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if err := state.ValidateRunTransition(t.run.Status, nextStatus); err != nil {
		return err
	}
	t.run.Status = nextStatus
	t.run.Message = nextMessage
	t.run.LastError = nextError
	if state.IsTerminalRunStatus(nextStatus) {
		t.run.FinishedAt = now
	}
	t.run.UpdatedAt = now
	t.run.Version++
	next := state.CloneRunState(t.run)

	if !t.started {
		if err := t.store.CreateRun(ctx, next); err != nil {
			return err
		}
		t.started = true
	} else {
		if err := t.store.UpdateRun(ctx, next); err != nil {
			return err
		}
	}

	t.notifyAsync(state.RunStateCallback{
		RunID:        next.RunID,
		WorkflowName: next.WorkflowName,
		Status:       next.Status,
		Timestamp:    now,
		Error:        next.LastError,
		Version:      next.Version,
	})
	return nil
}

func (t *runTracker) notifyAsync(payload state.RunStateCallback) {
	if t.notifier == nil {
		return
	}
	go func() {
		retries := t.notifyRetry
		if retries < 0 {
			retries = 0
		}
		var err error
		for attempt := 0; attempt <= retries; attempt++ {
			err = t.notifier.NotifyRunState(context.Background(), payload)
			if err == nil {
				return
			}
			if attempt < retries {
				time.Sleep(t.notifyDelay)
			}
		}
		t.recordNotifyError(err)
	}()
}

func (t *runTracker) recordNotifyError(err error) {
	if err == nil {
		return
	}
	t.mu.Lock()
	t.run.LastNotifyError = err.Error()
	t.run.UpdatedAt = time.Now().UTC()
	t.run.Version++
	run := state.CloneRunState(t.run)
	t.mu.Unlock()

	if updateErr := t.store.UpdateRun(context.Background(), run); updateErr != nil {
		logging.L().Warn("run tracker notify error persist failed",
			zap.String("run_id", run.RunID),
			zap.Error(updateErr),
		)
	}
}

func copyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func mergeRunArgs(current map[string]any, output map[string]any) map[string]any {
	exported := readRunArgsFromOutput(output)
	if len(exported) == 0 {
		return current
	}
	merged := current
	if merged == nil {
		merged = map[string]any{}
	}
	for k, v := range exported {
		merged[k] = v
	}
	return merged
}

func readRunArgsFromOutput(output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	raw, ok := output["vars"]
	if !ok || raw == nil {
		return nil
	}

	switch vars := raw.(type) {
	case map[string]any:
		if len(vars) == 0 {
			return nil
		}
		out := make(map[string]any, len(vars))
		for k, v := range vars {
			out[k] = v
		}
		return out
	case map[string]string:
		if len(vars) == 0 {
			return nil
		}
		out := make(map[string]any, len(vars))
		for k, v := range vars {
			out[k] = v
		}
		return out
	case map[any]any:
		if len(vars) == 0 {
			return nil
		}
		out := make(map[string]any, len(vars))
		for k, v := range vars {
			out[fmt.Sprint(k)] = v
		}
		return out
	default:
		return nil
	}
}
