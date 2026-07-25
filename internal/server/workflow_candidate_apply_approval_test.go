package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
	"runner/server/service"
	edit "runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowCandidateApplyApprovalUsesPersistedBindingAndStartsExactControlledTurn(t *testing.T) {
	fixture := newWorkflowCandidateApplyApprovalFixture(t)
	response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowCandidateApplyApprovalPayload())
	if response.Code != http.StatusAccepted || len(fixture.runtime.requests) != 1 {
		t.Fatalf("status=%d body=%s runtime calls=%d", response.Code, response.Body.String(), len(fixture.runtime.requests))
	}

	controlled := fixture.runtime.requests[0]
	if controlled.ToolCall.Name != workflowCandidateApplyToolName || controlled.Turn.SessionID != "session-a" || controlled.Turn.SessionType != runtimekernel.SessionTypeWorkspace || controlled.Turn.Mode != runtimekernel.ModeExecute {
		t.Fatalf("controlled turn = %#v", controlled)
	}
	if controlled.Turn.TrustedIdentity == nil || controlled.Turn.TrustedIdentity.TenantID != "tenant-a" || controlled.Turn.TrustedIdentity.UserID != "actor-a" || controlled.Turn.TrustedIdentity.SessionID != "session-a" {
		t.Fatalf("trusted identity = %#v", controlled.Turn.TrustedIdentity)
	}
	if controlled.ExecutionContext.VerifiedApproval != nil || controlled.ExecutionContext.ActionToken != "" || controlled.ExecutionContext.SessionID != controlled.Turn.SessionID || controlled.ExecutionContext.TurnID != controlled.Turn.TurnID || controlled.ExecutionContext.ToolCallID != controlled.ToolCall.ID {
		t.Fatalf("execution context = %#v", controlled.ExecutionContext)
	}
	var apply edit.ApplyPatchRequest
	if err := edit.DecodeStrict(controlled.ToolCall.Arguments, &apply); err != nil {
		t.Fatalf("decode exact apply arguments: %v", err)
	}
	candidate := fixture.store.revision
	if apply.WorkflowKey != candidate.Key || apply.ChangeSetID != candidate.ID || apply.PreviewHash != candidate.PreviewHash || apply.ExpectedRevision != candidate.Base.ResourceVersion {
		t.Fatalf("apply binding = %#v, candidate = %#v", apply, candidate)
	}
	if apply.CandidateGrantID == "" || apply.ApplyApprovalID == "" || apply.IdempotencyKey == "" {
		t.Fatalf("server-derived apply ids = %#v", apply)
	}
	if len(controlled.Turn.TurnArtifacts) < 1 || len(controlled.ExecutionContext.TurnArtifacts) != len(controlled.Turn.TurnArtifacts) {
		t.Fatalf("controlled artifacts = %#v", controlled.Turn.TurnArtifacts)
	}
	frozen, err := appui.DecodeWorkflowTurnContext(controlled.Turn.TurnArtifacts[0])
	if err != nil || frozen.WorkflowKey != candidate.Key || frozen.Revision != candidate.Base || frozen.Scope.ActorID != "actor-a" || frozen.Scope.SessionID != "session-a" || frozen.Environment.BindingVersion != candidate.Validation.Basis.EnvironmentBindingVersion {
		t.Fatalf("frozen workflow context = %#v, error=%v", frozen, err)
	}
	if fixture.application.contextCalls != 1 || fixture.store.applyChecks != 1 {
		t.Fatalf("context calls=%d eligibility checks=%d", fixture.application.contextCalls, fixture.store.applyChecks)
	}

	var result workflowCandidateApplyApprovalResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.SessionID != controlled.Turn.SessionID || result.TurnID != controlled.Turn.TurnID || result.Approval.ApprovalID == "" || result.Approval.ToolCallID != controlled.ToolCall.ID || result.Candidate.ChangeSetID != candidate.ID || result.Candidate.PreviewHash != candidate.PreviewHash {
		t.Fatalf("response = %#v", result)
	}
}

func TestWorkflowCandidateApplyApprovalSupportsUserRevisedAgentCandidateV2(t *testing.T) {
	fixture := newWorkflowAgentRevisedCandidateApplyApprovalFixture(t)
	if fixture.store.revision.Validation.Basis.SemanticHash == fixture.store.revision.Base.SemanticHash {
		t.Fatal("fixture must preserve the real store semantic split between committed base and revised Candidate")
	}
	response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowAgentRevisedCandidateApplyApprovalPayload())
	if response.Code != http.StatusAccepted || len(fixture.runtime.requests) != 1 {
		t.Fatalf("status=%d body=%s runtime calls=%d", response.Code, response.Body.String(), len(fixture.runtime.requests))
	}

	controlled := fixture.runtime.requests[0]
	var apply edit.ApplyPatchRequest
	if err := edit.DecodeStrict(controlled.ToolCall.Arguments, &apply); err != nil {
		t.Fatalf("decode revised Agent Candidate apply arguments: %v", err)
	}
	revised := fixture.store.revision
	if apply.WorkflowKey != revised.Key || apply.ChangeSetID != revised.ID || apply.PreviewHash != revised.PreviewHash || apply.ExpectedRevision != revised.Base.ResourceVersion {
		t.Fatalf("apply binding = %#v, revised candidate = %#v", apply, revised)
	}
	if controlled.Turn.SessionID != revised.AgentSessionID || controlled.Turn.TrustedIdentity == nil || controlled.Turn.TrustedIdentity.TenantID != revised.Key.TenantID || controlled.Turn.TrustedIdentity.UserID != revised.ActorID {
		t.Fatalf("trusted revised Agent Candidate turn = %#v", controlled.Turn)
	}
	if len(controlled.Turn.TurnArtifacts) != 1 {
		t.Fatalf("turn artifacts = %#v", controlled.Turn.TurnArtifacts)
	}
	frozen, err := appui.DecodeWorkflowTurnContext(controlled.Turn.TurnArtifacts[0])
	if err != nil || frozen.WorkflowKey != revised.Key || frozen.Revision != revised.Base || frozen.Scope.ActorID != revised.ActorID || frozen.Scope.CanvasSessionID != revised.CanvasSessionID || frozen.Scope.SessionID != revised.AgentSessionID {
		t.Fatalf("frozen revised Agent Candidate context = %#v, error=%v", frozen, err)
	}

	var result workflowCandidateApplyApprovalResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Candidate.CandidateID != revised.CandidateID || result.Candidate.CandidateRevision != 2 || result.Candidate.ChangeSetID != revised.ID || result.Candidate.PreviewHash != revised.PreviewHash || result.Candidate.ExpectedRevision != revised.Base.ResourceVersion {
		t.Fatalf("response binding = %#v", result.Candidate)
	}
}

