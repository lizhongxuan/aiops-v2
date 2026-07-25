package observability

import (
	"context"
	"strings"

	"runner/server/service"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type WorkflowObserver struct {
	tracer  trace.Tracer
	metrics *WorkflowMetrics
}

func NewWorkflowObserver(tracer trace.Tracer, metrics *WorkflowMetrics) WorkflowObserver {
	if tracer == nil {
		tracer = trace.NewNoopTracerProvider().Tracer("aiops-v2/workflow")
	}
	if metrics == nil {
		metrics = NewWorkflowMetrics()
	}
	return WorkflowObserver{tracer: tracer, metrics: metrics}
}

func (o WorkflowObserver) ObserveWorkflowEdit(ctx context.Context, observation service.WorkflowEditObservation) error {
	ctx = normalizeContext(ctx)
	_, span := o.tracer.Start(ctx, "workflow."+string(observation.Operation))
	attrs := []attribute.KeyValue{
		attribute.String("workflow.operation", boundedWorkflowAttribute(string(observation.Operation))),
		attribute.String("workflow.outcome", boundedWorkflowAttribute(string(observation.Outcome))),
		attribute.String("workflow.tenant_id", boundedWorkflowAttribute(observation.Key.TenantID)),
		attribute.String("workflow.id", boundedWorkflowAttribute(observation.Key.WorkflowID)),
		attribute.String("workflow.session_id", boundedWorkflowAttribute(observation.SessionID)),
		attribute.String("workflow.turn_id", boundedWorkflowAttribute(observation.TurnID)),
		attribute.String("workflow.tool_call_id", boundedWorkflowAttribute(observation.ToolCallID)),
		attribute.String("workflow.candidate_id", boundedWorkflowAttribute(observation.CandidateID)),
		attribute.String("workflow.change_set_id", boundedWorkflowAttribute(observation.ChangeSetID)),
		attribute.String("workflow.transaction_id", boundedWorkflowAttribute(observation.TransactionID)),
		attribute.String("workflow.event_id", boundedWorkflowAttribute(observation.EventID)),
		attribute.String("workflow.run_id", boundedWorkflowAttribute(observation.RunID)),
		attribute.String("workflow.error_code", boundedWorkflowAttribute(string(observation.ErrorCode))),
		attribute.Bool("workflow.receipt_replay", observation.ReceiptReplay),
		attribute.Int64("workflow.duration_ms", observation.Duration.Milliseconds()),
		attribute.Int64("workflow.event_lag_ms", observation.EventLag.Milliseconds()),
		attribute.Int64("workflow.gap_size", int64(observation.GapSize)),
	}
	span.SetAttributes(attrs...)
	if observation.Outcome != service.WorkflowEditOutcomeSuccess {
		span.SetStatus(codes.Error, string(observation.Outcome))
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
	o.metrics.Record(observation)
	return nil
}

func (o WorkflowObserver) Metrics() WorkflowMetricsSnapshot {
	return o.metrics.Snapshot()
}

func boundedWorkflowAttribute(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		return value[:128]
	}
	return value
}
