package runtimekernel

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestRuntimeKernelReplayArtifactSinkCapturesTypedFacts(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("typed final", nil)}}
	sink := &recordingReplayArtifactSink{}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	kernel.replayArtifactSink = sink

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-replay-artifacts", SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: "turn-replay-artifacts", Input: "inspect",
	})
	if err != nil || result.Status != "completed" {
		t.Fatalf("RunTurn() = %#v, %v; want completed", result, err)
	}
	if len(model.inputs) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(model.inputs))
	}
	wantKinds := []ReplayArtifactKind{
		ReplayArtifactKindTurnAssembly,
		ReplayArtifactKindStepContext,
		ReplayArtifactKindFinalFacts,
	}
	if len(sink.artifacts) != len(wantKinds) {
		t.Fatalf("artifacts = %#v, want kinds %v", sink.artifacts, wantKinds)
	}
	for index, artifact := range sink.artifacts {
		if artifact.Kind != wantKinds[index] {
			t.Fatalf("artifact[%d].Kind = %q, want %q", index, artifact.Kind, wantKinds[index])
		}
		if artifact.SessionID != "session-replay-artifacts" || artifact.TurnID != "turn-replay-artifacts" {
			t.Fatalf("artifact[%d] coordinates = %q/%q", index, artifact.SessionID, artifact.TurnID)
		}
		if err := artifact.Validate(); err != nil {
			t.Fatalf("artifact[%d].Validate() error = %v", index, err)
		}
	}
	if sink.artifacts[0].TurnAssembly == nil || sink.artifacts[1].StepContext == nil || sink.artifacts[2].Final == nil {
		t.Fatalf("typed artifact payloads are missing: %#v", sink.artifacts)
	}
	if sink.artifacts[2].Final.RuntimeFactsHash == "" || sink.artifacts[2].Final.ContractHash == "" {
		t.Fatalf("final replay hashes are missing: %#v", sink.artifacts[2].Final)
	}

	session := kernel.sessions.Get("session-replay-artifacts")
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.TurnAssembly == nil {
		t.Fatalf("runtime snapshot missing: %#v", session)
	}
	originalProfile := session.CurrentTurn.TurnAssembly.AdmissionFacts.Profile
	sink.artifacts[0].TurnAssembly.AdmissionFacts.Profile = "sink-mutation"
	if session.CurrentTurn.TurnAssembly.AdmissionFacts.Profile != originalProfile {
		t.Fatal("sink-owned TurnAssembly aliases runtime state")
	}
	if _, exists := session.CurrentTurn.Metadata["replayArtifact"]; exists || len(session.CurrentTurn.ExternalReferences) != 0 {
		t.Fatalf("replay artifacts leaked into model/runtime metadata: metadata=%#v refs=%#v", session.CurrentTurn.Metadata, session.CurrentTurn.ExternalReferences)
	}
}

func TestFreezeReplayArtifactDeepCopiesStepAndFinalFacts(t *testing.T) {
	step := mustFreezeRuntimeStepContextForTest(t, validRuntimeStepContextForHashTest())
	frozenStep, err := FreezeReplayArtifact(ReplayArtifact{
		Kind: ReplayArtifactKindStepContext, SessionID: step.Turn.SessionID,
		TurnID: step.Turn.TurnID, StepID: step.Hash, StepContext: &step,
	})
	if err != nil {
		t.Fatalf("FreezeReplayArtifact(step) error = %v", err)
	}
	step.ProviderRequest.Input[0].Content = "source mutation"
	step.ToolSurface.HiddenReasons["danger"][0] = "source mutation"
	if err := frozenStep.Validate(); err != nil {
		t.Fatalf("frozen step changed with source: %v", err)
	}

	facts := FinalRuntimeFacts{
		CompletionStatus: FinalCompletionStatusFailed,
		ToolOutcomes:     []string{"read#call-1:failed"},
		PostcheckStatus:  FinalPostcheckStatusNotRequired,
		RollbackStatus:   FinalRollbackStatusNotRequired,
		FailureCodes:     []string{"tool_not_found"},
		EvidenceState: FinalEvidenceState{
			FailedTools: []FailedToolImpact{{ToolName: "read", FailureClass: "tool_not_found"}},
		},
	}
	contract := BuildTerminalFinalContract("typed final", FinalContractStatusToolUnavailable, facts.FailureCodes)
	frozenFinal, err := FreezeReplayArtifact(ReplayArtifact{
		Kind: ReplayArtifactKindFinalFacts, SessionID: "session-final",
		TurnID: "turn-final", Final: &ReplayFinalArtifact{RuntimeFacts: facts, Contract: contract},
	})
	if err != nil {
		t.Fatalf("FreezeReplayArtifact(final) error = %v", err)
	}
	facts.ToolOutcomes[0] = "source mutation"
	facts.EvidenceState.FailedTools[0].ToolName = "source mutation"
	contract.Limitations[0] = "source mutation"
	if err := frozenFinal.Validate(); err != nil {
		t.Fatalf("frozen final changed with source: %v", err)
	}
	if got := frozenFinal.Final.RuntimeFacts.EvidenceState.FailedTools[0].ToolName; got != "read" {
		t.Fatalf("hidden typed facts were not deep frozen: %q", got)
	}

	tampered := frozenFinal
	tampered.Final.RuntimeFactsHash = "sha256:tampered"
	if err := tampered.Validate(); err == nil {
		t.Fatal("Validate() accepted a tampered final runtime facts hash")
	}
	unknown := frozenFinal
	unknown.SchemaVersion = "aiops.replay-artifact.v999"
	if _, err := FreezeReplayArtifact(unknown); err == nil {
		t.Fatal("FreezeReplayArtifact() accepted an unknown schema version")
	}
}

