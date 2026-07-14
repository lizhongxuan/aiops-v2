package runtimekernel

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/resourcebinding"
)

func TestTypedControlLifecycleDoesNotCarryLegacySessionHostIntoSecondTurn(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("first turn complete", nil),
		schema.AssistantMessage("second turn complete", nil),
	}}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	const sessionID = "session-typed-lifecycle-no-legacy-carryover"

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   sessionID,
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-legacy-host-source",
		HostID:      "host-legacy-only",
		Input:       "inspect this explicitly supplied host",
	})
	if err != nil {
		t.Fatalf("first RunTurn() error = %v", err)
	}
	session := kernel.sessions.Get(sessionID)
	if session == nil || session.HostID != "host-legacy-only" || session.SessionTargetSnapshot != nil {
		t.Fatalf("first turn did not establish legacy-only session state: %#v", session)
	}

	_, err = kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   sessionID,
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-without-typed-target",
		Input:       "continue without a typed target",
	})
	if err != nil {
		t.Fatalf("second RunTurn() error = %v", err)
	}
	session = kernel.sessions.Get(sessionID)
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.ID != "turn-without-typed-target" {
		t.Fatalf("missing second turn snapshot: %#v", session)
	}
	assembly := session.CurrentTurn.TurnAssembly
	if assembly == nil {
		t.Fatalf("second turn assembly is nil: %#v", session.CurrentTurn)
	}
	if len(assembly.AdmissionFacts.TargetRefs) != 0 || !assembly.AdmissionFacts.SessionTarget.IsZero() {
		t.Fatalf("second turn inherited legacy SessionState.HostID as target: %#v", assembly.AdmissionFacts)
	}
}

func TestTypedControlLifecycleClearedTargetRemovesSessionTargetDependentState(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("bound turn complete", nil),
		schema.AssistantMessage("clear turn complete", nil),
	}}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	const sessionID = "session-typed-lifecycle-clear"
	target := resourcebinding.NewSessionTargetSnapshot(resourcebinding.SessionTargetInput{
		HostIDs:           []string{"host-to-clear"},
		SourceTurnID:      "turn-bound-target",
		SourceMentionIDs:  []string{"mention-bound-target"},
		ExpiresAfterTurns: 6,
		Confidence:        1,
	})
	role := resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
		ResourceRef:  resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-to-clear"},
		Role:         "primary",
		SourceTurnID: "turn-bound-target",
		Confidence:   1,
	})
	conflict := resourcebinding.RoleBindingConflict{
		ResourceID: "host-to-clear",
		Role:       "primary",
		Reasons:    []string{"synthetic_role_conflict"},
	}

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:             sessionID,
		SessionType:           SessionTypeHost,
		Mode:                  ModeInspect,
		TurnID:                "turn-bound-target",
		HostID:                "host-to-clear",
		Input:                 "bind host and role state",
		SessionTargetSnapshot: target,
		ResourceRoleBindings:  []resourcebinding.ResourceRoleBinding{role},
		RoleBindingConflicts:  []resourcebinding.RoleBindingConflict{conflict},
	})
	if err != nil {
		t.Fatalf("bound RunTurn() error = %v", err)
	}
	session := kernel.sessions.Get(sessionID)
	if session == nil || session.HostID == "" || session.SessionTargetSnapshot == nil || len(session.ResourceRoleBindings) != 1 || len(session.RoleBindingConflicts) != 1 {
		t.Fatalf("bound turn did not persist target-dependent state: %#v", session)
	}

	cleared := resourcebinding.SessionTargetCleared("turn-clear-target")
	_, err = kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:             sessionID,
		SessionType:           SessionTypeHost,
		Mode:                  ModeInspect,
		TurnID:                "turn-clear-target",
		Input:                 "clear the active target",
		SessionTargetSnapshot: cleared,
	})
	if err != nil {
		t.Fatalf("clear RunTurn() error = %v", err)
	}
	session = kernel.sessions.Get(sessionID)
	if session == nil {
		t.Fatal("missing session after clear turn")
	}
	if session.SessionTargetSnapshot == nil || session.SessionTargetSnapshot.BindingMode != resourcebinding.BindingModeNone {
		t.Errorf("cleared typed target not persisted: %#v", session.SessionTargetSnapshot)
	}
	if session.HostID != "" {
		t.Errorf("legacy HostID survived typed target clear: %q", session.HostID)
	}
	if len(session.ResourceRoleBindings) != 0 {
		t.Errorf("role bindings survived typed target clear: %#v", session.ResourceRoleBindings)
	}
	if len(session.RoleBindingConflicts) != 0 {
		t.Errorf("role binding conflicts survived typed target clear: %#v", session.RoleBindingConflicts)
	}
	if session.CurrentTurn == nil || session.CurrentTurn.TurnAssembly == nil || len(session.CurrentTurn.TurnAssembly.AdmissionFacts.TargetRefs) != 0 {
		t.Errorf("clear turn retained target refs: %#v", session.CurrentTurn)
	}
}
