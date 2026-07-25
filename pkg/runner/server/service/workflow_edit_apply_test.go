package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowEditApplyCommitsVerifyingThenPostCheckSettlesAndReplays(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	validated, err := fixture.app.ValidateChange(context.Background(), edit.ValidateRequest{SchemaVersion: edit.SchemaVersion, WorkflowKey: fixture.request.WorkflowKey, ChangeSetID: fixture.request.ChangeSetID})
	if err != nil || !validated.Valid || validated.Basis.ReferenceSnapshotHash != fixture.trusted.ReferenceSnapshotHash {
		t.Fatalf("ValidateChange(candidate) = %#v, %v", validated, err)
	}
	receipt, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request)
	if err != nil || receipt.Status != "verifying" || receipt.RevisionAfter == nil {
		t.Fatalf("Apply() = %#v, %v", receipt, err)
	}
	snapshot, err := fixture.commits.Snapshot(context.Background(), fixture.request.WorkflowKey)
	if err != nil || snapshot.Generation != 2 || snapshot.ResourceVersion != receipt.RevisionAfter.ResourceVersion || len(snapshot.ApprovalConsumptions) != 1 {
		t.Fatalf("applied snapshot = %#v, %v", snapshot, err)
	}
	graph, err := parseCommittedVisualGraph(snapshot.GraphPayload, snapshot.ResourceVersion)
	if err != nil || nodePosition(graph, "check") != (workflowEditApplyPosition()) {
		t.Fatalf("committed graph position=%#v err=%v", nodePosition(graph, "check"), err)
	}
	approval, err := fixture.approvals.GetApplyApproval(context.Background(), fixture.request.WorkflowKey, fixture.request.ApplyApprovalID)
	if err != nil || approval.State != WorkflowApprovalConsumed {
		t.Fatalf("approval after apply = %#v, %v", approval, err)
	}

	settled, err := fixture.app.RecordPostCheck(context.Background(), WorkflowPostCheckFacts{Key: receipt.Key, TransactionID: receipt.TransactionID, Basis: receipt.Validation.Basis, Outcome: WorkflowPostCheckPassed, Diagnostics: []edit.WorkflowDiagnostic{}})
	if err != nil || settled.Status != "completed" {
		t.Fatalf("RecordPostCheck() = %#v, %v", settled, err)
	}
	afterPostCheck, _ := fixture.commits.Snapshot(context.Background(), receipt.Key)
	if afterPostCheck.Generation != 3 || len(afterPostCheck.LifecycleArtifacts) != 1 {
		t.Fatalf("post-check snapshot = %#v", afterPostCheck)
	}
	resolvedAfterCommit, err := fixture.app.ResolveTrustedApplyContext(context.Background(), WorkflowTrustedApplySelector{Key: fixture.request.WorkflowKey, ActorID: fixture.request.ActorID, CanvasSessionID: fixture.request.CanvasSessionID, ChangeSetID: fixture.request.ChangeSetID, ApprovalID: fixture.request.ApplyApprovalID})
	if err != nil || resolvedAfterCommit != fixture.trusted {
		t.Fatalf("ResolveTrustedApplyContext(consumed) = %#v, %v", resolvedAfterCommit, err)
	}
	replayed, err := fixture.app.Apply(context.Background(), resolvedAfterCommit, fixture.request)
	if err != nil || replayed.TransactionID != receipt.TransactionID || replayed.Status != "completed" {
		t.Fatalf("exact apply replay = %#v, %v", replayed, err)
	}
	afterReplay, _ := fixture.commits.Snapshot(context.Background(), receipt.Key)
	if afterReplay.Generation != 3 {
		t.Fatalf("replay advanced generation to %d", afterReplay.Generation)
	}
	reused := fixture.request
	reused.Reason = "different payload under same idempotency key"
	if _, err := fixture.app.Apply(context.Background(), fixture.trusted, reused); workflowCommitErrorCode(err) != edit.ErrorIdempotencyKeyReused {
		t.Fatalf("idempotency reuse error = %v", err)
	}
}

