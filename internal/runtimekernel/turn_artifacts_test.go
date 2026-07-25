package runtimekernel

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/tooling"
)

func TestTurnArtifactPersistsIntoSnapshotAndCannotChangeDuringTurn(t *testing.T) {
	first := runtimeTurnArtifact(t, map[string]string{"revision": "rv-1"})
	req := TurnRequest{SessionType: SessionTypeWorkspace, Mode: ModeInspect, TurnArtifacts: []runtimecontract.TurnArtifactEnvelope{first}}
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	session := &SessionState{ID: "session-1", Type: SessionTypeWorkspace, Mode: ModeInspect}
	kernel := &RuntimeKernel{}
	snapshot := kernel.ensureCurrentTurnSnapshot(session, req, "turn-1")
	if len(snapshot.TurnArtifacts) != 1 || snapshot.TurnArtifacts[0].Digest != first.Digest {
		t.Fatalf("snapshot artifacts = %#v", snapshot.TurnArtifacts)
	}

	req.TurnArtifacts[0].Inline[1] = 'X'
	if err := snapshot.TurnArtifacts[0].Validate(); err != nil {
		t.Fatalf("request mutation changed frozen snapshot: %v", err)
	}
	second := runtimeTurnArtifact(t, map[string]string{"revision": "rv-2"})
	reused := kernel.ensureCurrentTurnSnapshot(session, TurnRequest{SessionType: SessionTypeWorkspace, Mode: ModeInspect, TurnArtifacts: []runtimecontract.TurnArtifactEnvelope{second}}, "turn-1")
	if reused != snapshot || reused.TurnArtifacts[0].Digest != first.Digest {
		t.Fatal("same turn replaced its immutable artifact")
	}

	found, ok := TurnArtifactByKind(snapshot, "workflow.context")
	if !ok || found.Digest != first.Digest {
		t.Fatalf("artifact lookup = %#v, %v", found, ok)
	}
	found.Inline[1] = 'X'
	if err := snapshot.TurnArtifacts[0].Validate(); err != nil {
		t.Fatalf("artifact lookup exposed mutable snapshot bytes: %v", err)
	}
}

func TestTurnArtifactValidationRejectsDuplicateKindsAndTampering(t *testing.T) {
	first := runtimeTurnArtifact(t, map[string]string{"revision": "rv-1"})
	second := runtimeTurnArtifact(t, map[string]string{"revision": "rv-2"})
	request := TurnRequest{SessionType: SessionTypeWorkspace, Mode: ModeInspect, TurnArtifacts: []runtimecontract.TurnArtifactEnvelope{first, second}}
	if err := request.Validate(); err == nil {
		t.Fatal("duplicate artifact kind was accepted")
	}
	tampered := first.Clone()
	tampered.Digest = second.Digest
	request.TurnArtifacts = []runtimecontract.TurnArtifactEnvelope{tampered}
	if err := request.Validate(); err == nil {
		t.Fatal("tampered artifact was accepted")
	}
}

func TestTurnArtifactSnapshotJSONRoundTripSupportsRestart(t *testing.T) {
	artifact := runtimeTurnArtifact(t, map[string]string{"revision": "rv-1", "selection": "node-a"})
	session := &SessionState{ID: "session-1", Type: SessionTypeWorkspace, Mode: ModeInspect}
	snapshot := (&RuntimeKernel{}).ensureCurrentTurnSnapshot(session, TurnRequest{SessionType: SessionTypeWorkspace, Mode: ModeInspect, TurnArtifacts: []runtimecontract.TurnArtifactEnvelope{artifact}}, "turn-1")
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var restored TurnSnapshot
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatal(err)
	}
	if restored.TurnArtifacts[0].Digest != artifact.Digest {
		t.Fatalf("restored artifact = %#v", restored.TurnArtifacts)
	}
}

func TestTurnArtifactFlowsIntoGenericToolExecutionContext(t *testing.T) {
	artifact := runtimeTurnArtifact(t, map[string]string{"revision": "rv-1"})
	execution := toolExecutionContextForDispatch("", map[string]string{"workflowId": "workflow-a"}, []runtimecontract.TurnArtifactEnvelope{artifact})
	ctx := tooling.ContextWithToolExecution(context.Background(), execution)
	found, ok := runtimecontract.TurnArtifactFromContext(ctx, "workflow.context")
	if !ok || found.Digest != artifact.Digest {
		t.Fatalf("context artifact = %#v, %v", found, ok)
	}
	execution.TurnArtifacts[0].Inline[1] = 'X'
	again, _ := runtimecontract.TurnArtifactFromContext(ctx, "workflow.context")
	if err := again.Validate(); err != nil {
		t.Fatalf("dispatch context did not freeze artifacts: %v", err)
	}
}

func TestTrustedTurnIdentityPersistsAndOverridesMessageMetadataAtDispatch(t *testing.T) {
	identity := &runtimecontract.TurnIdentity{TenantID: "tenant-a", UserID: "user-a", SessionID: "session-a"}
	artifact := runtimeTurnArtifact(t, map[string]string{"revision": "rv-1"})
	session := &SessionState{ID: "session-a", Type: SessionTypeWorkspace, Mode: ModeInspect}
	snapshot := (&RuntimeKernel{}).ensureCurrentTurnSnapshot(session, TurnRequest{
		SessionType: SessionTypeWorkspace, Mode: ModeInspect, SessionID: "session-a",
		TrustedIdentity: identity, TurnArtifacts: []runtimecontract.TurnArtifactEnvelope{artifact},
	}, "turn-a")
	identity.TenantID = "mutated-after-send"
	execution := toolExecutionContextForSnapshot("", map[string]string{"tenantId": "tenant-forged", "userId": "user-forged"}, snapshot)
	if execution.TenantID != "tenant-a" || execution.UserID != "user-a" || execution.SessionID != "session-a" {
		t.Fatalf("dispatch identity = %#v", execution)
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var restored TurnSnapshot
	if err := json.Unmarshal(raw, &restored); err != nil || restored.Validate() != nil || restored.TrustedIdentity.TenantID != "tenant-a" {
		t.Fatalf("restored identity = %#v, %v", restored.TrustedIdentity, err)
	}
}

func runtimeTurnArtifact(t *testing.T, value any) runtimecontract.TurnArtifactEnvelope {
	t.Helper()
	artifact, err := runtimecontract.NewInlineTurnArtifact("workflow.context", value)
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}
