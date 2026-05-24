package opsmanual

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkflowManualTypesCompile(t *testing.T) {
	req := WorkflowManualGenerationRequest{
		WorkflowID:      "workflow-pg-restore",
		WorkflowVersion: "v0.1",
		WorkflowDigest:  "sha256:abc",
		StorageURI:      "file://pg-restore.yaml",
		RawYAML:         []byte("version: v0.1\nname: pg-restore\n"),
		ActionSpecs: []ActionSpecSummary{{
			Action:       "script.shell",
			Title:        "Shell Script",
			Category:     "script",
			Risk:         "high",
			RequiredArgs: []string{"script"},
			Outputs:      []string{"stdout"},
		}},
		RecentRuns: []RunRecord{{ID: "run-1", WorkflowID: "workflow-pg-restore"}},
		Options: WorkflowManualGenerationOptions{
			IncludeRecentRunRecords: true,
			UseLLMSummary:           true,
			GeneratedBy:             "test",
		},
	}
	if req.WorkflowID == "" || len(req.ActionSpecs) != 1 {
		t.Fatalf("request = %#v, want workflow id and action spec", req)
	}

	validation := ManualCandidateValidation{
		Status:   "blocked",
		Passed:   []ValidationIssue{{Code: "workflow_ref_present", Field: "workflow_ref.workflow_id", Message: "workflow id exists"}},
		Warnings: []ValidationIssue{{Code: "missing_run_record", Field: "run_records", Message: "no recent successful run"}},
		Blocking: []ValidationIssue{{Code: "missing_action", Field: "operation.action", Message: "operation action is required"}},
	}
	summary := ManualGenerationUserSummary{
		Understood: []string{"这是 PostgreSQL 恢复 Workflow。"},
		Missing:    []string{"缺少操作动作。"},
		NextSteps:  []string{"补齐 x_ops_manual.action。"},
	}
	candidate := ManualCandidate{
		ID:                         "candidate-workflow-pg-restore",
		SourceType:                 "workflow_reverse_generated",
		StructuredValidationReport: validation,
		UserSummary:                summary,
		ReviewStatus:               "needs_fix",
	}
	if candidate.StructuredValidationReport.Blocking[0].Field != "operation.action" {
		t.Fatalf("candidate validation = %#v, want structured blocking issue", candidate.StructuredValidationReport)
	}
	if candidate.UserSummary.NextSteps[0] == "" {
		t.Fatalf("candidate user summary = %#v, want next step", candidate.UserSummary)
	}
}

func TestWorkflowManualAnalyzerReadsVarsStepsAndGraphStages(t *testing.T) {
	raw := loadWorkflowReverseFixture(t, "redis_memory_dry_run.yaml")
	analysis, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID: "redis-memory-dry-run",
		RawYAML:    raw,
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual() error = %v", err)
	}
	if analysis.Name == "" || analysis.WorkflowVersion == "" || analysis.WorkflowDigest == "" {
		t.Fatalf("analysis identity = %#v, want name/version/digest", analysis)
	}
	for _, stage := range []string{"precheck", "approval", "dry_run", "execute", "validate", "rollback"} {
		if !analysisHasGraphStage(analysis, stage) {
			t.Fatalf("GraphStages = %#v, missing stage %q", analysis.GraphStages, stage)
		}
	}
	if len(analysis.Steps) == 0 {
		t.Fatalf("Steps = %#v, want workflow steps", analysis.Steps)
	}
}

func TestWorkflowManualAnalyzerInfersOperationFromRealNames(t *testing.T) {
	cases := []struct {
		fixture    string
		wantTarget string
		wantAction string
	}{
		{"pg_restore.yaml", "postgresql", "restore"},
		{"self_healing_kubelet.yaml", "kubelet", "repair"},
		{"builtin_probes.yaml", "network_service", "inspect"},
		{"http_chatops_ticket.yaml", "incident", "create_or_notify"},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			analysis, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
				WorkflowID: tc.fixture,
				RawYAML:    loadWorkflowReverseFixture(t, tc.fixture),
			})
			if err != nil {
				t.Fatalf("AnalyzeWorkflowForManual() error = %v", err)
			}
			if analysis.Operation.TargetType != tc.wantTarget || analysis.Operation.Action != tc.wantAction {
				t.Fatalf("Operation = %#v, want %s/%s", analysis.Operation, tc.wantTarget, tc.wantAction)
			}
			if len(analysis.Evidence["operation"]) == 0 {
				t.Fatalf("Evidence[operation] empty for %s", tc.fixture)
			}
		})
	}

	minimal, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID: "shell-run-minimal",
		RawYAML:    loadWorkflowReverseFixture(t, "shell_run_minimal.yaml"),
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual(shell_run_minimal) error = %v", err)
	}
	if minimal.Operation.TargetType != "" || minimal.Operation.Action != "" {
		t.Fatalf("minimal Operation = %#v, want no guessed operation", minimal.Operation)
	}
}