func TestFreezeReplayArtifactPreservesValidatedActionTokenHashAndExpiry(t *testing.T) {
	expiresAt := time.Date(2026, 7, 14, 20, 30, 0, 123, time.UTC)
	token := mustFreezeActionTokenForTest(t, ActionToken{
		ApprovalID: "approval-replay", TurnID: "turn-replay-token", ToolCallID: "call-replay", ToolName: "restart_service",
		ArgumentsHash: "sha256:arguments", TargetRefs: []string{"host:a"},
		ToolSurfaceFingerprint: "sha256:router", PermissionHash: "sha256:permission",
		RollbackHash: "sha256:rollback", CheckpointID: "checkpoint-replay", ExpiresAt: expiresAt,
	})
	originalHash := token.Hash
	frozen, err := FreezeReplayArtifact(ReplayArtifact{
		Kind: ReplayArtifactKindApprovalActionToken, SessionID: "session-replay-token",
		TurnID: token.TurnID, ActionToken: &token,
	})
	if err != nil {
		t.Fatalf("FreezeReplayArtifact(action token) error = %v", err)
	}
	token.TargetRefs[0] = "host:source-mutation"
	token.ExpiresAt = token.ExpiresAt.Add(time.Hour)
	token.Hash = "sha256:source-mutation"
	if err := frozen.Validate(); err != nil {
		t.Fatalf("frozen action token changed with source: %v", err)
	}
	if frozen.ActionToken == nil || frozen.ActionToken.Hash != originalHash || !frozen.ActionToken.ExpiresAt.Equal(expiresAt) || !reflect.DeepEqual(frozen.ActionToken.TargetRefs, []string{"host:a"}) {
		t.Fatalf("frozen action token = %#v", frozen.ActionToken)
	}

	tampered := *frozen.ActionToken
	tampered.ExpiresAt = tampered.ExpiresAt.Add(time.Second)
	if _, err := FreezeReplayArtifact(ReplayArtifact{
		Kind: ReplayArtifactKindApprovalActionToken, SessionID: frozen.SessionID,
		TurnID: frozen.TurnID, ActionToken: &tampered,
	}); err == nil {
		t.Fatal("FreezeReplayArtifact() recomputed and accepted a token whose expiry no longer matches its original hash")
	}
}

