package runtimekernel

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel/toolfailure"
)

func TestClassifyActiveTurnMigrationDecision(t *testing.T) {
	now := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		turn TurnSnapshot
		want ActiveTurnMigrationDecision
	}{
		{
			name: "resumable suspended approval with pending approval",
			turn: TurnSnapshot{
				ID: "turn-resumable", SessionID: "session-1", SessionType: SessionTypeHost, Mode: ModeExecute,
				Lifecycle: TurnLifecycleSuspended, ResumeState: TurnResumeStatePendingApproval,
				StartedAt: now, UpdatedAt: now,
				PendingApprovals: []PendingApproval{{
					ID: "approval-1", SessionID: "session-1", TurnID: "turn-resumable", Iteration: 1,
					ToolCallID: "call-1", ToolName: "exec_command", CreatedAt: now, UpdatedAt: now,
				}},
				Iterations: []IterationState{{
					ID: "iter-1", SessionID: "session-1", TurnID: "turn-resumable", Iteration: 1,
					Lifecycle: TurnLifecycleSuspended, ResumeState: TurnResumeStatePendingApproval,
					StartedAt: now, UpdatedAt: now,
				}},
			},
			want: ActiveTurnMigrationDecisionResumable,
		},
		{
			name: "blocked without pending data requires manual close",
			turn: TurnSnapshot{
				ID: "turn-manual", SessionID: "session-1", SessionType: SessionTypeHost, Mode: ModeExecute,
				Lifecycle: TurnLifecycleSuspended, ResumeState: TurnResumeStatePendingApproval,
				StartedAt: now, UpdatedAt: now,
				Iterations: []IterationState{{
					ID: "iter-1", SessionID: "session-1", TurnID: "turn-manual", Iteration: 1,
					Lifecycle: TurnLifecycleSuspended, ResumeState: TurnResumeStatePendingApproval,
					StartedAt: now, UpdatedAt: now,
				}},
			},
			want: ActiveTurnMigrationDecisionRequiresManualClose,
		},
		{
			name: "running without checkpoint or iterations is unrecoverable",
			turn: TurnSnapshot{
				ID: "turn-bad", SessionID: "session-1", SessionType: SessionTypeHost, Mode: ModeExecute,
				Lifecycle: TurnLifecycleRunning, ResumeState: TurnResumeStateNone,
				StartedAt: now, UpdatedAt: now,
			},
			want: ActiveTurnMigrationDecisionUnrecoverable,
		},
		{
			name: "completed is terminal noop",
			turn: TurnSnapshot{
				ID: "turn-done", SessionID: "session-1", SessionType: SessionTypeHost, Mode: ModeExecute,
				Lifecycle: TurnLifecycleCompleted, ResumeState: TurnResumeStateNone,
				StartedAt: now, UpdatedAt: now, CompletedAt: &now,
			},
			want: ActiveTurnMigrationDecisionTerminalNoop,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyActiveTurnForMigration(&tc.turn, now)
			if got.Decision != tc.want {
				t.Fatalf("Decision = %q, want %q, reasons=%v", got.Decision, tc.want, got.Reasons)
			}
		})
	}
}

func TestBuildActiveTurnMigrationReportScansCurrentAndHistory(t *testing.T) {
	now := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	session := &SessionState{
		ID:   "session-1",
		Type: SessionTypeHost,
		Mode: ModeExecute,
		CurrentTurn: &TurnSnapshot{
			ID: "turn-current", SessionID: "session-1", SessionType: SessionTypeHost, Mode: ModeExecute,
			Lifecycle: TurnLifecycleRunning, ResumeState: TurnResumeStateNone,
			StartedAt: now, UpdatedAt: now,
			Iterations: []IterationState{{
				ID: "iter-1", SessionID: "session-1", TurnID: "turn-current", Iteration: 1,
				Lifecycle: TurnLifecycleRunning, ResumeState: TurnResumeStateNone,
				ToolCalls: []ToolCall{{
					ID:        "call-1",
					Name:      "exec_command",
					Arguments: json.RawMessage(`{"secret":"raw-argument-must-not-leak"}`),
				}},
				ToolInvocations: []ToolInvocationState{{
					ID: "invocation-1", ToolCallID: "call-1", ToolName: "exec_command",
					ArgumentsHash: "sha256:already-hashed", Status: ToolInvocationRunning, Mutating: true,
				}},
				StartedAt: now, UpdatedAt: now,
			}},
		},
		TurnHistory: []TurnSnapshot{{
			ID: "turn-history", SessionID: "session-1", SessionType: SessionTypeHost, Mode: ModeExecute,
			Lifecycle: TurnLifecycleCompleted, ResumeState: TurnResumeStateNone,
			StartedAt: now, UpdatedAt: now, CompletedAt: &now,
		}},
	}

	report := BuildActiveTurnMigrationReport("run-1", []*SessionState{session}, ActiveTurnMigrationOptions{
		Now:         now,
		DryRun:      true,
		StoreDriver: "json",
	})

	if report.Summary.Total != 2 {
		t.Fatalf("Summary.Total = %d, want 2", report.Summary.Total)
	}
	if report.Summary.TerminalNoop != 1 {
		t.Fatalf("Summary.TerminalNoop = %d, want 1", report.Summary.TerminalNoop)
	}
	if report.Summary.RequiresManualClose+report.Summary.Unrecoverable == 0 {
		t.Fatalf("active current turn should require a non-terminal migration decision: %+v", report.Summary)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal(report) error = %v", err)
	}
	if strings.Contains(string(encoded), "raw-argument-must-not-leak") {
		t.Fatalf("report leaked raw tool arguments: %s", encoded)
	}
	if !strings.Contains(string(encoded), "sha256:already-hashed") {
		t.Fatalf("report should include argument hash evidence, got %s", encoded)
	}
}

