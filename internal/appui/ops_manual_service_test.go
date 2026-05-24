package appui

import (
	"testing"

	"aiops-v2/internal/opsmanual"
)

func TestOpsManualServiceRetrieveReturnsNeedMoreInfo(t *testing.T) {
	repo := opsmanual.NewMemoryStore()
	if err := repo.SaveManual(appuiRedisManual()); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	service := NewOpsManualService(opsmanual.NewService(repo))

	result, err := service.RetrieveManuals(OpsManualRetrieveRequest{Text: "排查 Redis"})
	if err != nil {
		t.Fatalf("RetrieveManuals() error = %v", err)
	}
	if len(result.Matches) != 1 || result.Matches[0].State != opsmanual.DecisionNeedMoreInfo {
		t.Fatalf("matches = %#v, want one need_more_info", result.Matches)
	}
}

func TestRetrieveManualsUsesSearchEngineCompatibility(t *testing.T) {
	repo := opsmanual.NewMemoryStore()
	if err := repo.SaveManual(appuiRedisManual()); err != nil {
		t.Fatal(err)
	}
	service := NewOpsManualService(opsmanual.NewService(repo))

	result, err := service.RetrieveManuals(OpsManualRetrieveRequest{Text: "排查 Redis"})
	if err != nil {
		t.Fatal(err)
	}
	if result.OperationFrame.Target.Type != "redis" {
		t.Fatalf("target = %q, want redis", result.OperationFrame.Target.Type)
	}
	if len(result.Matches) == 0 || result.Matches[0].State != opsmanual.DecisionNeedInfo {
		t.Fatalf("matches = %#v, want need_info compatibility result", result.Matches)
	}
}

func TestOpsManualServiceRunPreflight(t *testing.T) {
	repo := opsmanual.NewMemoryStore()
	manual := appuiRedisManual()
	manual.RunnableConditions.RequiredParams = []string{"target_instance"}
	manual.PreflightProbe = opsmanual.PreflightProbe{ID: "redis-readonly-probe", ReadOnly: true, RequiredOutputs: []string{"ssh_access", "metrics_available"}}
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := NewOpsManualService(opsmanual.NewService(repo))

	result, err := service.RunPreflight(opsmanual.PreflightRequest{
		ManualID: manual.ID,
		OperationFrame: opsmanual.BuildOperationFrame(
			"通过 ssh 排查 redis-local-01 Redis used_memory_rss p95 metrics symptom",
			nil,
		),
		Parameters: map[string]any{"target_instance": "redis-local-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != opsmanual.PreflightStatusPassed || !result.Ready || result.ArtifactType != "ops_manual_preflight_result" {
		t.Fatalf("result = %#v, want passed ready preflight artifact", result)
	}
}

func TestOpsManualServiceConfirmCandidateEntersVerifiedList(t *testing.T) {
	repo := opsmanual.NewMemoryStore()
	service := NewOpsManualService(opsmanual.NewService(repo))

	candidate, err := service.PrepareManualCandidate(OpsManualPrepareCandidateRequest{
		SourceType: "workflow",
		SourceRefs: []string{"workflow-redis-memory"},
		Manual:     appuiRedisManual(),
	})
	if err != nil {
		t.Fatalf("PrepareManualCandidate() error = %v", err)
	}
	manual, err := service.ConfirmManualCandidate(candidate.ID, OpsManualReviewRequest{Reviewer: "sre"})
	if err != nil {
		t.Fatalf("ConfirmManualCandidate() error = %v", err)
	}
	if manual.Status != opsmanual.ManualStatusVerified {
		t.Fatalf("status = %q, want verified", manual.Status)
	}
	list, err := service.ListManuals(OpsManualListRequest{Status: opsmanual.ManualStatusVerified})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("ListManuals() = %#v, %v", list, err)
	}
}

func TestOpsManualServiceDoesNotRetrieveUnconfirmedCandidate(t *testing.T) {
	repo := opsmanual.NewMemoryStore()
	service := NewOpsManualService(opsmanual.NewService(repo))
	if _, err := service.PrepareManualCandidate(OpsManualPrepareCandidateRequest{
		SourceType: "workflow",
		SourceRefs: []string{"workflow-redis-memory"},
		Manual:     appuiRedisManual(),
	}); err != nil {
		t.Fatalf("PrepareManualCandidate() error = %v", err)
	}
	result, err := service.RetrieveManuals(OpsManualRetrieveRequest{Text: "排查 Redis"})
	if err != nil {
		t.Fatalf("RetrieveManuals() error = %v", err)
	}
	if len(result.Matches) != 0 {
		t.Fatalf("matches = %#v, want no matches before confirmation", result.Matches)
	}
}

func appuiRedisManual() opsmanual.OpsManual {
	return opsmanual.OpsManual{
		ID:          "manual-redis-memory",
		Title:       "Redis memory pressure",
		Status:      opsmanual.ManualStatusVerified,
		WorkflowRef: opsmanual.WorkflowRef{WorkflowID: "workflow-redis-memory", WorkflowDigest: "sha256:test"},
		Operation:   opsmanual.OperationProfile{TargetType: "redis", Action: "rca_or_repair"},
		Applicability: opsmanual.ApplicabilityProfile{
			Middleware:       "redis",
			ExecutionSurface: []string{"ssh"},
		},
		RequiredContext: opsmanual.RequiredContext{
			RequiredInputs:   []string{"target_instance"},
			RequiredEvidence: []string{"used_memory_rss", "p95"},
		},
		Preconditions:    []string{"can connect"},
		Validation:       []string{"memory recovered"},
		CannotUseWhen:    []string{"目标实例未知"},
		ParameterRules:   map[string]opsmanual.ParameterRule{"target_instance": {Required: true, Source: "user"}},
		DocumentMarkdown: "Redis manual",
	}
}
