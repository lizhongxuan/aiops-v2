package opsmanual

import (
	"context"
	"strings"
	"testing"
)

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
		Manual:     confirmableRedisMemoryManual(),
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

func TestServiceGenerateManualCandidateFromWorkflowSavesCandidate(t *testing.T) {
	repo := NewMemoryStore()
	service := NewService(repo)
	result, err := service.GenerateManualCandidateFromWorkflow(context.Background(), WorkflowManualGenerationRequest{
		WorkflowID: "pg-restore",
		RawYAML:    loadWorkflowReverseFixture(t, "pg_restore.yaml"),
		ActionSpecs: []ActionSpecSummary{{
			Action: "script.shell",
			Risk:   "high",
		}},
	})
	if err != nil {
		t.Fatalf("GenerateManualCandidateFromWorkflow() error = %v", err)
	}
	if result.Candidate.ID == "" || result.Candidate.SourceType != "workflow_reverse_generated" {
		t.Fatalf("candidate = %#v, want workflow reverse generated candidate", result.Candidate)
	}
	if result.ValidationReport.Status == "" || len(result.UserSummary.Understood) == 0 {
		t.Fatalf("result = %#v, want validation report and user summary", result)
	}
	candidates, err := service.ListCandidates()
	if err != nil {
		t.Fatalf("ListCandidates() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != result.Candidate.ID {
		t.Fatalf("candidates = %#v, want saved generated candidate", candidates)
	}
}

func TestServiceGenerateManualCandidateFromWorkflowDoesNotSaveWhenRepoUnsupported(t *testing.T) {
	service := NewService(manualOnlyRepository{})
	_, err := service.GenerateManualCandidateFromWorkflow(context.Background(), WorkflowManualGenerationRequest{
		WorkflowID: "pg-restore",
		RawYAML:    loadWorkflowReverseFixture(t, "pg_restore.yaml"),
	})
	if err == nil || !strings.Contains(err.Error(), "candidate repository is not configured") {
		t.Fatalf("GenerateManualCandidateFromWorkflow() error = %v, want candidate repository error", err)
	}
}

func TestConfirmManualCandidateRequiresWorkflowDigest(t *testing.T) {
	repo := NewMemoryStore()
	service := NewService(repo)
	manual := confirmableRedisMemoryManual()
	manual.WorkflowRef.WorkflowDigest = ""
	candidate, err := service.PrepareManualCandidate(PrepareManualCandidateRequest{SourceType: "workflow", Manual: manual})
	if err != nil {
		t.Fatalf("PrepareManualCandidate() error = %v", err)
	}
	if _, err := service.ConfirmManualCandidate(candidate.ID, ConfirmManualCandidateRequest{Reviewer: "sre"}); err == nil {
		t.Fatal("ConfirmManualCandidate() error = nil, want digest validation error")
	}
}

func TestConfirmManualCandidateRequiresParameterRulesCoverRequiredInputs(t *testing.T) {
	repo := NewMemoryStore()
	service := NewService(repo)
	manual := confirmableRedisMemoryManual()
	manual.RequiredContext.RequiredInputs = []string{"target_instance", "backup_path"}
	manual.ParameterRules = map[string]ParameterRule{"target_instance": {Required: true}}
	candidate, err := service.PrepareManualCandidate(PrepareManualCandidateRequest{SourceType: "workflow", Manual: manual})
	if err != nil {
		t.Fatalf("PrepareManualCandidate() error = %v", err)
	}
	if _, err := service.ConfirmManualCandidate(candidate.ID, ConfirmManualCandidateRequest{Reviewer: "sre"}); err == nil {
		t.Fatal("ConfirmManualCandidate() error = nil, want parameter rule coverage error")
	}
}

func TestConfirmManualCandidateRejectsSensitiveDefaultValue(t *testing.T) {
	repo := NewMemoryStore()
	service := NewService(repo)
	manual := confirmableRedisMemoryManual()
	manual.ParameterRules["redis_password"] = ParameterRule{Required: true, DefaultValue: "plain-text"}
	candidate, err := service.PrepareManualCandidate(PrepareManualCandidateRequest{SourceType: "workflow", Manual: manual})
	if err != nil {
		t.Fatalf("PrepareManualCandidate() error = %v", err)
	}
	if _, err := service.ConfirmManualCandidate(candidate.ID, ConfirmManualCandidateRequest{Reviewer: "sre"}); err == nil {
		t.Fatal("ConfirmManualCandidate() error = nil, want sensitive default validation error")
	}
}

func TestConfirmManualCandidateRequiresRiskPolicyForHighRisk(t *testing.T) {
	repo := NewMemoryStore()
	service := NewService(repo)
	manual := confirmableRedisMemoryManual()
	manual.Operation.RiskLevel = "high"
	manual.RiskPolicy = RiskPolicy{}
	manual.RunnableConditions.RequiresApproval = false
	candidate, err := service.PrepareManualCandidate(PrepareManualCandidateRequest{SourceType: "workflow", Manual: manual})
	if err != nil {
		t.Fatalf("PrepareManualCandidate() error = %v", err)
	}
	if _, err := service.ConfirmManualCandidate(candidate.ID, ConfirmManualCandidateRequest{Reviewer: "sre"}); err == nil {
		t.Fatal("ConfirmManualCandidate() error = nil, want high-risk policy validation error")
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

func confirmableRedisMemoryManual() OpsManual {
	manual := redisMemoryManual()
	manual.WorkflowRef.WorkflowDigest = "sha256:test"
	manual.ParameterRules = map[string]ParameterRule{
		"target_instance": {Required: true, Source: "user"},
	}
	return manual
}

type manualOnlyRepository struct{}

func (manualOnlyRepository) ListManuals(ListManualsRequest) ([]OpsManual, error) {
	return nil, nil
}

func (manualOnlyRepository) GetManual(string) (OpsManual, error) {
	return OpsManual{}, nil
}

func (manualOnlyRepository) SaveManual(OpsManual) error {
	return nil
}

func (manualOnlyRepository) ListRunRecords(ListRunRecordsRequest) ([]RunRecord, error) {
	return nil, nil
}
