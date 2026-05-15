package opsmanual

import "testing"

func TestRetrieveManualsRedisTriageNeedsMoreInfo(t *testing.T) {
	repo := NewMemoryStore()
	if err := repo.SaveManual(redisMemoryManual()); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	matches, err := RetrieveManuals(repo, BuildOperationFrame("排查 Redis", nil))
	if err != nil {
		t.Fatalf("RetrieveManuals() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %#v, want one", matches)
	}
	match := matches[0]
	if match.State != DecisionNeedMoreInfo {
		t.Fatalf("state = %q, want %q", match.State, DecisionNeedMoreInfo)
	}
	for _, want := range []string{"target_instance", "used_memory_rss", "p95"} {
		if !contains(match.MissingContext, want) {
			t.Fatalf("missing context = %#v, want %q", match.MissingContext, want)
		}
	}
	for _, reason := range match.Reasons {
		if stringsContains(reason, "direct") || stringsContains(reason, "可直接执行") {
			t.Fatalf("reason %q must not claim direct execution", reason)
		}
	}
}

func TestRetrieveManualsFullRedisRequestDirect(t *testing.T) {
	repo := NewMemoryStore()
	if err := repo.SaveManual(redisMemoryManual()); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	frame := BuildOperationFrame("生产 payment-api 的 Redis used_memory_rss 持续上涨，Coroot 显示 p95 升高，请通过 ssh 排查", nil)
	matches, err := RetrieveManuals(repo, frame)
	if err != nil {
		t.Fatalf("RetrieveManuals() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %#v, want one", matches)
	}
	if matches[0].State != DecisionDirect {
		t.Fatalf("state = %q, want direct", matches[0].State)
	}
	for _, want := range []string{"fill_parameters", "run_precheck", "start_dry_run"} {
		if !contains(matches[0].RecommendedNextActions, want) {
			t.Fatalf("actions = %#v, want %q", matches[0].RecommendedNextActions, want)
		}
	}
	if contains(matches[0].RecommendedNextActions, "request_approval") {
		t.Fatalf("actions = %#v, must not include request_approval", matches[0].RecommendedNextActions)
	}
}

func TestRetrieveManualsCentOSPostgresBackupRequiresAdaptation(t *testing.T) {
	repo := NewMemoryStore()
	if err := repo.SaveManual(postgresBackupUbuntuManual()); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	frame := BuildOperationFrame("CentOS PostgreSQL backup /data/backups via ssh", map[string]any{"target_name": "pg-1"})
	matches, err := RetrieveManuals(repo, frame)
	if err != nil {
		t.Fatalf("RetrieveManuals() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %#v, want one", matches)
	}
	if matches[0].State != DecisionAdapt {
		t.Fatalf("state = %q, want adapt_required", matches[0].State)
	}
	if !contains(matches[0].CompatibilityGaps, "os") {
		t.Fatalf("compatibility gaps = %#v, want os", matches[0].CompatibilityGaps)
	}
	for _, action := range matches[0].RecommendedNextActions {
		if action == "start_dry_run" || action == "request_human_review" || action == "request_approval" {
			t.Fatalf("adapt_required actions = %#v, must not include %q", matches[0].RecommendedNextActions, action)
		}
	}
}

