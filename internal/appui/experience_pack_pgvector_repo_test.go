package appui

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestPGVectorExperiencePackRepositoryRetrievesWithHybridIndex(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("AIOPS_TEST_PGVECTOR_DSN"))
	if dsn == "" {
		t.Skip("set AIOPS_TEST_PGVECTOR_DSN to run pgvector integration test")
	}
	repo, err := NewPGVectorExperiencePackRepository(dsn)
	if err != nil {
		t.Fatalf("open pgvector repo: %v", err)
	}
	defer repo.Close()

	packID := "pack-pgvector-" + time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(func() {
		_, _ = repo.db.ExecContext(t.Context(), `DELETE FROM aiops_experience_pack_docs WHERE pack_id = $1`, packID)
	})
	pack := normalizeExperiencePack(ExperiencePack{
		ID:           packID,
		PackID:       packID,
		Title:        "PostgreSQL 主从部署经验包",
		Summary:      "在两台主机部署 PostgreSQL 主从复制，并把 pg_mon 放到第三台主机做监控。",
		Category:     "innovate",
		UsageShape:   "guided",
		Middleware:   "postgresql",
		Tags:         []string{"postgresql", "pg_mon", "replication", "主从"},
		Status:       "enabled",
		ReviewStatus: "approved",
		Enabled:      true,
		Skill: ExperiencePackSkill{
			ID:      "skill-postgres-replication",
			Name:    "PostgreSQL 主从部署 Skill",
			Summary: "说明何时适用 PG 主从部署、如何根据 OS 和主机角色调整 Runner、如何验证复制与 pg_mon。",
			Path:    "skills/SKILL.md",
		},
		History: ExperiencePackHistory{SuccessCount: 3, RecentResult: "success"},
		AdvancedRefs: ExperiencePackAdvancedRefs{
			GeneAssetID:     "sha256:gene-postgres-replication",
			CapsuleAssetIDs: []string{"sha256:capsule-postgres-replication-prod"},
		},
		ValidationGate: ExperiencePackValidationGate{Status: "passed"},
		RunnerBindings: []ExperiencePackRunnerBinding{{
			ID:           "binding-postgres-replication",
			WorkflowID:   "wf-postgres-replication",
			WorkflowName: "PostgreSQL 主从部署 Workflow",
			Status:       "published",
			ReviewStatus: "approved",
			Metadata:     map[string]any{"published": true},
		}},
		AuthorizationScopes: []ExperiencePackAuthorizationScope{{
			Type:       "environment",
			Value:      "prod/postgresql",
			Searchable: true,
		}},
		Metadata: map[string]any{
			"signals": []any{"postgresql", "pg_mon", "replication lag", "primary standby"},
			"commands": []any{
				"install postgres on hostA",
				"install postgres on hostB",
				"configure primary",
				"configure standby",
				"install pg_mon on hostC",
				"validate replication",
			},
			"env_fingerprint": map[string]any{"os": "linux", "postgresql": "16"},
		},
	})
	if err := repo.SaveExperiencePack(pack); err != nil {
		t.Fatalf("save pack: %v", err)
	}

	matches, err := repo.RetrieveExperiencePacks(ExperiencePackRetrieveRequest{
		UserText:    "给主机A和主机B搭建 PG 主从集群，pg_mon 放到主机C上",
		Signals:     []string{"postgresql", "pg_mon", "primary standby"},
		Environment: "prod/postgresql",
	})
	if err != nil {
		t.Fatalf("retrieve packs: %v", err)
	}
	var got ExperiencePackMatch
	for _, match := range matches.Items {
		if match.PackID == packID {
			got = match
			break
		}
	}
	if got.PackID == "" {
		t.Fatalf("matches = %+v, want match %q", matches, packID)
	}
	if got.Confidence < 0.5 {
		t.Fatalf("confidence=%v want >= 0.5", got.Confidence)
	}
	if !containsExperiencePackSignal(got.MatchReasons, "PostgreSQL pgvector 语义索引命中") {
		t.Fatalf("match reasons = %+v, want pgvector reason", got.MatchReasons)
	}
	if !containsExperiencePackAction(got.NextActions, "create_dry_run") {
		t.Fatalf("next actions = %+v, want create_dry_run for published runner", got.NextActions)
	}
}

