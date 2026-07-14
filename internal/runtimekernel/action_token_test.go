package runtimekernel

import (
	"reflect"
	"testing"
	"time"
)

func TestActionTokenFreezesCanonicalServerBinding(t *testing.T) {
	expires := time.Date(2026, 7, 13, 18, 0, 0, 0, time.UTC)
	token, err := FreezeActionToken(ActionToken{
		ApprovalID: "approval-1", TurnID: "turn-1", ToolCallID: "call-1", ToolName: "write_file",
		ArgumentsHash: "sha256:args", TargetRefs: []string{"service:b", "host:a", "host:a"},
		ToolSurfaceFingerprint: "sha256:router", PermissionHash: "sha256:permission",
		RollbackHash: "sha256:rollback", CheckpointID: "checkpoint-1", ExpiresAt: expires,
	})
	if err != nil {
		t.Fatalf("FreezeActionToken() error = %v", err)
	}
	if token.SchemaVersion != ActionTokenSchemaVersion || token.Hash == "" {
		t.Fatalf("token identity = %#v", token)
	}
	if !reflect.DeepEqual(token.TargetRefs, []string{"host:a", "service:b"}) {
		t.Fatalf("TargetRefs = %v", token.TargetRefs)
	}
	if err := token.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	tampered := token
	tampered.PermissionHash = "sha256:tampered"
	if err := tampered.Validate(); err == nil {
		t.Fatal("Validate() accepted a tampered token")
	}
}

func TestActionTokenVerifyReportsOnlyStableMismatchFields(t *testing.T) {
	expires := time.Date(2026, 7, 13, 18, 0, 0, 0, time.UTC)
	token := mustFreezeActionTokenForTest(t, ActionToken{
		ApprovalID: "approval-1", TurnID: "turn-1", ToolCallID: "call-1", ToolName: "write_file",
		ArgumentsHash: "sha256:args", TargetRefs: []string{"host:a"}, ToolSurfaceFingerprint: "sha256:router",
		PermissionHash: "sha256:permission", RollbackHash: "sha256:rollback", CheckpointID: "checkpoint-1", ExpiresAt: expires,
	})
	current := ActionTokenCurrentFacts{
		ApprovalID: token.ApprovalID, TurnID: token.TurnID, ToolCallID: token.ToolCallID, ToolName: token.ToolName,
		ArgumentsHash: "sha256:changed", TargetRefs: []string{"host:b"}, ToolSurfaceFingerprint: "sha256:changed-router",
		PermissionHash: "sha256:changed-permission", RollbackHash: "sha256:changed-rollback", CheckpointID: "checkpoint-2",
	}
	_, err := VerifyActionToken(token, current, expires.Add(-time.Minute))
	stale, ok := err.(*ApprovalContextStaleError)
	if !ok {
		t.Fatalf("VerifyActionToken() error = %T %v", err, err)
	}
	want := []string{"arguments", "checkpoint", "permission", "rollback", "target", "tool_router"}
	if !reflect.DeepEqual(stale.MismatchFields, want) || stale.Code != ApprovalContextStaleCode {
		t.Fatalf("stale error = %#v, want fields %v", stale, want)
	}
	if got := stale.Error(); got != "approval_context_stale: arguments,checkpoint,permission,rollback,target,tool_router" {
		t.Fatalf("Error() = %q", got)
	}
	if _, err := VerifyActionToken(token, ActionTokenCurrentFacts{
		ApprovalID: token.ApprovalID, TurnID: token.TurnID, ToolCallID: token.ToolCallID, ToolName: token.ToolName,
		ArgumentsHash: token.ArgumentsHash, TargetRefs: token.TargetRefs, ToolSurfaceFingerprint: token.ToolSurfaceFingerprint,
		PermissionHash: token.PermissionHash, RollbackHash: token.RollbackHash, CheckpointID: token.CheckpointID,
	}, expires.Add(time.Second)); err == nil {
		t.Fatal("VerifyActionToken() accepted expired token")
	}
}

func mustFreezeActionTokenForTest(t *testing.T, token ActionToken) ActionToken {
	t.Helper()
	frozen, err := FreezeActionToken(token)
	if err != nil {
		t.Fatalf("FreezeActionToken() error = %v", err)
	}
	return frozen
}