func TestWorkflowManualAnalyzerBuildsParameterRulesFromVarsAndActionSpecs(t *testing.T) {
	analysis, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID: "http-chatops-ticket",
		RawYAML:    loadWorkflowReverseFixture(t, "http_chatops_ticket.yaml"),
		ActionSpecs: []ActionSpecSummary{{
			Action:       "http.request",
			Risk:         "medium",
			RequiredArgs: []string{"url"},
			Outputs:      []string{"status_code"},
		}},
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual() error = %v", err)
	}
	if _, ok := analysis.ParameterRules["service"]; !ok {
		t.Fatalf("ParameterRules = %#v, want workflow var service", analysis.ParameterRules)
	}
	if rule := analysis.ParameterRules["url"]; !rule.Required || rule.Source != "action_spec:http.request" {
		t.Fatalf("ParameterRules[url] = %#v, want required action spec rule", rule)
	}
	if len(analysis.SecretFindings) == 0 {
		t.Fatalf("SecretFindings empty, want auth secret_ref findings")
	}
}

func TestWorkflowManualAnalyzerDetectsRiskPreflightValidationAndRollback(t *testing.T) {
	pg, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID: "pg-restore",
		RawYAML:    loadWorkflowReverseFixture(t, "pg_restore.yaml"),
		ActionSpecs: []ActionSpecSummary{{
			Action: "script.shell",
			Risk:   "high",
		}},
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual(pg_restore) error = %v", err)
	}
	if pg.Operation.RiskLevel != "high" || !analysisHasRisk(pg, "service_restart") || !analysisHasRisk(pg, "data_mutation") {
		t.Fatalf("pg risk = %#v / %#v, want high restart mutation", pg.Operation, pg.ActionRisks)
	}
	if len(pg.ValidationHints) == 0 {
		t.Fatalf("pg ValidationHints empty, want pg_isready/restore validation")
	}

	kubelet, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID: "self-healing-kubelet",
		RawYAML:    loadWorkflowReverseFixture(t, "self_healing_kubelet.yaml"),
		ActionSpecs: []ActionSpecSummary{{
			Action: "builtin.http_check",
			Risk:   "read_only",
		}, {
			Action: "script.shell",
			Risk:   "high",
		}},
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual(kubelet) error = %v", err)
	}
	if !analysisHasGraphStage(kubelet, "approval") || !analysisHasRisk(kubelet, "service_restart") {
		t.Fatalf("kubelet stages/risk = %#v / %#v, want approval and restart", kubelet.GraphStages, kubelet.ActionRisks)
	}

	probes, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID: "builtin-probes",
		RawYAML:    loadWorkflowReverseFixture(t, "builtin_probes.yaml"),
		ActionSpecs: []ActionSpecSummary{{
			Action: "builtin.dns_resolve",
			Risk:   "read_only",
		}, {
			Action: "builtin.tcp_ping",
			Risk:   "read_only",
		}, {
			Action: "builtin.http_check",
			Risk:   "read_only",
		}, {
			Action: "builtin.ssl_expiry_check",
			Risk:   "read_only",
		}},
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual(probes) error = %v", err)
	}
	if probes.Operation.RiskLevel != "read_only" || !analysisHasGraphStage(probes, "precheck") {
		t.Fatalf("probes risk/stages = %#v / %#v, want read_only precheck", probes.Operation, probes.GraphStages)
	}
}