func TestWorkflowValidateChangeUsesCommittedServerGraphAndRejectsStaleRevision(t *testing.T) {
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	app := newWorkflowEditTestApplication(t, backend, workflowEditTestAccess{}, &workflowEditTestReferences{})
	result, err := app.ValidateChange(context.Background(), edit.ValidateRequest{SchemaVersion: edit.SchemaVersion, WorkflowKey: backend.snapshot.Key, Revision: backend.snapshot.Revision.ResourceVersion})
	if err != nil || !result.Valid || result.Basis.ResourceVersion != backend.snapshot.Revision.ResourceVersion || backend.validateCalls != 1 || backend.compileCalls != 1 {
		t.Fatalf("ValidateChange(committed) = %#v, %v validate=%d compile=%d", result, err, backend.validateCalls, backend.compileCalls)
	}
	_, err = app.ValidateChange(context.Background(), edit.ValidateRequest{SchemaVersion: edit.SchemaVersion, WorkflowKey: backend.snapshot.Key, Revision: "stale"})
	if workflowCommitErrorCode(err) != edit.ErrorStaleRevision {
		t.Fatalf("ValidateChange(stale) error = %v", err)
	}
}

func TestWorkflowResolveTrustedApplyContextProjectsPersistedFactsAndRejectsScopeMismatch(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	selector := WorkflowTrustedApplySelector{
		Key:             fixture.request.WorkflowKey,
		ActorID:         fixture.request.ActorID,
		CanvasSessionID: fixture.request.CanvasSessionID,
		ChangeSetID:     fixture.request.ChangeSetID,
		ApprovalID:      fixture.request.ApplyApprovalID,
	}
	trusted, err := fixture.app.ResolveTrustedApplyContext(context.Background(), selector)
	if err != nil || trusted != fixture.trusted {
		t.Fatalf("ResolveTrustedApplyContext() = %#v, %v; want %#v", trusted, err, fixture.trusted)
	}

	tests := []struct {
		name   string
		mutate func(*WorkflowTrustedApplySelector)
	}{
		{"actor", func(value *WorkflowTrustedApplySelector) { value.ActorID = "other-actor" }},
		{"canvas session", func(value *WorkflowTrustedApplySelector) { value.CanvasSessionID = "other-canvas" }},
		{"change set", func(value *WorkflowTrustedApplySelector) { value.ChangeSetID = "other-change-set" }},
		{"approval", func(value *WorkflowTrustedApplySelector) { value.ApprovalID = "other-approval" }},
		{"workflow", func(value *WorkflowTrustedApplySelector) { value.Key.WorkflowID = "other-workflow" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mismatched := selector
			test.mutate(&mismatched)
			if _, err := fixture.app.ResolveTrustedApplyContext(context.Background(), mismatched); err == nil {
				t.Fatal("mismatched authenticated selector was accepted")
			}
		})
	}

	snapshot, err := fixture.commits.Snapshot(context.Background(), fixture.request.WorkflowKey)
	if err != nil || snapshot.Generation != 1 || len(snapshot.ApprovalConsumptions) != 0 {
		t.Fatalf("trusted context resolution mutated snapshot = %#v, %v", snapshot, err)
	}
	approval, err := fixture.approvals.GetApplyApproval(context.Background(), fixture.request.WorkflowKey, fixture.request.ApplyApprovalID)
	if err != nil || approval.State != WorkflowApprovalActive {
		t.Fatalf("trusted context resolution mutated approval = %#v, %v", approval, err)
	}
}

func TestWorkflowEditApplyPreCommitGateFailureKeepsGraphAndApproval(t *testing.T) {
	references := &workflowEditTestReferences{}
	fixture := newWorkflowEditApplyFixtureWithReferences(t, references)
	references.issues = []VisualWorkflowIssue{{Severity: "error", Type: "reference", Code: "REFERENCE_LOCKED", Message: "reference changed after preview"}}

	if _, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request); workflowCommitErrorCode(err) != edit.ErrorPublishGateFailed {
		t.Fatalf("pre-commit error = %v", err)
	}
	snapshot, err := fixture.commits.Snapshot(context.Background(), fixture.request.WorkflowKey)
	if err != nil || snapshot.Generation != 1 || snapshot.ResourceVersion != fixture.request.ExpectedRevision || len(snapshot.ApprovalConsumptions) != 0 {
		t.Fatalf("failed pre-commit mutated snapshot = %#v, %v", snapshot, err)
	}
	approval, err := fixture.approvals.GetApplyApproval(context.Background(), fixture.request.WorkflowKey, fixture.request.ApplyApprovalID)
	if err != nil || approval.State != WorkflowApprovalActive {
		t.Fatalf("failed pre-commit consumed approval = %#v, %v", approval, err)
	}
}

