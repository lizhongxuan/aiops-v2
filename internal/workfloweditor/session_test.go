package workfloweditor

import "testing"

func TestSessionBudgetPausesAfterThreePatchReviews(t *testing.T) {
	store := NewSessionStore()
	session := store.Start(CreateSessionRequest{WorkflowID: "workflow", BaseRevision: "rev"})

	for i := 0; i < 3; i++ {
		var err error
		session, err = store.RecordPatchReview(session.ID)
		if err != nil {
			t.Fatalf("RecordPatchReview(%d) error = %v", i, err)
		}
	}
	if session.Status != sessionStatusBudgetPaused {
		t.Fatalf("Status = %q, want budget_paused", session.Status)
	}
	if _, err := store.RecordPatchReview(session.ID); err == nil {
		t.Fatal("expected budget_paused error after three reviews")
	}
	session, err := store.Continue(session.ID)
	if err != nil {
		t.Fatalf("Continue() error = %v", err)
	}
	if session.Status != sessionStatusActive || session.StepBudget.UsedPatchReviews != 0 {
		t.Fatalf("session after continue = %#v, want active with reset budget", session)
	}
}

func TestSessionKeepsWorkflowIDStableAcrossRevisionChanges(t *testing.T) {
	store := NewSessionStore()
	session := store.Start(CreateSessionRequest{WorkflowID: "workflow", BaseRevision: "rev-1"})
	session.ActiveRevision = "rev-2"
	store.Save(session)
	got, ok := store.Get(session.ID)
	if !ok {
		t.Fatal("session not found")
	}
	if got.WorkflowID != "workflow" || got.BaseRevision != "rev-1" || got.ActiveRevision != "rev-2" {
		t.Fatalf("session = %#v, want stable workflow id and updated active revision", got)
	}
}

func TestSessionToolLogRefIsStableForDrawerSession(t *testing.T) {
	store := NewSessionStore()
	session := store.Start(CreateSessionRequest{DrawerSessionID: "drawer-1", WorkflowID: "workflow"})
	session.Status = "custom"
	store.Save(session)
	got, ok := store.Get("drawer-1")
	if !ok {
		t.Fatal("session not found")
	}
	if got.ToolLogRef != "workflow-ai-tool-log/drawer-1" {
		t.Fatalf("ToolLogRef = %q, want stable drawer session ref", got.ToolLogRef)
	}
}
