package workfloweditor

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"runner/workflow/visual"
)

const workflowKnownFailureEnv = "AIOPS_WORKFLOW_KNOWN_FAILURE"

func requireWorkflowKnownFailureRun(t *testing.T) {
	t.Helper()
	if os.Getenv(workflowKnownFailureEnv) != "1" {
		t.Skip("Phase 0 known-failure probe; set AIOPS_WORKFLOW_KNOWN_FAILURE=1")
	}
}

func TestKnownFailureTXN002ConcurrentApplyNeedsStoreCAS(t *testing.T) {
	requireWorkflowKnownFailureRun(t)
	record := WorkflowRecord{ID: "shared", Graph: workflowEditorTestGraph()}
	record.Revision = RevisionDigest(record.Graph)
	store := newBarrierWorkflowStore(record, 2)
	services := []*Service{NewService(store), NewService(store)}

	var succeeded atomic.Int32
	var wg sync.WaitGroup
	for index, service := range services {
		wg.Add(1)
		go func(index int, service *Service) {
			defer wg.Done()
			patch := workflowKnownFailurePatch(record.Revision, "concurrent-"+string(rune('a'+index)))
			if _, err := service.ApplyPatch(context.Background(), workflowKnownFailureApply(record, patch, "approval-concurrent")); err == nil {
				succeeded.Add(1)
			}
		}(index, service)
	}
	wg.Wait()
	if got := succeeded.Load(); got != 1 {
		t.Fatalf("contract probe: duplicate apply created duplicate mutation; successes=%d want=1", got)
	}
}

func TestKnownFailureSEC015ApplyApprovalMustBeSingleUse(t *testing.T) {
	requireWorkflowKnownFailureRun(t)
	store := NewMemoryWorkflowStore()
	record := store.PutWorkflow(WorkflowRecord{ID: "approval-reuse", Graph: workflowEditorTestGraph()})
	service := NewService(store)
	first := workflowKnownFailurePatch(record.Revision, "first")
	result, err := service.ApplyPatch(context.Background(), workflowKnownFailureApply(record, first, "same-approval"))
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	second := workflowKnownFailurePatch(result.RevisionAfter, "second")
	secondRecord := record
	secondRecord.Revision = result.RevisionAfter
	if _, err := service.ApplyPatch(context.Background(), workflowKnownFailureApply(secondRecord, second, "same-approval")); err == nil {
		t.Fatal("contract probe: apply approval was reusable")
	}
}

func TestKnownFailureSEC003WorkflowStoreMustPartitionTenant(t *testing.T) {
	requireWorkflowKnownFailureRun(t)
	store := NewMemoryWorkflowStore()
	tenantA := store.PutWorkflow(WorkflowRecord{ID: "same-workflow", Graph: workflowEditorTestGraph()})
	tenantBGraph := workflowEditorTestGraph()
	tenantBGraph.Workflow.Description = "tenant-b"
	store.PutWorkflow(WorkflowRecord{ID: "same-workflow", Graph: tenantBGraph})
	loaded, err := store.GetWorkflow(context.Background(), "same-workflow")
	if err != nil {
		t.Fatal(err)
	}
	if RevisionDigest(loaded.Graph) != tenantA.Revision {
		t.Fatal("contract probe: cross-tenant workflow disclosed because store key omits tenant")
	}
}

func TestKnownFailureVAL005PreviewMustRejectStructurallyInvalidGraph(t *testing.T) {
	requireWorkflowKnownFailureRun(t)
	store := NewMemoryWorkflowStore()
	record := store.PutWorkflow(WorkflowRecord{ID: "invalid-preview", Graph: workflowEditorTestGraph()})
	service := NewService(store)
	patch := WorkflowPatch{
		ID:           "delete-required-step",
		BaseRevision: record.Revision,
		Operations: []WorkflowPatchOperation{{
			Op:     PatchDeleteNode,
			NodeID: "collect",
		}},
	}
	if _, err := service.PreviewPatch(context.Background(), PreviewPatchRequest{
		WorkflowID:   record.ID,
		BaseRevision: record.Revision,
		Patch:        patch,
	}); err == nil {
		t.Fatal("contract probe: structurally invalid preview was accepted without authoritative validate/compile")
	}
}

