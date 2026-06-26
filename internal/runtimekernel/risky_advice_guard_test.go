package runtimekernel

import "testing"

func TestRiskyAdviceGuardFlagsArchiveDeletionWithoutEvidenceGate(t *testing.T) {
	answer := "清理 archive 中更高 timeline 的 WAL：rm -rf {{ARCHIVE_PATH}}/repos/archive/paf/15-1/*"
	result := EvaluateRiskyOperationalAdvice(answer)
	if !result.RequiresEvidenceGate {
		t.Fatalf("RequiresEvidenceGate = false, want true")
	}
	if result.Category != "destructive_archive_or_data_deletion" {
		t.Fatalf("Category = %q", result.Category)
	}
}

func TestRiskyAdviceGuardAllowsScopedMoveWithEvidenceAndBackup(t *testing.T) {
	answer := "先确认 pg_controldata、timeline history、备份可用，再将单个冲突 WAL move 到隔离目录，不要删除。"
	result := EvaluateRiskyOperationalAdvice(answer)
	if result.RequiresEvidenceGate {
		t.Fatalf("RequiresEvidenceGate = true, want false for scoped guarded move")
	}
}

func TestRiskyAdviceGuardFlagsManualServiceMutationCommandAdvice(t *testing.T) {
	answer := "可以先看状态，然后执行 sudo systemctl restart nginx，再用 systemctl status nginx 验收。"
	result := EvaluateRiskyOperationalAdvice(answer)
	if !result.RequiresEvidenceGate {
		t.Fatalf("RequiresEvidenceGate = false, want true for manual mutation command advice")
	}
	if result.Category != "ungated_mutation_command_advice" {
		t.Fatalf("Category = %q, want ungated_mutation_command_advice", result.Category)
	}
}

func TestRiskyAdviceGuardAllowsReadOnlyServiceStatusAdvice(t *testing.T) {
	answer := "先使用 systemctl status nginx 和 journalctl -u nginx --since '10 min ago' 收集只读证据。"
	result := EvaluateRiskyOperationalAdvice(answer)
	if result.RequiresEvidenceGate {
		t.Fatalf("RequiresEvidenceGate = true, want false for read-only status advice: %+v", result)
	}
}

func TestRiskyAdviceGuardFlagsGenericDataDirectoryDeletionWithoutGate(t *testing.T) {
	answer := "如果服务启动失败，可以直接删除数据目录后重新初始化。"
	result := EvaluateRiskyOperationalAdvice(answer)
	if !result.RequiresEvidenceGate {
		t.Fatalf("RequiresEvidenceGate = false, want true for destructive data directory advice")
	}
}

func TestRiskyAdviceGuardFlagsDestructiveDataDeletionWithoutMaintenanceWindow(t *testing.T) {
	answer := "清空从库 PGDATA 和删除 WAL archive 只能作为审批后的候选动作：先确认目标从库角色、备份可用、timeline 元数据和回滚验收方案。"
	result := EvaluateRiskyOperationalAdvice(answer)
	if !result.RequiresEvidenceGate {
		t.Fatalf("RequiresEvidenceGate = false, want true when destructive data/archive advice lacks a maintenance window")
	}
	if result.Category != "destructive_archive_or_data_deletion" {
		t.Fatalf("Category = %q, want destructive_archive_or_data_deletion", result.Category)
	}
}

func TestRiskyAdviceGuardFlagsDestructiveDataDeletionWhenOnlyLowTrafficWindow(t *testing.T) {
	answer := "清空从库 PGDATA 和删除 WAL archive 只能作为审批后的候选动作：先确认目标从库角色、备份可用、timeline 元数据和回滚验收方案，并安排在业务低峰期执行。"
	result := EvaluateRiskyOperationalAdvice(answer)
	if !result.RequiresEvidenceGate {
		t.Fatalf("RequiresEvidenceGate = false, want true because low-traffic timing is not a maintenance or service-stop window")
	}
	if result.Category != "destructive_archive_or_data_deletion" {
		t.Fatalf("Category = %q, want destructive_archive_or_data_deletion", result.Category)
	}
}

func TestRiskyAdviceGuardFlagsPgRewindWithoutApprovalBackupGate(t *testing.T) {
	answer := "恢复方向：pg_rewind 主机 B 回退到分叉点后重新 follow 主机 A。"
	result := EvaluateRiskyOperationalAdvice(answer)
	if !result.RequiresEvidenceGate {
		t.Fatalf("RequiresEvidenceGate = false, want true for pg_rewind without approval/backup gate")
	}
	if result.Category != "high_risk_replication_repair" {
		t.Fatalf("Category = %q, want high_risk_replication_repair", result.Category)
	}
}

func TestRiskyAdviceGuardAllowsApprovalGatedPgRewindCandidate(t *testing.T) {
	answer := "pg_rewind 只能作为审批后的候选动作：先确认 timeline history、备份可用、权威数据源、回滚和验收方案，再在维护窗口执行。"
	result := EvaluateRiskyOperationalAdvice(answer)
	if result.RequiresEvidenceGate {
		t.Fatalf("RequiresEvidenceGate = true, want false for approval-gated pg_rewind candidate: %+v", result)
	}
}

func TestRiskyAdviceGuardFlagsPrimaryPromotionWithoutAuthorityGate(t *testing.T) {
	answer := "恢复方案候选：将主机B提升为 Primary，再从主机A导出数据合并。"
	result := EvaluateRiskyOperationalAdvice(answer)
	if !result.RequiresEvidenceGate {
		t.Fatalf("RequiresEvidenceGate = false, want true for authority switch without gate")
	}
	if result.Category != "high_risk_replication_repair" {
		t.Fatalf("Category = %q, want high_risk_replication_repair", result.Category)
	}
}

func TestRiskyAdviceGuardFlagsUnsupportedLineageAuthorityInference(t *testing.T) {
	answer := "主机A 是过期主库，主机B timeline 9 拥有更权威的数据，建议以主机B为权威源重建主机A。"
	result := EvaluateRiskyOperationalAdvice(answer)
	if !result.RequiresEvidenceGate {
		t.Fatalf("RequiresEvidenceGate = false, want true for unsupported lineage authority inference")
	}
	if result.Category != "unsupported_lineage_authority_inference" {
		t.Fatalf("Category = %q, want unsupported_lineage_authority_inference", result.Category)
	}
}

func TestRiskyAdviceGuardAllowsTimelineAuthorityCaveat(t *testing.T) {
	answer := "timeline 9 更高不代表数据权威；无法确认权威数据源，需先检查 .history、WAL 连续性和控制面状态，再考虑重建受影响 standby。"
	result := EvaluateRiskyOperationalAdvice(answer)
	if result.RequiresEvidenceGate {
		t.Fatalf("RequiresEvidenceGate = true, want false for authority caveat: %+v", result)
	}
}
