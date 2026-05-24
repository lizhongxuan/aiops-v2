package opsmanual

import (
	"strings"
	"testing"
)

func TestWorkflowManualRealDataPgRestore(t *testing.T) {
	analysis, candidate := generateRealDataWorkflowManual(t, "pg-restore", "pg_restore.yaml", []ActionSpecSummary{{Action: "script.shell", Risk: "high"}})
	manual := candidate.ProposedManual
	if manual.Operation.TargetType != "postgresql" || manual.Operation.Action != "restore" || manual.Operation.RiskLevel != "high" {
		t.Fatalf("Operation = %#v, want postgresql restore high", manual.Operation)
	}
	if manual.RiskPolicy.ServiceRestart == "" || !manual.RunnableConditions.RequiresApproval {
		t.Fatalf("risk/runnable = %#v / %#v, want service restart approval", manual.RiskPolicy, manual.RunnableConditions)
	}
	if !strings.Contains(strings.ToLower(strings.Join(manual.Validation, " ")), "pg_isready") && !strings.Contains(strings.ToLower(strings.Join(manual.Validation, " ")), "ready") {
		t.Fatalf("Validation = %#v, want pg_isready/ready", manual.Validation)
	}
	if len(manual.CannotUseWhen) == 0 {
		t.Fatalf("CannotUseWhen empty")
	}
	report := ValidateManualCandidate(candidate, ManualCandidateValidationOptions{
		SearchSelfCheck: true,
		SearchFrame:     OperationFrame{Target: OperationTarget{Type: "postgresql"}, Operation: OperationProfile{TargetType: "postgresql", Action: "restore"}, Risk: RiskProfile{Level: "high"}},
		SearchQueryText: "postgresql restore pg_isready",
		RecentRuns:      analysis.RecentRuns,
	})
	if validationHasCode(report.Warnings, "search_self_check_failed") {
		t.Fatalf("search self-check report = %#v, want retrievable", report)
	}
}

func TestWorkflowManualRealDataSelfHealingKubelet(t *testing.T) {
	_, candidate := generateRealDataWorkflowManual(t, "self-healing-kubelet", "self_healing_kubelet.yaml", []ActionSpecSummary{
		{Action: "builtin.http_check", Risk: "read_only"},
		{Action: "script.shell", Risk: "high"},
	})
	manual := candidate.ProposedManual
	if manual.Operation.TargetType != "kubelet" && manual.Operation.TargetType != "kubernetes_node" {
		t.Fatalf("TargetType = %q, want kubelet/kubernetes_node", manual.Operation.TargetType)
	}
	if manual.Operation.Action != "repair" || manual.Operation.RiskLevel != "high" || !manual.RunnableConditions.RequiresApproval {
		t.Fatalf("operation/runnable = %#v / %#v, want repair high approval", manual.Operation, manual.RunnableConditions)
	}
	if !manual.PreflightProbe.ReadOnly {
		t.Fatalf("PreflightProbe = %#v, want read-only", manual.PreflightProbe)
	}
	if !strings.Contains(strings.ToLower(strings.Join(manual.Validation, " ")), "ready") {
		t.Fatalf("Validation = %#v, want ready validation", manual.Validation)
	}
}

func TestWorkflowManualRealDataBuiltinProbes(t *testing.T) {
	_, candidate := generateRealDataWorkflowManual(t, "builtin-probes", "builtin_probes.yaml", []ActionSpecSummary{
		{Action: "builtin.dns_resolve", Risk: "read_only"},
		{Action: "builtin.tcp_ping", Risk: "read_only"},
		{Action: "builtin.http_check", Risk: "read_only"},
		{Action: "builtin.ssl_expiry_check", Risk: "read_only"},
	})
	manual := candidate.ProposedManual
	if manual.Operation.RiskLevel != "read_only" || manual.RunnableConditions.RequiresApproval {
		t.Fatalf("risk/runnable = %#v / %#v, want read_only without approval", manual.Operation, manual.RunnableConditions)
	}
	if manual.PreflightProbe.Action == "" {
		t.Fatalf("PreflightProbe = %#v, want probe", manual.PreflightProbe)
	}
	for _, keyword := range []string{"dns", "tcp", "http", "tls"} {
		if !stringSliceContains(manual.RetrievalProfile.Keywords, keyword) {
			t.Fatalf("Keywords = %#v, missing %q", manual.RetrievalProfile.Keywords, keyword)
		}
	}
	if len(candidate.StructuredValidationReport.Blocking) != 0 {
		t.Fatalf("ValidationReport = %#v, want no blocking", candidate.StructuredValidationReport)
	}
}

