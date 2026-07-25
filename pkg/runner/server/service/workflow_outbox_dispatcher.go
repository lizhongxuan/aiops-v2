package service

import (
	"context"
	"fmt"
	"time"

	"runner/workflow/edit"
)

// WorkflowEventDelivery is the process-local handoff used by the durable
// outbox. Consumers recover missed deliveries from FileWorkflowEventLog.
type WorkflowEventDelivery func(context.Context, edit.WorkflowDomainEvent) error

type WorkflowOutboxDispatchResult struct {
	Delivered int `json:"delivered"`
	Retried   int `json:"retried"`
}

// WorkflowOutboxDispatcher delivers one WorkflowKey in sequence order. A
// failure stops the partition so a later event cannot overtake it.
type WorkflowOutboxDispatcher struct {
	outbox     *FileWorkflowOutbox
	deliver    WorkflowEventDelivery
	retryDelay time.Duration
}

func NewWorkflowOutboxDispatcher(outbox *FileWorkflowOutbox, deliver WorkflowEventDelivery, retryDelay time.Duration) *WorkflowOutboxDispatcher {
	if retryDelay <= 0 {
		retryDelay = time.Second
	}
	return &WorkflowOutboxDispatcher{outbox: outbox, deliver: deliver, retryDelay: retryDelay}
}

func (d *WorkflowOutboxDispatcher) Dispatch(ctx context.Context, key edit.WorkflowKey, now time.Time, limit int) (WorkflowOutboxDispatchResult, error) {
	if d == nil || d.outbox == nil || d.deliver == nil || now.IsZero() {
		return WorkflowOutboxDispatchResult{}, fmt.Errorf("%w: dispatcher is not configured", ErrWorkflowOutboxState)
	}
	records, err := d.outbox.Pending(ctx, key, now, limit)
	if err != nil {
		return WorkflowOutboxDispatchResult{}, err
	}
	var result WorkflowOutboxDispatchResult
	for _, record := range records {
		if err := d.deliver(ctx, record.Event); err != nil {
			if retryErr := d.outbox.Retry(ctx, key, record.Event.EventID, err, now.Add(d.retryDelay)); retryErr != nil {
				return result, retryErr
			}
			result.Retried++
			break
		}
		if err := d.outbox.Ack(ctx, key, record.Event.EventID, now); err != nil {
			return result, err
		}
		result.Delivered++
	}
	return result, nil
}
