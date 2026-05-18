package opsmanual

import "testing"

func TestServiceRetrieveReturnsNeedMoreInfo(t *testing.T) {
	repo := NewMemoryStore()
	if err := repo.SaveManual(redisMemoryManual()); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	service := NewService(repo)
	matches, err := service.RetrieveManuals(BuildOperationFrame("排查 Redis", nil))
	if err != nil {
		t.Fatalf("RetrieveManuals() error = %v", err)
	}
	if len(matches) != 1 || matches[0].State != DecisionNeedMoreInfo {
		t.Fatalf("matches = %#v, want one need_more_info", matches)
	}
}

func TestServiceConfirmCandidateEntersVerifiedList(t *testing.T) {
	repo := NewMemoryStore()
	service := NewService(repo)
	candidate, err := service.PrepareManualCandidate(PrepareManualCandidateRequest{
		SourceType: "workflow",
		SourceRefs: []string{"workflow-redis-memory"},
		Manual:     redisMemoryManual(),
	})
	if err != nil {
		t.Fatalf("PrepareManualCandidate() error = %v", err)
	}
	manual, err := service.ConfirmManualCandidate(candidate.ID, ConfirmManualCandidateRequest{Reviewer: "sre", ReviewNote: "verified"})
	if err != nil {
		t.Fatalf("ConfirmManualCandidate() error = %v", err)
	}
	if manual.Status != ManualStatusVerified {
		t.Fatalf("status = %q, want verified", manual.Status)
	}
	manuals, err := service.ListManuals(ListManualsRequest{Status: ManualStatusVerified})
	if err != nil || len(manuals) != 1 {
		t.Fatalf("ListManuals() = %#v, %v", manuals, err)
	}
}

func TestServiceConfirmCandidateRejectsUnsafeVerifiedManual(t *testing.T) {
	repo := NewMemoryStore()
	service := NewService(repo)
	candidate, err := service.PrepareManualCandidate(PrepareManualCandidateRequest{
		SourceType: "workflow",
		Manual: OpsManual{
			ID:            "manual-unsafe",
			Title:         "Unsafe manual",
			Operation:     OperationProfile{TargetType: "redis", Action: "rca_or_repair"},
			Validation:    []string{"memory recovered"},
			CannotUseWhen: []string{"目标实例未知"},
		},
	})
	if err != nil {
		t.Fatalf("PrepareManualCandidate() error = %v", err)
	}
	if _, err := service.ConfirmManualCandidate(candidate.ID, ConfirmManualCandidateRequest{Reviewer: "sre"}); err == nil {
		t.Fatal("ConfirmManualCandidate() error = nil, want workflow binding validation error")
	}
}

func TestServiceConfirmCandidateRejectsManualWithoutValidationOrCannotUseWhen(t *testing.T) {
	repo := NewMemoryStore()
	service := NewService(repo)
	candidate, err := service.PrepareManualCandidate(PrepareManualCandidateRequest{
		SourceType: "workflow",
		Manual: OpsManual{
			ID:          "manual-no-gates",
			Title:       "No gates",
			WorkflowRef: WorkflowRef{WorkflowID: "workflow-no-gates"},
			Operation:   OperationProfile{TargetType: "redis", Action: "rca_or_repair"},
		},
	})
	if err != nil {
		t.Fatalf("PrepareManualCandidate() error = %v", err)
	}
	if _, err := service.ConfirmManualCandidate(candidate.ID, ConfirmManualCandidateRequest{Reviewer: "sre"}); err == nil {
		t.Fatal("ConfirmManualCandidate() error = nil, want validation gate error")
	}
}

func TestServiceDoesNotRetrieveUnconfirmedCandidate(t *testing.T) {
	repo := NewMemoryStore()
	service := NewService(repo)
	if _, err := service.PrepareManualCandidate(PrepareManualCandidateRequest{
		SourceType: "workflow",
		SourceRefs: []string{"workflow-redis-memory"},
		Manual:     redisMemoryManual(),
	}); err != nil {
		t.Fatalf("PrepareManualCandidate() error = %v", err)
	}
	matches, err := service.RetrieveManuals(BuildOperationFrame("排查 Redis", nil))
	if err != nil {
		t.Fatalf("RetrieveManuals() error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("matches = %#v, want no matches before confirmation", matches)
	}
}
