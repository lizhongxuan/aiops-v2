package experiencepack

import (
	"context"
	"testing"
)

func TestGeneratorCreatesPGSkillGeneCapsule(t *testing.T) {
	bundle, err := GenerateCandidate(CandidateInput{
		PackID: "pack_pg_test", Name: "PG 主从部署经验", Summary: "部署 PostgreSQL 主从并配置 pg_mon",
		Trajectory: Trajectory{CaseID: "case-1", UserGoal: "部署 pg 主从，pg_mon 在 C", Commands: []string{"1", "2", "3", "4", "5", "6"}, ProofID: "proof-1", Outcome: "success", Environment: EnvironmentFingerprint{OS: "linux", OSDistribution: "ubuntu", PackageManager: "apt", HostCount: 3}},
	})
	if err != nil {
		t.Fatalf("generate candidate: %v", err)
	}
	if bundle.Skill.Path != "skills/SKILL.md" || bundle.Gene.Type != "Gene" || bundle.Capsule.Type != "Capsule" {
		t.Fatalf("unexpected bundle: %#v", bundle)
	}
	if bundle.RunnerCandidate == nil {
		t.Fatal("six reusable commands should produce runner candidate")
	}
}

func TestGeneratorFallsBackToDefaultSignals(t *testing.T) {
	bundle, err := GenerateCandidate(CandidateInput{
		PackID:  "pack_generic_runner",
		Name:    "通用 Runner 草稿经验包",
		Summary: "验证通用运维轨迹也能生成候选。",
		Trajectory: Trajectory{
			CaseID:   "case-generic-runner",
			UserGoal: "验证 Runner 草稿包含人工审批、Dry Run、受控执行、恢复验证和回滚。",
			ProofID:  "proof-generic",
			Outcome:  "success",
			Environment: EnvironmentFingerprint{
				OS:             "linux",
				OSDistribution: "unknown",
				HostCount:      1,
			},
		},
	})
	if err != nil {
		t.Fatalf("generate candidate: %v", err)
	}
	if len(bundle.Capsule.Trigger) < 3 {
		t.Fatalf("capsule trigger = %v, want default fallback signals", bundle.Capsule.Trigger)
	}
}

func TestFailedEvolutionCreatesAvoidCue(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	_, cue, err := ApplyEvolution(ctx, store, EvolutionTrigger{
		MatchedPackID: "pack_pg_failed", MatchedGeneID: "gene_pg", Outcome: "failed",
		Trajectory: Trajectory{CaseID: "case-fail", UserGoal: "部署 pg 主从但已有数据目录", Outcome: "failed", ProofID: "proof-fail"},
		Env:        EnvironmentFingerprint{OS: "linux", OSDistribution: "ubuntu", PackageManager: "apt"},
	})
	if err != nil {
		t.Fatalf("apply failed evolution: %v", err)
	}
	if cue == nil || cue.Warning == "" {
		t.Fatalf("expected avoid cue, got %#v", cue)
	}
}
