package experiencepack

import (
	"context"
	"testing"
)

func TestRetrieverMatchesPGSkillWithGeneAndEnv(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryStore())
	if err := SeedPGClusterFixture(ctx, svc); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}
	matches, err := svc.Retrieve(ctx, RetrievalQuery{
		UserText:      "我要在 A/B 部署 pg 主从，pg_mon 在 C",
		OSFingerprint: EnvironmentFingerprint{OSDistribution: "ubuntu", PackageManager: "apt"},
		UserScopes:    []AuthorizationScope{{Type: "environment", Value: "prod"}},
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected PG experience match")
	}
	if matches[0].PackID != "pack_aiops_pg_primary_standby" {
		t.Fatalf("unexpected match: %#v", matches[0])
	}
	if contains(matches[0].NextActions, "execute_now") {
		t.Fatal("experience_match must not expose execute_now")
	}
}

func TestRetrieverReturnsOSPreconditionGap(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	svc := NewService(store)
	if err := SeedPGClusterFixture(ctx, svc); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}
	binding := RunnerBinding{
		Type: "RunnerBinding", SchemaVersion: "aiops-runner-binding-v1", ID: "binding_ubuntu",
		WorkflowID: "wf", GeneID: "gene_pack_aiops_pg_primary_standby", Published: true,
		DryRunRequired: true, ApprovalRequired: true, HostLeaseRequired: true,
		EnvSelector: map[string]any{"os_distribution": "ubuntu"},
	}
	binding.AssetID = MustHashCanonicalJSON(binding)
	if err := store.AppendRunnerBinding(ctx, "pack_aiops_pg_primary_standby", binding); err != nil {
		t.Fatalf("append binding: %v", err)
	}
	matches, err := svc.Retrieve(ctx, RetrievalQuery{UserText: "部署 pg 主从", UserScopes: []AuthorizationScope{{Type: "environment", Value: "prod"}}})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(matches) == 0 || !contains(matches[0].PreconditionGaps, "需要确认目标主机操作系统") {
		t.Fatalf("expected OS gap, got %#v", matches)
	}
}

func TestRetrieverAvoidCueBlocksRunnerPlan(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	svc := NewService(store)
	if err := SeedPGClusterFixture(ctx, svc); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}
	cue := AvoidCue{Type: "AvoidCue", ID: "avoid_existing_data", GeneID: "gene_pack_aiops_pg_primary_standby", Signals: []string{"已有数据目录"}, Warning: "不要覆盖已有 PG 数据目录", Blocking: true}
	cue.AssetID = MustHashCanonicalJSON(cue)
	if err := store.AppendAvoidCue(ctx, "pack_aiops_pg_primary_standby", cue); err != nil {
		t.Fatalf("append avoid cue: %v", err)
	}
	matches, err := svc.Retrieve(ctx, RetrievalQuery{UserText: "部署 pg 主从，已有数据目录", OSFingerprint: EnvironmentFingerprint{OSDistribution: "ubuntu", PackageManager: "apt"}, UserScopes: []AuthorizationScope{{Type: "environment", Value: "prod"}}})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(matches) == 0 || len(matches[0].RiskWarnings) == 0 {
		t.Fatalf("expected risk warning, got %#v", matches)
	}
	if contains(matches[0].NextActions, "create_dry_run") {
		t.Fatalf("blocking avoid cue should suppress runner plan: %#v", matches[0].NextActions)
	}
}

func TestRetrieverRequiresMatchingAuthorizationScope(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryStore())
	if err := SeedPGClusterFixture(ctx, svc); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}
	_, err := svc.SaveScopes(ctx, "pack_aiops_pg_primary_standby", []AuthorizationScope{
		{Type: "environment", Value: "prod", Searchable: true},
	})
	if err != nil {
		t.Fatalf("save scopes: %v", err)
	}

	matches, err := svc.Retrieve(ctx, RetrievalQuery{UserText: "部署 pg 主从"})
	if err != nil {
		t.Fatalf("retrieve without scope: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no match without required scope, got %#v", matches)
	}

	matches, err = svc.Retrieve(ctx, RetrievalQuery{
		UserText:   "部署 pg 主从",
		UserScopes: []AuthorizationScope{{Type: "environment", Value: "prod"}},
	})
	if err != nil {
		t.Fatalf("retrieve with scope: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected match with required scope")
	}
}

func contains(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}