func TestPGVectorExperiencePackMatchRejectsDifferentMiddlewareWithOnlyGenericSignals(t *testing.T) {
	pack := normalizeExperiencePack(ExperiencePack{
		ID:         "pack-postgres-only",
		Title:      "PostgreSQL 主从部署经验包",
		Summary:    "PostgreSQL 主从部署和 pg_mon 验证经验。",
		Middleware: "postgresql",
		Skill:      ExperiencePackSkill{Name: "PostgreSQL 主从部署 Skill"},
	})
	match, ok := experiencePackMatchFromIndexedPack(
		pack,
		ExperiencePackRetrieveRequest{
			UserText:    "对 MySQL 数据库 aiops_biz 做一次备份并验证恢复",
			Signals:     []string{"mysql", "mysqldump", "backup", "验证"},
			Environment: "prod/mysql",
		},
		"对 MySQL 数据库 aiops_biz 做一次备份并验证恢复",
		[]string{"mysql", "mysqldump", "backup", "验证"},
		experiencePackSearchDocument(pack),
		experiencePackEnvironmentDocument(pack),
		0.31,
		0,
	)
	if ok {
		t.Fatalf("match = %+v, want different middleware rejected", match)
	}
}

func TestPGVectorExperiencePackMatchDowngradesDifferentOperationToReference(t *testing.T) {
	pack := normalizeExperiencePack(ExperiencePack{
		ID:         "pack-postgres-replication",
		Title:      "PostgreSQL 主从部署经验包",
		Summary:    "在两台主机部署 PostgreSQL 主从复制，配置 pg_basebackup 和 pg_mon，再执行恢复验证。",
		Middleware: "postgresql",
		Skill:      ExperiencePackSkill{Name: "PostgreSQL 主从部署 Skill"},
		RunnerBindings: []ExperiencePackRunnerBinding{{
			WorkflowID:   "wf-postgres-replication",
			WorkflowName: "PostgreSQL 主从部署 Workflow",
			Status:       "published",
			ReviewStatus: "approved",
		}},
		Metadata: map[string]any{
			"commands": []any{
				"pg_basebackup -h hostA -D /var/lib/postgresql/data",
				"systemctl restart postgresql",
				"psql -c 'select pg_is_in_recovery()'",
			},
			"signals": []any{"postgresql", "pg_basebackup", "主从", "恢复验证"},
		},
	})
	searchDoc := experiencePackSearchDocument(pack)
	envDoc := experiencePackEnvironmentDocument(pack)
	match, ok := experiencePackMatchFromIndexedPack(
		pack,
		ExperiencePackRetrieveRequest{
			UserText:    "对 PostgreSQL 数据库 appdb 执行 pg_dump 逻辑备份并做恢复验证",
			Signals:     []string{"postgresql", "pg_dump", "备份", "恢复验证"},
			Environment: "prod/postgresql",
		},
		"对 PostgreSQL 数据库 appdb 执行 pg_dump 逻辑备份并做恢复验证",
		[]string{"postgresql", "pg_dump", "备份", "恢复验证"},
		searchDoc,
		envDoc,
		0.62,
		0.2,
	)
	if !ok {
		t.Fatalf("want same middleware experience to remain visible as reference")
	}
	if match.CompatibilityStatus != "reference_only" {
		t.Fatalf("compatibility=%q want reference_only", match.CompatibilityStatus)
	}
	if containsExperiencePackAction(match.NextActions, "create_dry_run") {
		t.Fatalf("next actions = %+v, must not create dry run for reference-only match", match.NextActions)
	}
}