func TestWorkflowManualBuilderCreatesDraftCandidateWithRetrievalFields(t *testing.T) {
	analysis, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID:      "pg-restore",
		WorkflowVersion: "v0.1",
		StorageURI:      "file://pg_restore.yaml",
		RawYAML:         loadWorkflowReverseFixture(t, "pg_restore.yaml"),
		ActionSpecs: []ActionSpecSummary{{
			Action: "script.shell",
			Risk:   "high",
		}},
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual() error = %v", err)
	}

	candidate, err := BuildWorkflowManualCandidate(analysis)
	if err != nil {
		t.Fatalf("BuildWorkflowManualCandidate() error = %v", err)
	}

	manual := candidate.ProposedManual
	if candidate.SourceType != "workflow_reverse_generated" {
		t.Fatalf("SourceType = %q, want workflow_reverse_generated", candidate.SourceType)
	}
	if candidate.ReviewStatus != "pending" && candidate.ReviewStatus != "needs_fix" {
		t.Fatalf("ReviewStatus = %q, want pending or needs_fix", candidate.ReviewStatus)
	}
	if manual.Status != ManualStatusDraft {
		t.Fatalf("Status = %q, want draft", manual.Status)
	}
	if manual.WorkflowRef.WorkflowID != "pg-restore" || manual.WorkflowRef.WorkflowVersion == "" || manual.WorkflowRef.WorkflowDigest == "" {
		t.Fatalf("WorkflowRef = %#v, want id/version/digest", manual.WorkflowRef)
	}
	if manual.WorkflowRef.StorageURI != "file://pg_restore.yaml" {
		t.Fatalf("StorageURI = %q, want request storage uri", manual.WorkflowRef.StorageURI)
	}
	if manual.Operation.TargetType != "postgresql" || manual.Operation.Action != "restore" {
		t.Fatalf("Operation = %#v, want postgresql/restore", manual.Operation)
	}
	if manual.SearchDoc == "" {
		t.Fatalf("SearchDoc empty")
	}
	for _, keyword := range []string{"postgresql", "restore", "pg_isready"} {
		if !stringSliceContains(manual.RetrievalProfile.Keywords, keyword) {
			t.Fatalf("Keywords = %#v, missing %q", manual.RetrievalProfile.Keywords, keyword)
		}
	}
	for _, negative := range []string{"mysql", "redis", "kubernetes"} {
		if !stringSliceContains(manual.RetrievalProfile.NegativeKeywords, negative) {
			t.Fatalf("NegativeKeywords = %#v, missing %q", manual.RetrievalProfile.NegativeKeywords, negative)
		}
	}
	if manual.Metadata["source_type"] != "workflow_reverse_generated" || manual.Metadata["workflow_manual_generation_version"] != "p0-2026-05-20" {
		t.Fatalf("Metadata = %#v, want generation metadata", manual.Metadata)
	}
}

func TestWorkflowManualBuilderCreatesPreflightRiskFallbackAndVerification(t *testing.T) {
	probes, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID: "builtin-probes",
		RawYAML:    loadWorkflowReverseFixture(t, "builtin_probes.yaml"),
		ActionSpecs: []ActionSpecSummary{{
			Action: "builtin.dns_resolve",
			Risk:   "read_only",
		}, {
			Action: "builtin.tcp_ping",
			Risk:   "read_only",
		}, {
			Action: "builtin.http_check",
			Risk:   "read_only",
		}, {
			Action: "builtin.ssl_expiry_check",
			Risk:   "read_only",
		}},
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual(probes) error = %v", err)
	}
	probeCandidate, err := BuildWorkflowManualCandidate(probes)
	if err != nil {
		t.Fatalf("BuildWorkflowManualCandidate(probes) error = %v", err)
	}
	if !probeCandidate.ProposedManual.PreflightProbe.ReadOnly || probeCandidate.ProposedManual.PreflightProbe.Action == "" {
		t.Fatalf("PreflightProbe = %#v, want read-only action", probeCandidate.ProposedManual.PreflightProbe)
	}

	kubelet, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID: "self-healing-kubelet",
		RawYAML:    loadWorkflowReverseFixture(t, "self_healing_kubelet.yaml"),
		ActionSpecs: []ActionSpecSummary{{
			Action: "builtin.http_check",
			Risk:   "read_only",
		}, {
			Action: "script.shell",
			Risk:   "high",
		}},
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual(kubelet) error = %v", err)
	}
	kubeletCandidate, err := BuildWorkflowManualCandidate(kubelet)
	if err != nil {
		t.Fatalf("BuildWorkflowManualCandidate(kubelet) error = %v", err)
	}
	if len(kubeletCandidate.ProposedManual.RiskPolicy.ApprovalRequiredWhen) == 0 || !kubeletCandidate.ProposedManual.RunnableConditions.RequiresApproval {
		t.Fatalf("risk/runnable = %#v / %#v, want approval policy", kubeletCandidate.ProposedManual.RiskPolicy, kubeletCandidate.ProposedManual.RunnableConditions)
	}

	redis, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID: "redis-memory-dry-run",
		RawYAML:    loadWorkflowReverseFixture(t, "redis_memory_dry_run.yaml"),
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual(redis) error = %v", err)
	}
	redisCandidate, err := BuildWorkflowManualCandidate(redis)
	if err != nil {
		t.Fatalf("BuildWorkflowManualCandidate(redis) error = %v", err)
	}
	if redisCandidate.ProposedManual.FallbackGuide.Mode == "" || len(redisCandidate.ProposedManual.FallbackGuide.Steps) == 0 {
		t.Fatalf("FallbackGuide = %#v, want rollback-derived fallback", redisCandidate.ProposedManual.FallbackGuide)
	}

	pg, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID: "pg-restore",
		RawYAML:    loadWorkflowReverseFixture(t, "pg_restore.yaml"),
		ActionSpecs: []ActionSpecSummary{{
			Action: "script.shell",
			Risk:   "high",
		}},
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual(pg) error = %v", err)
	}
	pgCandidate, err := BuildWorkflowManualCandidate(pg)
	if err != nil {
		t.Fatalf("BuildWorkflowManualCandidate(pg) error = %v", err)
	}
	if pgCandidate.ProposedManual.Verification.RequiredRunnerDryRun {
		t.Fatalf("new workflow reverse manuals must not require runtime dry-run: %#v", pgCandidate.ProposedManual.Verification)
	}
	if !pgCandidate.ProposedManual.Verification.RequiredPreflightPlan {
		t.Fatalf("Verification = %#v, want preflight plan check required for high-risk restore", pgCandidate.ProposedManual.Verification)
	}
}

