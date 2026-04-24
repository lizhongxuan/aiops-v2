package engine

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"runner/executor"
	"runner/logging"
	"runner/modules"
	"runner/scheduler"
	"runner/state"
	"runner/workflow"
)

type Engine struct {
	Registry         *modules.Registry
	RunStore         state.RunStateStore
	Notifier         state.RunStateNotifier
	NotifyRetry      int
	NotifyDelay      time.Duration
	Verbose          bool
	Out              io.Writer
	fallbackWarnOnce sync.Once
	dispatcher       scheduler.Dispatcher
}

func New() *Engine {
	return NewWithRegistry(nil)
}

func NewWithRegistry(registry *modules.Registry) *Engine {
	if registry == nil {
		registry = DefaultRegistry()
	}
	return &Engine{
		Registry: registry,
		RunStore: state.NewInMemoryRunStore(),
	}
}

func (e *Engine) SetDispatcher(dispatcher scheduler.Dispatcher) {
	if e == nil {
		return
	}
	e.dispatcher = dispatcher
}

func (e *Engine) Apply(ctx context.Context, wf workflow.Workflow) error {
	_, err := e.ApplyWithRun(ctx, wf, RunOptions{})
	return err
}

func (e *Engine) ApplyWithState(ctx context.Context, wf workflow.Workflow) (state.RunState, error) {
	return e.ApplyWithRun(ctx, wf, RunOptions{})
}

func (e *Engine) ApplyWithRun(ctx context.Context, wf workflow.Workflow, opts RunOptions) (state.RunState, error) {
	logging.L().Debug("engine apply start",
		zap.String("workflow", wf.Name),
		zap.Int("steps", len(wf.Steps)),
	)
	if e.Registry == nil {
		e.Registry = DefaultRegistry()
	}
	store := opts.Store
	if store == nil {
		store = e.RunStore
	}
	if store == nil {
		store = state.NewInMemoryRunStore()
	}
	if isInMemoryStore(store) {
		e.fallbackWarnOnce.Do(func() {
			logging.L().Warn("run state store is in-memory only (non-durable); inject a durable RunStateStore for production")
		})
	}
	notifier := opts.Notifier
	if notifier == nil {
		notifier = e.Notifier
	}
	if opts.NotifyRetry == 0 {
		opts.NotifyRetry = e.NotifyRetry
	}
	if opts.NotifyDelay <= 0 {
		if e.NotifyDelay > 0 {
			opts.NotifyDelay = e.NotifyDelay
		} else {
			opts.NotifyDelay = 300 * time.Millisecond
		}
	}

	tracker, err := newRunTracker(wf, RunOptions{
		RunID:       opts.RunID,
		Store:       store,
		Notifier:    notifier,
		NotifyRetry: opts.NotifyRetry,
		NotifyDelay: opts.NotifyDelay,
	}, store)
	if err != nil {
		return state.RunState{}, err
	}
	if err := tracker.Start(ctx); err != nil {
		return state.RunState{}, err
	}

	baseRecorder := recorderFromContext(ctx)
	recorder := MultiRecorder(baseRecorder, tracker)
	env := envFromContext(ctx)
	dispatcher := e.dispatcher
	if dispatcher == nil {
		dispatcher = scheduler.NewHybridDispatcher(e.Registry)
	}
	runner := &dispatchRunner{
		dispatcher: dispatcher,
		verbose:    e.Verbose,
		out:        e.Out,
		recorder:   recorder,
		env:        env,
		runID:      tracker.RunID(),
	}
	exec := &executor.Executor{
		Runner:   runner,
		Observer: recorder,
	}
	err = exec.Run(ctx, wf)
	if err != nil {
		status := state.RunStatusFailed
		if ctx.Err() != nil {
			status = state.RunStatusCanceled
		}
		if finishErr := tracker.Finish(ctx, status, err.Error(), err); finishErr != nil {
			return tracker.Snapshot(), fmt.Errorf("finalize run status: %w (execution error: %v)", finishErr, err)
		}
		logging.L().Debug("engine apply failed",
			zap.String("workflow", wf.Name),
			zap.String("run_id", tracker.RunID()),
			zap.Error(err),
		)
		return tracker.Snapshot(), err
	}
	if finishErr := tracker.Finish(ctx, state.RunStatusSuccess, "", nil); finishErr != nil {
		return tracker.Snapshot(), finishErr
	}
	logging.L().Debug("engine apply done",
		zap.String("workflow", wf.Name),
		zap.String("run_id", tracker.RunID()),
	)
	return tracker.Snapshot(), nil
}

func (e *Engine) ReconcileRunning(ctx context.Context, store state.RunStateStore, reason string) (int, error) {
	targetStore := store
	if targetStore == nil {
		targetStore = e.RunStore
	}
	if targetStore == nil {
		return 0, nil
	}
	updated, err := targetStore.MarkInterruptedRunning(ctx, reason)
	if err != nil {
		return 0, err
	}
	if updated > 0 {
		logging.L().Info("reconciled interrupted runs",
			zap.Int("count", updated),
			zap.String("reason", reason),
		)
	}
	return updated, nil
}

func isInMemoryStore(store state.RunStateStore) bool {
	if store == nil {
		return true
	}
	_, ok := store.(*state.InMemoryRunStore)
	return ok
}

type dispatchRunner struct {
	dispatcher scheduler.Dispatcher
	verbose    bool
	out        io.Writer
	recorder   Recorder
	mu         sync.Mutex
	env        map[string]string
	runID      string
}

