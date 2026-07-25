package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"runner/workflow/edit"
)

const workflowVerifiedRuntimeApprovalTTL = 10 * time.Minute

const (
	workflowRuntimeApplyToolName      = "workflow.apply_patch"
	workflowRuntimeApplyProviderAlias = "workflow_apply_patch"
)

// WorkflowVerifiedRuntimeApplyRequest is a trusted adapter boundary. The
// caller must populate the Runtime fields only after RuntimeKernel verifies
// the current ActionToken against this exact workflow.apply_patch call.
// RuntimeSessionID identifies that verified agent execution. Canvas identity
// is deliberately absent and is resolved only from the persisted Candidate;
// browser/model JSON is never sufficient to construct this authority.
type WorkflowVerifiedRuntimeApplyRequest struct {
	SchemaVersion        string
	Key                  edit.WorkflowKey
	ActorID              string
	RuntimeSessionID     string
	ChangeSetID          string
	PreviewHash          string
	ExpectedRevision     string
	CandidateGrantID     string
	ApplyApprovalID      string
	RuntimeApprovalID    string
	RuntimeTurnID        string
	RuntimeToolCallID    string
	RuntimeToolName      string
	RuntimeArgumentsHash string
	RuntimeBindingHash   string
	RuntimeTargetRefs    []string
}