func TestWorkflowManualBuilderMarkdownOnlyUsesStructuredFields(t *testing.T) {
	pg, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID: "pg-restore",
		RawYAML:    loadWorkflowReverseFixture(t, "pg_restore.yaml"),
		ActionSpecs: []ActionSpecSummary{{
			Action: "script.shell",
			Risk:   "high",
		}},
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual(pg) error = %v", err)
	}
	pgCandidate, err := BuildWorkflowManualCandidate(pg)
	if err != nil {
		t.Fatalf("BuildWorkflowManualCandidate(pg) error = %v", err)
	}
	markdown := pgCandidate.ProposedManual.DocumentMarkdown
	for _, heading := range []string{"# ", "## 适用范围", "## 所需上下文", "## 前置检查", "## 执行步骤", "## 验证方式", "## 风险与审批", "## 不能使用", "## 降级处理"} {
		if !strings.Contains(markdown, heading) {
			t.Fatalf("markdown missing heading %q:\n%s", heading, markdown)
		}
	}
	for _, rawScript := range []string{"BOPS_EXPORT", "systemctl stop", "pgbackrest --stanza"} {
		if strings.Contains(markdown, rawScript) {
			t.Fatalf("markdown leaks raw script marker %q:\n%s", rawScript, markdown)
		}
	}
	if !strings.Contains(markdown, "high") {
		t.Fatalf("markdown = %q, want risk level high", markdown)
	}

	httpAnalysis, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID: "http-chatops-ticket",
		RawYAML:    loadWorkflowReverseFixture(t, "http_chatops_ticket.yaml"),
		ActionSpecs: []ActionSpecSummary{{
			Action:       "http.request",
			Risk:         "medium",
			RequiredArgs: []string{"url"},
		}},
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual(http) error = %v", err)
	}
	httpCandidate, err := BuildWorkflowManualCandidate(httpAnalysis)
	if err != nil {
		t.Fatalf("BuildWorkflowManualCandidate(http) error = %v", err)
	}
	for _, secret := range []string{"itsm/api-token", "chatops/bot-token", "Authorization"} {
		if strings.Contains(httpCandidate.ProposedManual.DocumentMarkdown, secret) {
			t.Fatalf("markdown leaks secret marker %q:\n%s", secret, httpCandidate.ProposedManual.DocumentMarkdown)
		}
	}
}

