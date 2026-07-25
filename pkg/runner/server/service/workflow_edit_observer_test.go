package service

import (
	"context"
	"testing"
	"time"

	"runner/workflow/edit"
)

func TestWorkflowEditObserverCorrelatesPreviewValidateApplyReplayAndAudit(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	observer := &recordingWorkflowEditObserver{}
	fixture.app.SetObserver(observer)

	if _, err := fixture.app.ValidateChange(context.Background(), edit.ValidateRequest{SchemaVersion: edit.SchemaVersion, WorkflowKey: fixture.request.WorkflowKey, ChangeSetID: fixture.request.ChangeSetID}); err != nil {
		t.Fatal(err)
	}
	receipt, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request); err != nil {
		t.Fatal(err)
	}

	observations := observer.snapshot()
	if len(observations) != 3 {
		t.Fatalf("observations = %#v", observations)
	}
	if observations[0].Operation != WorkflowEditObserveValidate || observations[0].ChangeSetID != fixture.request.ChangeSetID || observations[0].Outcome != WorkflowEditOutcomeSuccess {
		t.Fatalf("validate observation = %#v", observations[0])
	}
	if observations[1].Operation != WorkflowEditObserveApply || observations[1].TransactionID != receipt.TransactionID || observations[1].CandidateID != fixture.request.CandidateID || observations[1].ApprovalID != fixture.request.ApplyApprovalID || observations[1].ReceiptReplay {
		t.Fatalf("apply observation = %#v", observations[1])
	}
	if observations[2].Operation != WorkflowEditObserveApply || !observations[2].ReceiptReplay || observations[2].TransactionID != receipt.TransactionID {
		t.Fatalf("replay observation = %#v", observations[2])
	}
	for _, observation := range observations {
		if observation.Key != fixture.request.WorkflowKey || observation.Duration < 0 || observation.StartedAt.IsZero() {
			t.Fatalf("invalid common observation = %#v", observation)
		}
	}

	audit, err := fixture.app.QueryAudit(context.Background(), WorkflowEditAuditQuery{
		Key: fixture.request.WorkflowKey, TransactionID: receipt.TransactionID, ActorID: fixture.request.ActorID,
		From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour),
	})
	if err != nil || len(audit.Records) != 1 {
		t.Fatalf("QueryAudit() = %#v, %v", audit, err)
	}
	record := audit.Records[0]
	if record.TransactionID != receipt.TransactionID || record.BeforeRevision != receipt.RevisionBefore || record.AfterRevision != *receipt.RevisionAfter || record.Details["approvalId"] != fixture.request.ApplyApprovalID || record.Details["risk"] == "" || record.Details["result"] != receipt.Status {
		t.Fatalf("audit record = %#v", record)
	}
}

func TestWorkflowEditObserverPanicCannotInterruptCommittedMutation(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	fixture.app.SetObserver(panickingWorkflowEditObserver{})
	receipt, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request)
	if err != nil || receipt.TransactionID == "" {
		t.Fatalf("Apply() = %#v, %v", receipt, err)
	}
	snapshot, err := fixture.commits.Snapshot(context.Background(), fixture.request.WorkflowKey)
	if err != nil || snapshot.ResourceVersion != receipt.RevisionAfter.ResourceVersion {
		t.Fatalf("committed snapshot = %#v, %v", snapshot, err)
	}
}

func TestWorkflowEditObserverReportsInvalidCompletedFromRealPostCheckGuard(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	observer := &recordingWorkflowEditObserver{}
	fixture.app.SetObserver(observer)
	receipt, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request)
	if err != nil {
		t.Fatal(err)
	}

	_, err = fixture.app.RecordPostCheck(context.Background(), WorkflowPostCheckFacts{
		Key:           fixture.request.WorkflowKey,
		TransactionID: receipt.TransactionID,
		Basis:         receipt.Validation.Basis,
		Outcome:       WorkflowPostCheckPassed,
		Diagnostics:   []edit.WorkflowDiagnostic{{Code: "INVALID_COMPLETED", Severity: "error", Message: "completed result still has a blocking diagnostic"}},
	})
	if err == nil {
		t.Fatal("RecordPostCheck() accepted an invalid completed result")
	}

	observations := observer.snapshot()
	last := observations[len(observations)-1]
	if last.Operation != WorkflowEditObservePostCheck || last.Outcome != WorkflowEditOutcomeInvalidCompleted || last.TransactionID != receipt.TransactionID {
		t.Fatalf("post-check observation = %#v", last)
	}
}

type recordingWorkflowEditObserver struct {
	observations []WorkflowEditObservation
}

func (o *recordingWorkflowEditObserver) ObserveWorkflowEdit(_ context.Context, observation WorkflowEditObservation) error {
	o.observations = append(o.observations, observation)
	return nil
}

func (o *recordingWorkflowEditObserver) snapshot() []WorkflowEditObservation {
	return append([]WorkflowEditObservation(nil), o.observations...)
}

type panickingWorkflowEditObserver struct{}

func (panickingWorkflowEditObserver) ObserveWorkflowEdit(context.Context, WorkflowEditObservation) error {
	panic("telemetry exporter failed")
}