// AuthorizeVerifiedRuntimeApply atomically materializes the exact persisted
// Grant/ApplyApproval required by Apply. Repeating the same verified Runtime
// binding is safe, including after its Apply has been durably consumed.
func (a *WorkflowEditApplication) AuthorizeVerifiedRuntimeApply(ctx context.Context, request WorkflowVerifiedRuntimeApplyRequest) (WorkflowEditTrustedApplyContext, error) {
	if a == nil || a.candidates == nil || a.approvals == nil || a.commits == nil || a.now == nil {
		return WorkflowEditTrustedApplyContext{}, ErrWorkflowEditConfiguration
	}
	if err := validateWorkflowVerifiedRuntimeApplyRequest(request); err != nil {
		return WorkflowEditTrustedApplyContext{}, err
	}
	candidate, err := a.candidates.ResolveChangeSet(ctx, request.Key, request.ChangeSetID)
	if err != nil {
		return WorkflowEditTrustedApplyContext{}, err
	}
	if candidate.PreviewHash != request.PreviewHash {
		return WorkflowEditTrustedApplyContext{}, workflowPreviewDomainError(edit.ErrorPreviewMismatch, request.Key, candidate.PreviewHash, request.PreviewHash, "Runtime approval does not match the current preview", "review and approve the current preview")
	}
	if candidate.Base.ResourceVersion != request.ExpectedRevision {
		return WorkflowEditTrustedApplyContext{}, workflowPreviewDomainError(edit.ErrorStaleRevision, request.Key, candidate.Base.ResourceVersion, request.ExpectedRevision, "Runtime approval does not match the candidate base revision", "reload and approve the current candidate")
	}
	// Preview and Apply are separate tool calls, so the Candidate ToolCallID is
	// provenance rather than the current Apply call ID. RuntimeToolCallID below
	// remains bound to the exact verified Apply approval and plan hash.
	if candidate.ActorID != request.ActorID || candidate.AgentSessionID != request.RuntimeSessionID || (candidate.TurnID != "" && candidate.TurnID != request.RuntimeTurnID) {
		return WorkflowEditTrustedApplyContext{}, workflowApprovalError(edit.ErrorApprovalRequired, request.Key, "verified Runtime approval does not match the Candidate editing scope", "review the Candidate in the current authenticated session")
	}
	at := a.now().UTC()
	planHash, err := workflowVerifiedRuntimeApplyPlanHash(request, candidate)
	if err != nil {
		return WorkflowEditTrustedApplyContext{}, err
	}
	trusted := WorkflowEditTrustedApplyContext{
		Key: request.Key, ActorID: request.ActorID, CanvasSessionID: candidate.CanvasSessionID,
		CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision,
		ChangeSetID: candidate.ID, PreviewHash: candidate.PreviewHash, ExpectedRevision: candidate.Base.ResourceVersion,
		GrantID: request.CandidateGrantID, ApprovalID: request.ApplyApprovalID, PlanHash: planHash,
		BaseSemanticHash: candidate.Base.SemanticHash, ReferenceSnapshotHash: candidate.Validation.Basis.ReferenceSnapshotHash,
	}
	facts, err := workflowEditApplyFacts(candidate, trusted)
	if err != nil {
		return WorkflowEditTrustedApplyContext{}, err
	}
	expiresAt := at.Add(workflowVerifiedRuntimeApprovalTTL)
	if candidate.ExpiresAt.Before(expiresAt) {
		expiresAt = candidate.ExpiresAt
	}
	grant := WorkflowEditGrantRecord{Grant: edit.WorkflowEditGrant{
		ID: facts.GrantID, Key: facts.Key, ActorID: facts.ActorID, PlanHash: facts.PlanHash,
		BaseSemanticHash: facts.BaseSemanticHash, AllowedOps: append([]edit.OperationKind(nil), facts.Operations...),
		MaxOperations: facts.OperationCount, MaxAddedNodes: facts.AddedNodeCount,
		AllowDeleteExisting: facts.DeletesExisting, ExpiresAt: expiresAt,
	}, CanvasSessionID: facts.CanvasSessionID, RiskScope: WorkflowGrantRiskScope{MaxRisk: facts.Risk}, State: WorkflowApprovalActive, IssuedAt: at}
	approval := WorkflowApplyApprovalRecord{Approval: edit.WorkflowApplyApproval{
		ID: facts.ApprovalID, ActorID: facts.ActorID, Key: facts.Key, CandidateID: facts.CandidateID,
		ChangeSetID: facts.ChangeSetID, PreviewHash: facts.PreviewHash, ExpectedRevision: facts.ExpectedRevision,
		ExpiresAt: expiresAt,
	}, GrantID: facts.GrantID, CanvasSessionID: facts.CanvasSessionID, PlanHash: facts.PlanHash, State: WorkflowApprovalActive, IssuedAt: at}
	consumed, err := a.approvals.verifiedRuntimeApplyConsumed(ctx, grant, approval)
	if err != nil {
		return WorkflowEditTrustedApplyContext{}, err
	}
	if consumed {
		return a.ResolveTrustedApplyContext(ctx, WorkflowTrustedApplySelector{
			Key: request.Key, ActorID: request.ActorID, CanvasSessionID: candidate.CanvasSessionID,
			ChangeSetID: request.ChangeSetID, ApprovalID: request.ApplyApprovalID,
		})
	}
	binding := WorkflowCandidateApplyBinding{Key: request.Key, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, ChangeSetID: candidate.ID, PreviewHash: candidate.PreviewHash}
	if _, err := a.candidates.CheckApplyEligibility(ctx, binding, at); err != nil {
		return WorkflowEditTrustedApplyContext{}, err
	}
	if err := a.approvals.issueVerifiedRuntimeApply(ctx, grant, approval, at); err != nil {
		return WorkflowEditTrustedApplyContext{}, err
	}
	return a.ResolveTrustedApplyContext(ctx, WorkflowTrustedApplySelector{
		Key: request.Key, ActorID: request.ActorID, CanvasSessionID: candidate.CanvasSessionID,
		ChangeSetID: request.ChangeSetID, ApprovalID: request.ApplyApprovalID,
	})
}

