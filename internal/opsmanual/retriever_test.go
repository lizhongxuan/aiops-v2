package opsmanual

import (
	"context"
	"strings"
	"testing"
	"time"
)

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

func TestRetrieveManualsPreservesDiagnosisProfileAsProcedureAsset(t *testing.T) {
	repo := NewMemoryStore()
	manual := redisRcaManual()
	manual.Diagnosis = DiagnosisProfile{
		ApplicableSymptoms:       []string{"connection_timeout"},
		AllowedEvidenceSources:   []string{"redis-cli", "metrics"},
		RecommendedEvidenceOrder: []string{"scope", "ping", "info"},
		KeyJudgmentRules:         []string{"command_not_allowed is missing evidence, not target state"},
		CommonMisdiagnoses:       []string{"using stale docker port after host switch"},
		ConfidenceCriteria:       []string{"high requires direct support and checked refuting evidence"},
		ConservativeWording:      []string{"cannot confirm root cause without listener evidence"},
		MinimumRiskNextSteps:     []string{"collect read-only INFO"},
	}
	mustSaveManual(t, repo, manual)

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{Text: "排查 Redis 连接失败"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Manuals) != 1 {
		t.Fatalf("manuals = %#v, want one", result.Manuals)
	}
	got := result.Manuals[0].Manual.Diagnosis
	if !contains(got.KeyJudgmentRules, "command_not_allowed is missing evidence, not target state") ||
		!contains(got.CommonMisdiagnoses, "using stale docker port after host switch") {
		t.Fatalf("diagnosis profile = %#v", got)
	}
}