func TestWorkflowCandidateApplyApprovalSupportsCurrentUserRevisedAgentCandidateV3(t *testing.T) {
	fixture := newWorkflowAgentTwiceRevisedCandidateApplyApprovalFixture(t)
	response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowAgentTwiceRevisedCandidateApplyApprovalPayload())
	if response.Code != http.StatusAccepted || len(fixture.runtime.requests) != 1 {
		t.Fatalf("status=%d body=%s runtime calls=%d", response.Code, response.Body.String(), len(fixture.runtime.requests))
	}
	var apply edit.ApplyPatchRequest
	if err := edit.DecodeStrict(fixture.runtime.requests[0].ToolCall.Arguments, &apply); err != nil {
		t.Fatal(err)
	}
	current := fixture.store.revision
	if apply.ChangeSetID != current.ID || apply.PreviewHash != current.PreviewHash || apply.ExpectedRevision != current.Base.ResourceVersion {
		t.Fatalf("v3 apply binding = %#v, current = %#v", apply, current)
	}
	var result workflowCandidateApplyApprovalResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Candidate.CandidateRevision != 3 || result.Candidate.ChangeSetID != current.ID || result.Candidate.PreviewHash != current.PreviewHash {
		t.Fatalf("v3 response binding = %#v", result.Candidate)
	}
}

func TestWorkflowCandidateApplyApprovalRejectsRevisedAgentCandidateWithBrokenLineage(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*service.WorkflowCandidateRevision, *service.WorkflowCandidateRevision)
	}{
		{name: "revision two is not user authored", mutate: func(_ *service.WorkflowCandidateRevision, revised *service.WorkflowCandidateRevision) {
			revised.Actor = edit.ActorKind("agent")
		}},
		{name: "revision two parent change set", mutate: func(_ *service.WorkflowCandidateRevision, revised *service.WorkflowCandidateRevision) {
			revised.ParentChangeSetID = "changeset-other"
		}},
		{name: "revision two parent revision", mutate: func(_ *service.WorkflowCandidateRevision, revised *service.WorkflowCandidateRevision) {
			revised.ParentRevision = 0
		}},
		{name: "revision one is not agent authored", mutate: func(root *service.WorkflowCandidateRevision, _ *service.WorkflowCandidateRevision) {
			root.Actor = edit.ActorKind("user")
		}},
		{name: "revision one is not superseded", mutate: func(root *service.WorkflowCandidateRevision, _ *service.WorkflowCandidateRevision) {
			root.State = service.WorkflowCandidateRevisionReady
		}},
		{name: "revision one forged change set", mutate: func(root *service.WorkflowCandidateRevision, _ *service.WorkflowCandidateRevision) {
			root.ID = "changeset-forged"
		}},
		{name: "revision one base", mutate: func(root *service.WorkflowCandidateRevision, _ *service.WorkflowCandidateRevision) {
			root.Base.ResourceVersion = "rv-other"
			root.Validation.Basis.ResourceVersion = "rv-other"
		}},
		{name: "revision one actor", mutate: func(root *service.WorkflowCandidateRevision, _ *service.WorkflowCandidateRevision) {
			root.ActorID = "actor-b"
		}},
		{name: "revision one canvas", mutate: func(root *service.WorkflowCandidateRevision, _ *service.WorkflowCandidateRevision) {
			root.CanvasSessionID = "canvas-b"
		}},
		{name: "revision one runtime session", mutate: func(root *service.WorkflowCandidateRevision, _ *service.WorkflowCandidateRevision) {
			root.AgentSessionID = "session-b"
		}},
		{name: "revision one turn", mutate: func(root *service.WorkflowCandidateRevision, _ *service.WorkflowCandidateRevision) { root.TurnID = "" }},
		{name: "revision one tool call", mutate: func(root *service.WorkflowCandidateRevision, _ *service.WorkflowCandidateRevision) {
			root.ToolCallID = ""
		}},
		{name: "revision one candidate", mutate: func(root *service.WorkflowCandidateRevision, _ *service.WorkflowCandidateRevision) {
			root.CandidateID = "candidate-" + strings.Repeat("b", 24)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkflowAgentRevisedCandidateApplyApprovalFixture(t)
			test.mutate(fixture.store.parent, &fixture.store.revision)
			response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowAgentRevisedCandidateApplyApprovalPayload())
			if response.Code != http.StatusForbidden || len(fixture.runtime.requests) != 0 {
				t.Fatalf("status=%d body=%s runtime=%d", response.Code, response.Body.String(), len(fixture.runtime.requests))
			}
		})
	}
}

func TestWorkflowCandidateApplyApprovalRejectsMissingForgedGappedOrMixedIntermediateLineage(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*recordingWorkflowCandidateApplyApprovalStore)
	}{
		{name: "missing revision", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) { delete(store.intermediates, 2) }},
		{name: "forged change set", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.intermediates[2].ID = "changeset-revise-forged"
		}},
		{name: "gapped candidate revision", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.intermediates[2].CandidateRevision = 4
		}},
		{name: "gapped parent revision", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) { store.intermediates[2].ParentRevision = 0 }},
		{name: "gapped parent change set", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.revision.ParentChangeSetID = "changeset-other"
		}},
		{name: "mixed agent intermediate", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.intermediates[2].Actor = edit.ActorKind("agent")
		}},
		{name: "mixed ready intermediate", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.intermediates[2].State = service.WorkflowCandidateRevisionReady
		}},
		{name: "mixed intermediate actor", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) { store.intermediates[2].ActorID = "actor-b" }},
		{name: "mixed intermediate canvas", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.intermediates[2].CanvasSessionID = "canvas-b"
		}},
		{name: "mixed intermediate runtime", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.intermediates[2].AgentSessionID = "session-b"
		}},
		{name: "mixed intermediate turn", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.intermediates[2].TurnID = "agent-turn-b"
		}},
		{name: "mixed intermediate tool", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.intermediates[2].ToolCallID = "agent-call-b"
		}},
		{name: "mixed intermediate candidate", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.intermediates[2].CandidateID = "candidate-" + strings.Repeat("b", 24)
		}},
		{name: "mixed intermediate tenant", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.intermediates[2].Key.TenantID = "tenant-b"
		}},
		{name: "mixed intermediate base", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.intermediates[2].Base.ResourceVersion = "rv-other"
			store.intermediates[2].Validation.Basis.ResourceVersion = "rv-other"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkflowAgentTwiceRevisedCandidateApplyApprovalFixture(t)
			test.mutate(fixture.store)
			response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowAgentTwiceRevisedCandidateApplyApprovalPayload())
			if response.Code != http.StatusForbidden || len(fixture.runtime.requests) != 0 {
				t.Fatalf("status=%d body=%s runtime=%d", response.Code, response.Body.String(), len(fixture.runtime.requests))
			}
		})
	}
}

