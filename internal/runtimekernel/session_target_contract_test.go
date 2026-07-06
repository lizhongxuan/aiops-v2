package runtimekernel

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/resourcebinding"
)

func TestRunTurnPersistsSessionTargetSnapshotForHostCarryover(t *testing.T) {
	target := resourcebinding.NewSessionTargetSnapshot(resourcebinding.SessionTargetInput{
		HostIDs:           []string{"host-kme-b2c1b82d"},
		SourceTurnID:      "turn-host-carryover-1",
		SourceMentionIDs:  []string{"mention-host"},
		ExpiresAfterTurns: 6,
		Confidence:        1,
	})
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("已记录目标主机，后续继续使用同一主机上下文。", nil),
	}}
	kernel := newLoopKernel(t, model, nil, nil, nil)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:             "sess-host-carryover",
		SessionType:           SessionTypeHost,
		Mode:                  ModeInspect,
		TurnID:                "turn-host-carryover-1",
		HostID:                "host-kme-b2c1b82d",
		Input:                 "查看 172.18.13.11 agent 状态",
		SessionTargetSnapshot: target,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	session := kernel.sessions.Get("sess-host-carryover")
	if session == nil || session.SessionTargetSnapshot == nil {
		t.Fatalf("session target snapshot was not persisted: %#v", session)
	}
	if got := resourcebinding.HostIDsFromSessionTarget(session.SessionTargetSnapshot); len(got) != 1 || got[0] != "host-kme-b2c1b82d" {
		t.Fatalf("session target host ids = %#v", got)
	}
	if session.SessionTargetSnapshot.ActiveTargetSetID != target.ActiveTargetSetID {
		t.Fatalf("target set id = %q, want %q", session.SessionTargetSnapshot.ActiveTargetSetID, target.ActiveTargetSetID)
	}
}

func TestRunTurnPersistsSessionTargetRoleBindingsAndConflicts(t *testing.T) {
	binding := resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
		ResourceRef:  resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"},
		Role:         "primary",
		SourceTurnID: "turn-host-target-conflict",
		Confidence:   0.9,
	})
	conflict := resourcebinding.RoleBindingConflict{
		ResourceID: "host-b",
		Role:       "primary",
		Reasons:    []string{"role_binding_conflict"},
	}
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("需要确认目标主机。", nil),
	}}
	kernel := newLoopKernel(t, model, nil, nil, nil)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:            "sess-host-target-conflict",
		SessionType:          SessionTypeWorkspace,
		Mode:                 ModeChat,
		TurnID:               "turn-host-target-conflict",
		Input:                "继续检查主机",
		ResourceRoleBindings: []resourcebinding.ResourceRoleBinding{binding},
		RoleBindingConflicts: []resourcebinding.RoleBindingConflict{conflict},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	session := kernel.sessions.Get("sess-host-target-conflict")
	if session == nil {
		t.Fatal("missing session")
	}
	if len(session.ResourceRoleBindings) != 1 || session.ResourceRoleBindings[0].ResourceRef.ID != "host-a" {
		t.Fatalf("resource role bindings = %#v", session.ResourceRoleBindings)
	}
	if len(session.RoleBindingConflicts) != 1 || len(session.RoleBindingConflicts[0].Reasons) != 1 || session.RoleBindingConflicts[0].Reasons[0] != "role_binding_conflict" {
		t.Fatalf("role binding conflicts = %#v", session.RoleBindingConflicts)
	}
}
