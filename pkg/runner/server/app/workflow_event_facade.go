package app

import (
	"context"
	"time"

	"runner/server/service"
	"runner/workflow/edit"
)

// WorkflowEventFacade is the Runner/root boundary for durable replay and
// process-local subscriptions. Shutdown stops delivery workers; persisted log
// and outbox records remain recoverable by the next process.
type WorkflowEventFacade struct {
	events *service.WorkflowDomainEventService
}

func (r *Runtime) OpenWorkflowEventFacade() (*WorkflowEventFacade, error) {
	events, err := service.NewWorkflowDomainEventService(r.WorkflowEditApplication())
	if err != nil {
		return nil, err
	}
	return &WorkflowEventFacade{events: events}, nil
}

func (f *WorkflowEventFacade) Replay(ctx context.Context, key edit.WorkflowKey, afterSequence uint64, limit int) (service.WorkflowEventReplay, error) {
	return f.events.Replay(ctx, key, afterSequence, limit)
}

func (f *WorkflowEventFacade) Recover(ctx context.Context, key edit.WorkflowKey) error {
	return f.events.Recover(ctx, key)
}

func (f *WorkflowEventFacade) Snapshot(ctx context.Context, key edit.WorkflowKey) (service.WorkflowEditSnapshot, error) {
	return f.events.Snapshot(ctx, key)
}

func (f *WorkflowEventFacade) Subscribe(ctx context.Context, key edit.WorkflowKey, afterSequence uint64) (<-chan edit.WorkflowDomainEvent, func(), error) {
	return f.events.Subscribe(ctx, key, afterSequence)
}

func (f *WorkflowEventFacade) Dispatch(ctx context.Context, key edit.WorkflowKey, now time.Time, limit int) (service.WorkflowOutboxDispatchResult, error) {
	return f.events.Dispatch(ctx, key, now, limit)
}

func (f *WorkflowEventFacade) Shutdown(ctx context.Context) error { return f.events.Shutdown(ctx) }