func TestManualCandidateValidatorBlocksIncompleteManual(t *testing.T) {
	base := validWorkflowManualCandidateForValidation(t)
	cases := []struct {
		name   string
		mutate func(*ManualCandidate)
		field  string
	}{
		{
			name: "workflow id empty",
			mutate: func(candidate *ManualCandidate) {
				candidate.ProposedManual.WorkflowRef.WorkflowID = ""
			},
			field: "workflow_ref.workflow_id",
		},
		{
			name: "digest empty",
			mutate: func(candidate *ManualCandidate) {
				candidate.ProposedManual.WorkflowRef.WorkflowDigest = ""
			},
			field: "workflow_ref.workflow_digest",
		},
		{
			name: "target type empty",
			mutate: func(candidate *ManualCandidate) {
				candidate.ProposedManual.Operation.TargetType = ""
			},
			field: "operation.target_type",
		},
		{
			name: "action empty",
			mutate: func(candidate *ManualCandidate) {
				candidate.ProposedManual.Operation.Action = ""
			},
			field: "operation.action",
		},
		{
			name: "validation empty",
			mutate: func(candidate *ManualCandidate) {
				candidate.ProposedManual.Validation = nil
			},
			field: "validation",
		},
		{
			name: "cannot use empty",
			mutate: func(candidate *ManualCandidate) {
				candidate.ProposedManual.CannotUseWhen = nil
			},
			field: "cannot_use_when",
		},
		{
			name: "markdown empty",
			mutate: func(candidate *ManualCandidate) {
				candidate.ProposedManual.DocumentMarkdown = ""
			},
			field: "document_markdown",
		},
		{
			name: "sensitive default",
			mutate: func(candidate *ManualCandidate) {
				candidate.ProposedManual.ParameterRules["api_token"] = ParameterRule{Required: true, DefaultValue: "plain-token"}
			},
			field: "parameter_rules.api_token.default_value",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := cloneCandidate(base)
			tc.mutate(&candidate)
			report := ValidateManualCandidate(candidate, ManualCandidateValidationOptions{})
			if report.Status != "blocked" {
				t.Fatalf("Status = %q, want blocked; report=%#v", report.Status, report)
			}
			if !validationHasIssue(report.Blocking, tc.field) {
				t.Fatalf("Blocking = %#v, missing field %q", report.Blocking, tc.field)
			}
		})
	}
}

func TestManualCandidateValidatorWarnsButAllowsDraft(t *testing.T) {
	candidate := validWorkflowManualCandidateForValidation(t)
	candidate.ProposedManual.PreflightProbe = PreflightProbe{}
	candidate.ProposedManual.RetrievalProfile.NegativeKeywords = nil
	candidate.ProposedManual.Operation.RiskLevel = "high"
	candidate.ProposedManual.RiskPolicy.ApprovalRequiredWhen = nil
	candidate.ProposedManual.RunnableConditions.RequiresApproval = false
	candidate.ProposedManual.FallbackGuide = FallbackGuide{
		Mode:  "operator_review_required",
		Steps: []string{"Workflow 未声明 rollback stage，发布前需要补充回滚负责人、回滚触发条件和恢复验证方式。"},
	}

	report := ValidateManualCandidate(candidate, ManualCandidateValidationOptions{
		ActionSpecs: []ActionSpecSummary{{Action: "script.shell", Deprecated: true}},
	})
	if report.Status != "warning" || len(report.Blocking) != 0 {
		t.Fatalf("report = %#v, want warning without blocking", report)
	}
	for _, code := range []string{
		"missing_preflight_probe",
		"missing_recent_successful_run",
		"deprecated_action",
		"missing_negative_keywords",
		"missing_high_risk_approval_policy",
		"template_fallback_requires_review",
	} {
		if !validationHasCode(report.Warnings, code) {
			t.Fatalf("Warnings = %#v, missing code %q", report.Warnings, code)
		}
	}
}

func TestManualCandidateSearchSelfCheckUsesRetriever(t *testing.T) {
	cases := []struct {
		name   string
		raw    []byte
		specs  []ActionSpecSummary
		target string
		action string
	}{
		{
			name:   "pg-restore",
			raw:    loadWorkflowReverseFixture(t, "pg_restore.yaml"),
			specs:  []ActionSpecSummary{{Action: "script.shell", Risk: "high"}},
			target: "postgresql",
			action: "restore",
		},
		{
			name: "builtin-probes",
			raw:  loadWorkflowReverseFixture(t, "builtin_probes.yaml"),
			specs: []ActionSpecSummary{
				{Action: "builtin.dns_resolve", Risk: "read_only"},
				{Action: "builtin.tcp_ping", Risk: "read_only"},
				{Action: "builtin.http_check", Risk: "read_only"},
				{Action: "builtin.ssl_expiry_check", Risk: "read_only"},
			},
			target: "network_service",
			action: "inspect",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			analysis, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
				WorkflowID:  tc.name,
				RawYAML:     tc.raw,
				ActionSpecs: tc.specs,
				RecentRuns:  []RunRecord{{ID: "run-ok", WorkflowID: tc.name, ExecutionStatus: "success", ValidationStatus: "success"}},
				StorageURI:  "file://" + tc.name + ".yaml",
			})
			if err != nil {
				t.Fatalf("AnalyzeWorkflowForManual() error = %v", err)
			}
			candidate, err := BuildWorkflowManualCandidate(analysis)
			if err != nil {
				t.Fatalf("BuildWorkflowManualCandidate() error = %v", err)
			}
			report := ValidateManualCandidate(candidate, ManualCandidateValidationOptions{
				RecentRuns:       analysis.RecentRuns,
				SearchSelfCheck:  true,
				SearchFrame:      OperationFrame{Target: OperationTarget{Type: tc.target}, Operation: OperationProfile{TargetType: tc.target, Action: tc.action}, Risk: RiskProfile{Level: candidate.ProposedManual.Operation.RiskLevel}},
				SearchQueryText:  tc.target + " " + tc.action,
				AllowNeedInfoHit: true,
			})
			if report.Status == "blocked" || validationHasCode(report.Warnings, "search_self_check_failed") {
				t.Fatalf("report = %#v, want retriever self-check pass or non-search warnings only", report)
			}
			if !validationHasCode(report.Passed, "search_self_check_passed") {
				t.Fatalf("Passed = %#v, want search self-check passed", report.Passed)
			}
		})
	}
}