func TestPGVectorExperiencePackMatchAcceptsSameDomainBackupWithGenericSignals(t *testing.T) {
	pack := normalizeExperiencePack(ExperiencePack{
		ID:         "pack-mysql-backup",
		Title:      "MySQL 逻辑备份经验包",
		Summary:    "使用 mysqldump 对 MySQL 数据库做逻辑备份，并在恢复演练库验证 orders 行数。",
		Middleware: "mysql",
		Skill:      ExperiencePackSkill{Name: "MySQL 逻辑备份 Skill"},
		Metadata: map[string]any{
			"commands": []any{
				"mysql -h127.0.0.1 -P33306 aiops_biz -e 'SELECT COUNT(*) FROM orders'",
				"mysqldump -h127.0.0.1 -P33306 --single-transaction aiops_biz orders > backup.sql",
				"mysql aiops_biz_restore_verify < backup.sql",
			},
			"signals": []any{"mysql", "mysqldump", "备份", "恢复验证", "orders"},
		},
	})
	searchDoc := experiencePackSearchDocument(pack)
	envDoc := experiencePackEnvironmentDocument(pack)
	match, ok := experiencePackMatchFromIndexedPack(
		pack,
		ExperiencePackRetrieveRequest{
			UserText:    "对 MySQL aiops_biz.orders 做逻辑备份并恢复验证 order_count=2",
			Signals:     []string{"mysql", "mysqldump", "备份", "恢复验证", "orders"},
			Environment: "prod/mysql",
		},
		"对 MySQL aiops_biz.orders 做逻辑备份并恢复验证 order_count=2",
		[]string{"mysql", "mysqldump", "备份", "恢复验证", "orders"},
		searchDoc,
		envDoc,
		0.45,
		0.2,
	)
	if !ok {
		t.Fatalf("want MySQL backup experience to match")
	}
	if match.Confidence < 0.45 {
		t.Fatalf("confidence=%v want >= 0.45", match.Confidence)
	}
	if !containsExperiencePackSignal(match.MatchReasons, "硬兼容条件通过") {
		t.Fatalf("match reasons = %+v, want compatibility gate reason", match.MatchReasons)
	}
	if match.CompatibilityStatus != "direct" {
		t.Fatalf("compatibility=%q want direct", match.CompatibilityStatus)
	}
}

func TestPGVectorExperiencePackMatchAdaptsSameOperationDifferentOS(t *testing.T) {
	pack := normalizeExperiencePack(ExperiencePack{
		ID:         "pack-postgres-backup-ubuntu",
		Title:      "Ubuntu PostgreSQL 逻辑备份经验包",
		Summary:    "在 Ubuntu 主机上使用 pg_dump 对 PostgreSQL 数据库做逻辑备份，并在恢复演练库验证。",
		Middleware: "postgresql",
		Skill:      ExperiencePackSkill{Name: "Ubuntu PostgreSQL 逻辑备份 Skill"},
		RunnerBindings: []ExperiencePackRunnerBinding{{
			WorkflowID:   "wf-postgres-backup-ubuntu",
			WorkflowName: "Ubuntu PostgreSQL Backup Workflow",
			Status:       "published",
			ReviewStatus: "approved",
		}},
		Metadata: map[string]any{
			"commands": []any{
				"apt-get install -y postgresql-client",
				"pg_dump -h pg-primary -d appdb > appdb.sql",
				"psql -d appdb_restore_verify < appdb.sql",
			},
			"signals":         []any{"postgresql", "pg_dump", "备份", "恢复验证"},
			"env_fingerprint": map[string]any{"os": "ubuntu", "postgresql": "16"},
		},
	})
	searchDoc := experiencePackSearchDocument(pack)
	envDoc := experiencePackEnvironmentDocument(pack)
	match, ok := experiencePackMatchFromIndexedPack(
		pack,
		ExperiencePackRetrieveRequest{
			UserText:    "在 CentOS 主机上对 PostgreSQL appdb 执行 pg_dump 逻辑备份并恢复验证",
			Signals:     []string{"postgresql", "pg_dump", "备份", "恢复验证"},
			OS:          "centos",
			Environment: "prod/postgresql",
		},
		"在 CentOS 主机上对 PostgreSQL appdb 执行 pg_dump 逻辑备份并恢复验证",
		[]string{"postgresql", "pg_dump", "备份", "恢复验证"},
		searchDoc,
		envDoc,
		0.55,
		0.2,
	)
	if !ok {
		t.Fatalf("want same middleware and operation to match as adaptable")
	}
	if match.CompatibilityStatus != "adapt_required" {
		t.Fatalf("compatibility=%q want adapt_required", match.CompatibilityStatus)
	}
	if !containsExperiencePackAction(match.NextActions, "create_adaptation_plan") {
		t.Fatalf("next actions = %+v, want create_adaptation_plan", match.NextActions)
	}
	if containsExperiencePackAction(match.NextActions, "create_dry_run") {
		t.Fatalf("next actions = %+v, must not directly dry-run original Runner", match.NextActions)
	}
	if len(match.CompatibilityGaps) == 0 {
		t.Fatalf("want OS compatibility gap")
	}
}