func TestRuntimeKernelApprovalActionTokenCapturePrecedesCanonicalEventAndFailsClosed(t *testing.T) {
	token := mustFreezeActionTokenForTest(t, ActionToken{
		ApprovalID: "approval-capture", TurnID: "turn-token-capture", ToolCallID: "call-capture", ToolName: "restart_service",
		ArgumentsHash: "sha256:arguments", TargetRefs: []string{"host:a"},
		ToolSurfaceFingerprint: "sha256:router", PermissionHash: "sha256:permission",
		RollbackHash: "sha256:rollback", CheckpointID: "checkpoint-capture", ExpiresAt: time.Now().Add(time.Hour),
	})
	approval := PendingApproval{
		ID: token.ApprovalID, ToolCallID: token.ToolCallID, ToolName: token.ToolName, ActionToken: &token,
	}

	t.Run("capture then append", func(t *testing.T) {
		sink := &recordingReplayArtifactSink{}
		kernel := NewRuntimeKernel(RuntimeKernelConfig{ReplayArtifactSink: sink})
		snapshot := &TurnSnapshot{ID: token.TurnID, SessionID: "session-token-capture"}
		if err := kernel.recordCanonicalApprovalRequested(context.Background(), snapshot, approval); err != nil {
			t.Fatalf("recordCanonicalApprovalRequested() error = %v", err)
		}
		if len(sink.artifacts) != 1 || sink.artifacts[0].Kind != ReplayArtifactKindApprovalActionToken || sink.artifacts[0].ActionToken == nil {
			t.Fatalf("captured artifacts = %#v", sink.artifacts)
		}
		if sink.artifacts[0].ActionToken.Hash != token.Hash || !sink.artifacts[0].ActionToken.ExpiresAt.Equal(token.ExpiresAt) {
			t.Fatalf("captured token = %#v, want original hash/expiry", sink.artifacts[0].ActionToken)
		}
		events, err := kernel.CanonicalRolloutEvents(context.Background(), snapshot.SessionID, snapshot.ID)
		if err != nil || len(events) != 1 || events[0].Kind != "approval_requested" {
			t.Fatalf("canonical events = %#v, %v", events, err)
		}
	})

	t.Run("sink failure", func(t *testing.T) {
		failure := errors.New("capture unavailable")
		kernel := NewRuntimeKernel(RuntimeKernelConfig{ReplayArtifactSink: &recordingReplayArtifactSink{
			failKind: ReplayArtifactKindApprovalActionToken, failErr: failure,
		}})
		snapshot := &TurnSnapshot{ID: token.TurnID, SessionID: "session-token-capture-fail"}
		err := kernel.recordCanonicalApprovalRequested(context.Background(), snapshot, approval)
		if err == nil || !errors.Is(err, failure) {
			t.Fatalf("recordCanonicalApprovalRequested() error = %v, want sink failure", err)
		}
		events, readErr := kernel.CanonicalRolloutEvents(context.Background(), snapshot.SessionID, snapshot.ID)
		if readErr != nil || len(events) != 0 {
			t.Fatalf("events after failed capture = %#v, %v", events, readErr)
		}
	})
}

func TestRuntimeKernelReplayArtifactSinkFailsClosedAtOwnedBoundary(t *testing.T) {
	failure := errors.New("replay artifact sink unavailable")
	for _, test := range []struct {
		kind              ReplayArtifactKind
		wantProviderCalls int
	}{
		{kind: ReplayArtifactKindTurnAssembly, wantProviderCalls: 0},
		{kind: ReplayArtifactKindStepContext, wantProviderCalls: 0},
		{kind: ReplayArtifactKindFinalFacts, wantProviderCalls: 1},
	} {
		t.Run(string(test.kind), func(t *testing.T) {
			model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("must not commit", nil)}}
			kernel := newLoopKernel(t, model, nil, nil, nil)
			kernel.replayArtifactSink = &recordingReplayArtifactSink{failKind: test.kind, failErr: failure}
			_, err := kernel.RunTurn(context.Background(), TurnRequest{
				SessionID: "session-fail-" + string(test.kind), SessionType: SessionTypeHost,
				Mode: ModeInspect, TurnID: "turn-fail-" + string(test.kind), Input: "inspect",
			})
			if err == nil || !strings.Contains(err.Error(), "replay artifact") {
				t.Fatalf("RunTurn() error = %v, want replay artifact failure", err)
			}
			if !errors.Is(err, failure) {
				t.Fatalf("RunTurn() error = %v, want wrapped sink failure", err)
			}
			if len(model.inputs) != test.wantProviderCalls {
				t.Fatalf("provider calls = %d, want %d", len(model.inputs), test.wantProviderCalls)
			}
			session := kernel.sessions.Get("session-fail-" + string(test.kind))
			if session != nil && session.CurrentTurn != nil && test.kind == ReplayArtifactKindFinalFacts && strings.TrimSpace(session.CurrentTurn.FinalOutput) != "" {
				t.Fatalf("final output committed after sink failure: %q", session.CurrentTurn.FinalOutput)
			}
		})
	}
}

func TestRuntimeKernelConfigWiresReplayArtifactSink(t *testing.T) {
	sink := &recordingReplayArtifactSink{}
	kernel := NewRuntimeKernel(RuntimeKernelConfig{ReplayArtifactSink: sink})
	if kernel.replayArtifactSink != sink {
		t.Fatal("RuntimeKernelConfig.ReplayArtifactSink was not wired")
	}
}

type recordingReplayArtifactSink struct {
	artifacts []ReplayArtifact
	failKind  ReplayArtifactKind
	failErr   error
}

func (s *recordingReplayArtifactSink) CaptureReplayArtifact(_ context.Context, artifact ReplayArtifact) error {
	if artifact.Kind == s.failKind {
		return s.failErr
	}
	s.artifacts = append(s.artifacts, artifact)
	return nil
}