func TestWorkflowEditApplyRejectsEveryUntrustedIdentityMismatch(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	tests := []struct {
		name   string
		mutate func(*WorkflowEditTrustedApplyContext, *WorkflowEditApplyRequest)
	}{
		{"workflow key", func(_ *WorkflowEditTrustedApplyContext, r *WorkflowEditApplyRequest) {
			r.WorkflowKey.WorkflowID = "other"
		}},
		{"actor", func(_ *WorkflowEditTrustedApplyContext, r *WorkflowEditApplyRequest) { r.ActorID = "other" }},
		{"canvas", func(_ *WorkflowEditTrustedApplyContext, r *WorkflowEditApplyRequest) { r.CanvasSessionID = "other" }},
		{"candidate", func(_ *WorkflowEditTrustedApplyContext, r *WorkflowEditApplyRequest) { r.CandidateID = "other" }},
		{"candidate revision", func(_ *WorkflowEditTrustedApplyContext, r *WorkflowEditApplyRequest) { r.CandidateRevision++ }},
		{"change set", func(_ *WorkflowEditTrustedApplyContext, r *WorkflowEditApplyRequest) { r.ChangeSetID = "other" }},
		{"preview", func(_ *WorkflowEditTrustedApplyContext, r *WorkflowEditApplyRequest) {
			r.PreviewHash = workflowApprovalHash("other")
		}},
		{"expected revision", func(_ *WorkflowEditTrustedApplyContext, r *WorkflowEditApplyRequest) { r.ExpectedRevision = "other" }},
		{"grant", func(_ *WorkflowEditTrustedApplyContext, r *WorkflowEditApplyRequest) { r.CandidateGrantID = "other" }},
		{"approval", func(_ *WorkflowEditTrustedApplyContext, r *WorkflowEditApplyRequest) { r.ApplyApprovalID = "other" }},
		{"plan", func(c *WorkflowEditTrustedApplyContext, _ *WorkflowEditApplyRequest) {
			c.PlanHash = workflowApprovalHash("other")
		}},
		{"base semantic", func(c *WorkflowEditTrustedApplyContext, _ *WorkflowEditApplyRequest) {
			c.BaseSemanticHash = workflowApprovalHash("other")
		}},
		{"reference snapshot", func(c *WorkflowEditTrustedApplyContext, _ *WorkflowEditApplyRequest) {
			c.ReferenceSnapshotHash = workflowApprovalHash("other")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trusted, request := fixture.trusted, fixture.request
			test.mutate(&trusted, &request)
			if _, err := fixture.app.Apply(context.Background(), trusted, request); err == nil {
				t.Fatal("mismatched trusted identity was accepted")
			}
		})
	}
	snapshot, err := fixture.commits.Snapshot(context.Background(), fixture.request.WorkflowKey)
	if err != nil || snapshot.Generation != 1 || len(snapshot.ApprovalConsumptions) != 0 {
		t.Fatalf("identity rejection mutated snapshot = %#v, %v", snapshot, err)
	}
}