func TestWorkflowCandidateApplyApprovalRejectsInvalidCurrentAndAncestorValidation(t *testing.T) {
	for _, test := range []struct {
		name       string
		wantStatus int
		mutate     func(*recordingWorkflowCandidateApplyApprovalStore)
	}{
		{name: "invalid current", wantStatus: http.StatusInternalServerError, mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.revision.Validation.Valid = false
		}},
		{name: "empty current Candidate semantic hash", wantStatus: http.StatusInternalServerError, mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.revision.Validation.Basis.SemanticHash = ""
		}},
		{name: "illegal current Candidate semantic hash", wantStatus: http.StatusInternalServerError, mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.revision.Validation.Basis.SemanticHash = "not-a-canonical-semantic-hash"
		}},
		{name: "tampered current Candidate semantic hash", wantStatus: http.StatusForbidden, mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.revision.Validation.Basis.SemanticHash = "sha256:" + strings.Repeat("e", 64)
		}},
		{name: "invalid intermediate", wantStatus: http.StatusForbidden, mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.intermediates[2].Validation.Valid = false
		}},
		{name: "invalid Agent root", wantStatus: http.StatusForbidden, mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.parent.Validation.Valid = false
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkflowAgentTwiceRevisedCandidateApplyApprovalFixture(t)
			test.mutate(fixture.store)
			response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowAgentTwiceRevisedCandidateApplyApprovalPayload())
			if response.Code != test.wantStatus || len(fixture.runtime.requests) != 0 {
				t.Fatalf("status=%d body=%s runtime=%d", response.Code, response.Body.String(), len(fixture.runtime.requests))
			}
		})
	}
}

func TestWorkflowCandidateApplyApprovalRejectsNormalizedOperationChainReplacement(t *testing.T) {
	foreignOperation := func() edit.WorkflowOperation {
		label := "forged operation chain"
		return edit.NewOperation(&edit.UpdateNodeOperation{NodeID: "check", Set: edit.NodePatch{Label: &label}})
	}
	for _, test := range []struct {
		name   string
		mutate func(*recordingWorkflowCandidateApplyApprovalStore)
	}{
		{name: "ancestor chain replaced", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.intermediates[2].NormalizedOperations[0] = foreignOperation()
		}},
		{name: "current is not ancestor prefix", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.revision.NormalizedOperations[1] = foreignOperation()
		}},
		{name: "current truncates ancestor chain", mutate: func(store *recordingWorkflowCandidateApplyApprovalStore) {
			store.revision.NormalizedOperations = store.revision.NormalizedOperations[:1]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkflowAgentTwiceRevisedCandidateApplyApprovalFixture(t)
			test.mutate(fixture.store)
			response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowAgentTwiceRevisedCandidateApplyApprovalPayload())
			if response.Code != http.StatusForbidden || len(fixture.runtime.requests) != 0 {
				t.Fatalf("status=%d body=%s runtime=%d", response.Code, response.Body.String(), len(fixture.runtime.requests))
			}
		})
	}
}

