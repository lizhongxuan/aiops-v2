package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"runner/workflow/edit"
	"runner/workflowstore"
)

// WorkflowPublishedRunTarget identifies one immutable server-owned publication.
// The browser may select this identity, but it never supplies the executable
// Graph. Resolution happens against the durable transaction history.
type WorkflowPublishedRunTarget struct {
	WorkflowKey     edit.WorkflowKey `json:"workflowKey"`
	ResourceVersion string           `json:"resourceVersion"`
	SemanticHash    string           `json:"semanticHash"`
}

func (target WorkflowPublishedRunTarget) validate() error {
	if target.WorkflowKey.Validate() != nil ||
		!canonicalWorkflowRunHash(target.ResourceVersion) ||
		!canonicalWorkflowRunHash(target.SemanticHash) {
		return fmt.Errorf("%w: invalid immutable published workflow target", ErrInvalid)
	}
	return nil
}

func canonicalWorkflowRunHash(value string) bool {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	for _, char := range value[len("sha256:"):] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

// SubmitPublishedRun resolves the exact publication from the durable history
// before submitting it. Current draft bytes are never consulted as a fallback.
func (s *VisualWorkflowService) SubmitPublishedRun(
	ctx context.Context,
	target WorkflowPublishedRunTarget,
	vars map[string]any,
	triggeredBy string,
	idempotencyKey string,
	opts VisualWorkflowRunOptions,
) (*RunResponse, error) {
	if s == nil || s.runSvc == nil || s.commitStore == nil {
		return nil, fmt.Errorf("%w: immutable published workflow execution is not configured", ErrUnavailable)
	}
	if err := target.validate(); err != nil {
		return nil, err
	}
	if target.WorkflowKey.TenantID != s.tenantID {
		return nil, fmt.Errorf("%w: published workflow target is outside the configured tenant", ErrNotFound)
	}
	if strings.TrimSpace(idempotencyKey) == "" || idempotencyKey != strings.TrimSpace(idempotencyKey) {
		return nil, fmt.Errorf("%w: published workflow idempotency key is required", ErrInvalid)
	}

	snapshot, err := s.commitStore.Snapshot(ctx, target.WorkflowKey)
	if err != nil {
		return nil, err
	}
	if snapshot.Key != target.WorkflowKey {
		return nil, fmt.Errorf("%w: published workflow target binding changed", ErrConflict)
	}
	var history workflowHistoryPayload
	if err := json.Unmarshal(snapshot.History.Payload, &history); err != nil {
		return nil, fmt.Errorf("%w: published workflow history is unavailable", ErrUnavailable)
	}

	for _, version := range history.Versions {
		if version.Reason != "publish" ||
			normalizeWorkflowStatus(version.Status) != WorkflowStatusPublished ||
			version.PublishedAt.IsZero() {
			continue
		}
		revision, ok := publishedVersionRevision(snapshot.Receipts, version.ID)
		if !ok ||
			revision.ResourceVersion != target.ResourceVersion ||
			revision.SemanticHash != target.SemanticHash {
			continue
		}
		raw := []byte(version.YAML)
		if version.Checksum == "" || version.Checksum != workflowYAMLChecksum(raw) {
			return nil, fmt.Errorf("%w: immutable published workflow checksum mismatch", ErrConflict)
		}
		hashes, err := workflowYAMLGraphHashes(raw)
		if err != nil || hashes.SemanticHash != target.SemanticHash {
			return nil, fmt.Errorf("%w: immutable published workflow semantic hash mismatch", ErrConflict)
		}
		graph, err := parseCommittedVisualGraph(raw, target.ResourceVersion)
		if err != nil {
			return nil, err
		}
		return s.SubmitGraphRunWithOptions(ctx, graph, vars, triggeredBy, idempotencyKey, opts)
	}
	return nil, fmt.Errorf("%w: immutable published workflow revision was not found", ErrConflict)
}

func publishedVersionRevision(receipts []workflowstore.PersistedReceipt, transactionID string) (edit.Revision, bool) {
	for _, persisted := range receipts {
		if persisted.Receipt.TransactionID == transactionID && persisted.Receipt.RevisionAfter != nil {
			return *persisted.Receipt.RevisionAfter, true
		}
	}
	return edit.Revision{}, false
}
