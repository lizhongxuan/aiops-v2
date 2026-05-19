package opsmanual

import "testing"

func TestScoreExactManualGetsStrongStructuralScore(t *testing.T) {
	manual := pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu")
	frame := BuildOperationFrame("在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常", map[string]any{"target_name": "pg-ubuntu-01"})

	breakdown := calculateScoreBreakdown(manual, frame, RunRecordSummary{SuccessCount: 3, RecentResult: "passed"}, nil)
	if breakdown.StructuralScore < 0.80 {
		t.Fatalf("structural score = %.2f, want strong score; breakdown=%#v", breakdown.StructuralScore, breakdown)
	}
	if breakdown.FinalScore < directExecuteMinScore {
		t.Fatalf("final score = %.2f, want >= %.2f; breakdown=%#v", breakdown.FinalScore, directExecuteMinScore, breakdown)
	}
}

func TestScorePenalizesCrossObjectManual(t *testing.T) {
	manual := pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu")
	frame := BuildOperationFrame("在 Ubuntu 主机 mysql-01 上通过 ssh 对 MySQL 做备份，备份到 /data/backups，已确认 ssh_access 正常", nil)

	breakdown := calculateScoreBreakdown(manual, frame, RunRecordSummary{}, nil)
	if breakdown.Penalty < 0.30 {
		t.Fatalf("penalty = %.2f, want object mismatch penalty; breakdown=%#v", breakdown.Penalty, breakdown)
	}
	if breakdown.FinalScore >= candidateMinScore {
		t.Fatalf("final score = %.2f, want below candidate threshold %.2f; breakdown=%#v", breakdown.FinalScore, candidateMinScore, breakdown)
	}
}

func TestScoreUsesMetadataRetrievalProfileCompat(t *testing.T) {
	manual := pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu")
	manual.RetrievalProfile = RetrievalProfile{}
	manual.Metadata = map[string]any{
		"retrieval_profile": map[string]any{
			"keywords": []any{"pg_dump", "pg_isready"},
			"aliases": map[string]any{
				"postgresql": []any{"pg"},
			},
		},
	}
	frame := BuildOperationFrame("用 pg_dump 给 PG 做备份，目标 pg-01，通过 ssh，备份到 /data/backups，pg_isready 正常", nil)

	breakdown := calculateScoreBreakdown(manual, frame, RunRecordSummary{}, nil)
	if breakdown.KeywordScore <= 0 {
		t.Fatalf("keyword score = %.2f, want metadata retrieval profile keywords to contribute; breakdown=%#v", breakdown.KeywordScore, breakdown)
	}
}

func TestScoreWithoutVectorScorerStaysStable(t *testing.T) {
	manual := pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu")
	frame := BuildOperationFrame("在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常", map[string]any{"target_name": "pg-ubuntu-01"})

	breakdown := calculateScoreBreakdown(manual, frame, RunRecordSummary{SuccessCount: 1, LatestStatus: "passed"}, nil)
	if breakdown.VectorScore != 0 {
		t.Fatalf("vector score = %.2f, want 0 without VectorScorer", breakdown.VectorScore)
	}
	if breakdown.FinalScore < directExecuteMinScore {
		t.Fatalf("final score = %.2f, want stable direct score without vector scorer; breakdown=%#v", breakdown.FinalScore, breakdown)
	}
}

func TestManualHintsOnlyPromoteSameObjectActionNearTie(t *testing.T) {
	repo := NewMemoryStore()
	manualA := pgBackupManual("manual-pg-backup-a", "ubuntu", "ssh", "workflow-pg-backup-a")
	manualB := pgBackupManual("manual-pg-backup-b", "ubuntu", "ssh", "workflow-pg-backup-b")
	for _, manual := range []OpsManual{manualA, manualB} {
		if err := repo.SaveManual(manual); err != nil {
			t.Fatalf("SaveManual() error = %v", err)
		}
	}
	service := NewService(repo, WithHintProvider(staticHintProvider{
		manualHints: []ManualHint{{
			ManualID:   "manual-pg-backup-b",
			ObjectType: "postgresql",
			Action:     "backup",
			Source:     "memory_hint",
			Redacted:   true,
			Score:      0.7,
		}},
	}))
	result, err := service.SearchOpsManuals(SearchOpsManualsRequest{
		Text: "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
	})
	if err != nil {
		t.Fatalf("SearchOpsManuals() error = %v", err)
	}
	if len(result.Manuals) < 2 || result.Manuals[0].Manual.ID != "manual-pg-backup-b" {
		t.Fatalf("manual order = %#v, want near-tie hinted manual first", result.Manuals)
	}
	if len(result.Manuals[0].HintSources) != 1 || result.Manuals[0].HintSources[0] != "memory_hint" {
		t.Fatalf("hint sources = %#v, want memory_hint marker", result.Manuals[0].HintSources)
	}

	service = NewService(repo, WithHintProvider(staticHintProvider{
		manualHints: []ManualHint{{
			ManualID:   "manual-pg-backup-b",
			ObjectType: "mysql",
			Action:     "backup",
			Source:     "memory_hint",
			Redacted:   true,
			Score:      0.7,
		}},
	}))
	result, err = service.SearchOpsManuals(SearchOpsManualsRequest{
		Text: "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
	})
	if err != nil {
		t.Fatalf("SearchOpsManuals() error = %v", err)
	}
	if len(result.Manuals) < 2 || result.Manuals[0].Manual.ID != "manual-pg-backup-a" {
		t.Fatalf("manual order = %#v, cross-object hint must not promote", result.Manuals)
	}
}