func TestKnownFailureTXN006ReceiptMustSurviveServiceRestart(t *testing.T) {
	requireWorkflowKnownFailureRun(t)
	store := NewMemoryWorkflowStore()
	record := store.PutWorkflow(WorkflowRecord{ID: "restart-receipt", Graph: workflowEditorTestGraph()})
	firstService := NewService(store)
	patch := workflowKnownFailurePatch(record.Revision, "restart")
	result, err := firstService.ApplyPatch(context.Background(), workflowKnownFailureApply(record, patch, "restart-approval"))
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}

	restartedService := NewService(store)
	replayed := patch
	replayed.BaseRevision = result.RevisionAfter
	effect, err := restartedService.DetectPatchEffect(context.Background(), DetectPatchEffectRequest{
		WorkflowID: record.ID,
		Patch:      replayed,
	})
	if err != nil {
		t.Fatalf("detect replay after restart: %v", err)
	}
	if effect.Status != EffectDuplicate {
		t.Fatalf("contract probe: receipt was lost after service restart; effect=%s want=%s", effect.Status, EffectDuplicate)
	}
}

func TestKnownFailureSEC009HighRiskDeleteRequiresGrantAndBoundApproval(t *testing.T) {
	requireWorkflowKnownFailureRun(t)
	store := NewMemoryWorkflowStore()
	record := store.PutWorkflow(WorkflowRecord{ID: "high-risk-delete", Graph: workflowEditorTestGraph()})
	service := NewService(store)
	patch := WorkflowPatch{
		ID:           "unguarded-delete",
		BaseRevision: record.Revision,
		Operations: []WorkflowPatchOperation{{
			Op:     PatchDeleteNode,
			NodeID: "collect",
		}},
	}
	if _, err := service.ApplyPatch(context.Background(), workflowKnownFailureApply(record, patch, "unbound-confirmation")); err == nil {
		t.Fatal("contract probe: high-risk deletion committed without WorkflowEditGrant and bound WorkflowApplyApproval")
	}
}

func workflowKnownFailurePatch(revision, suffix string) WorkflowPatch {
	return WorkflowPatch{
		ID:           "known-failure-" + suffix,
		BaseRevision: revision,
		Operations: []WorkflowPatchOperation{{
			Op:     PatchUpdateNode,
			NodeID: "collect",
			Fields: map[string]any{"label": "Collect " + suffix},
		}},
	}
}

func workflowKnownFailureApply(record WorkflowRecord, patch WorkflowPatch, approval string) ApplyPatchRequest {
	return ApplyPatchRequest{
		WorkflowID:         record.ID,
		BaseRevision:       patch.BaseRevision,
		PatchID:            patch.ID,
		Patch:              patch,
		UserConfirmationID: approval,
		DrawerSessionID:    "known-failure-drawer",
		Reason:             "known failure contract probe",
	}
}

type barrierWorkflowStore struct {
	mu       sync.Mutex
	record   WorkflowRecord
	wantGets int32
	gets     atomic.Int32
	release  chan struct{}
	once     sync.Once
}

func newBarrierWorkflowStore(record WorkflowRecord, wantGets int32) *barrierWorkflowStore {
	return &barrierWorkflowStore{record: record, wantGets: wantGets, release: make(chan struct{})}
}

func (s *barrierWorkflowStore) GetWorkflow(ctx context.Context, _ string) (WorkflowRecord, error) {
	if s.gets.Add(1) == s.wantGets {
		s.once.Do(func() { close(s.release) })
	}
	select {
	case <-ctx.Done():
		return WorkflowRecord{}, ctx.Err()
	case <-s.release:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.record
	result.Graph = CloneGraph(result.Graph)
	return result, nil
}

func (s *barrierWorkflowStore) SaveWorkflow(_ context.Context, record WorkflowRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.record = WorkflowRecord{ID: record.ID, Revision: record.Revision, Graph: visual.Graph(record.Graph), UpdatedAt: record.UpdatedAt}
	return nil
}
