package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"runner/executor"
	"runner/logging"
	"runner/scheduler"
	"runner/state"
	"runner/workflow"
)

type RunOptions struct {
	RunID           string
	Store           state.RunStateStore
	Notifier        state.RunStateNotifier
	NotifyRetry     int
	NotifyDelay     time.Duration
	ApprovalRuntime executor.ApprovalRuntime
	SubflowRuntime  executor.SubflowRuntime
}

type runTracker struct {
	mu                   sync.Mutex
	store                state.RunStateStore
	notifier             state.RunStateNotifier
	notifyRetry          int
	notifyDelay          time.Duration
	run                  state.RunState
	started              bool
	activeLoopIterations map[string]loopIterationContext
}

type loopIterationContext struct {
	LoopID string
	Index  int
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
			Graph:           initialGraphRunState(wf, now),
		},
	}
	return tracker, nil
}

func initialGraphRunState(wf workflow.Workflow, now time.Time) *state.GraphRunState {
	if wf.XRunnerGraph == nil {
		return nil
	}
	graph := &state.GraphRunState{
		GraphVersion: wf.XRunnerGraph.Version,
		Nodes:        map[string]state.NodeState{},
		Edges:        map[string]state.EdgeState{},
		UpdatedAt:    now,
	}
	for _, node := range wf.XRunnerGraph.Nodes {
		id := strings.TrimSpace(node.ID)
		if id == "" {
			continue
		}
		graph.Nodes[id] = state.NodeState{
			ID:       id,
			Name:     graphNodeName(node),
			Type:     strings.TrimSpace(node.Type),
			ParentID: strings.TrimSpace(node.ParentID),
			Status:   state.RunStatusQueued,
		}
	}
	for _, edge := range wf.XRunnerGraph.Edges {
		id := strings.TrimSpace(edge.ID)
		if id == "" {
			continue
		}
		graph.Edges[id] = state.EdgeState{
			ID:     id,
			Source: strings.TrimSpace(edge.Source),
			Target: strings.TrimSpace(edge.Target),
			Kind:   strings.TrimSpace(edge.Kind),
			Status: state.RunStatusQueued,
		}
	}
	return graph
}