func TestWorkflowManualRealDataHTTPChatOpsTicket(t *testing.T) {
	analysis, candidate := generateRealDataWorkflowManual(t, "http-chatops-ticket", "http_chatops_ticket.yaml", []ActionSpecSummary{{Action: "http.request", Risk: "medium", RequiredArgs: []string{"url"}}})
	manual := candidate.ProposedManual
	if riskLevelRank(manual.Operation.RiskLevel) < riskLevelRank("medium") {
		t.Fatalf("RiskLevel = %q, want medium or higher", manual.Operation.RiskLevel)
	}
	for _, secret := range []string{"itsm/api-token", "chatops/bot-token", "Authorization"} {
		if strings.Contains(manual.DocumentMarkdown, secret) {
			t.Fatalf("DocumentMarkdown leaks %q:\n%s", secret, manual.DocumentMarkdown)
		}
	}
	if len(analysis.SecretFindings) == 0 {
		t.Fatalf("SecretFindings empty, want sensitive auth usage detected")
	}
	if validationHasCode(candidate.StructuredValidationReport.Blocking, "sensitive_default_value") {
		t.Fatalf("ValidationReport = %#v, want no sensitive default blocking", candidate.StructuredValidationReport)
	}
}

func TestWorkflowManualRealDataRedisMemoryDryRunGraphStages(t *testing.T) {
	analysis, candidate := generateRealDataWorkflowManual(t, "redis-memory-dry-run", "redis_memory_dry_run.yaml", nil)
	for _, stage := range []string{"precheck", "approval", "dry_run", "execute", "validate", "rollback"} {
		if !analysisHasGraphStage(analysis, stage) {
			t.Fatalf("GraphStages = %#v / steps=%#v, missing %q", analysis.GraphStages, analysis.Steps, stage)
		}
	}
	summary := strings.ToLower(strings.Join(append(append([]string{}, candidate.UserSummary.Understood...), candidate.UserSummary.NextSteps...), " "))
	if !strings.Contains(summary, "预检计划检查") || !strings.Contains(summary, "rollback") {
		t.Fatalf("UserSummary = %#v, want preflight plan check and rollback", candidate.UserSummary)
	}
	if candidate.ProposedManual.FallbackGuide.Mode == "" || len(candidate.ProposedManual.FallbackGuide.Steps) == 0 {
		t.Fatalf("FallbackGuide = %#v, want fallback", candidate.ProposedManual.FallbackGuide)
	}
	if candidate.StructuredValidationReport.Status == "" {
		t.Fatalf("ValidationReport = %#v, want status", candidate.StructuredValidationReport)
	}
}

func TestWorkflowManualRealDataShellRunMinimalBlocked(t *testing.T) {
	_, candidate := generateRealDataWorkflowManual(t, "shell-run-minimal", "shell_run_minimal.yaml", []ActionSpecSummary{{Action: "script.shell", Risk: "high"}})
	if candidate.ID == "" {
		t.Fatalf("candidate = %#v, want generated candidate", candidate)
	}
	if candidate.StructuredValidationReport.Status != "blocked" {
		t.Fatalf("ValidationReport = %#v, want blocked", candidate.StructuredValidationReport)
	}
	hasExpectedBlocking := validationHasIssue(candidate.StructuredValidationReport.Blocking, "operation.target_type") ||
		validationHasIssue(candidate.StructuredValidationReport.Blocking, "operation.action") ||
		validationHasIssue(candidate.StructuredValidationReport.Blocking, "validation") ||
		validationHasIssue(candidate.StructuredValidationReport.Blocking, "cannot_use_when")
	if !hasExpectedBlocking {
		t.Fatalf("Blocking = %#v, want operation or validation gap", candidate.StructuredValidationReport.Blocking)
	}
	if len(candidate.UserSummary.Missing) == 0 {
		t.Fatalf("UserSummary = %#v, want missing list", candidate.UserSummary)
	}
}

func generateRealDataWorkflowManual(t *testing.T, workflowID string, fixture string, specs []ActionSpecSummary) (WorkflowManualAnalysis, ManualCandidate) {
	t.Helper()
	analysis, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID:  workflowID,
		RawYAML:     loadWorkflowReverseFixture(t, fixture),
		ActionSpecs: specs,
		StorageURI:  "file://" + fixture,
		RecentRuns:  []RunRecord{{ID: "run-ok", WorkflowID: workflowID, ExecutionStatus: "success", ValidationStatus: "success"}},
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual(%s) error = %v", fixture, err)
	}
	candidate, err := BuildWorkflowManualCandidate(analysis)
	if err != nil {
		t.Fatalf("BuildWorkflowManualCandidate(%s) error = %v", fixture, err)
	}
	return analysis, candidate
}