func TestWorkflowCandidateApplyApprovalAcceptsRealPersistedAgentRevisionLineage(t *testing.T) {
	root, revised := workflowAgentRevisedCandidateApplyApprovalRevisions()
	root.State = service.WorkflowCandidateRevisionReady
	store := service.NewWorkflowCandidateFileStore(t.TempDir())
	ctx := context.Background()
	if _, err := store.Create(ctx, root); err != nil {
		t.Fatalf("create Agent v1: %v", err)
	}
	if _, err := store.Revise(ctx, revised.Key, revised.CandidateID, 1, revised); err != nil {
		t.Fatalf("persist user-revised v2: %v", err)
	}
	current, err := store.GetCurrent(ctx, revised.Key, revised.CandidateID)
	if err != nil {
		t.Fatal(err)
	}
	rootView, err := store.GetRevision(ctx, revised.Key, revised.CandidateID, 1)
	if err != nil {
		t.Fatal(err)
	}
	revisedView, err := store.GetRevision(ctx, revised.Key, revised.CandidateID, 2)
	if err != nil {
		t.Fatal(err)
	}
	eligible, err := store.CheckApplyEligibility(ctx, service.WorkflowCandidateApplyBinding{
		Key: revised.Key, CandidateID: revised.CandidateID, CandidateRevision: 2, ChangeSetID: revised.ID, PreviewHash: revised.PreviewHash,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !validPersistedWorkflowCandidate(revised.Key, current, revisedView, eligible) || !validPersistedWorkflowAgentRevisionLineage([]service.WorkflowCandidateRevision{revisedView, rootView}) {
		t.Fatalf("real persisted lineage was rejected: current=%#v root=%#v revised=%#v", current, rootView, revisedView)
	}
}

func TestWorkflowCandidateApplyApprovalAcceptsRealPersistedAgentRevisionLineageV3(t *testing.T) {
	root, intermediate, currentRevision := workflowAgentTwiceRevisedCandidateApplyApprovalRevisions()
	root.State = service.WorkflowCandidateRevisionReady
	intermediate.State = service.WorkflowCandidateRevisionReady
	store := service.NewWorkflowCandidateFileStore(t.TempDir())
	ctx := context.Background()
	if _, err := store.Create(ctx, root); err != nil {
		t.Fatalf("create Agent v1: %v", err)
	}
	if _, err := store.Revise(ctx, intermediate.Key, intermediate.CandidateID, 1, intermediate); err != nil {
		t.Fatalf("persist user-revised v2: %v", err)
	}
	if _, err := store.Revise(ctx, currentRevision.Key, currentRevision.CandidateID, 2, currentRevision); err != nil {
		t.Fatalf("persist user-revised v3: %v", err)
	}
	current, err := store.GetCurrent(ctx, currentRevision.Key, currentRevision.CandidateID)
	if err != nil {
		t.Fatal(err)
	}
	views := make([]service.WorkflowCandidateRevision, 0, 3)
	for number := uint64(3); number >= 1; number-- {
		view, viewErr := store.GetRevision(ctx, currentRevision.Key, currentRevision.CandidateID, number)
		if viewErr != nil {
			t.Fatal(viewErr)
		}
		views = append(views, view)
		if number == 1 {
			break
		}
	}
	eligible, err := store.CheckApplyEligibility(ctx, service.WorkflowCandidateApplyBinding{
		Key: currentRevision.Key, CandidateID: currentRevision.CandidateID, CandidateRevision: 3,
		ChangeSetID: currentRevision.ID, PreviewHash: currentRevision.PreviewHash,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !validPersistedWorkflowCandidate(currentRevision.Key, current, views[0], eligible) || !validPersistedWorkflowAgentRevisionLineage(views) {
		t.Fatalf("real persisted v3 lineage was rejected: current=%#v lineage=%#v", current, views)
	}
}

func TestWorkflowCandidateApplyApprovalRejectsEligibilityResolutionBindingDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*service.WorkflowCandidateRevision)
	}{
		{name: "tenant", mutate: func(value *service.WorkflowCandidateRevision) { value.Key.TenantID = "tenant-b" }},
		{name: "actor", mutate: func(value *service.WorkflowCandidateRevision) { value.ActorID = "actor-b" }},
		{name: "canvas session", mutate: func(value *service.WorkflowCandidateRevision) { value.CanvasSessionID = "canvas-b" }},
		{name: "runtime session", mutate: func(value *service.WorkflowCandidateRevision) { value.AgentSessionID = "session-b" }},
		{name: "runtime turn", mutate: func(value *service.WorkflowCandidateRevision) { value.TurnID = "old-agent-turn" }},
		{name: "candidate", mutate: func(value *service.WorkflowCandidateRevision) {
			value.CandidateID = "candidate-" + strings.Repeat("b", 24)
		}},
		{name: "candidate revision", mutate: func(value *service.WorkflowCandidateRevision) { value.CandidateRevision = 1 }},
		{name: "change set", mutate: func(value *service.WorkflowCandidateRevision) { value.ID = "changeset-other" }},
		{name: "preview hash", mutate: func(value *service.WorkflowCandidateRevision) {
			value.PreviewHash = "sha256:" + strings.Repeat("7", 64)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkflowAgentRevisedCandidateApplyApprovalFixture(t)
			eligible := fixture.store.revision
			test.mutate(&eligible)
			fixture.store.eligible = &eligible
			response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowAgentRevisedCandidateApplyApprovalPayload())
			if response.Code != http.StatusInternalServerError || len(fixture.runtime.requests) != 0 {
				t.Fatalf("status=%d body=%s runtime=%d", response.Code, response.Body.String(), len(fixture.runtime.requests))
			}
		})
	}
}

func TestWorkflowCandidateApplyApprovalDoesNotLetAgentV1BorrowV2Approval(t *testing.T) {
	fixture := newWorkflowAgentRevisedCandidateApplyApprovalFixture(t)
	oldSelector := workflowAgentRevisedCandidateApplyApprovalPayload()
	oldSelector["candidateRevision"] = uint64(1)
	old := postWorkflowCandidateApplyApproval(t, fixture.server, oldSelector)
	if old.Code != http.StatusBadRequest || len(fixture.runtime.requests) != 0 {
		t.Fatalf("old Agent selector status=%d body=%s runtime=%d", old.Code, old.Body.String(), len(fixture.runtime.requests))
	}

	approved := postWorkflowCandidateApplyApproval(t, fixture.server, workflowAgentRevisedCandidateApplyApprovalPayload())
	if approved.Code != http.StatusAccepted || len(fixture.runtime.requests) != 1 {
		t.Fatalf("revised selector status=%d body=%s runtime=%d", approved.Code, approved.Body.String(), len(fixture.runtime.requests))
	}
	var apply edit.ApplyPatchRequest
	if err := edit.DecodeStrict(fixture.runtime.requests[0].ToolCall.Arguments, &apply); err != nil {
		t.Fatal(err)
	}
	if apply.ChangeSetID == fixture.store.parent.ID || apply.PreviewHash == fixture.store.parent.PreviewHash || apply.ChangeSetID != fixture.store.revision.ID || apply.PreviewHash != fixture.store.revision.PreviewHash {
		t.Fatalf("approval was not bound exclusively to v2: %#v", apply)
	}
}

func TestWorkflowCandidateControlledApprovalCorrelationBindsExactRevision(t *testing.T) {
	fixture := newWorkflowAgentRevisedCandidateApplyApprovalFixture(t)
	scope := WorkflowCommandTrustedScope{TenantID: "tenant-a", ActorID: "actor-a", CanvasSessionID: "canvas-a", SessionID: "session-a"}
	root, revised := *fixture.store.parent, fixture.store.revision
	rootRequest, rootHash, err := buildWorkflowCandidateControlledApplyRequest(scope, root, "same-client-idempotency")
	if err != nil {
		t.Fatal(err)
	}
	revisedRequest, revisedHash, err := buildWorkflowCandidateControlledApplyRequest(scope, revised, "same-client-idempotency")
	if err != nil {
		t.Fatal(err)
	}
	if rootRequest.Turn.TurnID == revisedRequest.Turn.TurnID || rootRequest.ToolCall.ID == revisedRequest.ToolCall.ID || rootHash == revisedHash {
		t.Fatalf("runtime approval correlation can be borrowed across revisions: root=%s/%s/%s revised=%s/%s/%s", rootRequest.Turn.TurnID, rootRequest.ToolCall.ID, rootHash, revisedRequest.Turn.TurnID, revisedRequest.ToolCall.ID, revisedHash)
	}
}