func TestWorkflowManualLLMSummaryCannotChangeStructuredFields(t *testing.T) {
	req := WorkflowManualGenerationRequest{
		WorkflowID: "builtin-probes",
		RawYAML:    loadWorkflowReverseFixture(t, "builtin_probes.yaml"),
		ActionSpecs: []ActionSpecSummary{
			{Action: "builtin.dns_resolve", Risk: "read_only"},
			{Action: "builtin.tcp_ping", Risk: "read_only"},
			{Action: "builtin.http_check", Risk: "read_only"},
			{Action: "builtin.ssl_expiry_check", Risk: "read_only"},
		},
	}
	deterministic, err := GenerateWorkflowManualCandidate(context.Background(), req, NoopWorkflowManualLLMSummarizer{})
	if err != nil {
		t.Fatalf("GenerateWorkflowManualCandidate(noop) error = %v", err)
	}
	req.Options.UseLLMSummary = true
	withLLM, err := GenerateWorkflowManualCandidate(context.Background(), req, fakeWorkflowManualLLMSummarizer{
		result: WorkflowManualLLMSummaryResult{
			DocumentMarkdown: deterministic.Candidate.ProposedManual.DocumentMarkdown + "\n\nLLM 润色补充：保持只读检查边界。",
			UserSummary: ManualGenerationUserSummary{
				Understood: []string{"这是一个只读网络探测 Workflow。"},
				NextSteps:  []string{"审核后发布。"},
			},
		},
	})
	if err != nil {
		t.Fatalf("GenerateWorkflowManualCandidate(llm) error = %v", err)
	}
	if withLLM.Candidate.ProposedManual.DocumentMarkdown == deterministic.Candidate.ProposedManual.DocumentMarkdown {
		t.Fatalf("DocumentMarkdown unchanged, want fake LLM markdown applied")
	}
	assertStructuredWorkflowManualFieldsEqual(t, deterministic.Candidate, withLLM.Candidate)
	if !reflect.DeepEqual(deterministic.ValidationReport, withLLM.ValidationReport) {
		t.Fatalf("ValidationReport changed:\nno llm=%#v\nllm=%#v", deterministic.ValidationReport, withLLM.ValidationReport)
	}
}

func TestWorkflowManualLLMRejectsUnsafeOutputAndKeepsDeterministic(t *testing.T) {
	req := WorkflowManualGenerationRequest{
		WorkflowID: "http-chatops-ticket",
		RawYAML:    loadWorkflowReverseFixture(t, "http_chatops_ticket.yaml"),
		ActionSpecs: []ActionSpecSummary{{
			Action:       "http.request",
			Risk:         "medium",
			RequiredArgs: []string{"url"},
		}},
	}
	deterministic, err := GenerateWorkflowManualCandidate(context.Background(), req, NoopWorkflowManualLLMSummarizer{})
	if err != nil {
		t.Fatalf("GenerateWorkflowManualCandidate(noop) error = %v", err)
	}
	req.Options.UseLLMSummary = true
	withLLM, err := GenerateWorkflowManualCandidate(context.Background(), req, fakeWorkflowManualLLMSummarizer{
		result: WorkflowManualLLMSummaryResult{
			DocumentMarkdown: "请带 Authorization 直接使用 secret_ref itsm/api-token。",
			UserSummary: ManualGenerationUserSummary{
				Understood: []string{"泄露 chatops/bot-token"},
			},
		},
	})
	if err != nil {
		t.Fatalf("GenerateWorkflowManualCandidate(llm) error = %v", err)
	}
	if withLLM.Candidate.ProposedManual.DocumentMarkdown != deterministic.Candidate.ProposedManual.DocumentMarkdown {
		t.Fatalf("DocumentMarkdown changed after unsafe output rejection")
	}
	if !validationHasCode(withLLM.Candidate.StructuredValidationReport.Warnings, "llm_output_rejected") {
		t.Fatalf("Warnings = %#v, want llm_output_rejected", withLLM.Candidate.StructuredValidationReport.Warnings)
	}
	assertStructuredWorkflowManualFieldsEqual(t, deterministic.Candidate, withLLM.Candidate)
}

