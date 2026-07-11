package runtimekernel

import (
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
)

func TestSessionManagerPublishesImmutableSnapshot(t *testing.T) {
	manager := NewSessionManager()
	working := manager.GetOrCreate("session-snapshot", SessionTypeHost, ModeChat)
	working.CurrentTurn = sessionSnapshotTestTurn("turn-snapshot", "initial")
	manager.Update(working)

	published, err := manager.GetSnapshot(working.ID)
	if err != nil {
		t.Fatalf("GetSnapshot() error = %v", err)
	}
	if published == nil || published.CurrentTurn == nil {
		t.Fatal("published session snapshot is missing current turn")
	}
	if published == working || published.CurrentTurn == working.CurrentTurn {
		t.Fatal("published session snapshot shares mutable runtime pointers")
	}

	working.CurrentTurn.Lifecycle = TurnLifecycleCompleted
	working.CurrentTurn.Metadata["phase"] = "mutated"
	working.CurrentTurn.ToolSurfaceSnapshot.ToolNames[0] = "mutated_tool"
	working.CurrentTurn.ContextGovernanceEvents[0].Kind = "mutated"

	assertSessionSnapshotTestTurn(t, published.CurrentTurn, TurnLifecycleRunning, "initial")

	manager.Update(working)
	next, err := manager.GetSnapshot(working.ID)
	if err != nil {
		t.Fatalf("GetSnapshot() after update error = %v", err)
	}
	if next == nil || next.CurrentTurn == nil {
		t.Fatal("updated published session snapshot is missing current turn")
	}
	assertSessionSnapshotTestTurn(t, next.CurrentTurn, TurnLifecycleCompleted, "mutated")
	assertSessionSnapshotTestTurn(t, published.CurrentTurn, TurnLifecycleRunning, "initial")
}

func TestSessionManagerListsPublishedSnapshots(t *testing.T) {
	manager := NewSessionManager()
	working := manager.GetOrCreate("session-snapshot-list", SessionTypeHost, ModeChat)
	working.CurrentTurn = sessionSnapshotTestTurn("turn-snapshot-list", "initial")
	working.CurrentTurn.SessionID = working.ID
	manager.Update(working)

	snapshots, err := manager.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}
	if len(snapshots) != 1 || snapshots[0] == nil || snapshots[0].CurrentTurn == nil {
		t.Fatalf("published snapshots = %#v, want one session with a current turn", snapshots)
	}
	working.CurrentTurn.Metadata["phase"] = "mutated"
	if got := snapshots[0].CurrentTurn.Metadata["phase"]; got != "initial" {
		t.Fatalf("listed snapshot phase = %q, want immutable initial value", got)
	}
}

func TestSessionManagerPublishesRepositorySnapshots(t *testing.T) {
	repo := newTestSessionRepository()
	repo.sessions["session-hydrated"] = &SessionState{
		ID:        "session-hydrated",
		Type:      SessionTypeHost,
		Mode:      ModeChat,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	t.Run("get", func(t *testing.T) {
		manager := NewSessionManager(repo)
		working := manager.Get("session-hydrated")
		published, err := manager.GetSnapshot("session-hydrated")
		if err != nil {
			t.Fatalf("GetSnapshot() error = %v", err)
		}
		if working == nil || published == nil {
			t.Fatalf("working=%#v published=%#v, want both hydrated", working, published)
		}
		if working == published {
			t.Fatal("hydrated working session shares its published pointer")
		}
	})

	t.Run("list", func(t *testing.T) {
		manager := NewSessionManager(repo)
		if sessions := manager.List(); len(sessions) != 1 {
			t.Fatalf("hydrated sessions = %d, want 1", len(sessions))
		}
		published, err := manager.ListSnapshots()
		if err != nil {
			t.Fatalf("ListSnapshots() error = %v", err)
		}
		if len(published) != 1 || published[0] == nil || published[0].ID != "session-hydrated" {
			t.Fatalf("published hydrated snapshots = %#v, want session-hydrated", published)
		}
	})
}

func TestSessionManagerCloneFailureClearsStaleSnapshot(t *testing.T) {
	manager := NewSessionManager()
	working := manager.GetOrCreate("session-clone-failure", SessionTypeHost, ModeChat)
	working.CurrentTurn = sessionSnapshotTestTurn("turn-clone-failure", "initial")
	manager.Update(working)

	working.CurrentTurn.AgentItems = []agentstate.TurnItem{{
		ID:     "invalid-payload",
		Type:   agentstate.TurnItemTypeModelCall,
		Status: agentstate.ItemStatusRunning,
		Payload: agentstate.PayloadEnvelope{
			Data: json.RawMessage(`{"unterminated":`),
		},
		CreatedAt: time.Now().UTC(),
	}}
	manager.Update(working)

	published, err := manager.GetSnapshot(working.ID)
	if err == nil {
		t.Fatal("GetSnapshot() error = nil, want clone failure")
	}
	if published != nil {
		t.Fatalf("GetSnapshot() = %#v after clone failure, want no stale snapshot", published)
	}
}

func sessionSnapshotTestTurn(turnID, phase string) *TurnSnapshot {
	now := time.Now().UTC()
	return &TurnSnapshot{
		ID:          turnID,
		SessionID:   "session-snapshot",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		Metadata:    map[string]string{"phase": phase},
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now,
		ToolSurfaceSnapshot: &ToolSurfaceSnapshotRef{
			ID:          "surface-1",
			Fingerprint: "surface-hash",
			ToolNames:   []string{"read_status"},
			CreatedAt:   now,
		},
		ContextGovernanceEvents: []ContextGovernanceEvent{{
			ID:        "governance-1",
			Layer:     ContextGovernanceLayerL1,
			Kind:      phase,
			CreatedAt: now,
		}},
	}
}

func assertSessionSnapshotTestTurn(t *testing.T, turn *TurnSnapshot, lifecycle TurnLifecycleState, phase string) {
	t.Helper()
	if turn.Lifecycle != lifecycle {
		t.Fatalf("snapshot lifecycle = %q, want %q", turn.Lifecycle, lifecycle)
	}
	if got := turn.Metadata["phase"]; got != phase {
		t.Fatalf("snapshot metadata phase = %q, want %q", got, phase)
	}
	wantTool := "read_status"
	wantKind := "initial"
	if phase == "mutated" {
		wantTool = "mutated_tool"
		wantKind = "mutated"
	}
	if got := turn.ToolSurfaceSnapshot.ToolNames[0]; got != wantTool {
		t.Fatalf("snapshot tool name = %q, want %q", got, wantTool)
	}
	if got := turn.ContextGovernanceEvents[0].Kind; got != wantKind {
		t.Fatalf("snapshot governance kind = %q, want %q", got, wantKind)
	}
}