func graphNodeName(node workflow.GraphNodeSpec) string {
	for _, value := range []string{node.Step, node.StepName, node.Data.StepName, node.StepID, node.Handler, node.HandlerName, node.Data.HandlerName, node.Label, node.ID} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return strings.TrimSpace(node.ID)
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
	if nodeID, ok := t.graphNodeIDForStepLocked(step.Name, step.ID); ok {
		if ctx, ok := t.activeLoopIterationForNodeLocked(nodeID); ok {
			t.run.UpsertGraphNodeIterationNodeStartByID(ctx.LoopID, ctx.Index, nodeID, now)
		} else {
			t.run.UpsertGraphNodeStartByID(nodeID, now)
		}
	}
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
	if nodeID, ok := t.graphNodeIDForStepLocked(step.Name, step.ID); ok {
		if ctx, ok := t.activeLoopIterationForNodeLocked(nodeID); ok {
			t.run.UpsertGraphNodeIterationNodeFinishByID(ctx.LoopID, ctx.Index, nodeID, stepStatus, "", now)
		} else {
			t.run.UpsertGraphNodeFinishByID(nodeID, stepStatus, "", now)
		}
	}
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
	if nodeID, ok := t.graphNodeIDForStepLocked(step.Name, step.ID); ok {
		if ctx, ok := t.activeLoopIterationForNodeLocked(nodeID); ok {
			t.run.UpsertStepHostResult(step.Name, hostResult)
			t.run.UpsertGraphNodeIterationHostResultByID(ctx.LoopID, ctx.Index, nodeID, hostResult)
		} else {
			t.run.UpsertHostResult(step.Name, hostResult)
		}
	} else {
		t.run.UpsertStepHostResult(step.Name, hostResult)
	}
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

func (t *runTracker) GraphNodeStart(nodeID string) {
	t.mu.Lock()
	now := time.Now().UTC()
	nodeID = strings.TrimSpace(nodeID)
	if ctx, ok := t.activeLoopIterationForNodeLocked(nodeID); ok {
		t.run.UpsertGraphNodeIterationNodeStartByID(ctx.LoopID, ctx.Index, nodeID, now)
	} else {
		t.run.UpsertGraphNodeStartByID(nodeID, now)
	}
	t.run.UpdatedAt = now
	t.run.Version++
	run := state.CloneRunState(t.run)
	t.mu.Unlock()

	if err := t.store.UpdateRun(context.Background(), run); err != nil {
		logging.L().Warn("run tracker persist graph node start failed",
			zap.String("run_id", run.RunID),
			zap.String("node_id", nodeID),
			zap.Error(err),
		)
	}
}

func (t *runTracker) GraphNodeFinish(nodeID, status, message string) {
	t.mu.Lock()
	now := time.Now().UTC()
	nodeID = strings.TrimSpace(nodeID)
	if ctx, ok := t.activeLoopIterationForNodeLocked(nodeID); ok {
		t.run.UpsertGraphNodeIterationNodeFinishByID(ctx.LoopID, ctx.Index, nodeID, strings.ToLower(strings.TrimSpace(status)), strings.TrimSpace(message), now)
	} else {
		t.run.UpsertGraphNodeFinishByID(nodeID, strings.ToLower(strings.TrimSpace(status)), strings.TrimSpace(message), now)
	}
	t.run.UpdatedAt = now
	t.run.Version++
	run := state.CloneRunState(t.run)
	t.mu.Unlock()

	if err := t.store.UpdateRun(context.Background(), run); err != nil {
		logging.L().Warn("run tracker persist graph node finish failed",
			zap.String("run_id", run.RunID),
			zap.String("node_id", nodeID),
			zap.Error(err),
		)
	}
}

func (t *runTracker) GraphApprovalWaiting(nodeID string) {
	t.mu.Lock()
	now := time.Now().UTC()
	t.run.UpsertGraphNodeWaitingByID(strings.TrimSpace(nodeID), now)
	t.run.UpdatedAt = now
	t.run.Version++
	run := state.CloneRunState(t.run)
	t.mu.Unlock()

	if err := t.store.UpdateRun(context.Background(), run); err != nil {
		logging.L().Warn("run tracker persist graph approval waiting failed",
			zap.String("run_id", run.RunID),
			zap.String("node_id", nodeID),
			zap.Error(err),
		)
	}
}

func (t *runTracker) GraphApprovalResolved(nodeID, status, message string) {
	t.mu.Lock()
	now := time.Now().UTC()
	t.run.UpsertGraphNodeFinishByID(strings.TrimSpace(nodeID), strings.ToLower(strings.TrimSpace(status)), strings.TrimSpace(message), now)
	t.run.UpdatedAt = now
	t.run.Version++
	run := state.CloneRunState(t.run)
	t.mu.Unlock()

	if err := t.store.UpdateRun(context.Background(), run); err != nil {
		logging.L().Warn("run tracker persist graph approval resolved failed",
			zap.String("run_id", run.RunID),
			zap.String("node_id", nodeID),
			zap.Error(err),
		)
	}
}

func (t *runTracker) GraphNodeIterationStart(nodeID string, iteration int, item any) {
	t.mu.Lock()
	now := time.Now().UTC()
	nodeID = strings.TrimSpace(nodeID)
	t.run.UpsertGraphNodeIterationStartByID(nodeID, iteration, item, now)
	if t.activeLoopIterations == nil {
		t.activeLoopIterations = map[string]loopIterationContext{}
	}
	t.activeLoopIterations[nodeID] = loopIterationContext{LoopID: nodeID, Index: iteration}
	t.run.UpdatedAt = now
	t.run.Version++
	run := state.CloneRunState(t.run)
	t.mu.Unlock()

	if err := t.store.UpdateRun(context.Background(), run); err != nil {
		logging.L().Warn("run tracker persist graph node iteration start failed",
			zap.String("run_id", run.RunID),
			zap.String("node_id", nodeID),
			zap.Int("iteration", iteration),
			zap.Error(err),
		)
	}
}

func (t *runTracker) GraphNodeIterationFinish(nodeID string, iteration int, status, message string) {
	t.mu.Lock()
	now := time.Now().UTC()
	nodeID = strings.TrimSpace(nodeID)
	t.run.UpsertGraphNodeIterationFinishByID(nodeID, iteration, strings.ToLower(strings.TrimSpace(status)), strings.TrimSpace(message), now)
	delete(t.activeLoopIterations, nodeID)
	t.run.UpdatedAt = now
	t.run.Version++
	run := state.CloneRunState(t.run)
	t.mu.Unlock()

	if err := t.store.UpdateRun(context.Background(), run); err != nil {
		logging.L().Warn("run tracker persist graph node iteration finish failed",
			zap.String("run_id", run.RunID),
			zap.String("node_id", nodeID),
			zap.Int("iteration", iteration),
			zap.Error(err),
		)
	}
}

func (t *runTracker) GraphEdgeSelected(edge workflow.GraphEdgeSpec) {
	t.mu.Lock()
	now := time.Now().UTC()
	t.run.UpsertGraphEdgeSelected(strings.TrimSpace(edge.ID), now)
	t.run.UpdatedAt = now
	t.run.Version++
	run := state.CloneRunState(t.run)
	t.mu.Unlock()

	if err := t.store.UpdateRun(context.Background(), run); err != nil {
		logging.L().Warn("run tracker persist graph edge selected failed",
			zap.String("run_id", run.RunID),
			zap.String("edge_id", edge.ID),
			zap.Error(err),
		)
	}
}

func (t *runTracker) graphNodeIDForStepLocked(stepName, stepID string) (string, bool) {
	if t.run.Graph == nil {
		return "", false
	}
	stepName = strings.TrimSpace(stepName)
	stepID = strings.TrimSpace(stepID)
	if stepID != "" {
		if _, ok := t.run.Graph.Nodes[stepID]; ok {
			return stepID, true
		}
	}
	for id, node := range t.run.Graph.Nodes {
		if node.Name == stepName || node.Name == stepID || id == stepID {
			return id, true
		}
	}
	return "", false
}

func (t *runTracker) activeLoopIterationForNodeLocked(nodeID string) (loopIterationContext, bool) {
	if t.run.Graph == nil || len(t.activeLoopIterations) == 0 {
		return loopIterationContext{}, false
	}
	node, ok := t.run.Graph.Nodes[strings.TrimSpace(nodeID)]
	if !ok {
		return loopIterationContext{}, false
	}
	parentID := strings.TrimSpace(node.ParentID)
	if parentID == "" {
		return loopIterationContext{}, false
	}
	ctx, ok := t.activeLoopIterations[parentID]
	return ctx, ok
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