func TestSearchOpsManualsDirectExecuteForExactPostgresBackup(t *testing.T) {
	repo := NewMemoryStore()
	mustSaveManual(t, repo, pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu"))
	mustSaveRunRecord(t, repo, RunRecord{
		ID: "rr-1", ManualID: "manual-pg-backup-ubuntu", WorkflowID: "workflow-pg-backup-ubuntu",
		ExecutionStatus: "passed", ValidationStatus: "passed", CompletedAt: "2026-05-15T01:00:00Z",
	})

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		OperationFrame: BuildOperationFrame("在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常", nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionDirectExecute {
		t.Fatalf("decision = %q, want direct_execute; result=%#v", result.Decision, result)
	}
	if len(result.Manuals) != 1 || result.Manuals[0].RecommendedAction != "run_bound_workflow" {
		t.Fatalf("manuals = %#v, want run_bound_workflow", result.Manuals)
	}
}

func TestSearchOpsManualsNeedInfoForShortRedisTriage(t *testing.T) {
	repo := NewMemoryStore()
	mustSaveManual(t, repo, redisRcaManual())

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{Text: "排查 Redis"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionNeedInfo {
		t.Fatalf("decision = %q, want need_info; result=%#v", result.Decision, result)
	}
	if len(result.NextQuestions) == 0 {
		t.Fatalf("next questions empty, want user-facing missing context questions")
	}
}

func TestSearchOpsManualsAdaptWhenOnlyOSDiffers(t *testing.T) {
	repo := NewMemoryStore()
	mustSaveManual(t, repo, pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu"))

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text: "在 CentOS 主机 pg-centos-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionAdapt {
		t.Fatalf("decision = %q, want adapt; result=%#v", result.Decision, result)
	}
	if !hasAny(result.Manuals[0].EnvironmentDiffs, "os") {
		t.Fatalf("environment diffs = %#v, want os", result.Manuals[0].EnvironmentDiffs)
	}
	if result.Manuals[0].RecommendedAction != "generate_workflow_variant" {
		t.Fatalf("recommended action = %q, want generate_workflow_variant", result.Manuals[0].RecommendedAction)
	}
}

func TestSearchOpsManualsDoesNotDirectExecutePostgresManualForMySQLBackup(t *testing.T) {
	repo := NewMemoryStore()
	mustSaveManual(t, repo, pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu"))

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text: "在 Ubuntu 主机 mysql-01 上通过 ssh 对 MySQL 做备份，备份到 /data/backups，已确认 ssh_access 正常",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision == DecisionDirectExecute || result.Decision == DecisionAdapt {
		t.Fatalf("decision = %q, want reference_only or no_match for cross middleware", result.Decision)
	}
}

func TestSearchOpsManualsReferenceOnlyWhenManualHasNoWorkflow(t *testing.T) {
	repo := NewMemoryStore()
	manual := pgBackupManual("manual-pg-backup-doc-only", "ubuntu", "ssh", "")
	manual.WorkflowRef = WorkflowRef{}
	mustSaveManual(t, repo, manual)

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text: "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionReference {
		t.Fatalf("decision = %q, want reference_only; result=%#v", result.Decision, result)
	}
}

func TestSearchOpsManualsNeedInfoForSameObjectSameOperationBeforeReferenceOnly(t *testing.T) {
	repo := NewMemoryStore()
	mustSaveManual(t, repo, redisRcaManual())
	mustSaveManual(t, repo, pgRcaReferenceManual())

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{Text: "排查 Redis"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionNeedInfo {
		t.Fatalf("decision = %q, want need_info over cross-object reference; result=%#v", result.Decision, result)
	}
	if len(result.Manuals) == 0 || result.Manuals[0].Manual.ID != "manual-redis-rca-ssh" {
		t.Fatalf("manual order = %#v, want Redis same-object manual first", result.Manuals)
	}
}

func TestSearchOpsManualsRiskAboveManualBoundaryIsReferenceOnly(t *testing.T) {
	repo := NewMemoryStore()
	manual := pgBackupManual("manual-pg-backup-low-risk", "ubuntu", "ssh", "workflow-pg-backup-low-risk")
	manual.Operation.RiskLevel = "low"
	mustSaveManual(t, repo, manual)

	frame := BuildOperationFrame("在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 恢复，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常", nil)
	frame.Operation.Action = "backup"
	frame.OperationType = "backup"
	frame.Intent = "backup"
	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{OperationFrame: frame})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionReference {
		t.Fatalf("decision = %q, want reference_only when requested risk exceeds manual boundary; result=%#v", result.Decision, result)
	}
	if !hasAny(result.Manuals[0].BlockedReasons, "requested risk level exceeds manual risk boundary") {
		t.Fatalf("blocked reasons = %#v, want risk boundary reason", result.Manuals[0].BlockedReasons)
	}
}

func TestSearchOpsManualsNoCandidateButMissingExecutionSurfaceAndRiskNeedsInfo(t *testing.T) {
	repo := NewMemoryStore()

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		OperationFrame: OperationFrame{
			Target:        OperationTarget{Type: "postgresql", Name: "pg-unknown"},
			ObjectType:    "postgresql",
			Operation:     OperationProfile{TargetType: "postgresql", Action: "backup"},
			OperationType: "backup",
			RawText:       "PostgreSQL 备份",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionNeedInfo {
		t.Fatalf("decision = %q, want need_info for incomplete operation frame with no candidates; result=%#v", result.Decision, result)
	}
	for _, want := range []string{"execution_surface", "risk_level"} {
		if !hasAny(result.NextQuestions, want) && !containsQuestionForField(result.NextQuestions, want) {
			t.Fatalf("next questions = %#v, want question for %s", result.NextQuestions, want)
		}
	}
}

func redisMemoryManual() OpsManual {
	return OpsManual{
		ID:          "manual-redis-memory",
		Title:       "Redis memory pressure triage",
		Status:      ManualStatusVerified,
		WorkflowRef: WorkflowRef{WorkflowID: "workflow-redis-memory"},
		Operation:   OperationProfile{TargetType: "redis", Action: "rca_or_repair", Stateful: true},
		Applicability: ApplicabilityProfile{
			Middleware:       "redis",
			ExecutionSurface: []string{"ssh"},
		},
		RequiredContext: RequiredContext{
			RequiredInputs:   []string{"target_instance"},
			RequiredEvidence: []string{"used_memory_rss", "p95"},
		},
		Preconditions:    []string{"can connect to Redis"},
		Validation:       []string{"memory pressure recovered"},
		CannotUseWhen:    []string{"目标实例未知"},
		DocumentMarkdown: "Run Redis memory checks.",
	}
}

func postgresBackupUbuntuManual() OpsManual {
	return OpsManual{
		ID:          "manual-pg-backup-ubuntu",
		Title:       "PostgreSQL backup Ubuntu",
		Status:      ManualStatusVerified,
		WorkflowRef: WorkflowRef{WorkflowID: "workflow-pg-backup-ubuntu"},
		Operation:   OperationProfile{TargetType: "postgresql", Action: "backup", Stateful: true},
		Applicability: ApplicabilityProfile{
			Middleware:       "postgresql",
			OS:               []string{"ubuntu"},
			ExecutionSurface: []string{"ssh"},
		},
		RequiredContext:  RequiredContext{RequiredInputs: []string{"target_instance", "backup_path"}},
		Preconditions:    []string{"ssh access"},
		Validation:       []string{"backup file exists"},
		CannotUseWhen:    []string{},
		DocumentMarkdown: "Back up PostgreSQL on Ubuntu.",
	}
}

func pgBackupManual(id, osName, executionSurface, workflowID string) OpsManual {
	return OpsManual{
		ID:          id,
		Title:       "PostgreSQL 备份 Ubuntu 运维手册",
		Status:      ManualStatusVerified,
		WorkflowRef: WorkflowRef{WorkflowID: workflowID},
		Operation:   OperationProfile{TargetType: "postgresql", Action: "backup", Stateful: true},
		Applicability: ApplicabilityProfile{
			Middleware:       "postgresql",
			OS:               []string{osName},
			ExecutionSurface: []string{executionSurface},
		},
		RequiredContext: RequiredContext{
			RequiredInputs:   []string{"target_instance", "backup_path"},
			RequiredEvidence: []string{"ssh_access", "pg_isready"},
		},
		Preconditions:    []string{"ssh access"},
		Validation:       []string{"pg_isready", "backup file exists"},
		CannotUseWhen:    []string{"目标实例未知"},
		DocumentMarkdown: "Back up PostgreSQL on Ubuntu.",
	}
}

func redisRcaManual() OpsManual {
	return OpsManual{
		ID:          "manual-redis-rca-ssh",
		Title:       "Redis SSH 排障运维手册",
		Status:      ManualStatusVerified,
		WorkflowRef: WorkflowRef{WorkflowID: "workflow-redis-rca-ssh"},
		Operation:   OperationProfile{TargetType: "redis", Action: "rca_or_repair", Stateful: true},
		Applicability: ApplicabilityProfile{
			Middleware:       "redis",
			ExecutionSurface: []string{"ssh"},
		},
		RequiredContext: RequiredContext{
			RequiredInputs:   []string{"target_instance"},
			RequiredEvidence: []string{"symptom", "metrics"},
		},
		Preconditions:    []string{"can connect to Redis"},
		Validation:       []string{"Redis metrics recovered"},
		CannotUseWhen:    []string{"目标实例未知"},
		DocumentMarkdown: "Redis RCA manual.",
	}
}

func pgRcaReferenceManual() OpsManual {
	return OpsManual{
		ID:          "manual-pg-rca-ssh",
		Title:       "PostgreSQL SSH 排障运维手册",
		Status:      ManualStatusVerified,
		WorkflowRef: WorkflowRef{WorkflowID: "workflow-pg-rca-ssh"},
		Operation:   OperationProfile{TargetType: "postgresql", Action: "rca_or_repair", Stateful: true},
		Applicability: ApplicabilityProfile{
			Middleware:       "postgresql",
			ExecutionSurface: []string{"ssh"},
		},
		RequiredContext: RequiredContext{
			RequiredInputs:   []string{"target_instance"},
			RequiredEvidence: []string{"symptom", "metrics"},
		},
		Preconditions:    []string{"can connect to PostgreSQL"},
		Validation:       []string{"PostgreSQL metrics recovered"},
		CannotUseWhen:    []string{"目标实例未知"},
		DocumentMarkdown: "PostgreSQL RCA manual.",
	}
}

func containsQuestionForField(questions []string, field string) bool {
	for _, question := range questions {
		switch field {
		case "execution_surface":
			if stringsContains(question, "执行方式") {
				return true
			}
		case "risk_level":
			if stringsContains(question, "风险等级") {
				return true
			}
		}
	}
	return false
}

func mustSaveManual(t *testing.T, repo *MemoryStore, manual OpsManual) {
	t.Helper()
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
}

func mustSaveRunRecord(t *testing.T, repo *MemoryStore, record RunRecord) {
	t.Helper()
	if err := repo.SaveRunRecord(record); err != nil {
		t.Fatal(err)
	}
}

func stringsContains(value, substr string) bool {
	return len(substr) == 0 || (len(value) >= len(substr) && containsBytes(value, substr))
}

func containsBytes(value, substr string) bool {
	for i := 0; i+len(substr) <= len(value); i++ {
		if value[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