func TestGeneratedManualCanBeRetrievedAfterVerification(t *testing.T) {
	repo := NewMemoryStore()
	pg := saveGeneratedManualForSearch(t, repo, "pg-restore", "pg_restore.yaml", []ActionSpecSummary{{Action: "script.shell", Risk: "high"}})
	probes := saveGeneratedManualForSearch(t, repo, "builtin-probes", "builtin_probes.yaml", []ActionSpecSummary{
		{Action: "builtin.dns_resolve", Risk: "read_only"},
		{Action: "builtin.tcp_ping", Risk: "read_only"},
		{Action: "builtin.http_check", Risk: "read_only"},
		{Action: "builtin.ssl_expiry_check", Risk: "read_only"},
	})

	pgResult, err := SearchOpsManualsWithHintProvider(context.Background(), repo, SearchOpsManualsRequest{
		Text: "在 Ubuntu 主机上恢复 PostgreSQL，备份文件已准备好，通过 ssh 执行并验证 pg_isready",
		OperationFrame: OperationFrame{
			Target:      OperationTarget{Type: "postgresql", Name: "pg-restore-01"},
			Operation:   OperationProfile{TargetType: "postgresql", Action: "restore"},
			Environment: EnvironmentProfile{ExecutionSurface: "ssh", Platform: "vm", OS: "ubuntu"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("SearchOpsManualsWithHintProvider(pg) error = %v", err)
	}
	if pgResult.Decision == DecisionNoMatch || len(pgResult.Manuals) == 0 || pgResult.Manuals[0].Manual.ID != pg.ID {
		t.Fatalf("pg search result = %#v, want generated pg manual top hit", pgResult)
	}
	if pgResult.Manuals[0].UsableMode == DecisionDirectExecute {
		t.Fatalf("pg usable mode = %q, want non-direct for high-risk restore without params", pgResult.Manuals[0].UsableMode)
	}

	probeResult, err := SearchOpsManualsWithHintProvider(context.Background(), repo, SearchOpsManualsRequest{
		Text: "检查 api.example.com 的 DNS TCP HTTP TLS 状态",
		OperationFrame: OperationFrame{
			Target:      OperationTarget{Type: "network_service", Name: "api.example.com"},
			Operation:   OperationProfile{TargetType: "network_service", Action: "inspect"},
			Environment: EnvironmentProfile{ExecutionSurface: "runner", Platform: "vm"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("SearchOpsManualsWithHintProvider(probes) error = %v", err)
	}
	if probeResult.Decision == DecisionNoMatch || len(probeResult.Manuals) == 0 || probeResult.Manuals[0].Manual.ID != probes.ID {
		t.Fatalf("probe search result = %#v, want generated probe manual top hit", probeResult)
	}
	if probeResult.Manuals[0].PreflightStatus != PreflightStatusNotRun && probeResult.Manuals[0].PreflightStatus != PreflightStatusNotApplicable {
		t.Fatalf("probe preflight status = %#v, want not_run or not_applicable", probeResult.Manuals[0].PreflightStatus)
	}

	negativeCases := []struct {
		name string
		req  SearchOpsManualsRequest
	}{
		{
			name: "mysql backup not pg direct",
			req: SearchOpsManualsRequest{
				Text:           "MySQL backup on vm",
				OperationFrame: OperationFrame{Target: OperationTarget{Type: "mysql"}, Operation: OperationProfile{TargetType: "mysql", Action: "backup"}, Environment: EnvironmentProfile{ExecutionSurface: "ssh", Platform: "vm"}},
			},
		},
		{
			name: "redis rca not probes direct",
			req: SearchOpsManualsRequest{
				Text:           "Redis RCA memory pressure",
				OperationFrame: OperationFrame{Target: OperationTarget{Type: "redis"}, Operation: OperationProfile{TargetType: "redis", Action: "rca_or_repair"}, Environment: EnvironmentProfile{ExecutionSurface: "ssh"}},
			},
		},
		{
			name: "sql query not executable",
			req:  SearchOpsManualsRequest{Text: "写 SQL 查询"},
		},
	}
	for _, tc := range negativeCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := SearchOpsManualsWithHintProvider(context.Background(), repo, tc.req, nil)
			if err != nil {
				t.Fatalf("SearchOpsManualsWithHintProvider() error = %v", err)
			}
			if len(result.Manuals) > 0 && result.Manuals[0].UsableMode == DecisionDirectExecute {
				t.Fatalf("result = %#v, want no direct generated manual", result)
			}
		})
	}
}

func loadWorkflowReverseFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "workflow_reverse", "real", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}

func analysisHasGraphStage(analysis WorkflowManualAnalysis, stage string) bool {
	for _, item := range analysis.GraphStages {
		if item.Stage == stage {
			return true
		}
	}
	for _, item := range analysis.Steps {
		if item.Stage == stage {
			return true
		}
	}
	return false
}

func saveGeneratedManualForSearch(t *testing.T, repo *MemoryStore, workflowID string, fixture string, specs []ActionSpecSummary) OpsManual {
	t.Helper()
	analysis, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID:  workflowID,
		RawYAML:     loadWorkflowReverseFixture(t, fixture),
		ActionSpecs: specs,
		StorageURI:  "file://" + fixture,
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual(%s) error = %v", fixture, err)
	}
	candidate, err := BuildWorkflowManualCandidate(analysis)
	if err != nil {
		t.Fatalf("BuildWorkflowManualCandidate(%s) error = %v", fixture, err)
	}
	manual := cloneManual(candidate.ProposedManual)
	manual.Status = ManualStatusVerified
	if err := repo.SaveManual(manual); err != nil {
		t.Fatalf("SaveManual(%s) error = %v", manual.ID, err)
	}
	return manual
}

type fakeWorkflowManualLLMSummarizer struct {
	result WorkflowManualLLMSummaryResult
	err    error
}

func (f fakeWorkflowManualLLMSummarizer) SummarizeWorkflowManual(context.Context, WorkflowManualLLMSummaryRequest) (WorkflowManualLLMSummaryResult, error) {
	return f.result, f.err
}

func assertStructuredWorkflowManualFieldsEqual(t *testing.T, left ManualCandidate, right ManualCandidate) {
	t.Helper()
	leftManual := left.ProposedManual
	rightManual := right.ProposedManual
	if !reflect.DeepEqual(leftManual.WorkflowRef, rightManual.WorkflowRef) ||
		!reflect.DeepEqual(leftManual.Operation, rightManual.Operation) ||
		!reflect.DeepEqual(leftManual.ParameterRules, rightManual.ParameterRules) ||
		!reflect.DeepEqual(leftManual.RiskPolicy, rightManual.RiskPolicy) {
		t.Fatalf("structured fields changed:\nleft=%#v\nright=%#v", leftManual, rightManual)
	}
}

func validWorkflowManualCandidateForValidation(t *testing.T) ManualCandidate {
	t.Helper()
	analysis, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID: "pg-restore",
		RawYAML:    loadWorkflowReverseFixture(t, "pg_restore.yaml"),
		ActionSpecs: []ActionSpecSummary{{
			Action: "script.shell",
			Risk:   "high",
		}},
		RecentRuns: []RunRecord{{ID: "run-ok", WorkflowID: "pg-restore", ExecutionStatus: "success", ValidationStatus: "success"}},
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual() error = %v", err)
	}
	candidate, err := BuildWorkflowManualCandidate(analysis)
	if err != nil {
		t.Fatalf("BuildWorkflowManualCandidate() error = %v", err)
	}
	return candidate
}

func validationHasIssue(issues []ValidationIssue, field string) bool {
	for _, issue := range issues {
		if issue.Field == field {
			return true
		}
	}
	return false
}

func validationHasCode(issues []ValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func analysisHasRisk(analysis WorkflowManualAnalysis, kind string) bool {
	for _, item := range analysis.ActionRisks {
		switch kind {
		case "service_restart":
			if item.ServiceRestart {
				return true
			}
		case "data_mutation":
			if item.DataMutation {
				return true
			}
		}
	}
	return false
}
