package server

import (
	"context"
	"sync"
	"time"

	"runner/server/service"
	"runner/workflow/edit"
)

// workflowEventBindingCoordinator owns process-lifetime subscriptions for the
// Workflow workspaces that have actually been opened. It deliberately sits at
// the root HTTP composition boundary: React still consumes only the canonical
// AppSnapshot stream.
type workflowEventBindingCoordinator struct {
	ctx         context.Context
	broadcaster *AppSnapshotBroadcaster
	source      WorkflowEventSource

	mu    sync.Mutex
	stops map[edit.WorkflowKey]func()
}

// Refresh republishes non-Graph lifecycle facts (for example a settled
// Dry-run) through the existing main AppSnapshot stream. It never creates a
// page-local transport and does not change the committed mutation outcome.
func (c *workflowEventBindingCoordinator) Refresh(key edit.WorkflowKey) error {
	if c == nil || c.broadcaster == nil || c.source == nil || key.Validate() != nil {
		return service.ErrWorkflowEditConfiguration
	}
	if err := c.Ensure(key); err != nil {
		return err
	}
	replay, err := c.source.Replay(c.ctx, key, 0, 1)
	if err != nil {
		return err
	}
	snapshot, err := c.source.Snapshot(c.ctx, key)
	if err != nil {
		return err
	}
	projected := workflowAuthoritativeSnapshot(snapshot, replay.LatestSequence)
	if projected.LoadedAt.IsZero() {
		projected.LoadedAt = time.Now().UTC()
	}
	c.broadcaster.OnWorkflowSnapshot(projected)
	return nil
}

func newWorkflowEventBindingCoordinator(ctx context.Context, broadcaster *AppSnapshotBroadcaster, source WorkflowEventSource) *workflowEventBindingCoordinator {
	if ctx == nil {
		ctx = context.Background()
	}
	return &workflowEventBindingCoordinator{
		ctx:         ctx,
		broadcaster: broadcaster,
		source:      source,
		stops:       make(map[edit.WorkflowKey]func()),
	}
}

func (c *workflowEventBindingCoordinator) Ensure(key edit.WorkflowKey) error {
	if c == nil || c.broadcaster == nil || c.source == nil || key.Validate() != nil {
		return service.ErrWorkflowEditConfiguration
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.stops[key]; exists {
		return nil
	}
	stop, err := c.broadcaster.BindWorkflowEvents(c.ctx, key, c.source)
	if err != nil {
		return err
	}
	c.stops[key] = stop
	return nil
}

func (c *workflowEventBindingCoordinator) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	stops := make([]func(), 0, len(c.stops))
	for key, stop := range c.stops {
		delete(c.stops, key)
		stops = append(stops, stop)
	}
	c.mu.Unlock()
	for _, stop := range stops {
		if stop != nil {
			stop()
		}
	}
}

// WithWorkflowEventSource connects Runner's durable event facade to the
// existing AppSnapshot broadcaster. Workspaces are bound lazily when their
// authoritative graph is opened or a manual command commits.
func WithWorkflowEventSource(ctx context.Context, source WorkflowEventSource) HTTPServerOption {
	return func(s *HTTPServer) {
		s.workflowEventBindings = newWorkflowEventBindingCoordinator(ctx, s.appSnapshots, source)
	}
}

// CloseWorkflowEventBindings releases active workspace subscriptions before
// the Runner event facade is shut down.
func (s *HTTPServer) CloseWorkflowEventBindings() {
	if s != nil && s.workflowEventBindings != nil {
		s.workflowEventBindings.Close()
	}
}

func (s *HTTPServer) ensureWorkflowEventBinding(key edit.WorkflowKey) {
	if s == nil || s.workflowEventBindings == nil {
		return
	}
	// Projection is observational. A failure must never rewrite an already
	// committed mutation response into a false failure; durable replay retries
	// the binding when the workspace is opened again.
	_ = s.workflowEventBindings.Ensure(key)
}