func TestWorkflowCandidateApplyApprovalRejectsBrowserMutationAndIdentityBindings(t *testing.T) {
	for _, field := range []string{"tenantId", "actorId", "canvasSessionId", "sessionId", "changeSetId", "previewHash", "expectedRevision", "candidateGrantId", "applyApprovalId", "reason"} {
		t.Run(field, func(t *testing.T) {
			fixture := newWorkflowCandidateApplyApprovalFixture(t)
			payload := workflowCandidateApplyApprovalPayload()
			payload[field] = "forged"
			response := postWorkflowCandidateApplyApproval(t, fixture.server, payload)
			if response.Code != http.StatusBadRequest || len(fixture.runtime.requests) != 0 || fixture.store.applyChecks != 0 {
				t.Fatalf("status=%d body=%s runtime=%d checks=%d", response.Code, response.Body.String(), len(fixture.runtime.requests), fixture.store.applyChecks)
			}
		})
	}
}

func TestWorkflowCandidateApplyApprovalRejectsCrossActorCanvasAndRuntimeSession(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*service.WorkflowCandidateRevision)
	}{
		{name: "actor", mutate: func(value *service.WorkflowCandidateRevision) { value.ActorID = "actor-b" }},
		{name: "canvas session", mutate: func(value *service.WorkflowCandidateRevision) { value.CanvasSessionID = "canvas-b" }},
		{name: "runtime session", mutate: func(value *service.WorkflowCandidateRevision) { value.AgentSessionID = "session-b" }},
		{name: "agent turn", mutate: func(value *service.WorkflowCandidateRevision) { value.TurnID = "turn-agent" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkflowCandidateApplyApprovalFixture(t)
			test.mutate(&fixture.store.revision)
			response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowCandidateApplyApprovalPayload())
			if response.Code != http.StatusForbidden || len(fixture.runtime.requests) != 0 {
				t.Fatalf("status=%d body=%s runtime=%d", response.Code, response.Body.String(), len(fixture.runtime.requests))
			}
		})
	}
}