func TestWorkflowEditApplyConcurrentExactRequestsReturnOneTransaction(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	start := make(chan struct{})
	results := make(chan struct {
		receipt edit.WorkflowMutationReceipt
		err     error
	}, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			receipt, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request)
			results <- struct {
				receipt edit.WorkflowMutationReceipt
				err     error
			}{receipt: receipt, err: err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	transactionID := ""
	for result := range results {
		if result.err != nil || result.receipt.Status != "verifying" {
			t.Fatalf("concurrent Apply() = %#v, %v", result.receipt, result.err)
		}
		if transactionID != "" && result.receipt.TransactionID != transactionID {
			t.Fatalf("concurrent Apply() transaction=%q, want %q", result.receipt.TransactionID, transactionID)
		}
		transactionID = result.receipt.TransactionID
	}
	snapshot, err := fixture.commits.Snapshot(context.Background(), fixture.request.WorkflowKey)
	if err != nil || snapshot.Generation != 2 || len(snapshot.ApprovalConsumptions) != 1 || len(snapshot.Receipts) != 2 {
		t.Fatalf("concurrent apply snapshot = %#v, %v", snapshot, err)
	}
}

func TestWorkflowPostCheckMapsBlockingOutcomesAndRejectsStaleBasis(t *testing.T) {
	tests := []struct {
		name    string
		outcome WorkflowPostCheckOutcome
		status  string
	}{
		{"configuration", WorkflowPostCheckConfigurationRequired, "configuration_required"},
		{"failed", WorkflowPostCheckFailed, "saved_with_issues"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkflowEditApplyFixture(t)
			receipt, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request)
			if err != nil {
				t.Fatal(err)
			}
			settled, err := fixture.app.RecordPostCheck(context.Background(), WorkflowPostCheckFacts{Key: receipt.Key, TransactionID: receipt.TransactionID, Basis: receipt.Validation.Basis, Outcome: test.outcome, Diagnostics: []edit.WorkflowDiagnostic{{Code: "BLOCKED", Severity: "error", Message: "server-side check blocked"}}})
			if err != nil || settled.Status != test.status || settled.Validation.Valid {
				t.Fatalf("blocking post-check = %#v, %v", settled, err)
			}
		})
	}
	fixture := newWorkflowEditApplyFixture(t)
	receipt, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	stale := receipt.Validation.Basis
	stale.CatalogVersion += "-changed"
	_, err = fixture.app.RecordPostCheck(context.Background(), WorkflowPostCheckFacts{Key: receipt.Key, TransactionID: receipt.TransactionID, Basis: stale, Outcome: WorkflowPostCheckPassed, Diagnostics: []edit.WorkflowDiagnostic{}})
	if workflowCommitErrorCode(err) != edit.ErrorStaleRevision {
		t.Fatalf("stale post-check error = %v", err)
	}
	current, _ := fixture.commits.Snapshot(context.Background(), receipt.Key)
	if current.Receipt.Status != "verifying" || len(current.LifecycleArtifacts) != 0 {
		t.Fatalf("stale post-check changed lifecycle = %#v", current)
	}
}

type workflowEditApplyFixture struct {
	app       *WorkflowEditApplication
	commits   *WorkflowCommitStore
	approvals *WorkflowApprovalFileStore
	request   WorkflowEditApplyRequest
	trusted   WorkflowEditTrustedApplyContext
}

func newWorkflowEditApplyFixture(t *testing.T) workflowEditApplyFixture {
	return newWorkflowEditApplyFixtureWithReferences(t, &workflowEditTestReferences{})
}