func (s *WorkflowApprovalFileStore) verifiedRuntimeApplyConsumed(ctx context.Context, grant WorkflowEditGrantRecord, approval WorkflowApplyApprovalRecord) (bool, error) {
	currentApproval, err := s.GetApplyApproval(ctx, approval.Approval.Key, approval.Approval.ID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil || currentApproval.State != WorkflowApprovalConsumed {
		return false, err
	}
	currentGrant, err := s.GetGrant(ctx, grant.Grant.Key, grant.Grant.ID)
	if err != nil {
		return false, err
	}
	if currentApproval.ConsumptionID == "" || currentApproval.Approval.ConsumedAt == nil || !workflowRuntimeAuthorizationMatches(currentGrant, grant, currentApproval, approval) {
		return false, workflowApprovalError(edit.ErrorApprovalRequired, grant.Grant.Key, "consumed workflow approval does not match this verified Runtime binding", "read the existing receipt or approve a new exact request")
	}
	return true, nil
}

func validateWorkflowVerifiedRuntimeApplyRequest(request WorkflowVerifiedRuntimeApplyRequest) error {
	values := []string{
		request.ActorID, request.RuntimeSessionID, request.ChangeSetID, request.PreviewHash, request.ExpectedRevision,
		request.CandidateGrantID, request.ApplyApprovalID, request.RuntimeApprovalID, request.RuntimeTurnID,
		request.RuntimeToolCallID, request.RuntimeArgumentsHash, request.RuntimeBindingHash,
	}
	if request.SchemaVersion != edit.SchemaVersion || request.Key.Validate() != nil || !validWorkflowRuntimeApplyToolName(request.RuntimeToolName) || len(request.RuntimeTargetRefs) == 0 {
		return fmt.Errorf("%w: invalid verified Runtime workflow approval", ErrInvalid)
	}
	for _, value := range values {
		if !validWorkflowApprovalValue(value, false) {
			return fmt.Errorf("%w: invalid verified Runtime workflow approval", ErrInvalid)
		}
	}
	if !validWorkflowRuntimeApprovalHash(request.RuntimeArgumentsHash) || !validWorkflowRuntimeApprovalHash(request.RuntimeBindingHash) {
		return fmt.Errorf("%w: invalid verified Runtime workflow approval digest", ErrInvalid)
	}
	if !validWorkflowRuntimeTargetRefs(request.Key, request.RuntimeTargetRefs) {
		return workflowApprovalError(edit.ErrorApprovalRequired, request.Key, "verified Runtime approval targets another resource", "approve this exact workflow.apply_patch call")
	}
	return nil
}

func validWorkflowRuntimeApplyToolName(value string) bool {
	return value == workflowRuntimeApplyToolName || value == workflowRuntimeApplyProviderAlias
}

func validWorkflowRuntimeTargetRefs(key edit.WorkflowKey, refs []string) bool {
	if len(refs) == 0 || !slices.IsSorted(refs) {
		return false
	}
	exact := "type=workflow id=" + key.TenantID + ":" + key.WorkflowID + " op=apply"
	exactCount := 0
	for index, ref := range refs {
		if ref == "" || ref != strings.TrimSpace(ref) || (index > 0 && refs[index-1] == ref) {
			return false
		}
		switch ref {
		case "type=workflow", "type=workflow_candidate":
			// Runtime discovery scopes carry no ID and therefore grant no
			// mutation authority by themselves.
		case exact:
			exactCount++
		default:
			return false
		}
	}
	return exactCount == 1
}

func workflowVerifiedRuntimeApplyPlanHash(request WorkflowVerifiedRuntimeApplyRequest, candidate WorkflowCandidateRevision) (string, error) {
	return workflowQueryDigest([]any{
		edit.SchemaVersion, "verified-runtime-workflow-apply", request.Key, request.ActorID, request.RuntimeSessionID,
		candidate.CanvasSessionID, candidate.AgentSessionID, candidate.TurnID, candidate.ToolCallID,
		candidate.CandidateID, candidate.CandidateRevision, candidate.ID, candidate.PreviewHash, candidate.Base,
		request.CandidateGrantID, request.ApplyApprovalID, request.RuntimeApprovalID, request.RuntimeTurnID,
		request.RuntimeToolCallID, request.RuntimeToolName, request.RuntimeArgumentsHash, request.RuntimeBindingHash,
		request.RuntimeTargetRefs,
	})
}

func validWorkflowRuntimeApprovalHash(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(decoded) == 32
}

func (s *WorkflowApprovalFileStore) issueVerifiedRuntimeApply(ctx context.Context, grant WorkflowEditGrantRecord, approval WorkflowApplyApprovalRecord, at time.Time) error {
	if s == nil || s.transactions == nil || grant.Grant.Key != approval.Approval.Key || grant.Grant.ID != approval.GrantID || grant.CanvasSessionID != approval.CanvasSessionID || grant.Grant.PlanHash != approval.PlanHash || grant.IssuedAt != approval.IssuedAt || grant.Grant.ExpiresAt != approval.Approval.ExpiresAt {
		return ErrWorkflowEditConfiguration
	}
	if err := validateWorkflowGrantRecord(grant); err != nil {
		return err
	}
	if err := validateWorkflowApplyApprovalRecord(approval); err != nil {
		return err
	}
	return s.withState(ctx, grant.Grant.Key, true, func(state *workflowApprovalFile) (bool, error) {
		currentGrant, grantFound := state.Grants[grant.Grant.ID]
		currentApproval, approvalFound := state.Approvals[approval.Approval.ID]
		if grantFound || approvalFound {
			if !grantFound || !approvalFound || !workflowRuntimeAuthorizationMatches(currentGrant, grant, currentApproval, approval) {
				return false, workflowApprovalError(edit.ErrorApprovalRequired, grant.Grant.Key, "persistent workflow approval ID is bound to another Runtime approval", "use new exact grant and approval IDs")
			}
			if currentApproval.State == WorkflowApprovalConsumed && currentApproval.ConsumptionID != "" && currentApproval.Approval.ConsumedAt != nil {
				return false, nil
			}
			dirty := expireWorkflowApprovals(state, at) > 0
			currentGrant, currentApproval = state.Grants[grant.Grant.ID], state.Approvals[approval.Approval.ID]
			if currentGrant.State == WorkflowApprovalActive && currentApproval.State == WorkflowApprovalActive {
				return dirty, nil
			}
			return dirty, workflowApprovalError(edit.ErrorApprovalRequired, grant.Grant.Key, "persistent workflow Runtime approval is no longer active", "approve the current Candidate again with new IDs")
		}
		grant.IssuedAt, grant.Grant.ExpiresAt = grant.IssuedAt.UTC(), grant.Grant.ExpiresAt.UTC()
		approval.IssuedAt, approval.Approval.ExpiresAt = approval.IssuedAt.UTC(), approval.Approval.ExpiresAt.UTC()
		state.Grants[grant.Grant.ID], state.Approvals[approval.Approval.ID] = grant, approval
		state.UpdatedAt = grant.IssuedAt
		return true, nil
	})
}

func workflowRuntimeGrantMatches(current, expected WorkflowEditGrantRecord) bool {
	return current.Grant.ID == expected.Grant.ID && current.Grant.Key == expected.Grant.Key && current.Grant.ActorID == expected.Grant.ActorID &&
		current.Grant.PlanHash == expected.Grant.PlanHash && current.Grant.BaseSemanticHash == expected.Grant.BaseSemanticHash &&
		slices.Equal(current.Grant.AllowedOps, expected.Grant.AllowedOps) && current.Grant.MaxOperations == expected.Grant.MaxOperations &&
		current.Grant.MaxAddedNodes == expected.Grant.MaxAddedNodes && current.Grant.AllowDeleteExisting == expected.Grant.AllowDeleteExisting &&
		current.CanvasSessionID == expected.CanvasSessionID && current.RiskScope == expected.RiskScope
}

func workflowRuntimeApprovalMatches(current, expected WorkflowApplyApprovalRecord) bool {
	return current.Approval.ID == expected.Approval.ID && current.Approval.ActorID == expected.Approval.ActorID && current.Approval.Key == expected.Approval.Key &&
		current.Approval.CandidateID == expected.Approval.CandidateID && current.Approval.ChangeSetID == expected.Approval.ChangeSetID &&
		current.Approval.PreviewHash == expected.Approval.PreviewHash && current.Approval.ExpectedRevision == expected.Approval.ExpectedRevision &&
		current.GrantID == expected.GrantID && current.CanvasSessionID == expected.CanvasSessionID && current.PlanHash == expected.PlanHash
}

func workflowRuntimeAuthorizationMatches(currentGrant, expectedGrant WorkflowEditGrantRecord, currentApproval, expectedApproval WorkflowApplyApprovalRecord) bool {
	return workflowRuntimeGrantMatches(currentGrant, expectedGrant) && workflowRuntimeApprovalMatches(currentApproval, expectedApproval) &&
		currentGrant.Grant.ExpiresAt == currentApproval.Approval.ExpiresAt &&
		!currentGrant.Grant.ExpiresAt.After(expectedGrant.Grant.ExpiresAt) && !currentApproval.Approval.ExpiresAt.After(expectedApproval.Approval.ExpiresAt)
}
