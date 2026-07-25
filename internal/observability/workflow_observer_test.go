package observability

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"runner/server/service"
	"runner/workflow/edit"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestWorkflowObserverRecordsBoundedTraceMetricsAndZeroToleranceAlerts(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	metrics := NewWorkflowMetrics()
	observer := NewWorkflowObserver(provider.Tracer("test"), metrics)
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}

	observations := []service.WorkflowEditObservation{
		{Operation: service.WorkflowEditObservePreview, Outcome: service.WorkflowEditOutcomeSuccess, Key: key, SessionID: "session-a", TurnID: "turn-a", ToolCallID: "call-a", CandidateID: "candidate-a", ChangeSetID: "change-a", Duration: 20 * time.Millisecond},
		{Operation: service.WorkflowEditObserveApply, Outcome: service.WorkflowEditOutcomeSuccess, Key: key, TransactionID: "transaction-a", ReceiptReplay: true, Duration: 30 * time.Millisecond},
		{Operation: service.WorkflowEditObserveValidate, Outcome: service.WorkflowEditOutcomeConflict, Key: key, ErrorCode: edit.ErrorStaleRevision, Duration: 10 * time.Millisecond},
		{Operation: service.WorkflowEditObserveApply, Outcome: service.WorkflowEditOutcomeAuthorizationDenied, Key: key, ErrorCode: edit.ErrorApprovalRequired},
		{Operation: service.WorkflowEditObserveRecovery, Outcome: service.WorkflowEditOutcomeFailure, Key: key},
		{Operation: service.WorkflowEditObserveGap, Outcome: service.WorkflowEditOutcomeGapUnrecoverable, Key: key, GapSize: 2},
		{Operation: service.WorkflowEditObservePostCheck, Outcome: service.WorkflowEditOutcomeInvalidCompleted, Key: key},
	}
	for _, observation := range observations {
		if err := observer.ObserveWorkflowEdit(context.Background(), observation); err != nil {
			t.Fatal(err)
		}
	}

	snapshot := metrics.Snapshot()
	if snapshot.Operations[string(service.WorkflowEditObservePreview)].Count != 1 || snapshot.Operations[string(service.WorkflowEditObserveApply)].Count != 2 || snapshot.Signals[WorkflowMetricReceiptReplay] != 1 || snapshot.Signals[WorkflowMetricConflict] != 1 || snapshot.Signals[WorkflowMetricAuthorizationDeny] != 1 {
		t.Fatalf("metrics snapshot = %#v", snapshot)
	}
	for _, alert := range []string{WorkflowAlertRecoveryFailed, WorkflowAlertUnauthorizedMutation, WorkflowAlertEventGapUnrecoverable, WorkflowAlertInvalidCompleted} {
		if snapshot.Alerts[alert] != 1 {
			t.Fatalf("alert %q = %d, snapshot=%#v", alert, snapshot.Alerts[alert], snapshot)
		}
	}

	spans := recorder.Ended()
	if len(spans) != len(observations) {
		t.Fatalf("ended spans = %d", len(spans))
	}
	var renderedKeys strings.Builder
	for _, span := range spans {
		for _, item := range span.Attributes() {
			fmt.Fprint(&renderedKeys, item.Key, " ")
		}
	}
	text := strings.ToLower(renderedKeys.String())
	for _, forbidden := range []string{"graph", "prompt", "password", "authorization", "raw_input"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("workflow span leaked forbidden attribute %q: %s", forbidden, text)
		}
	}
	assertSpanAttribute(t, spans[0].Attributes(), "workflow.session_id", "session-a")
	assertSpanAttribute(t, spans[0].Attributes(), "workflow.change_set_id", "change-a")
}
