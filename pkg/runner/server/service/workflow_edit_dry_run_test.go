package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"runner/workflow/edit"
	"runner/workflow/visual"
)

type workflowDryRunTestExecutor struct {
	mu        sync.Mutex
	result    WorkflowEditDryRunExecutionResult
	calls     int
	onExecute func()
}

func (e *workflowDryRunTestExecutor) DryRunWorkflow(_ context.Context, _ edit.WorkflowKey, _ visual.Graph, request WorkflowEditDryRunExecutionRequest) (WorkflowEditDryRunExecutionResult, error) {
	e.mu.Lock()
	e.calls++
	onExecute := e.onExecute
	result := e.result
	e.mu.Unlock()
	if request.Mode != WorkflowDryRunModeDraftNonProduction {
		return WorkflowEditDryRunExecutionResult{}, ErrInvalid
	}
	if onExecute != nil {
		onExecute()
	}
	return result, nil
}

func (e *workflowDryRunTestExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func TestWorkflowDryRunFailsClosedWithoutServerExecutor(t *testing.T) {
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	app := newWorkflowEditTestApplication(t, backend, workflowEditTestAccess{}, &workflowEditTestReferences{})
	basis, err := app.workflowPreviewValidationBasis(context.Background(), backend.snapshot, backend.snapshot.Graph)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.DryRun(context.Background(), WorkflowEditDryRunRequest{Key: backend.snapshot.Key, ExpectedBasis: basis}); !errors.Is(err, ErrWorkflowEditConfiguration) {
		t.Fatalf("unwired dry-run error = %v", err)
	}
}

func TestWorkflowDryRunBindsFullValidationBasisAndRejectsEveryStaleDimension(t *testing.T) {
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	executor := &workflowDryRunTestExecutor{result: WorkflowEditDryRunExecutionResult{Outcome: WorkflowPostCheckPassed}}
	app := newWorkflowEditTestApplication(t, backend, workflowEditTestAccess{}, &workflowEditTestReferences{})
	app.SetWorkflowDryRunExecutor(executor)
	basis, err := app.workflowPreviewValidationBasis(context.Background(), backend.snapshot, backend.snapshot.Graph)
	if err != nil {
		t.Fatal(err)
	}

	result, err := app.DryRun(context.Background(), WorkflowEditDryRunRequest{Key: backend.snapshot.Key, ExpectedBasis: basis, Environment: "staging"})
	if err != nil || result.Basis != basis || result.Mode != WorkflowDryRunModeDraftNonProduction || result.Status != WorkflowDryRunStatusPassed {
		t.Fatalf("DryRun() = %#v, %v", result, err)
	}
	if executor.callCount() != 1 {
		t.Fatalf("executor calls = %d; want 1", executor.callCount())
	}

	tests := []struct {
		name   string
		mutate func(*edit.ValidationBasis)
	}{
		{"resource version", func(value *edit.ValidationBasis) { value.ResourceVersion += "-stale" }},
		{"semantic hash", func(value *edit.ValidationBasis) { value.SemanticHash += "-stale" }},
		{"catalog version", func(value *edit.ValidationBasis) { value.CatalogVersion += "-stale" }},
		{"action schema", func(value *edit.ValidationBasis) { value.ActionSchemaDigest += "-stale" }},
		{"references", func(value *edit.ValidationBasis) { value.ReferenceSnapshotHash += "-stale" }},
		{"environment", func(value *edit.ValidationBasis) { value.EnvironmentBindingVersion += "-stale" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stale := basis
			test.mutate(&stale)
			_, err := app.DryRun(context.Background(), WorkflowEditDryRunRequest{Key: backend.snapshot.Key, ExpectedBasis: stale, Environment: "staging"})
			if workflowCommitErrorCode(err) != edit.ErrorValidationBasisStale {
				t.Fatalf("stale basis error = %v", err)
			}
		})
	}
	_, err = app.DryRun(context.Background(), WorkflowEditDryRunRequest{Key: backend.snapshot.Key, ExpectedBasis: basis, Environment: "production"})
	if workflowCommitErrorCode(err) != edit.ErrorValidationBasisStale {
		t.Fatalf("unbound environment error = %v", err)
	}
	if executor.callCount() != 1 {
		t.Fatalf("stale requests executed dry-run: calls=%d", executor.callCount())
	}
}

func TestWorkflowDryRunRejectsBasisThatChangesDuringExecution(t *testing.T) {
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	app := newWorkflowEditTestApplication(t, backend, workflowEditTestAccess{}, &workflowEditTestReferences{})
	basis, err := app.workflowPreviewValidationBasis(context.Background(), backend.snapshot, backend.snapshot.Graph)
	if err != nil {
		t.Fatal(err)
	}
	executor := &workflowDryRunTestExecutor{result: WorkflowEditDryRunExecutionResult{Outcome: WorkflowPostCheckPassed}}
	executor.onExecute = func() {
		backend.mu.Lock()
		defer backend.mu.Unlock()
		graph := backend.snapshot.Graph
		graph.Workflow.Description = "changed while the dry-run was executing"
		hashes, hashErr := VisualWorkflowGraphHashes(graph)
		if hashErr != nil {
			t.Errorf("hash changed graph: %v", hashErr)
			return
		}
		backend.snapshot.Graph = graph
		backend.snapshot.Revision = edit.Revision{ResourceVersion: "runner-rv-2", SemanticHash: hashes.SemanticHash, LayoutHash: hashes.LayoutHash}
	}
	app.SetWorkflowDryRunExecutor(executor)

	_, err = app.DryRun(context.Background(), WorkflowEditDryRunRequest{Key: backend.snapshot.Key, ExpectedBasis: basis, Environment: "staging"})
	if workflowCommitErrorCode(err) != edit.ErrorValidationBasisStale {
		t.Fatalf("concurrent basis change error = %v", err)
	}
}

func TestWorkflowDryRunConfigurationRequiredSettlesReceiptWithoutSecretValue(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	receipt, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	executor := &workflowDryRunTestExecutor{result: WorkflowEditDryRunExecutionResult{
		Outcome:       WorkflowPostCheckConfigurationRequired,
		Diagnostics:   []edit.WorkflowDiagnostic{{Code: "CONNECTOR_AUTH_FAILED", Severity: "error", Message: "api token value is do-not-leak"}},
		Configuration: WorkflowConfigurationRequirements{SecretRefs: []string{"secret://workflow/cloud-api"}, Environments: []string{"staging"}},
	}}
	fixture.app.SetWorkflowDryRunExecutor(executor)

	result, err := fixture.app.DryRunCurrent(context.Background(), receipt.Key, receipt.Validation.Basis, "staging")
	if err != nil || result.Status != "configuration_required" || result.Receipt == nil || result.Receipt.Status != "configuration_required" {
		t.Fatalf("DryRun(configuration) = %#v, %v", result, err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{"secret://workflow/cloud-api", "staging"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("result omitted configuration name %q: %s", expected, text)
		}
	}
	if strings.Contains(text, "do-not-leak") {
		t.Fatalf("result leaked secret value: %s", text)
	}
	committed, err := fixture.commits.Snapshot(context.Background(), receipt.Key)
	if err != nil || committed.Receipt.Status != "configuration_required" || len(committed.LifecycleArtifacts) != 1 || strings.Contains(string(committed.LifecycleArtifacts[0].Payload), "do-not-leak") {
		t.Fatalf("persisted configuration artifact = %#v, %v", committed, err)
	}
}

func TestWorkflowDryRunFailureSettlesAppliedReceiptAsSavedWithIssues(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	receipt, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	fixture.app.SetWorkflowDryRunExecutor(&workflowDryRunTestExecutor{result: WorkflowEditDryRunExecutionResult{
		Outcome:     WorkflowPostCheckFailed,
		Diagnostics: []edit.WorkflowDiagnostic{{Code: "PATH_BLOCKED", Severity: "error", Message: "server-side draft path simulation failed"}},
	}})
	result, err := fixture.app.DryRun(context.Background(), WorkflowEditDryRunRequest{Key: receipt.Key, TransactionID: receipt.TransactionID, ExpectedBasis: receipt.Validation.Basis, Environment: "staging"})
	if err != nil || result.Status != "saved_with_issues" || result.Valid || result.Receipt == nil || result.Receipt.Status != "saved_with_issues" {
		t.Fatalf("DryRun(failed) = %#v, %v", result, err)
	}
	snapshot, err := fixture.commits.Snapshot(context.Background(), receipt.Key)
	if err != nil || snapshot.Receipt.Status != "saved_with_issues" || snapshot.ResourceVersion != receipt.RevisionAfter.ResourceVersion {
		t.Fatalf("failed dry-run did not retain committed graph: %#v, %v", snapshot, err)
	}
}

func TestWorkflowSemanticGateLayoutOnlyEligibilityRequiresEveryNonResourceBasisField(t *testing.T) {
	previous := edit.ValidationBasis{ResourceVersion: "rv-1", SemanticHash: "semantic", CatalogVersion: "catalog", ActionSchemaDigest: "schema", ReferenceSnapshotHash: "references", EnvironmentBindingVersion: "environment"}
	current := previous
	current.ResourceVersion = "rv-2"
	if err := validateWorkflowLayoutOnlyGateReissue(previous, current, []string{"/nodes/check/position", "/layout/viewport"}); err != nil {
		t.Fatalf("layout-only basis rejected: %v", err)
	}
	tests := []struct {
		name   string
		paths  []string
		mutate func(*edit.ValidationBasis)
	}{
		{"semantic path", []string{"/nodes/check/step"}, func(*edit.ValidationBasis) {}},
		{"same resource version", []string{"/nodes/check/position"}, func(value *edit.ValidationBasis) { value.ResourceVersion = previous.ResourceVersion }},
		{"semantic hash", []string{"/nodes/check/position"}, func(value *edit.ValidationBasis) { value.SemanticHash += "-changed" }},
		{"catalog", []string{"/nodes/check/position"}, func(value *edit.ValidationBasis) { value.CatalogVersion += "-changed" }},
		{"action schema", []string{"/nodes/check/position"}, func(value *edit.ValidationBasis) { value.ActionSchemaDigest += "-changed" }},
		{"reference", []string{"/nodes/check/position"}, func(value *edit.ValidationBasis) { value.ReferenceSnapshotHash += "-changed" }},
		{"environment", []string{"/nodes/check/position"}, func(value *edit.ValidationBasis) { value.EnvironmentBindingVersion += "-changed" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := current
			test.mutate(&changed)
			if err := validateWorkflowLayoutOnlyGateReissue(previous, changed, test.paths); err == nil {
				t.Fatal("unsafe lifecycle gate reissue was accepted")
			}
		})
	}
}

func TestWorkflowSemanticGateReissuesNewServerArtifactForCommittedLayoutOnlyTransaction(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	first, err := fixture.app.Apply(context.Background(), fixture.trusted, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.app.RecordPostCheck(context.Background(), WorkflowPostCheckFacts{Key: first.Key, TransactionID: first.TransactionID, Basis: first.Validation.Basis, Outcome: WorkflowPostCheckPassed}); err != nil {
		t.Fatal(err)
	}
	previous, err := fixture.commits.Snapshot(context.Background(), first.Key)
	if err != nil || len(previous.LifecycleArtifacts) != 1 {
		t.Fatalf("previous lifecycle = %#v, %v", previous, err)
	}
	previousArtifact := append([]byte(nil), previous.LifecycleArtifacts[0].Payload...)
	graph, err := parseCommittedVisualGraph(previous.GraphPayload, previous.ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	before := graph
	for index := range graph.Nodes {
		if graph.Nodes[index].ID == "check" {
			graph.Nodes[index].Position.X += 40
		}
	}
	compiled, err := fixture.app.backend.CompileWorkflow(context.Background(), first.Key, graph)
	if err != nil {
		t.Fatal(err)
	}
	hashes, err := VisualWorkflowGraphHashes(graph)
	if err != nil {
		t.Fatal(err)
	}
	if hashes.SemanticHash != first.RevisionAfter.SemanticHash || hashes.LayoutHash == first.RevisionAfter.LayoutHash {
		t.Fatalf("layout mutation hashes = %#v; previous=%#v", hashes, first.RevisionAfter)
	}
	nextVersion := workflowApprovalHash("layout-only-next-resource-version")
	nextBasis := previous.ValidationBasis
	nextBasis.ResourceVersion = nextVersion
	nextRevision := edit.Revision{ResourceVersion: nextVersion, SemanticHash: hashes.SemanticHash, LayoutHash: hashes.LayoutHash}
	nextReceipt := edit.WorkflowMutationReceipt{
		SchemaVersion: edit.SchemaVersion, Kind: "receipt", TransactionID: "txn-layout-only-reissue", ChangeSetID: "change-layout-only-reissue",
		Key: first.Key, Status: "verifying", RevisionBefore: *first.RevisionAfter, RevisionAfter: &nextRevision,
		Delta:      edit.DiffGraphs(before, graph).Delta,
		Validation: edit.ValidationResult{Kind: "validation", Basis: nextBasis, Valid: true, Diagnostics: []edit.WorkflowDiagnostic{}}, AuditID: "audit-layout-only-reissue",
	}
	input := workflowCommitInputFixture(first.Key, previous.ResourceVersion, nextVersion, nextReceipt.TransactionID, "layout-only-reissue-idempotency", "layout-only-reissue-digest")
	input.GraphPayload, input.Receipt, input.ValidationBasis = []byte(compiled.YAML), nextReceipt, nextBasis
	input.History, err = visualWriteEnvelope("history", map[string]any{"transactionId": nextReceipt.TransactionID, "touchedPaths": []string{"/nodes/check/position"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.commits.Commit(context.Background(), input); err != nil {
		t.Fatal(err)
	}

	settled, err := fixture.app.ReissueLayoutOnlyLifecycleGate(context.Background(), first.Key, nextReceipt.TransactionID)
	if err != nil || settled.Status != "completed" || settled.Validation.Basis != nextBasis {
		t.Fatalf("ReissueLayoutOnlyLifecycleGate() = %#v, %v", settled, err)
	}
	current, err := fixture.commits.Snapshot(context.Background(), first.Key)
	if err != nil || len(current.LifecycleArtifacts) != 2 {
		t.Fatalf("reissued lifecycle = %#v, %v", current, err)
	}
	latest := current.LifecycleArtifacts[len(current.LifecycleArtifacts)-1]
	if string(latest.Payload) == string(previousArtifact) {
		t.Fatal("layout-only gate reused the previous serialized artifact")
	}
	var artifact workflowPostCheckArtifact
	if err := json.Unmarshal(latest.Payload, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.Basis != nextBasis || artifact.TransactionID != nextReceipt.TransactionID || artifact.ResignedFromResourceVersion != previous.ResourceVersion {
		t.Fatalf("reissued artifact = %#v", artifact)
	}
}

func TestWorkflowSemanticGateLayoutOnlyApplyAutomaticallyReissuesCurrentArtifact(t *testing.T) {
	fixture := newWorkflowEditHistoryFixture(t)
	first := fixture.applyOperations(t, "candidate-layout-first", "agent-layout", updatePosition("check", 180, 90))
	settled, err := fixture.app.RecordPostCheck(context.Background(), WorkflowPostCheckFacts{
		Key: first.Key, TransactionID: first.TransactionID, Basis: first.Validation.Basis,
		Outcome: WorkflowPostCheckPassed,
	})
	if err != nil || settled.Status != "completed" {
		t.Fatalf("first post-check = %#v, %v", settled, err)
	}

	second := fixture.applyOperations(t, "candidate-layout-second", "agent-layout", updatePosition("check", 240, 120))
	if second.Status != "completed" || second.RevisionAfter == nil || second.RevisionAfter.SemanticHash != first.RevisionAfter.SemanticHash {
		t.Fatalf("layout-only Apply did not return reissued gate = %#v", second)
	}
	current, err := fixture.commits.Snapshot(context.Background(), second.Key)
	if err != nil || len(current.LifecycleArtifacts) != 2 || current.Receipt.Status != "completed" {
		t.Fatalf("automatic reissue snapshot = %#v, %v", current, err)
	}
	var artifact workflowPostCheckArtifact
	latest := current.LifecycleArtifacts[len(current.LifecycleArtifacts)-1]
	if err := json.Unmarshal(latest.Payload, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.TransactionID != second.TransactionID || artifact.Basis.ResourceVersion != second.RevisionAfter.ResourceVersion || artifact.ResignedFromResourceVersion != first.RevisionAfter.ResourceVersion {
		t.Fatalf("automatic reissue artifact = %#v", artifact)
	}
}
