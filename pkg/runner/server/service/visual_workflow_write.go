package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"runner/workflow/edit"
	"runner/workflow/visual"
	"runner/workflowstore"
)

// VisualWorkflowDefaultTenantID is the explicit compatibility partition used
// by the single-tenant Runner HTTP API until authentication supplies a tenant.
const VisualWorkflowDefaultTenantID = "runner-default"

type visualWorkflowWriteState struct {
	expectedVersion string
	previousGraph   []byte
	meta            workflowMeta
}

func (s *VisualWorkflowService) visualWorkflowKey(name string) edit.WorkflowKey {
	return edit.WorkflowKey{TenantID: strings.TrimSpace(s.tenantID), WorkflowID: strings.TrimSpace(name)}
}

func (s *VisualWorkflowService) requireVisualWorkflowWriter(name string) (edit.WorkflowKey, error) {
	if s == nil || s.commitStore == nil {
		return edit.WorkflowKey{}, fmt.Errorf("%w: workflow commit store is not configured", ErrUnavailable)
	}
	key := s.visualWorkflowKey(name)
	if err := key.Validate(); err != nil {
		return edit.WorkflowKey{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return key, nil
}

func (s *VisualWorkflowService) getGraph(ctx context.Context, name string) (visual.Graph, error) {
	name = strings.TrimSpace(name)
	if s == nil {
		return visual.Graph{}, fmt.Errorf("%w: visual workflow service is not configured", ErrUnavailable)
	}
	if s.commitStore != nil && s.tenantID != "" {
		snapshot, err := s.commitStore.Snapshot(ctx, s.visualWorkflowKey(name))
		if err == nil {
			return parseCommittedVisualGraph(snapshot.GraphPayload, snapshot.ResourceVersion)
		}
		if !errors.Is(err, ErrNotFound) {
			return visual.Graph{}, err
		}
	}
	if s.workflowSvc == nil {
		return visual.Graph{}, fmt.Errorf("%w: workflow reader is not configured", ErrUnavailable)
	}
	record, err := s.workflowSvc.Get(ctx, name)
	if err != nil {
		return visual.Graph{}, err
	}
	return parseCommittedVisualGraph(record.RawYAML, graphResourceVersion(record.RawYAML))
}

// EnsureWorkflowEditBaseline migrates a legacy file-backed workflow into the
// transaction store before the Workflow Edit application attempts a CAS
// mutation. The returned boolean is true only when an edit based on expected
// can be safely rebound to the returned revision: either the committed graph
// still matches that legacy basis or a prior receipt proves an idempotent
// replay of the same command.
func (s *VisualWorkflowService) EnsureWorkflowEditBaseline(ctx context.Context, name, expected, idempotencyKey string) (edit.Revision, bool, error) {
	rawExpected, rawIdempotencyKey := expected, idempotencyKey
	name, expected, idempotencyKey = strings.TrimSpace(name), strings.TrimSpace(expected), strings.TrimSpace(idempotencyKey)
	if expected == "" || expected != rawExpected || idempotencyKey == "" || idempotencyKey != rawIdempotencyKey {
		return edit.Revision{}, false, fmt.Errorf("%w: workflow edit baseline identity is required", ErrInvalid)
	}
	key, err := s.requireVisualWorkflowWriter(name)
	if err != nil {
		return edit.Revision{}, false, err
	}
	if current, snapshotErr := s.commitStore.Snapshot(ctx, key); snapshotErr == nil {
		return s.workflowEditBaselineFromCommitted(ctx, name, expected, idempotencyKey, current)
	} else if !errors.Is(snapshotErr, ErrNotFound) {
		return edit.Revision{}, false, snapshotErr
	}
	if s.workflowSvc == nil {
		return edit.Revision{}, false, ErrNotFound
	}
	legacy, err := s.workflowSvc.getLegacyWorkflow(ctx, name)
	if err != nil {
		return edit.Revision{}, false, err
	}
	legacyRevision, err := workflowEditRevisionForYAML(legacy.RawYAML, graphResourceVersion(legacy.RawYAML))
	if err != nil {
		return edit.Revision{}, false, err
	}
	if legacyRevision.ResourceVersion != expected {
		return legacyRevision, false, nil
	}
	graph, err := parseCommittedVisualGraph(legacy.RawYAML, legacyRevision.ResourceVersion)
	if err != nil {
		return edit.Revision{}, false, err
	}
	_, err = s.SaveGraphWithOptions(ctx, name, graph, VisualWorkflowSaveOptions{
		operation: "workflow-edit-bootstrap", rawYAML: legacy.RawYAML,
	})
	if err != nil && !errors.Is(err, ErrConflict) && !isStaleRevision(err) {
		return edit.Revision{}, false, err
	}
	current, snapshotErr := s.commitStore.Snapshot(ctx, key)
	if snapshotErr != nil {
		return edit.Revision{}, false, snapshotErr
	}
	return s.workflowEditBaselineFromCommitted(ctx, name, expected, idempotencyKey, current)
}

func (s *VisualWorkflowService) workflowEditBaselineFromCommitted(ctx context.Context, name, expected, idempotencyKey string, current workflowstore.TransactionSnapshot) (edit.Revision, bool, error) {
	revision, err := workflowEditRevisionForYAML(current.GraphPayload, current.ResourceVersion)
	if err != nil {
		return revision, false, err
	}
	idempotencyHash := workflowCommitIdempotencyHash(idempotencyKey)
	var replayBefore *edit.Revision
	for _, persisted := range current.Receipts {
		if persisted.IdempotencyKeyHash != idempotencyHash {
			continue
		}
		before := persisted.Receipt.RevisionBefore
		if before.ResourceVersion == expected {
			return before, true, nil
		}
		replayBefore = &before
		break
	}
	if revision.ResourceVersion == expected {
		return revision, false, nil
	}
	if s.workflowSvc == nil {
		return revision, false, nil
	}
	legacy, legacyErr := s.workflowSvc.getLegacyWorkflow(ctx, name)
	if errors.Is(legacyErr, ErrNotFound) {
		return revision, false, nil
	}
	if legacyErr != nil {
		return edit.Revision{}, false, legacyErr
	}
	legacyRevision, legacyErr := workflowEditRevisionForYAML(legacy.RawYAML, graphResourceVersion(legacy.RawYAML))
	if legacyErr != nil {
		return edit.Revision{}, false, legacyErr
	}
	if legacyRevision.ResourceVersion == expected && replayBefore != nil &&
		legacyRevision.SemanticHash == replayBefore.SemanticHash && legacyRevision.LayoutHash == replayBefore.LayoutHash {
		return *replayBefore, true, nil
	}
	compatible := legacyRevision.ResourceVersion == expected &&
		legacyRevision.SemanticHash == revision.SemanticHash && legacyRevision.LayoutHash == revision.LayoutHash
	return revision, compatible, nil
}

func workflowEditRevisionForYAML(raw []byte, resourceVersion string) (edit.Revision, error) {
	hashes, err := workflowYAMLGraphHashes(raw)
	if err != nil {
		return edit.Revision{}, err
	}
	return edit.Revision{
		ResourceVersion: strings.TrimSpace(resourceVersion), SemanticHash: hashes.SemanticHash, LayoutHash: hashes.LayoutHash,
	}, nil
}

func parseCommittedVisualGraph(raw []byte, resourceVersion string) (visual.Graph, error) {
	graph, err := visual.ParseYAMLToGraph(raw)
	if err != nil {
		return visual.Graph{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	graph = normalizeGraph(graph)
	graph.UI = withGraphResourceVersion(graph.UI, resourceVersion)
	return graph, nil
}

func (s *VisualWorkflowService) CreateGraph(ctx context.Context, graph visual.Graph, opts VisualWorkflowCreateOptions) (*CreatedVisualWorkflow, error) {
	graph = normalizeGraph(graph)
	name := strings.TrimSpace(graph.Workflow.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: workflow name is required", ErrInvalid)
	}
	key, err := s.requireVisualWorkflowWriter(name)
	if err != nil {
		return nil, err
	}
	graph.Workflow.Name = name
	graph.UI = withoutGraphResourceVersion(graph.UI)
	compiled, err := s.Compile(ctx, graph)
	if err != nil {
		return nil, err
	}
	expected, previous, exists, err := s.visualWorkflowCreateBasis(ctx, key, name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrAlreadyExists
	}
	meta := workflowMeta{Name: name, Labels: copyStringMap(opts.Labels), Status: WorkflowStatusDraft}
	if opts.SaveNoteSet || strings.TrimSpace(opts.SaveNote) != "" {
		meta.SaveNote = strings.TrimSpace(opts.SaveNote)
	}
	operation := strings.TrimSpace(opts.operation)
	if operation == "" {
		operation = "create"
	}
	if err := s.commitVisualGraph(ctx, key, expected, expected, previous, meta, compiled, operation, opts.versions, opts.replaceHistory, opts.rawYAML); err != nil {
		if isStaleRevision(err) {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}
	createdGraph, err := s.getGraph(ctx, name)
	if err != nil {
		return nil, err
	}
	return &CreatedVisualWorkflow{
		Name: name, Status: WorkflowStatusDraft, Workflow: cloneWorkflow(compiled.Workflow),
		Graph: createdGraph, YAML: compiled.YAML, Warnings: append([]VisualWorkflowIssue{}, compiled.Warnings...),
	}, nil
}

func (s *VisualWorkflowService) SaveGraphWithOptions(ctx context.Context, name string, graph visual.Graph, opts VisualWorkflowSaveOptions) (*CompiledVisualWorkflow, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: workflow name is required", ErrInvalid)
	}
	key, err := s.requireVisualWorkflowWriter(name)
	if err != nil {
		return nil, err
	}
	graph = normalizeGraph(graph)
	if strings.TrimSpace(graph.Workflow.Name) == "" {
		graph.Workflow.Name = name
	}
	if strings.TrimSpace(graph.Workflow.Name) != name {
		return nil, fmt.Errorf("%w: workflow name mismatch", ErrInvalid)
	}
	clientExpected := graphExpectedResourceVersion(graph)
	graph.UI = withoutGraphResourceVersion(graph.UI)
	compiled, err := s.Compile(ctx, graph)
	if err != nil {
		return nil, err
	}
	state, err := s.visualWorkflowWriteState(ctx, key, name, clientExpected)
	if err != nil {
		return nil, err
	}
	state.meta = updatedVisualWorkflowMeta(state.meta, opts, compiled)
	operation := strings.TrimSpace(opts.operation)
	if operation == "" {
		operation = "update"
	}
	if err := s.commitVisualGraph(ctx, key, state.expectedVersion, clientExpected, state.previousGraph, state.meta, compiled, operation, opts.versions, opts.replaceHistory, opts.rawYAML); err != nil {
		if isStaleRevision(err) {
			return nil, fmt.Errorf("%w: workflow graph changed since it was loaded", ErrConflict)
		}
		return nil, err
	}
	return compiled, nil
}

func (s *VisualWorkflowService) visualWorkflowCreateBasis(ctx context.Context, key edit.WorkflowKey, name string) (string, []byte, bool, error) {
	if snapshot, err := s.commitStore.Snapshot(ctx, key); err == nil {
		meta := workflowMeta{}
		if err := json.Unmarshal(snapshot.Meta.Payload, &meta); err != nil {
			return "", nil, false, err
		}
		if meta.Deleted {
			return snapshot.ResourceVersion, append([]byte(nil), snapshot.GraphPayload...), false, nil
		}
		return "", nil, true, nil
	} else if !errors.Is(err, ErrNotFound) {
		return "", nil, false, err
	}
	if s.workflowSvc == nil {
		return "", nil, false, nil
	}
	if _, err := s.workflowSvc.Get(ctx, name); err == nil {
		return "", nil, true, nil
	} else if !errors.Is(err, ErrNotFound) {
		return "", nil, false, err
	}
	return "", nil, false, nil
}

func (s *VisualWorkflowService) visualWorkflowWriteState(ctx context.Context, key edit.WorkflowKey, name, clientExpected string) (visualWorkflowWriteState, error) {
	snapshot, err := s.commitStore.Snapshot(ctx, key)
	if err == nil {
		meta := workflowMeta{Name: name, Status: WorkflowStatusDraft}
		if err := json.Unmarshal(snapshot.Meta.Payload, &meta); err != nil {
			return visualWorkflowWriteState{}, fmt.Errorf("decode workflow commit metadata: %w", err)
		}
		expected := clientExpected
		if expected == "" {
			expected = snapshot.ResourceVersion
		}
		return visualWorkflowWriteState{expectedVersion: expected, previousGraph: append([]byte(nil), snapshot.GraphPayload...), meta: meta}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return visualWorkflowWriteState{}, err
	}
	if s.workflowSvc == nil {
		return visualWorkflowWriteState{}, ErrNotFound
	}
	record, err := s.workflowSvc.Get(ctx, name)
	if err != nil {
		return visualWorkflowWriteState{}, err
	}
	legacyVersion := graphResourceVersion(record.RawYAML)
	if clientExpected != "" && clientExpected != legacyVersion {
		return visualWorkflowWriteState{}, fmt.Errorf("%w: workflow graph changed since it was loaded", ErrConflict)
	}
	return visualWorkflowWriteState{previousGraph: append([]byte(nil), record.RawYAML...), meta: workflowMetaFromRecord(record)}, nil
}

func workflowMetaFromRecord(record *WorkflowRecord) workflowMeta {
	return workflowMeta{
		Name: record.Name, Labels: copyStringMap(record.Labels), SaveNote: record.SaveNote, Status: normalizeWorkflowStatus(record.Status),
		ValidatedGraphHash: record.ValidatedGraphHash, ValidatedLayoutHash: record.ValidatedLayoutHash, ValidatedAt: record.ValidatedAt, ValidatedBy: record.ValidatedBy,
		DryRunGraphHash: record.DryRunGraphHash, DryRunLayoutHash: record.DryRunLayoutHash, DryRunAt: record.DryRunAt, DryRunBy: record.DryRunBy,
		PublishedGraphHash: record.PublishedGraphHash, PublishedAt: record.PublishedAt, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func updatedVisualWorkflowMeta(meta workflowMeta, opts VisualWorkflowSaveOptions, compiled *CompiledVisualWorkflow) workflowMeta {
	previousStatus := normalizeWorkflowStatus(meta.Status)
	meta.Name, meta.Status, meta.Deleted = compiled.Workflow.Name, WorkflowStatusDraft, false
	if opts.SaveNoteSet || strings.TrimSpace(opts.SaveNote) != "" {
		meta.SaveNote = strings.TrimSpace(opts.SaveNote)
	}
	if opts.labelsSet || opts.labels != nil {
		meta.Labels = copyStringMap(opts.labels)
	}
	hashes, err := workflowYAMLGraphHashes([]byte(compiled.YAML))
	if err != nil || meta.ValidatedGraphHash == "" || meta.ValidatedGraphHash != hashes.SemanticHash {
		meta.ValidatedGraphHash, meta.ValidatedLayoutHash, meta.ValidatedAt, meta.ValidatedBy = "", "", time.Time{}, ""
		meta.DryRunGraphHash, meta.DryRunLayoutHash, meta.DryRunBy = "", "", ""
		meta.DryRunAt = time.Time{}
		return meta
	}
	meta.Status, meta.ValidatedLayoutHash = previousStatus, hashes.LayoutHash
	if meta.DryRunGraphHash == hashes.SemanticHash {
		meta.DryRunLayoutHash = hashes.LayoutHash
	} else {
		meta.DryRunGraphHash, meta.DryRunLayoutHash, meta.DryRunBy = "", "", ""
		meta.DryRunAt = time.Time{}
	}
	if meta.Status != WorkflowStatusPublished {
		meta.PublishedGraphHash = ""
	}
	return meta
}

func (s *VisualWorkflowService) commitVisualGraph(ctx context.Context, key edit.WorkflowKey, expected, clientExpected string, previous []byte, meta workflowMeta, compiled *CompiledVisualWorkflow, operation string, importedVersions []WorkflowBundleVersion, replaceHistory bool, rawYAML []byte) error {
	committedYAML := []byte(compiled.YAML)
	if len(rawYAML) > 0 {
		committedYAML = append([]byte(nil), rawYAML...)
	}
	afterHashes, err := workflowYAMLGraphHashes(committedYAML)
	if err != nil {
		return err
	}
	beforeHashes := WorkflowGraphHashes{}
	if len(previous) > 0 {
		beforeHashes, err = workflowYAMLGraphHashes(previous)
		if err != nil {
			return err
		}
	}
	catalogDigest, err := visualWriteDigest(s.ListActions(ctx, ActionCatalogFilter{}))
	if err != nil {
		return err
	}
	referenceDigest, err := visualWriteDigest(map[string]any{"steps": compiled.Workflow.Steps, "handlers": compiled.Workflow.Handlers, "tests": compiled.Workflow.Tests})
	if err != nil {
		return err
	}
	environmentDigest, err := visualWriteDigest(map[string]any{"inventory": compiled.Workflow.Inventory, "envPackages": compiled.Workflow.EnvPackages, "validationEnv": compiled.Workflow.ValidationEnv})
	if err != nil {
		return err
	}
	basisVersion := expected
	if basisVersion == "" {
		basisVersion = "initial"
	}
	basis := edit.ValidationBasis{ResourceVersion: basisVersion, SemanticHash: afterHashes.SemanticHash, CatalogVersion: catalogDigest, ActionSchemaDigest: catalogDigest, ReferenceSnapshotHash: referenceDigest, EnvironmentBindingVersion: environmentDigest}
	requestBaseVersion := clientExpected
	if requestBaseVersion == "" {
		requestBaseVersion = expected
	}
	requestDigest, err := visualWriteDigest(map[string]any{"operation": operation, "key": key, "base": requestBaseVersion, "graph": string(committedYAML), "meta": meta})
	if err != nil {
		return err
	}
	token := strings.TrimPrefix(requestDigest, "sha256:")
	nextVersion := visualWriteHash(expected + "\x00" + requestDigest)
	// The validation basis describes the graph that is about to become current,
	// not the revision that was used for CAS. Binding it to the old/"initial"
	// version makes the canonical workspace projector reject a freshly created
	// Visual Workflow and leaves real browser clients without an authoritative
	// committed snapshot.
	basis.ResourceVersion = nextVersion
	diagnostics := make([]edit.WorkflowDiagnostic, 0, len(compiled.Warnings))
	for _, warning := range compiled.Warnings {
		diagnostics = append(diagnostics, edit.WorkflowDiagnostic{Code: warning.Code, Severity: warning.Severity, Message: warning.Message, NodeID: warning.NodeID, EdgeID: warning.EdgeID, Field: warning.Field, Suggestion: warning.Suggestion})
	}
	receipt := edit.WorkflowMutationReceipt{
		SchemaVersion: edit.SchemaVersion, Kind: "receipt", TransactionID: "txn-" + token[:32], ChangeSetID: "visual-" + token[:24], Key: key, Status: "committed",
		RevisionBefore: edit.Revision{ResourceVersion: expected, SemanticHash: beforeHashes.SemanticHash, LayoutHash: beforeHashes.LayoutHash},
		RevisionAfter:  &edit.Revision{ResourceVersion: nextVersion, SemanticHash: afterHashes.SemanticHash, LayoutHash: afterHashes.LayoutHash},
		Validation:     edit.ValidationResult{Kind: "validation", Basis: basis, Valid: true, Diagnostics: diagnostics},
		CheckpointID:   "checkpoint-" + token[:24], InverseTransactionID: "inverse-" + token[:24], AuditID: "audit-" + token[:24],
	}
	history, err := s.nextWorkflowHistory(ctx, key, string(committedYAML), meta, operation, receipt.TransactionID, importedVersions, replaceHistory)
	if err != nil {
		return err
	}
	importResult := map[string]any{"applied": false, "reason": "not_import"}
	if operation == "import" {
		importResult = map[string]any{"applied": true, "versions": importedVersions}
	}
	rollbackResult := map[string]any{"applied": false, "reason": "not_rollback"}
	if strings.HasPrefix(operation, "rollback:") {
		rollbackResult = map[string]any{"applied": true, "versionId": strings.TrimPrefix(operation, "rollback:")}
	}
	artifacts := map[string]any{
		"meta":             meta,
		"history":          history,
		"import_result":    importResult,
		"rollback_result":  rollbackResult,
		"checkpoint":       map[string]any{"id": receipt.CheckpointID, "resourceVersion": expected, "graph": string(previous)},
		"inverse":          map[string]any{"transactionId": receipt.InverseTransactionID, "restoreGraph": string(previous)},
		"approval_consume": map[string]any{"required": false, "source": "visual_http"},
		"audit":            map[string]any{"auditId": receipt.AuditID, "operation": operation, "tenantId": key.TenantID, "workflowId": key.WorkflowID, "transactionId": receipt.TransactionID, "before": expected, "after": nextVersion},
		"event":            map[string]any{"eventId": "event-" + token[:24], "type": "workflow.graph_committed", "transactionId": receipt.TransactionID, "resourceVersion": nextVersion},
		"outbox":           map[string]any{"eventId": "event-" + token[:24], "transactionId": receipt.TransactionID, "status": "pending"},
	}
	envelopes := make(map[string]workflowstore.TypedEnvelope, len(artifacts))
	for kind, payload := range artifacts {
		envelopes[kind], err = visualWriteEnvelope(kind, payload)
		if err != nil {
			return err
		}
	}
	_, err = s.commitStore.Commit(ctx, WorkflowCommitInput{
		Key: key, ExpectedResourceVersion: expected, IdempotencyKey: "visual-" + token, RequestDigest: requestDigest, GraphPayload: committedYAML,
		Meta: envelopes["meta"], History: envelopes["history"], ImportResult: envelopes["import_result"], RollbackResult: envelopes["rollback_result"],
		Receipt: receipt, ValidationBasis: basis, Checkpoint: envelopes["checkpoint"], Inverse: envelopes["inverse"], ApprovalConsume: envelopes["approval_consume"], Audit: envelopes["audit"],
		Events: []workflowstore.TypedEnvelope{envelopes["event"]}, Outbox: []workflowstore.TypedEnvelope{envelopes["outbox"]},
	})
	return err
}

func visualWriteEnvelope(kind string, payload any) (workflowstore.TypedEnvelope, error) {
	raw, err := json.Marshal(payload)
	return workflowstore.TypedEnvelope{Kind: kind, SchemaVersion: edit.SchemaVersion, Payload: raw}, err
}

func visualWriteDigest(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return visualWriteHash(string(raw)), nil
}

func visualWriteHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func isStaleRevision(err error) bool {
	var domainErr *edit.DomainError
	return errors.As(err, &domainErr) && domainErr.Code == edit.ErrorStaleRevision
}
