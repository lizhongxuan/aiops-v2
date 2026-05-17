package service

import (
	"context"
	"errors"
	"testing"
)

type stubWorkflowReferenceChecker struct {
	refs []WorkflowReference
	err  error
}

func (s stubWorkflowReferenceChecker) ReferencesForWorkflow(_ context.Context, workflowID string) ([]WorkflowReference, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]WorkflowReference{}, s.refs...), nil
}

func TestWorkflowDeleteBlockedWhenVerifiedManualReferencesIt(t *testing.T) {
	svc := NewWorkflowService(t.TempDir())
	svc.SetWorkflowReferenceChecker(stubWorkflowReferenceChecker{refs: []WorkflowReference{
		{ManualID: "manual-postgres-backup-ubuntu", Status: "verified", Title: "PostgreSQL Backup"},
	}})
	if err := svc.Create(context.Background(), &WorkflowRecord{
		Name:    "locked-delete",
		RawYAML: []byte(testWorkflowYAML("locked-delete", "echo initial")),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	err := svc.Delete(context.Background(), "locked-delete")
	if !errors.Is(err, ErrWorkflowInUse) {
		t.Fatalf("delete error = %v, want ErrWorkflowInUse", err)
	}
	var coded interface {
		Code() string
		WorkflowReferences() []WorkflowReference
	}
	if !errors.As(err, &coded) {
		t.Fatalf("delete error should expose code and references: %T", err)
	}
	if coded.Code() != "workflow_in_use" {
		t.Fatalf("error code = %q", coded.Code())
	}
	if refs := coded.WorkflowReferences(); len(refs) != 1 || refs[0].ManualID != "manual-postgres-backup-ubuntu" {
		t.Fatalf("references mismatch: %+v", refs)
	}

	if _, getErr := svc.Get(context.Background(), "locked-delete"); getErr != nil {
		t.Fatalf("workflow should remain after blocked delete: %v", getErr)
	}
}

func TestWorkflowModifyBlockedWhenVerifiedManualReferencesIt(t *testing.T) {
	svc := NewWorkflowService(t.TempDir())
	svc.SetWorkflowReferenceChecker(stubWorkflowReferenceChecker{refs: []WorkflowReference{
		{ManualID: "manual-redis-restart", Status: "verified"},
	}})
	if err := svc.Create(context.Background(), &WorkflowRecord{
		Name:    "locked-update",
		RawYAML: []byte(testWorkflowYAML("locked-update", "echo initial")),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	err := svc.Update(context.Background(), "locked-update", &WorkflowRecord{
		Name:    "locked-update",
		RawYAML: []byte(testWorkflowYAML("locked-update", "echo changed")),
	})
	if !errors.Is(err, ErrWorkflowVersionLocked) {
		t.Fatalf("update error = %v, want ErrWorkflowVersionLocked", err)
	}
	var coded interface{ Code() string }
	if !errors.As(err, &coded) || coded.Code() != "workflow_version_locked" {
		t.Fatalf("update error should expose workflow_version_locked, got %T %v", err, err)
	}

	record, getErr := svc.Get(context.Background(), "locked-update")
	if getErr != nil {
		t.Fatalf("get workflow: %v", getErr)
	}
	if string(record.RawYAML) != testWorkflowYAML("locked-update", "echo initial") {
		t.Fatalf("workflow should not be overwritten:\n%s", string(record.RawYAML))
	}
}

func TestWorkflowModifyAllowedWithWarningWhenReferenceGuardDowngraded(t *testing.T) {
	svc := NewWorkflowService(t.TempDir())
	svc.SetWorkflowReferenceChecker(stubWorkflowReferenceChecker{refs: []WorkflowReference{
		{ManualID: "manual-redis-restart", Status: "verified", Title: "Redis restart"},
	}})
	svc.SetWorkflowReferenceGuardMode(WorkflowReferenceGuardModeWarn)
	if err := svc.Create(context.Background(), &WorkflowRecord{
		Name:    "warning-update",
		RawYAML: []byte(testWorkflowYAML("warning-update", "echo initial")),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	err := svc.Update(context.Background(), "warning-update", &WorkflowRecord{
		Name:    "warning-update",
		RawYAML: []byte(testWorkflowYAML("warning-update", "echo changed")),
	})
	if err != nil {
		t.Fatalf("update should be allowed in warning mode: %v", err)
	}
	record, getErr := svc.Get(context.Background(), "warning-update")
	if getErr != nil {
		t.Fatalf("get workflow: %v", getErr)
	}
	if string(record.RawYAML) != testWorkflowYAML("warning-update", "echo changed") {
		t.Fatalf("workflow should be overwritten in warning mode:\n%s", string(record.RawYAML))
	}
	warnings, warnErr := svc.WorkflowReferenceWarnings(context.Background(), "warning-update")
	if warnErr != nil {
		t.Fatalf("WorkflowReferenceWarnings() error = %v", warnErr)
	}
	if len(warnings) != 1 || warnings[0].Code != WorkflowErrorCodeVersionLocked || len(warnings[0].References) != 1 {
		t.Fatalf("warnings mismatch: %+v", warnings)
	}
}

func TestWorkflowRollbackBlockedWhenVerifiedManualReferencesIt(t *testing.T) {
	svc := NewWorkflowService(t.TempDir())
	if err := svc.Create(context.Background(), &WorkflowRecord{
		Name:    "locked-rollback",
		RawYAML: []byte(testWorkflowYAML("locked-rollback", "echo initial")),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := svc.Update(context.Background(), "locked-rollback", &WorkflowRecord{
		Name:    "locked-rollback",
		RawYAML: []byte(testWorkflowYAML("locked-rollback", "echo changed")),
	}); err != nil {
		t.Fatalf("update workflow: %v", err)
	}
	versions, err := svc.ListVersions(context.Background(), "locked-rollback")
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	var initialVersionID string
	for _, version := range versions {
		if string(version.RawYAML) == testWorkflowYAML("locked-rollback", "echo initial") {
			initialVersionID = version.ID
			break
		}
	}
	if initialVersionID == "" {
		t.Fatalf("initial version not found: %+v", versions)
	}

	svc.SetWorkflowReferenceChecker(stubWorkflowReferenceChecker{refs: []WorkflowReference{
		{ManualID: "manual-locked-rollback", Status: "verified"},
	}})
	_, err = svc.Rollback(context.Background(), "locked-rollback", initialVersionID, WorkflowRollbackOptions{})
	if !errors.Is(err, ErrWorkflowVersionLocked) {
		t.Fatalf("rollback error = %v, want ErrWorkflowVersionLocked", err)
	}
	record, getErr := svc.Get(context.Background(), "locked-rollback")
	if getErr != nil {
		t.Fatalf("get workflow: %v", getErr)
	}
	if string(record.RawYAML) != testWorkflowYAML("locked-rollback", "echo changed") {
		t.Fatalf("workflow should not be rolled back:\n%s", string(record.RawYAML))
	}
}

func TestWorkflowImportOverwriteBlockedWhenVerifiedManualReferencesIt(t *testing.T) {
	targetSvc := NewWorkflowService(t.TempDir())
	if err := targetSvc.Create(context.Background(), &WorkflowRecord{
		Name:    "locked-import",
		RawYAML: []byte(testWorkflowYAML("locked-import", "echo current")),
	}); err != nil {
		t.Fatalf("create target workflow: %v", err)
	}

	sourceSvc := NewWorkflowService(t.TempDir())
	if err := sourceSvc.Create(context.Background(), &WorkflowRecord{
		Name:    "locked-import",
		RawYAML: []byte(testWorkflowYAML("locked-import", "echo imported")),
	}); err != nil {
		t.Fatalf("create source workflow: %v", err)
	}
	bundle, err := sourceSvc.ExportBundle(context.Background(), "locked-import")
	if err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	targetSvc.SetWorkflowReferenceChecker(stubWorkflowReferenceChecker{refs: []WorkflowReference{
		{ManualID: "manual-locked-import", Status: "verified"},
	}})
	_, err = targetSvc.ImportBundle(context.Background(), bundle, WorkflowImportOptions{Overwrite: true})
	if !errors.Is(err, ErrWorkflowVersionLocked) {
		t.Fatalf("import overwrite error = %v, want ErrWorkflowVersionLocked", err)
	}
	record, getErr := targetSvc.Get(context.Background(), "locked-import")
	if getErr != nil {
		t.Fatalf("get workflow: %v", getErr)
	}
	if string(record.RawYAML) != testWorkflowYAML("locked-import", "echo current") {
		t.Fatalf("workflow should not be overwritten by import:\n%s", string(record.RawYAML))
	}
}

func TestWorkflowDeleteBlockedWhenCandidateReferencesIt(t *testing.T) {
	svc := NewWorkflowService(t.TempDir())
	svc.SetWorkflowReferenceChecker(stubWorkflowReferenceChecker{refs: []WorkflowReference{
		{ManualID: "candidate-postgres-backup", Status: "candidate"},
	}})
	if err := svc.Create(context.Background(), &WorkflowRecord{
		Name:    "candidate-delete",
		RawYAML: []byte(testWorkflowYAML("candidate-delete", "echo initial")),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	err := svc.Delete(context.Background(), "candidate-delete")
	if !errors.Is(err, ErrWorkflowReferencedByCandidates) {
		t.Fatalf("delete error = %v, want ErrWorkflowReferencedByCandidates", err)
	}
	var coded interface{ Code() string }
	if !errors.As(err, &coded) || coded.Code() != "workflow_referenced_by_candidates" {
		t.Fatalf("delete error should expose workflow_referenced_by_candidates, got %T %v", err, err)
	}
}

func TestWorkflowDigestMismatchBlocksExecution(t *testing.T) {
	raw := []byte(testWorkflowYAML("digest-demo", "echo initial"))
	digest := DigestWorkflowContent(raw)
	if digest == "" {
		t.Fatal("digest should not be empty")
	}
	if err := VerifyWorkflowDigest(digest, raw); err != nil {
		t.Fatalf("verify matching digest: %v", err)
	}
	if err := VerifyWorkflowDigest("", raw); err != nil {
		t.Fatalf("empty digest should be migration-compatible: %v", err)
	}
	if err := VerifyWorkflowDigest(digest, []byte(testWorkflowYAML("digest-demo", "echo changed"))); !errors.Is(err, ErrWorkflowDigestMismatch) {
		t.Fatalf("verify mismatched digest error = %v, want ErrWorkflowDigestMismatch", err)
	}
}

func TestWorkflowGuardOpsManualPreflight(t *testing.T) {
	tests := []struct {
		name    string
		req     *RunRequest
		wantErr bool
	}{
		{
			name:    "no preflight blocked",
			req:     &RunRequest{ManualID: "manual-pg-backup"},
			wantErr: true,
		},
		{
			name:    "failed preflight blocked",
			req:     &RunRequest{ManualID: "manual-pg-backup", PreflightStatus: "failed"},
			wantErr: true,
		},
		{
			name:    "blocked preflight metadata blocked",
			req:     &RunRequest{Metadata: map[string]any{"manual_id": "manual-pg-backup", "preflight_status": "blocked"}},
			wantErr: true,
		},
		{
			name:    "passed preflight allowed",
			req:     &RunRequest{ManualID: "manual-pg-backup", PreflightStatus: "passed", PreflightEvidenceRef: "preflight:ok"},
			wantErr: false,
		},
		{
			name:    "passed preflight metadata allowed",
			req:     &RunRequest{Metadata: map[string]any{"ops_manual_id": "manual-pg-backup", "preflight_status": "passed"}},
			wantErr: false,
		},
		{
			name:    "non ops manual run unchanged",
			req:     &RunRequest{WorkflowName: "ordinary"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyOpsManualPreflight(tt.req)
			if tt.wantErr {
				if !errors.Is(err, ErrOpsManualPreflightRequired) {
					t.Fatalf("VerifyOpsManualPreflight() error = %v, want ErrOpsManualPreflightRequired", err)
				}
				var coded interface{ Code() string }
				if !errors.As(err, &coded) || coded.Code() != WorkflowErrorCodeOpsManualPreflight {
					t.Fatalf("preflight error should expose %s, got %T %v", WorkflowErrorCodeOpsManualPreflight, err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("VerifyOpsManualPreflight() error = %v, want nil", err)
			}
		})
	}
}