func TestWorkflowCandidateApplyApprovalRejectsStaleCandidateAndWorkflow(t *testing.T) {
	t.Run("candidate revision", func(t *testing.T) {
		fixture := newWorkflowCandidateApplyApprovalFixture(t)
		fixture.store.current.CurrentRevision = 2
		response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowCandidateApplyApprovalPayload())
		if response.Code != http.StatusConflict || len(fixture.runtime.requests) != 0 {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("committed workflow revision", func(t *testing.T) {
		fixture := newWorkflowCandidateApplyApprovalFixture(t)
		fixture.application.result.Revision.ResourceVersion = "rv-new"
		response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowCandidateApplyApprovalPayload())
		if response.Code != http.StatusConflict || len(fixture.runtime.requests) != 0 {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("expired candidate", func(t *testing.T) {
		fixture := newWorkflowCandidateApplyApprovalFixture(t)
		fixture.store.err = &edit.DomainError{Kind: "error", Code: edit.ErrorStaleChangeSet, Message: "candidate expired"}
		response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowCandidateApplyApprovalPayload())
		if response.Code != http.StatusConflict || len(fixture.runtime.requests) != 0 {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
}

func TestWorkflowCandidateApplyApprovalExactReplayIsStable(t *testing.T) {
	fixture := newWorkflowCandidateApplyApprovalFixture(t)
	first := postWorkflowCandidateApplyApproval(t, fixture.server, workflowCandidateApplyApprovalPayload())
	second := postWorkflowCandidateApplyApproval(t, fixture.server, workflowCandidateApplyApprovalPayload())
	if first.Code != http.StatusAccepted || second.Code != http.StatusAccepted || len(fixture.runtime.requests) != 2 {
		t.Fatalf("statuses=%d/%d runtime=%d", first.Code, second.Code, len(fixture.runtime.requests))
	}
	if !reflect.DeepEqual(fixture.runtime.requests[0], fixture.runtime.requests[1]) {
		t.Fatalf("controlled replay drift:\nfirst=%#v\nsecond=%#v", fixture.runtime.requests[0], fixture.runtime.requests[1])
	}
	var firstResult, secondResult workflowCandidateApplyApprovalResponse
	if json.NewDecoder(first.Body).Decode(&firstResult) != nil || json.NewDecoder(second.Body).Decode(&secondResult) != nil || !reflect.DeepEqual(firstResult, secondResult) {
		t.Fatalf("response replay drift: %#v / %#v", firstResult, secondResult)
	}
}

func TestWorkflowCandidateApplyApprovalFailsClosedWithoutRuntimeOrNormalPendingApproval(t *testing.T) {
	t.Run("runtime missing", func(t *testing.T) {
		fixture := newWorkflowCandidateApplyApprovalFixture(t)
		fixture.server.workflowCandidateApplyApprovalRuntime = nil
		response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowCandidateApplyApprovalPayload())
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("runtime executed instead of blocking", func(t *testing.T) {
		fixture := newWorkflowCandidateApplyApprovalFixture(t)
		fixture.runtime.completed = true
		response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowCandidateApplyApprovalPayload())
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("session missing", func(t *testing.T) {
		fixture := newWorkflowCandidateApplyApprovalFixture(t)
		fixture.sessions = runtimekernel.NewSessionManager()
		fixture.server = newWorkflowCandidateApplyApprovalServer(fixture.sessions, fixture.application, fixture.store, fixture.runtime)
		response := postWorkflowCandidateApplyApproval(t, fixture.server, workflowCandidateApplyApprovalPayload())
		if response.Code != http.StatusConflict || len(fixture.runtime.requests) != 0 {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
}

type workflowCandidateApplyApprovalFixture struct {
	server      *HTTPServer
	sessions    *runtimekernel.SessionManager
	application *recordingWorkflowCandidateApplyApprovalApplication
	store       *recordingWorkflowCandidateApplyApprovalStore
	runtime     *recordingWorkflowCandidateApplyApprovalRuntime
}

func newWorkflowCandidateApplyApprovalFixture(t *testing.T) workflowCandidateApplyApprovalFixture {
	t.Helper()
	revision := workflowCandidateApplyApprovalRevision()
	store := &recordingWorkflowCandidateApplyApprovalStore{
		current: edit.WorkflowCandidate{
			ID: revision.CandidateID, Key: revision.Key, CurrentChangeSetID: revision.ID,
			CurrentRevision: revision.CandidateRevision, CurrentPreviewHash: revision.PreviewHash,
			State: service.WorkflowCandidateStateActive, ExpiresAt: revision.ExpiresAt,
		},
		revision: revision,
	}
	application := &recordingWorkflowCandidateApplyApprovalApplication{result: service.WorkflowContextResult{Key: revision.Key, Revision: revision.Base}}
	runtime := &recordingWorkflowCandidateApplyApprovalRuntime{}
	sessions := runtimekernel.NewSessionManager()
	sessions.GetOrCreate("session-a", runtimekernel.SessionTypeWorkspace, runtimekernel.ModeExecute)
	return workflowCandidateApplyApprovalFixture{
		server:   newWorkflowCandidateApplyApprovalServer(sessions, application, store, runtime),
		sessions: sessions, application: application, store: store, runtime: runtime,
	}
}

func newWorkflowCandidateApplyApprovalServer(sessions *runtimekernel.SessionManager, application *recordingWorkflowCandidateApplyApprovalApplication, store *recordingWorkflowCandidateApplyApprovalStore, runtime *recordingWorkflowCandidateApplyApprovalRuntime) *HTTPServer {
	return NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, sessions),
		withWorkflowCommandApplication(application, workflowCommandTestScopeResolver()),
		WithWorkflowCandidateApplyApproval(store, runtime),
	)
}

type recordingWorkflowCandidateApplyApprovalApplication struct {
	contextCalls int
	result       service.WorkflowContextResult
	err          error
}

func (application *recordingWorkflowCandidateApplyApprovalApplication) ExecuteManualCommand(context.Context, service.WorkflowManualCommandRequest) (service.WorkflowManualCommandResult, error) {
	return service.WorkflowManualCommandResult{}, nil
}

func (application *recordingWorkflowCandidateApplyApprovalApplication) GetContext(context.Context, edit.WorkflowKey, service.WorkflowContextQuery) (service.WorkflowContextResult, error) {
	application.contextCalls++
	return application.result, application.err
}

type recordingWorkflowCandidateApplyApprovalStore struct {
	current       edit.WorkflowCandidate
	revision      service.WorkflowCandidateRevision
	parent        *service.WorkflowCandidateRevision
	intermediates map[uint64]*service.WorkflowCandidateRevision
	eligible      *service.WorkflowCandidateRevision
	err           error
	applyChecks   int
}

func (store *recordingWorkflowCandidateApplyApprovalStore) GetCurrent(_ context.Context, key edit.WorkflowKey, candidateID string) (edit.WorkflowCandidate, error) {
	if store.err != nil && errors.Is(store.err, service.ErrNotFound) {
		return edit.WorkflowCandidate{}, store.err
	}
	if key != store.current.Key || candidateID != store.current.ID {
		return edit.WorkflowCandidate{}, service.ErrNotFound
	}
	return store.current, nil
}

func (store *recordingWorkflowCandidateApplyApprovalStore) GetRevision(_ context.Context, key edit.WorkflowKey, candidateID string, revision uint64) (service.WorkflowCandidateRevision, error) {
	if store.parent != nil && key == store.parent.Key && candidateID == store.parent.CandidateID && revision == store.parent.CandidateRevision {
		return *store.parent, nil
	}
	if intermediate := store.intermediates[revision]; intermediate != nil && key == intermediate.Key && candidateID == intermediate.CandidateID {
		return *intermediate, nil
	}
	if key != store.revision.Key || candidateID != store.revision.CandidateID || revision != store.revision.CandidateRevision {
		return service.WorkflowCandidateRevision{}, service.ErrNotFound
	}
	return store.revision, nil
}

func (store *recordingWorkflowCandidateApplyApprovalStore) CheckApplyEligibility(_ context.Context, binding service.WorkflowCandidateApplyBinding, _ time.Time) (service.WorkflowCandidateRevision, error) {
	store.applyChecks++
	if store.err != nil {
		return service.WorkflowCandidateRevision{}, store.err
	}
	if binding.Key != store.revision.Key || binding.CandidateID != store.revision.CandidateID || binding.CandidateRevision != store.revision.CandidateRevision || binding.ChangeSetID != store.revision.ID || binding.PreviewHash != store.revision.PreviewHash {
		return service.WorkflowCandidateRevision{}, errors.New("unexpected candidate eligibility binding")
	}
	if store.eligible != nil {
		return *store.eligible, nil
	}
	return store.revision, nil
}

type recordingWorkflowCandidateApplyApprovalRuntime struct {
	requests  []runtimekernel.ControlledToolTurnRequest
	err       error
	completed bool
}

func (runtime *recordingWorkflowCandidateApplyApprovalRuntime) BeginControlledToolTurn(_ context.Context, request runtimekernel.ControlledToolTurnRequest) (runtimekernel.ControlledToolTurnResult, error) {
	runtime.requests = append(runtime.requests, request)
	if runtime.err != nil {
		return runtimekernel.ControlledToolTurnResult{}, runtime.err
	}
	hash, _ := actionproposal.NormalizedInputHash(request.ToolCall.Arguments)
	result := runtimekernel.ControlledToolTurnResult{TurnResult: runtimekernel.TurnResult{
		SessionType: request.Turn.SessionType, Mode: request.Turn.Mode, SessionID: request.Turn.SessionID, TurnID: request.Turn.TurnID, Status: "blocked",
	}}
	if runtime.completed {
		result.Status = "completed"
		result.ToolResult = &runtimekernel.ToolResult{ToolCallID: request.ToolCall.ID, Content: "unexpected execution"}
		return result, nil
	}
	result.Approval = &runtimekernel.ControlledToolApproval{
		ApprovalID: "approval-controlled", SessionID: request.Turn.SessionID, TurnID: request.Turn.TurnID,
		ToolCallID: request.ToolCall.ID, ToolName: request.ToolCall.Name, ArgumentsHash: hash,
	}
	return result, nil
}

func workflowCandidateApplyApprovalRevision() service.WorkflowCandidateRevision {
	now := time.Now().UTC()
	base := edit.Revision{
		ResourceVersion: "rv-17", SemanticHash: "sha256:" + strings.Repeat("1", 64), LayoutHash: "sha256:" + strings.Repeat("2", 64),
	}
	position := visual.Position{X: 240, Y: 120}
	revision := service.WorkflowCandidateRevision{
		WorkflowChangeSet: edit.WorkflowChangeSet{
			SchemaVersion: edit.SchemaVersion, ID: "changeset-history-a",
			Key:         edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"},
			CandidateID: "history-" + strings.Repeat("a", 64), CandidateRevision: 1,
			Actor: edit.ActorKind("user"), Intent: "undo workflow transaction txn-17", Base: base,
			Operations:  []edit.WorkflowOperation{edit.NewOperation(&edit.UpdateNodeOperation{NodeID: "check", Set: edit.NodePatch{Position: &position}})},
			PreviewHash: "sha256:" + strings.Repeat("3", 64),
			Validation: edit.ValidationResult{Kind: "validation", Basis: edit.ValidationBasis{
				ResourceVersion: base.ResourceVersion, SemanticHash: base.SemanticHash,
				CatalogVersion: "catalog-1", ActionSchemaDigest: "actions-1", ReferenceSnapshotHash: "refs-1", EnvironmentBindingVersion: "env-1",
			}, Valid: true, Diagnostics: []edit.WorkflowDiagnostic{}},
			State: service.WorkflowCandidateRevisionReady, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		},
		NormalizedOperations: []edit.WorkflowOperation{edit.NewOperation(&edit.UpdateNodeOperation{NodeID: "check", Set: edit.NodePatch{Position: &position}})},
		Risk:                 json.RawMessage(`{"level":"high"}`), Diff: json.RawMessage(`{"graph":{}}`),
		ActorID: "actor-a", CanvasSessionID: "canvas-a", AgentSessionID: "session-a",
	}
	revision.PreviewHash = workflowCandidateApplyApprovalPreviewHash(revision)
	return revision
}

func newWorkflowAgentRevisedCandidateApplyApprovalFixture(t *testing.T) workflowCandidateApplyApprovalFixture {
	t.Helper()
	root, revised := workflowAgentRevisedCandidateApplyApprovalRevisions()
	store := &recordingWorkflowCandidateApplyApprovalStore{
		current: edit.WorkflowCandidate{
			ID: revised.CandidateID, Key: revised.Key, CurrentChangeSetID: revised.ID,
			CurrentRevision: revised.CandidateRevision, CurrentPreviewHash: revised.PreviewHash,
			State: service.WorkflowCandidateStateActive, ExpiresAt: revised.ExpiresAt,
		},
		revision: revised,
		parent:   &root,
	}
	application := &recordingWorkflowCandidateApplyApprovalApplication{result: service.WorkflowContextResult{Key: revised.Key, Revision: revised.Base}}
	runtime := &recordingWorkflowCandidateApplyApprovalRuntime{}
	sessions := runtimekernel.NewSessionManager()
	sessions.GetOrCreate("session-a", runtimekernel.SessionTypeWorkspace, runtimekernel.ModeExecute)
	return workflowCandidateApplyApprovalFixture{
		server:   newWorkflowCandidateApplyApprovalServer(sessions, application, store, runtime),
		sessions: sessions, application: application, store: store, runtime: runtime,
	}
}

func newWorkflowAgentTwiceRevisedCandidateApplyApprovalFixture(t *testing.T) workflowCandidateApplyApprovalFixture {
	t.Helper()
	root, intermediate, currentRevision := workflowAgentTwiceRevisedCandidateApplyApprovalRevisions()
	store := &recordingWorkflowCandidateApplyApprovalStore{
		current: edit.WorkflowCandidate{
			ID: currentRevision.CandidateID, Key: currentRevision.Key, CurrentChangeSetID: currentRevision.ID,
			CurrentRevision: currentRevision.CandidateRevision, CurrentPreviewHash: currentRevision.PreviewHash,
			State: service.WorkflowCandidateStateActive, ExpiresAt: currentRevision.ExpiresAt,
		},
		revision:      currentRevision,
		parent:        &root,
		intermediates: map[uint64]*service.WorkflowCandidateRevision{2: &intermediate},
	}
	application := &recordingWorkflowCandidateApplyApprovalApplication{result: service.WorkflowContextResult{Key: currentRevision.Key, Revision: currentRevision.Base}}
	runtime := &recordingWorkflowCandidateApplyApprovalRuntime{}
	sessions := runtimekernel.NewSessionManager()
	sessions.GetOrCreate("session-a", runtimekernel.SessionTypeWorkspace, runtimekernel.ModeExecute)
	return workflowCandidateApplyApprovalFixture{
		server:   newWorkflowCandidateApplyApprovalServer(sessions, application, store, runtime),
		sessions: sessions, application: application, store: store, runtime: runtime,
	}
}

func workflowAgentRevisedCandidateApplyApprovalRevisions() (service.WorkflowCandidateRevision, service.WorkflowCandidateRevision) {
	now := time.Now().UTC()
	base := edit.Revision{
		ResourceVersion: "rv-17", SemanticHash: "sha256:" + strings.Repeat("1", 64), LayoutHash: "sha256:" + strings.Repeat("2", 64),
	}
	position := visual.Position{X: 240, Y: 120}
	rootOperation := edit.NewOperation(&edit.UpdateNodeOperation{NodeID: "check", Set: edit.NodePatch{Position: &position}})
	validation := edit.ValidationResult{Kind: "validation", Basis: edit.ValidationBasis{
		ResourceVersion: base.ResourceVersion, SemanticHash: base.SemanticHash,
		CatalogVersion: "catalog-1", ActionSchemaDigest: "actions-1", ReferenceSnapshotHash: "refs-1", EnvironmentBindingVersion: "env-1",
	}, Valid: true, Diagnostics: []edit.WorkflowDiagnostic{}}
	candidateID := "candidate-" + strings.Repeat("a", 24)
	root := service.WorkflowCandidateRevision{
		WorkflowChangeSet: edit.WorkflowChangeSet{
			SchemaVersion: edit.SchemaVersion, ID: "changeset-" + strings.Repeat("8", 64), Key: edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"},
			CandidateID: candidateID, CandidateRevision: 1, Actor: edit.ActorKind("agent"), Intent: "raise threshold", Base: base,
			Operations: []edit.WorkflowOperation{rootOperation}, PreviewHash: "sha256:" + strings.Repeat("3", 64), Validation: validation,
			State: service.WorkflowCandidateRevisionSuperseded, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		},
		NormalizedOperations: []edit.WorkflowOperation{rootOperation}, Risk: json.RawMessage(`{"level":"high"}`), Diff: json.RawMessage(`{"graph":{}}`),
		ActorID: "actor-a", CanvasSessionID: "canvas-a", AgentSessionID: "session-a", TurnID: "agent-turn-a", ToolCallID: "agent-call-a",
	}
	root.PreviewHash = workflowCandidateApplyApprovalPreviewHash(root)
	label := "Reviewed threshold"
	userOperation := edit.NewOperation(&edit.UpdateNodeOperation{NodeID: "check", Set: edit.NodePatch{Label: &label}})
	revisedValidation := validation
	revisedValidation.Basis.SemanticHash = "sha256:" + strings.Repeat("7", 64)
	revised := service.WorkflowCandidateRevision{
		WorkflowChangeSet: edit.WorkflowChangeSet{
			SchemaVersion: edit.SchemaVersion, ID: "changeset-revise-" + strings.Repeat("4", 64) + "-" + strings.Repeat("5", 64),
			Key: root.Key, CandidateID: candidateID, CandidateRevision: 2, ParentChangeSetID: root.ID,
			Actor: edit.ActorKind("user"), Intent: "review and refine the Agent candidate", Base: base,
			Operations: []edit.WorkflowOperation{rootOperation, userOperation}, Validation: revisedValidation,
			State: service.WorkflowCandidateRevisionReady, CreatedAt: now.Add(time.Second), ExpiresAt: now.Add(time.Hour),
		},
		ParentRevision: 1, NormalizedOperations: []edit.WorkflowOperation{rootOperation, userOperation},
		Risk: json.RawMessage(`{"level":"high"}`), Diff: json.RawMessage(`{"graph":{}}`),
		ActorID: "actor-a", CanvasSessionID: "canvas-a", AgentSessionID: "session-a",
	}
	revised.PreviewHash = workflowCandidateApplyApprovalPreviewHash(revised)
	return root, revised
}

func workflowAgentTwiceRevisedCandidateApplyApprovalRevisions() (service.WorkflowCandidateRevision, service.WorkflowCandidateRevision, service.WorkflowCandidateRevision) {
	root, intermediate := workflowAgentRevisedCandidateApplyApprovalRevisions()
	intermediate.State = service.WorkflowCandidateRevisionSuperseded
	finalLabel := "Final reviewed threshold"
	finalOperation := edit.NewOperation(&edit.UpdateNodeOperation{NodeID: "check", Set: edit.NodePatch{Label: &finalLabel}})
	current := service.WorkflowCandidateRevision{
		WorkflowChangeSet: edit.WorkflowChangeSet{
			SchemaVersion: edit.SchemaVersion, ID: "changeset-revise-" + strings.Repeat("9", 64) + "-" + strings.Repeat("a", 64),
			Key: root.Key, CandidateID: root.CandidateID, CandidateRevision: 3, ParentChangeSetID: intermediate.ID,
			Actor: edit.ActorKind("user"), Intent: "final user refinement", Base: root.Base,
			Operations:  append(append([]edit.WorkflowOperation(nil), intermediate.Operations...), finalOperation),
			PreviewHash: "sha256:" + strings.Repeat("b", 64), Validation: intermediate.Validation,
			State: service.WorkflowCandidateRevisionReady, CreatedAt: intermediate.CreatedAt.Add(time.Second), ExpiresAt: intermediate.ExpiresAt,
		},
		ParentRevision:       2,
		NormalizedOperations: append(append([]edit.WorkflowOperation(nil), intermediate.NormalizedOperations...), finalOperation),
		Risk:                 append(json.RawMessage(nil), intermediate.Risk...), Diff: append(json.RawMessage(nil), intermediate.Diff...),
		ActorID: intermediate.ActorID, CanvasSessionID: intermediate.CanvasSessionID, AgentSessionID: intermediate.AgentSessionID,
	}
	current.Validation.Basis.SemanticHash = "sha256:" + strings.Repeat("8", 64)
	current.PreviewHash = workflowCandidateApplyApprovalPreviewHash(current)
	return root, intermediate, current
}

func workflowCandidateApplyApprovalPreviewHash(revision service.WorkflowCandidateRevision) string {
	digest, err := workflowCandidateApplyDigest([]any{
		edit.SchemaVersion, revision.Key, revision.CandidateRevision, revision.ParentChangeSetID,
		revision.NormalizedOperations, revision.Validation.Basis,
	})
	if err != nil {
		panic(err)
	}
	return "sha256:" + digest
}

func workflowAgentRevisedCandidateApplyApprovalPayload() map[string]any {
	return map[string]any{
		"schemaVersion": edit.SchemaVersion, "workflowId": "workflow-a",
		"candidateId": "candidate-" + strings.Repeat("a", 24), "candidateRevision": uint64(2),
		"idempotencyKey": "agent-revised-apply-once", "action": workflowCandidateApplyApprovalAction,
	}
}

func workflowAgentTwiceRevisedCandidateApplyApprovalPayload() map[string]any {
	payload := workflowAgentRevisedCandidateApplyApprovalPayload()
	payload["candidateRevision"] = uint64(3)
	payload["idempotencyKey"] = "agent-twice-revised-apply-once"
	return payload
}

func workflowCandidateApplyApprovalPayload() map[string]any {
	return map[string]any{
		"schemaVersion": edit.SchemaVersion, "workflowId": "workflow-a",
		"candidateId": "history-" + strings.Repeat("a", 64), "candidateRevision": 1,
		"idempotencyKey": "history-apply-once", "action": workflowCandidateApplyApprovalAction,
	}
}

func postWorkflowCandidateApplyApproval(t *testing.T, server *HTTPServer, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/runner-studio/workflow-candidate-apply-approval", bytes.NewReader(raw))
	server.Handler().ServeHTTP(response, request)
	return response
}