func TestApplyActiveTurnMigrationMarksUnrecoverable(t *testing.T) {
	now := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	session := &SessionState{
		ID:   "session-1",
		Type: SessionTypeHost,
		Mode: ModeExecute,
		CurrentTurn: &TurnSnapshot{
			ID: "turn-bad", SessionID: "session-1", SessionType: SessionTypeHost, Mode: ModeExecute,
			Lifecycle: TurnLifecycleRunning, ResumeState: TurnResumeStateNone,
			StartedAt: now, UpdatedAt: now,
			Iterations: []IterationState{{
				ID: "iter-1", SessionID: "session-1", TurnID: "turn-bad", Iteration: 1,
				Lifecycle: TurnLifecycleRunning, ResumeState: TurnResumeStateNone,
				ToolInvocations: []ToolInvocationState{{
					ID: "invocation-1", ToolCallID: "call-1", ToolName: "exec_command",
					ArgumentsHash: "sha256:mutating", Status: ToolInvocationRunning, Mutating: true,
				}},
				StartedAt: now, UpdatedAt: now,
			}},
		},
	}
	report := ActiveTurnMigrationReport{
		RunID: "run-1",
		Items: []ActiveTurnMigrationItem{{
			SessionID: "session-1",
			TurnID:    "turn-bad",
			Decision:  ActiveTurnMigrationDecisionUnrecoverable,
		}},
	}

	dryRunSession := *session
	dryRunSession.CurrentTurn = cloneTurnSnapshot(session.CurrentTurn)
	changed := ApplyActiveTurnMigration(&dryRunSession, report, ActiveTurnMigrationApplyOptions{Now: now, DryRun: true})
	if changed != 0 {
		t.Fatalf("dry-run changed %d turns, want 0", changed)
	}
	if dryRunSession.CurrentTurn.Lifecycle != TurnLifecycleRunning {
		t.Fatalf("dry-run Lifecycle = %q, want running", dryRunSession.CurrentTurn.Lifecycle)
	}

	changed = ApplyActiveTurnMigration(session, report, ActiveTurnMigrationApplyOptions{Now: now, DryRun: false})
	if changed != 1 {
		t.Fatalf("changed = %d, want 1", changed)
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleFailed {
		t.Fatalf("Lifecycle = %q, want failed", session.CurrentTurn.Lifecycle)
	}
	if session.CurrentTurn.CompletedAt == nil || !session.CurrentTurn.CompletedAt.Equal(now) {
		t.Fatalf("CompletedAt = %v, want %v", session.CurrentTurn.CompletedAt, now)
	}
	if !strings.Contains(session.CurrentTurn.Error, "unrecoverable active turn migration") {
		t.Fatalf("Error = %q, want unrecoverable migration marker", session.CurrentTurn.Error)
	}
	if got := session.CurrentTurn.Metadata["recovery.decision"]; got != string(ActiveTurnMigrationDecisionUnrecoverable) {
		t.Fatalf("recovery.decision = %q, want unrecoverable", got)
	}
	inv := session.CurrentTurn.Iterations[0].ToolInvocations[0]
	if inv.Status != ToolInvocationFailed {
		t.Fatalf("invocation status = %q, want failed", inv.Status)
	}
	if inv.FailureKind != string(toolfailure.KindSideEffectUnknown) {
		t.Fatalf("failure kind = %q, want side_effect_unknown", inv.FailureKind)
	}
}

func cloneTurnSnapshot(turn *TurnSnapshot) *TurnSnapshot {
	if turn == nil {
		return nil
	}
	cloned := *turn
	if turn.Metadata != nil {
		cloned.Metadata = make(map[string]string, len(turn.Metadata))
		for k, v := range turn.Metadata {
			cloned.Metadata[k] = v
		}
	}
	cloned.Iterations = append([]IterationState(nil), turn.Iterations...)
	for idx := range cloned.Iterations {
		cloned.Iterations[idx].ToolInvocations = append([]ToolInvocationState(nil), cloned.Iterations[idx].ToolInvocations...)
	}
	return &cloned
}
