package experiencepack

import (
	"context"
	"testing"
)

func TestGEPSkillBundleClosedLoop(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	svc := NewService(store)

	suggestion := svc.EvaluateSuggestion(SuggestionInput{CommandCount: 6, LLMOperationalValueScore: 0.9, Outcome: "success", RedactionStatus: "redacted", MemoryGraphWritable: true, ReusableStepCount: 6})
	if !suggestion.Visible {
		t.Fatalf("expected suggestion buttons: %#v", suggestion)
	}

	bundle, err := svc.GenerateAndPersistCandidate(ctx, CandidateInput{
		PackID: "pack_closed_loop_pg", Name: "PG 主从部署经验", Summary: "部署 PostgreSQL 主从并配置 pg_mon",
		Trajectory: Trajectory{CaseID: "case-closed", UserGoal: "部署 pg 主从，pg_mon 在 C", Commands: []string{"1", "2", "3", "4", "5", "6"}, ProofID: "proof-closed", Outcome: "success", Environment: EnvironmentFingerprint{OS: "linux", OSDistribution: "ubuntu", PackageManager: "apt", HostCount: 3}},
	})
	if err != nil {
		t.Fatalf("confirm candidate: %v", err)
	}
	if bundle.EvolutionEvent.ID == "" || len(bundle.MemoryGraphEvents) == 0 {
		t.Fatal("candidate generation must write evolution and memory graph events")
	}

	matches, err := svc.Retrieve(ctx, RetrievalQuery{UserText: "部署 pg 主从", OSFingerprint: EnvironmentFingerprint{OSDistribution: "ubuntu"}})
	if err != nil {
		t.Fatalf("retrieve before review: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("candidate should not be retrieved before review/enable: %#v", matches)
	}

	if _, err := svc.Review(ctx, "pack_closed_loop_pg", true); err != nil {
		t.Fatalf("review: %v", err)
	}
	events, err := store.ListEvolutionEvents(ctx, "pack_closed_loop_pg")
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("review should append EvolutionEvent, got %d events", len(events))
	}
	if _, err := svc.SaveScopes(ctx, "pack_closed_loop_pg", []AuthorizationScope{{Type: "environment", Value: "prod", Searchable: true}}); err != nil {
		t.Fatalf("scopes: %v", err)
	}
	if _, err := svc.Enable(ctx, "pack_closed_loop_pg", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	matches, err = svc.Retrieve(ctx, RetrievalQuery{UserText: "部署 pg 主从，pg_mon 在 C", OSFingerprint: EnvironmentFingerprint{OSDistribution: "ubuntu", PackageManager: "apt"}, UserScopes: []AuthorizationScope{{Type: "environment", Value: "prod"}}})
	if err != nil {
		t.Fatalf("retrieve after enable: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("enabled pack should be retrievable")
	}
}

func TestClosedLoopMatchedExperienceSuccessEvolution(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	svc := NewService(store)
	if err := SeedPGClusterFixture(ctx, svc); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}

	matches, err := svc.Retrieve(ctx, RetrievalQuery{
		UserText:      "部署 pg 主从，pg_mon 在 C",
		OSFingerprint: EnvironmentFingerprint{OSDistribution: "ubuntu", PackageManager: "apt"},
		UserScopes:    []AuthorizationScope{{Type: "environment", Value: "prod"}},
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected experience_match")
	}
	suggestion := svc.EvaluateSuggestion(SuggestionInput{MatchedPackID: matches[0].PackID, CommandCount: 6, LLMOperationalValueScore: 0.9, Outcome: "success", RedactionStatus: "redacted", MemoryGraphWritable: true})
	if !suggestion.Visible || suggestion.Suggestions[0].Type != "evolve_current_experience" {
		t.Fatalf("expected evolve suggestion, got %#v", suggestion)
	}
	beforeCapsules, _ := store.ListCapsules(ctx, matches[0].PackID)
	beforeMemory, _ := store.ListMemoryGraphEvents(ctx, matches[0].PackID)
	if _, _, err := ApplyEvolution(ctx, store, EvolutionTrigger{
		MatchedPackID: matches[0].PackID, MatchedGeneID: matches[0].SelectedGeneID, Outcome: "success",
		Trajectory: Trajectory{CaseID: "case-success-evolve", UserGoal: "部署 pg 主从", Commands: []string{"1", "2", "3", "4", "5", "6"}, ProofID: "proof-success-evolve", Outcome: "success"},
		Env:        EnvironmentFingerprint{OS: "linux", OSDistribution: "ubuntu", PackageManager: "apt", HostCount: 3},
	}); err != nil {
		t.Fatalf("apply success evolution: %v", err)
	}
	afterCapsules, _ := store.ListCapsules(ctx, matches[0].PackID)
	afterMemory, _ := store.ListMemoryGraphEvents(ctx, matches[0].PackID)
	if len(afterCapsules) <= len(beforeCapsules) || len(afterMemory) <= len(beforeMemory) {
		t.Fatalf("expected appended capsule and memory event, capsules %d->%d memory %d->%d", len(beforeCapsules), len(afterCapsules), len(beforeMemory), len(afterMemory))
	}
}

func TestClosedLoopMatchedExperienceFailureCreatesAvoidCue(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	svc := NewService(store)
	if err := SeedPGClusterFixture(ctx, svc); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}
	matches, err := svc.Retrieve(ctx, RetrievalQuery{
		UserText:      "部署 pg 主从",
		OSFingerprint: EnvironmentFingerprint{OSDistribution: "ubuntu", PackageManager: "apt"},
		UserScopes:    []AuthorizationScope{{Type: "environment", Value: "prod"}},
	})
	if err != nil || len(matches) == 0 {
		t.Fatalf("retrieve match err=%v matches=%#v", err, matches)
	}

	_, cue, err := ApplyEvolution(ctx, store, EvolutionTrigger{
		MatchedPackID: matches[0].PackID, MatchedGeneID: matches[0].SelectedGeneID, Outcome: "failed",
		Trajectory: Trajectory{CaseID: "case-failed-evolve", UserGoal: "部署 pg 主从但已有数据目录", Outcome: "failed", ProofID: "proof-failed-evolve"},
		Env:        EnvironmentFingerprint{OS: "linux", OSDistribution: "ubuntu", PackageManager: "apt"},
	})
	if err != nil {
		t.Fatalf("apply failed evolution: %v", err)
	}
	if cue == nil || cue.Warning == "" {
		t.Fatalf("expected avoid cue, got %#v", cue)
	}
	matches, err = svc.Retrieve(ctx, RetrievalQuery{
		UserText:      "部署 pg 主从，已有数据目录",
		OSFingerprint: EnvironmentFingerprint{OSDistribution: "ubuntu", PackageManager: "apt"},
		UserScopes:    []AuthorizationScope{{Type: "environment", Value: "prod"}},
	})
	if err != nil || len(matches) == 0 {
		t.Fatalf("retrieve after avoid err=%v matches=%#v", err, matches)
	}
	if contains(matches[0].NextActions, "create_dry_run") {
		t.Fatalf("avoid cue should block dry run action: %#v", matches[0].NextActions)
	}
}

func TestClosedLoopSelectsOSVariant(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	svc := NewService(store)
	if err := SeedPGClusterFixture(ctx, svc); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}
	genes, _ := store.ListGenes(ctx, "pack_aiops_pg_primary_standby")
	if len(genes) == 0 {
		t.Fatal("expected seeded gene")
	}
	baseGene := genes[0]
	ubuntuBinding := RunnerBinding{Type: "RunnerBinding", SchemaVersion: "aiops-runner-binding-v1", ID: "binding_ubuntu_pg", WorkflowID: "wf_ubuntu_pg", GeneID: baseGene.ID, Published: true, DryRunRequired: true, ApprovalRequired: true, HostLeaseRequired: true, EnvSelector: map[string]any{"os_distribution": "ubuntu"}}
	ubuntuBinding.AssetID = MustHashCanonicalJSON(ubuntuBinding)
	if err := store.AppendRunnerBinding(ctx, "pack_aiops_pg_primary_standby", ubuntuBinding); err != nil {
		t.Fatalf("append ubuntu binding: %v", err)
	}
	rhelGene := baseGene
	rhelGene.ID = "gene_pg_rhel"
	rhelGene.EnvSelector = map[string]any{"os_distribution": "rhel", "package_manager": "dnf"}
	rhelGene.AssetID = MustHashCanonicalJSON(rhelGene)
	if err := store.AppendGene(ctx, "pack_aiops_pg_primary_standby", rhelGene); err != nil {
		t.Fatalf("append rhel gene: %v", err)
	}
	rhelBinding := RunnerBinding{Type: "RunnerBinding", SchemaVersion: "aiops-runner-binding-v1", ID: "binding_rhel_pg", WorkflowID: "wf_rhel_pg", GeneID: rhelGene.ID, Published: true, DryRunRequired: true, ApprovalRequired: true, HostLeaseRequired: true, EnvSelector: map[string]any{"os_distribution": "rhel", "package_manager": "dnf"}}
	rhelBinding.AssetID = MustHashCanonicalJSON(rhelBinding)
	if err := store.AppendRunnerBinding(ctx, "pack_aiops_pg_primary_standby", rhelBinding); err != nil {
		t.Fatalf("append rhel binding: %v", err)
	}

	ubuntu, err := svc.Retrieve(ctx, RetrievalQuery{UserText: "部署 pg 主从", OSFingerprint: EnvironmentFingerprint{OSDistribution: "ubuntu", PackageManager: "apt"}, UserScopes: []AuthorizationScope{{Type: "environment", Value: "prod"}}})
	if err != nil || len(ubuntu) == 0 {
		t.Fatalf("ubuntu retrieve err=%v matches=%#v", err, ubuntu)
	}
	if ubuntu[0].SelectedGeneID != baseGene.ID || ubuntu[0].SelectedRunnerBindingID != ubuntuBinding.ID {
		t.Fatalf("ubuntu selected %#v, want %s/%s", ubuntu[0], baseGene.ID, ubuntuBinding.ID)
	}
	rhel, err := svc.Retrieve(ctx, RetrievalQuery{UserText: "部署 pg 主从", OSFingerprint: EnvironmentFingerprint{OSDistribution: "rhel", PackageManager: "dnf"}, UserScopes: []AuthorizationScope{{Type: "environment", Value: "prod"}}})
	if err != nil || len(rhel) == 0 {
		t.Fatalf("rhel retrieve err=%v matches=%#v", err, rhel)
	}
	if rhel[0].SelectedGeneID != rhelGene.ID || rhel[0].SelectedRunnerBindingID != rhelBinding.ID {
		t.Fatalf("rhel selected %#v, want %s/%s", rhel[0], rhelGene.ID, rhelBinding.ID)
	}
	unknown, err := svc.Retrieve(ctx, RetrievalQuery{UserText: "部署 pg 主从", UserScopes: []AuthorizationScope{{Type: "environment", Value: "prod"}}})
	if err != nil || len(unknown) == 0 || !contains(unknown[0].PreconditionGaps, "需要确认目标主机操作系统") {
		t.Fatalf("unknown OS should return precondition gap err=%v matches=%#v", err, unknown)
	}
}