func newWorkflowEditApplyFixtureWithReferences(t *testing.T, references *workflowEditTestReferences) workflowEditApplyFixture {
	t.Helper()
	canonicalizer := NewVisualWorkflowService(VisualWorkflowServiceConfig{})
	initialCompiled, err := canonicalizer.Compile(context.Background(), workflowEditTestGraph())
	if err != nil {
		t.Fatal(err)
	}
	initialGraph, err := parseCommittedVisualGraph([]byte(initialCompiled.YAML), "runner-rv-1")
	if err != nil {
		t.Fatal(err)
	}
	backend := newWorkflowEditTestBackend(t, initialGraph)
	commits, err := NewWorkflowCommitStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := backend.CompileWorkflow(context.Background(), backend.snapshot.Key, backend.snapshot.Graph)
	if err != nil {
		t.Fatal(err)
	}
	seed := workflowCommitInputFixture(backend.snapshot.Key, "", backend.snapshot.Revision.ResourceVersion, "transaction-seed", "idem-seed", "digest-seed")
	seed.GraphPayload = []byte(compiled.YAML)
	seed.Receipt.RevisionAfter.SemanticHash, seed.Receipt.RevisionAfter.LayoutHash = backend.snapshot.Revision.SemanticHash, backend.snapshot.Revision.LayoutHash
	if _, err := commits.Commit(context.Background(), seed); err != nil {
		t.Fatal(err)
	}
	candidates := NewWorkflowCandidateFileStore(t.TempDir())
	approvals := NewWorkflowApprovalFileStore(t.TempDir(), commits)
	app, err := NewWorkflowEditMutationApplication(backend, workflowEditTestAccess{}, references, candidates, approvals, commits)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := app.Preview(context.Background(), newWorkflowPreviewRequest(backend.snapshot, "candidate-apply", updatePosition("check", workflowEditApplyPosition().X, workflowEditApplyPosition().Y)))
	if err != nil {
		t.Fatal(err)
	}
	now := preview.ChangeSet.CreatedAt.Add(time.Minute)
	app.now = func() time.Time { return now }
	planHash := workflowApprovalHash("plan")
	grant := WorkflowEditGrantRecord{Grant: edit.WorkflowEditGrant{ID: "grant-apply", Key: backend.snapshot.Key, ActorID: "actor-1", PlanHash: planHash, BaseSemanticHash: preview.ChangeSet.Base.SemanticHash, AllowedOps: []edit.OperationKind{edit.OperationUpdateNode}, MaxOperations: 2, MaxAddedNodes: 0, ExpiresAt: now.Add(time.Hour)}, CanvasSessionID: "canvas-1", RiskScope: WorkflowGrantRiskScope{MaxRisk: WorkflowRiskMedium}, State: WorkflowApprovalActive, IssuedAt: now}
	if err := approvals.IssueGrant(context.Background(), grant); err != nil {
		t.Fatal(err)
	}
	approval := WorkflowApplyApprovalRecord{Approval: edit.WorkflowApplyApproval{ID: "approval-apply", ActorID: "actor-1", Key: backend.snapshot.Key, CandidateID: preview.Candidate.ID, ChangeSetID: preview.ChangeSet.ID, PreviewHash: preview.PreviewHash, ExpectedRevision: preview.ChangeSet.Base.ResourceVersion, ExpiresAt: now.Add(30 * time.Minute)}, GrantID: grant.Grant.ID, CanvasSessionID: grant.CanvasSessionID, PlanHash: planHash, State: WorkflowApprovalActive, IssuedAt: now}
	if err := approvals.IssueApplyApproval(context.Background(), approval); err != nil {
		t.Fatal(err)
	}
	request := WorkflowEditApplyRequest{ApplyPatchRequest: edit.ApplyPatchRequest{SchemaVersion: edit.SchemaVersion, WorkflowKey: backend.snapshot.Key, ChangeSetID: preview.ChangeSet.ID, PreviewHash: preview.PreviewHash, ExpectedRevision: preview.ChangeSet.Base.ResourceVersion, IdempotencyKey: "apply-idempotency", CandidateGrantID: grant.Grant.ID, ApplyApprovalID: approval.Approval.ID, Reason: "apply reviewed candidate"}, CandidateID: preview.Candidate.ID, CandidateRevision: preview.CandidateRevision, ActorID: "actor-1", CanvasSessionID: "canvas-1"}
	trusted := WorkflowEditTrustedApplyContext{Key: backend.snapshot.Key, ActorID: request.ActorID, CanvasSessionID: request.CanvasSessionID, CandidateID: request.CandidateID, CandidateRevision: request.CandidateRevision, ChangeSetID: request.ChangeSetID, PreviewHash: request.PreviewHash, ExpectedRevision: request.ExpectedRevision, GrantID: request.CandidateGrantID, ApprovalID: request.ApplyApprovalID, PlanHash: planHash, BaseSemanticHash: preview.ChangeSet.Base.SemanticHash, ReferenceSnapshotHash: preview.ValidationBasis.ReferenceSnapshotHash}
	return workflowEditApplyFixture{app: app, commits: commits, approvals: approvals, request: request, trusted: trusted}
}

func workflowEditApplyPosition() visual.Position { return visual.Position{X: 180, Y: 90} }

func assertWorkflowEditApplyError(t *testing.T, err error, code edit.ErrorCode) {
	t.Helper()
	var domain *edit.DomainError
	if !errors.As(err, &domain) || domain.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}