func (r *dispatchRunner) Run(ctx context.Context, step workflow.Step, host workflow.HostSpec, vars map[string]any) (executor.RunResult, error) {
	if r.dispatcher == nil {
		return executor.RunResult{}, fmt.Errorf("dispatcher is nil")
	}
	logging.L().Debug("dispatch run",
		zap.String("run_id", r.runID),
		zap.String("step", step.Name),
		zap.String("action", step.Action),
		zap.String("host", host.Name),
	)
	taskVars := r.injectEnv(vars)
	taskID := fmt.Sprintf("task-%s-%s-%d", step.Name, host.Name, time.Now().UTC().UnixNano())
	if strings.TrimSpace(r.runID) != "" {
		taskID = fmt.Sprintf("%s-%s", r.runID, taskID)
	}
	result, err := r.dispatcher.Dispatch(ctx, scheduler.Task{
		ID:    taskID,
		RunID: r.runID,
		Step:  step,
		Host:  host,
		Vars:  taskVars,
	})
	if err != nil {
		if strings.TrimSpace(result.Status) == "" {
			result.Status = state.RunStatusFailed
		}
		if strings.TrimSpace(result.Error) == "" {
			result.Error = err.Error()
		}
	}
	if strings.TrimSpace(result.Status) != "" && !strings.EqualFold(strings.TrimSpace(result.Status), "success") && strings.TrimSpace(result.Error) == "" {
		result.Error = fallbackDispatchError(result, step, host)
	}
	if r.verbose {
		r.printResult(step, host, result)
	}
	if r.recorder != nil {
		r.recorder.HostResult(step, host, result)
	}
	if step.Action == "env.set" {
		r.mergeEnvFromOutput(result.Output)
	}
	if err != nil {
		logging.L().Warn("dispatch failed",
			zap.String("run_id", r.runID),
			zap.String("step", step.Name),
			zap.String("host", host.Name),
			zap.String("status", result.Status),
			zap.String("result_error", result.Error),
			zap.String("resolved_address", readRunnerDebugString(result.Output, "resolved_address")),
			zap.Error(err),
		)
		return executor.RunResult{Output: result.Output}, err
	}
	if result.Status != "success" {
		logging.L().Warn("dispatch result not success",
			zap.String("run_id", r.runID),
			zap.String("step", step.Name),
			zap.String("host", host.Name),
			zap.String("status", result.Status),
			zap.String("result_error", result.Error),
			zap.String("resolved_address", readRunnerDebugString(result.Output, "resolved_address")),
		)
		return executor.RunResult{Output: result.Output}, fmt.Errorf("task failed: %s", result.Error)
	}
	logging.L().Debug("dispatch done",
		zap.String("run_id", r.runID),
		zap.String("step", step.Name),
		zap.String("host", host.Name),
	)
	return executor.RunResult{Output: result.Output}, nil
}

func fallbackDispatchError(result scheduler.Result, step workflow.Step, host workflow.HostSpec) string {
	resolvedAddress := readRunnerDebugString(result.Output, "resolved_address")
	parts := []string{
		fmt.Sprintf("step=%s", strings.TrimSpace(step.Name)),
		fmt.Sprintf("host=%s", strings.TrimSpace(host.Name)),
	}
	if resolvedAddress != "" {
		parts = append(parts, fmt.Sprintf("address=%s", resolvedAddress))
	}
	return fmt.Sprintf("dispatcher returned status=%s without error (%s)", strings.TrimSpace(result.Status), strings.Join(parts, ", "))
}

func readRunnerDebugString(output map[string]any, key string) string {
	if len(output) == 0 {
		return ""
	}
	debug, ok := output["runner_debug"].(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(debug[key]))
}

func (r *dispatchRunner) injectEnv(vars map[string]any) map[string]any {
	if len(r.env) == 0 {
		return vars
	}

	envCopy := map[string]any{}
	r.mu.Lock()
	for k, v := range r.env {
		envCopy[k] = v
	}
	r.mu.Unlock()

	out := make(map[string]any, len(vars)+1)
	for k, v := range vars {
		out[k] = v
	}
	out["env"] = envCopy
	return out
}

func (r *dispatchRunner) mergeEnvFromOutput(output map[string]any) {
	if len(output) == 0 {
		return
	}
	raw, ok := output["env"]
	if !ok {
		return
	}

	parsed := map[string]string{}
	switch env := raw.(type) {
	case map[string]string:
		for k, v := range env {
			parsed[k] = v
		}
	case map[string]any:
		for k, v := range env {
			parsed[k] = fmt.Sprint(v)
		}
	case map[any]any:
		for k, v := range env {
			parsed[fmt.Sprint(k)] = fmt.Sprint(v)
		}
	default:
		return
	}

	if len(parsed) == 0 {
		return
	}

	r.mu.Lock()
	if r.env == nil {
		r.env = map[string]string{}
	}
	for k, v := range parsed {
		r.env[k] = v
	}
	r.mu.Unlock()
}

func (r *dispatchRunner) printResult(step workflow.Step, host workflow.HostSpec, result scheduler.Result) {
	if r.out == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	fmt.Fprintf(r.out, "step=%s host=%s status=%s\n", step.Name, host.Name, result.Status)

	if stdout := readOutputString(result.Output, "stdout"); stdout != "" {
		fmt.Fprintf(r.out, "stdout:\n%s\n", stdout)
	}
	if stderr := readOutputString(result.Output, "stderr"); stderr != "" {
		fmt.Fprintf(r.out, "stderr:\n%s\n", stderr)
	}
}

func readOutputString(output map[string]any, key string) string {
	if len(output) == 0 {
		return ""
	}
	value, ok := output[key]
	if !ok {
		return ""
	}
	raw := fmt.Sprint(value)
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	return raw
}