func TestRetrieveManualsFullRedisRequestDirect(t *testing.T) {
	repo := NewMemoryStore()
	if err := repo.SaveManual(redisMemoryManual()); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	mustSaveRunRecord(t, repo, RunRecord{
		ID: "rr-redis-success", ManualID: "manual-redis-memory", WorkflowID: "workflow-redis-memory",
		ExecutionStatus: "passed", ValidationStatus: "passed", CompletedAt: "2026-05-15T01:00:00Z",
	})
	frame := BuildOperationFrame("生产 payment-api 的 Redis used_memory_rss 持续上涨，Coroot 显示 p95 升高，请通过 ssh 排查", map[string]any{"target_name": "redis-local-01"})
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
	for _, forbidden := range []string{"start_dry_run"} {
		if contains(matches[0].RecommendedNextActions, forbidden) {
			t.Fatalf("actions = %#v, must not contain %s", matches[0].RecommendedNextActions, forbidden)
		}
	}
	for _, want := range []string{"fill_parameters", "run_preflight_probe", "confirm_execution"} {
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
		OperationFrame: BuildOperationFrame("在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常", map[string]any{"target_name": "pg-ubuntu-01"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionDirectExecute {
		t.Fatalf("decision = %q, want direct_execute; result=%#v", result.Decision, result)
	}
	if len(result.Manuals) != 1 || result.Manuals[0].RecommendedAction != "run_preflight_probe" {
		t.Fatalf("manuals = %#v, want run_preflight_probe", result.Manuals)
	}
	if result.Manuals[0].PreflightStatus != PreflightStatusNotRun {
		t.Fatalf("preflight status = %q, want not_run", result.Manuals[0].PreflightStatus)
	}
	if result.RecommendedNextAction != "运行 Node 0 预检，通过后确认或审批执行。" {
		t.Fatalf("recommended next action = %q, want preflight guidance", result.RecommendedNextAction)
	}
}

func TestSearchOpsManualsLatestRunRecordFailureSuppressesDirectExecution(t *testing.T) {
	repo := NewMemoryStore()
	mustSaveManual(t, repo, pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu"))
	mustSaveRunRecord(t, repo, RunRecord{
		ID: "rr-success", ManualID: "manual-pg-backup-ubuntu", WorkflowID: "workflow-pg-backup-ubuntu",
		ExecutionStatus: "passed", ValidationStatus: "passed", CompletedAt: "2026-05-15T01:00:00Z",
	})
	mustSaveRunRecord(t, repo, RunRecord{
		ID: "rr-failed", ManualID: "manual-pg-backup-ubuntu", WorkflowID: "workflow-pg-backup-ubuntu",
		ExecutionStatus: "failed", ValidationStatus: "failed", CompletedAt: "2026-05-15T02:00:00Z",
	})

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text:     "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
		Metadata: map[string]any{"target_name": "pg-ubuntu-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionReference {
		t.Fatalf("decision = %q, want reference_only after latest failed run; result=%#v", result.Decision, result)
	}
	if len(result.Manuals) == 0 || !result.Manuals[0].RunRecordSummary.Suppressed {
		t.Fatalf("run summary = %#v, want suppressed", result.Manuals)
	}
	if result.Manuals[0].RecommendedAction != "reference_manual" {
		t.Fatalf("recommended action = %q, want reference_manual", result.Manuals[0].RecommendedAction)
	}
}

func TestSearchOpsManualsRecentRecoveryRestoresDirectExecution(t *testing.T) {
	repo := NewMemoryStore()
	mustSaveManual(t, repo, pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu"))
	mustSaveRunRecord(t, repo, RunRecord{
		ID: "rr-failed", ManualID: "manual-pg-backup-ubuntu", WorkflowID: "workflow-pg-backup-ubuntu",
		ExecutionStatus: "failed", ValidationStatus: "failed", CompletedAt: "2026-05-15T01:00:00Z",
	})
	mustSaveRunRecord(t, repo, RunRecord{
		ID: "rr-recovered", ManualID: "manual-pg-backup-ubuntu", WorkflowID: "workflow-pg-backup-ubuntu",
		ExecutionStatus: "passed", ValidationStatus: "passed", CompletedAt: "2026-05-15T02:00:00Z",
	})

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text:     "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
		Metadata: map[string]any{"target_name": "pg-ubuntu-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionDirectExecute {
		t.Fatalf("decision = %q, want direct_execute after latest passed recovery; result=%#v", result.Decision, result)
	}
	if result.Manuals[0].RunRecordSummary.Suppressed || result.Manuals[0].RunRecordSummary.LatestStatus != "passed" {
		t.Fatalf("run summary = %#v, want latest passed and unsuppressed", result.Manuals[0].RunRecordSummary)
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
	if len(result.NextQuestions) > 4 {
		t.Fatalf("next questions = %#v, want compact form fields", result.NextQuestions)
	}
	if containsAnyQuestionText(result.NextQuestions, "Coroot", "监控指标") {
		t.Fatalf("next questions = %#v, should not ask the user whether Coroot or monitoring evidence exists", result.NextQuestions)
	}
	if containsQuestionForField(result.NextQuestions, "risk_level") {
		t.Fatalf("next questions = %#v, should not ask risk in the first short RCA prompt", result.NextQuestions)
	}
	for _, want := range []string{"目标实例是哪一个？", "这是生产、测试还是其他环境？", "执行方式是 SSH、kubectl、docker exec 还是其他方式？", "当前现象是什么？"} {
		if !hasAny(result.NextQuestions, want) {
			t.Fatalf("next questions = %#v, want compact form question %q", result.NextQuestions, want)
		}
	}
}

func TestSearchOpsManualsStatusCheckCanReuseRedisRCAManual(t *testing.T) {
	repo := NewMemoryStore()
	mustSaveManual(t, repo, redisRcaManual())

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text:     "检查 Redis 状态。当前主机就是 server-local，请优先使用运维手册、真实只读发现和预检，不要执行变更。",
		Metadata: map[string]any{"selected_host": "server-local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OperationFrame.Intent != "status_check" || result.OperationFrame.Target.Name != "" {
		t.Fatalf("operation frame = %#v, want status_check without fake target name", result.OperationFrame)
	}
	if result.Decision != DecisionNeedInfo {
		t.Fatalf("decision = %q, want need_info for param resolution; result=%#v", result.Decision, result)
	}
	if len(result.Manuals) != 1 || result.Manuals[0].Manual.ID != "manual-redis-rca-ssh" {
		t.Fatalf("manuals = %#v, want redis RCA manual", result.Manuals)
	}
	if !contains(result.Manuals[0].MatchedFields, "operation_type") {
		t.Fatalf("matched fields = %#v, want compatible operation_type", result.Manuals[0].MatchedFields)
	}
	if hasAny(result.Manuals[0].MissingFields, "symptom") || hasAny(result.Manuals[0].MissingFields, "metrics") {
		t.Fatalf("missing fields = %#v, status check should not require RCA symptom/metrics", result.Manuals[0].MissingFields)
	}
	if !hasAny(result.Manuals[0].MissingFields, "target_instance") {
		t.Fatalf("missing fields = %#v, want target_instance for resolver", result.Manuals[0].MissingFields)
	}
}

func TestSearchOpsManualsAdaptWhenOnlyOSDiffers(t *testing.T) {
	repo := NewMemoryStore()
	mustSaveManual(t, repo, pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu"))

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text:     "在 CentOS 主机 pg-centos-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
		Metadata: map[string]any{"target_name": "pg-centos-01"},
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

func TestSearchOpsManualsDoesNotExposeCrossObjectManualForMySQLBackup(t *testing.T) {
	repo := NewMemoryStore()
	mustSaveManual(t, repo, pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu"))

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text:     "在 Ubuntu 主机 mysql-01 上通过 ssh 对 MySQL 做备份，备份到 /data/backups，已确认 ssh_access 正常",
		Metadata: map[string]any{"target_name": "mysql-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionNoMatch || len(result.Manuals) != 0 {
		t.Fatalf("result = %#v, want no_match without exposing cross-object manuals", result)
	}
}

func TestSearchOpsManualsDoesNotExposeK8sManualForKafkaLag(t *testing.T) {
	repo := NewMemoryStore()
	manual := OpsManual{
		ID:      "manual-k8s-pod-crashloop-rca",
		Title:   "Kubernetes Pod CrashLoop/OOM 排障运维手册",
		Status:  ManualStatusVerified,
		Version: "v1",
		WorkflowRef: WorkflowRef{
			WorkflowID: "workflow-k8s-pod-crashloop-rca",
		},
		Operation: OperationProfile{
			TargetType: "kubernetes_pod",
			Action:     "rca_or_repair",
			RiskLevel:  "medium",
		},
		Applicability: ApplicabilityProfile{
			Middleware: "kubernetes",
		},
		SearchDoc:        "Kubernetes pod CrashLoopBackOff OOMKilled restart logs events",
		DocumentMarkdown: "用于 Kubernetes Pod CrashLoop/OOM 排障。",
	}
	mustSaveManual(t, repo, manual)

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text: "Kafka consumer group checkout-prod lag 持续升高，需要排查 broker 和 partition rebalance，先只读分析。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionNoMatch || len(result.Manuals) != 0 {
		t.Fatalf("result = %#v, want no_match without exposing Kubernetes manual for Kafka", result)
	}
	if !strings.Contains(result.Summary, "Kafka") {
		t.Fatalf("summary = %q, want Kafka-specific no-match summary", result.Summary)
	}
	if !strings.Contains(result.RecommendedNextAction, "AI 会继续自动尝试只读排查") {
		t.Fatalf("recommended action = %q, want AI auto read-only investigation", result.RecommendedNextAction)
	}
}

func TestSearchOpsManualsIgnoresUnverifiedManuals(t *testing.T) {
	repo := NewMemoryStore()
	manual := pgBackupManual("manual-pg-backup-draft", "ubuntu", "ssh", "workflow-pg-backup-draft")
	manual.Status = ManualStatusDraft
	mustSaveManual(t, repo, manual)

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text:     "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
		Metadata: map[string]any{"target_name": "pg-ubuntu-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Manuals) != 0 || result.Decision == DecisionDirectExecute {
		t.Fatalf("result = %#v, want no verified executable candidates", result)
	}
}

func TestSearchOpsManualsDisabledWorkflowIsReferenceOnly(t *testing.T) {
	repo := NewMemoryStore()
	manual := pgBackupManual("manual-pg-backup-disabled", "ubuntu", "ssh", "workflow-pg-backup-disabled")
	manual.Metadata = map[string]any{"workflow_status": "disabled"}
	mustSaveManual(t, repo, manual)

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text:     "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
		Metadata: map[string]any{"target_name": "pg-ubuntu-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionReference {
		t.Fatalf("decision = %q, want reference_only; result=%#v", result.Decision, result)
	}
	if !hasAny(result.Manuals[0].BlockedReasons, "bound workflow is not enabled") {
		t.Fatalf("blocked reasons = %#v, want workflow disabled", result.Manuals[0].BlockedReasons)
	}
}

func TestSearchOpsManualsNoRestartDoesNotDirectRestartWorkflow(t *testing.T) {
	repo := NewMemoryStore()
	manual := redisRcaManual()
	manual.ID = "manual-redis-restart"
	manual.Title = "Redis restart workflow"
	manual.Operation.Action = "restart"
	manual.WorkflowRef.WorkflowID = "workflow-redis-restart"
	mustSaveManual(t, repo, manual)

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{Text: "只读排查 Redis redis-01，不重启服务，只看 metrics"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision == DecisionDirectExecute || result.Decision == DecisionAdapt {
		t.Fatalf("decision = %q, want no executable restart path; result=%#v", result.Decision, result)
	}
}

func TestSearchOpsManualsReferenceOnlyWhenManualHasNoWorkflow(t *testing.T) {
	repo := NewMemoryStore()
	manual := pgBackupManual("manual-pg-backup-doc-only", "ubuntu", "ssh", "")
	manual.WorkflowRef = WorkflowRef{}
	mustSaveManual(t, repo, manual)

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text:     "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
		Metadata: map[string]any{"target_name": "pg-ubuntu-01"},
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

	frame := BuildOperationFrame("在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 恢复，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常", map[string]any{"target_name": "pg-ubuntu-01"})
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

func TestSearchOpsManualsProvidesGenericStatefulClusterRepairFallback(t *testing.T) {
	store := NewMemoryStore()
	result, err := SearchOpsManuals(store, SearchOpsManualsRequest{
		Text: "主机A和主机B的Redis主从集群异常，请帮忙恢复，只需要Redis集群正常运行，sentinel部署在主机C。",
	})
	if err != nil {
		t.Fatalf("SearchOpsManuals() error = %v", err)
	}
	if result.Decision != DecisionReference || len(result.Manuals) != 1 {
		t.Fatalf("result = %#v, want one reference-only generic fallback manual", result)
	}
	hit := result.Manuals[0]
	if hit.Manual.ID != "manual-generic-stateful-cluster-repair" {
		t.Fatalf("manual id = %q, want generic stateful fallback", hit.Manual.ID)
	}
	if hit.UsableMode != DecisionReference || hit.RecommendedAction != "reference_manual" {
		t.Fatalf("generic fallback mode/action = %q/%q, want reference_only/reference_manual", hit.UsableMode, hit.RecommendedAction)
	}
	if hit.Manual.Operation.TargetType != "redis" || !hit.Manual.Operation.Stateful {
		t.Fatalf("manual operation = %#v, want request target type and stateful", hit.Manual.Operation)
	}
	if !hit.Manual.PreflightProbe.ReadOnly || len(hit.Manual.PreflightProbe.RequiredOutputs) == 0 {
		t.Fatalf("preflight probe = %#v, want read-only generic outputs", hit.Manual.PreflightProbe)
	}
}

func TestBuildOperationFrameKeepsRecoveryIntentWhenTopologyMentionsDeployment(t *testing.T) {
	frame := BuildOperationFrame("主机A和主机B的Redis主从集群异常，请帮忙恢复，只需要Redis集群正常运行，sentinel部署在主机C。", nil)
	if frame.Operation.Action != "rca_or_repair" {
		t.Fatalf("operation action = %q, want rca_or_repair", frame.Operation.Action)
	}
	if len(frame.ObservationPoints) != 1 || frame.ObservationPoints[0].Role != "sentinel" {
		t.Fatalf("observation points = %#v, want sentinel monitor deployment preserved", frame.ObservationPoints)
	}
}

func TestSearchOpsManualsNoMatchAccuracyForUnrelatedRequests(t *testing.T) {
	repo := NewMemoryStore()
	mustSaveManual(t, repo, pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu"))
	for _, text := range []string{"帮我看一下网络慢", "安装一个工具", "写一个 SQL 查询"} {
		result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{Text: text})
		if err != nil {
			t.Fatalf("SearchOpsManuals(%q) error = %v", text, err)
		}
		if result.Decision == DecisionDirectExecute || result.Decision == DecisionAdapt {
			t.Fatalf("SearchOpsManuals(%q) decision = %q, want no executable match; result=%#v", text, result.Decision, result)
		}
	}
}

func TestSearchOpsManualsDoesNotRecommendRedisManualForPGTimelineQuestion(t *testing.T) {
	repo := NewMemoryStore()
	mustSaveManual(t, repo, redisRcaManual())

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text: "pgbackrest 恢复主机A后，从节点执行 pg_autoctl create postgres 加入集群，timeline 比主机A高导致无法同步，这是为什么？",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision == DecisionDirectExecute || result.Decision == DecisionAdapt {
		t.Fatalf("decision = %q, want no executable Redis manual for PG timeline; result=%#v", result.Decision, result)
	}
	for _, hit := range result.Manuals {
		if hit.Manual.Operation.TargetType == "redis" || hit.Manual.Applicability.Middleware == "redis" {
			t.Fatalf("manuals = %#v, should not expose Redis manual for PG timeline", result.Manuals)
		}
	}
}

func TestOpsManualSuppressionFiltersSameManualScope(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := NewMemoryStore()
	store := NewMemorySessionOpsContextStore()
	service := NewService(repo, WithSessionOpsContextStore(store))
	mustSaveManual(t, repo, pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu"))

	if err := store.UpsertFact(ctx, "sess-suppressed", NewOpsManualSuppressionFact(OpsManualSuppression{
		ManualID:    "manual-pg-backup-ubuntu",
		ObjectType:  "postgresql",
		Action:      "backup",
		TargetScope: "host:pg-ubuntu-01",
		Reason:      "user_opt_out",
	}, now)); err != nil {
		t.Fatalf("UpsertFact() error = %v", err)
	}

	result, err := service.SearchOpsManuals(SearchOpsManualsRequest{
		Text: "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
		Metadata: map[string]any{
			"session_id":  "sess-suppressed",
			"target_name": "pg-ubuntu-01",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionNoMatch {
		t.Fatalf("decision = %q, want no_match after suppression; result=%#v", result.Decision, result)
	}
	if len(result.Manuals) != 0 {
		t.Fatalf("manuals = %#v, want suppressed manual removed", result.Manuals)
	}
	if !hasAny(result.SuppressedManuals, "manual-pg-backup-ubuntu") {
		t.Fatalf("suppressed manuals = %#v, want manual-pg-backup-ubuntu", result.SuppressedManuals)
	}
	if result.SuppressionReason != "user_opt_out" {
		t.Fatalf("suppression reason = %q, want user_opt_out", result.SuppressionReason)
	}
}

func TestOpsManualSuppressionDoesNotFilterDifferentScopeOrAction(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	for _, tt := range []struct {
		name        string
		suppression OpsManualSuppression
	}{
		{
			name: "different scope",
			suppression: OpsManualSuppression{
				ManualID:    "manual-pg-backup-ubuntu",
				ObjectType:  "postgresql",
				Action:      "backup",
				TargetScope: "host:pg-ubuntu-02",
				Reason:      "user_opt_out",
			},
		},
		{
			name: "different action",
			suppression: OpsManualSuppression{
				ManualID:    "manual-pg-backup-ubuntu",
				ObjectType:  "postgresql",
				Action:      "restore",
				TargetScope: "host:pg-ubuntu-01",
				Reason:      "user_opt_out",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMemoryStore()
			store := NewMemorySessionOpsContextStore()
			service := NewService(repo, WithSessionOpsContextStore(store))
			mustSaveManual(t, repo, pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu"))
			mustSaveRunRecord(t, repo, RunRecord{
				ID: "rr-success", ManualID: "manual-pg-backup-ubuntu", WorkflowID: "workflow-pg-backup-ubuntu",
				ExecutionStatus: "passed", ValidationStatus: "passed", CompletedAt: "2026-05-15T01:00:00Z",
			})
			if err := store.UpsertFact(ctx, "sess-suppressed", NewOpsManualSuppressionFact(tt.suppression, now)); err != nil {
				t.Fatalf("UpsertFact() error = %v", err)
			}
			result, err := service.SearchOpsManuals(SearchOpsManualsRequest{
				Text: "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
				Metadata: map[string]any{
					"session_id":  "sess-suppressed",
					"target_name": "pg-ubuntu-01",
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Decision != DecisionDirectExecute {
				t.Fatalf("decision = %q, want direct_execute; result=%#v", result.Decision, result)
			}
			if len(result.Manuals) != 1 || result.Manuals[0].Manual.ID != "manual-pg-backup-ubuntu" {
				t.Fatalf("manuals = %#v, want backup manual", result.Manuals)
			}
		})
	}
}

func TestOpsManualSuppressionCanBeOverriddenByExplicitUse(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := NewMemoryStore()
	store := NewMemorySessionOpsContextStore()
	service := NewService(repo, WithSessionOpsContextStore(store))
	mustSaveManual(t, repo, pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu"))
	mustSaveRunRecord(t, repo, RunRecord{
		ID: "rr-success", ManualID: "manual-pg-backup-ubuntu", WorkflowID: "workflow-pg-backup-ubuntu",
		ExecutionStatus: "passed", ValidationStatus: "passed", CompletedAt: "2026-05-15T01:00:00Z",
	})
	if err := store.UpsertFact(ctx, "sess-suppressed", NewOpsManualSuppressionFact(OpsManualSuppression{
		ManualID:    "manual-pg-backup-ubuntu",
		ObjectType:  "postgresql",
		Action:      "backup",
		TargetScope: "host:pg-ubuntu-01",
		Reason:      "user_opt_out",
	}, now)); err != nil {
		t.Fatalf("UpsertFact() error = %v", err)
	}

	result, err := service.SearchOpsManuals(SearchOpsManualsRequest{
		Text: "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
		Metadata: map[string]any{
			"session_id":      "sess-suppressed",
			"target_name":     "pg-ubuntu-01",
			"opsManualAction": "use_ops_manual",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionDirectExecute {
		t.Fatalf("decision = %q, want direct_execute when explicit use overrides suppression; result=%#v", result.Decision, result)
	}
	if len(result.Manuals) != 1 || result.Manuals[0].Manual.ID != "manual-pg-backup-ubuntu" {
		t.Fatalf("manuals = %#v, want backup manual", result.Manuals)
	}
}

func TestOpsManualSuppressionExpires(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := NewMemoryStore()
	store := NewMemorySessionOpsContextStore()
	service := NewService(repo, WithSessionOpsContextStore(store))
	mustSaveManual(t, repo, pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu"))
	mustSaveRunRecord(t, repo, RunRecord{
		ID: "rr-success", ManualID: "manual-pg-backup-ubuntu", WorkflowID: "workflow-pg-backup-ubuntu",
		ExecutionStatus: "passed", ValidationStatus: "passed", CompletedAt: "2026-05-15T01:00:00Z",
	})
	if err := store.UpsertFact(ctx, "sess-suppressed", NewOpsManualSuppressionFact(OpsManualSuppression{
		ManualID:    "manual-pg-backup-ubuntu",
		ObjectType:  "postgresql",
		Action:      "backup",
		TargetScope: "host:pg-ubuntu-01",
		Reason:      "user_opt_out",
	}, now.Add(-2*time.Hour))); err != nil {
		t.Fatalf("UpsertFact() error = %v", err)
	}

	result, err := service.SearchOpsManuals(SearchOpsManualsRequest{
		Text: "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
		Metadata: map[string]any{
			"session_id":  "sess-suppressed",
			"target_name": "pg-ubuntu-01",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionDirectExecute {
		t.Fatalf("decision = %q, want direct_execute after suppression expiry; result=%#v", result.Decision, result)
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
		case "environment":
			if stringsContains(question, "生产") || stringsContains(question, "测试") || stringsContains(question, "环境") {
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

func containsAnyQuestionText(questions []string, fragments ...string) bool {
	for _, question := range questions {
		for _, fragment := range fragments {
			if stringsContains(question, fragment) {
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
