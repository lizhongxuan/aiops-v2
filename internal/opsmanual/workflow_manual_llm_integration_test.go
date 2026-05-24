package opsmanual

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestWorkflowManualRealLLM(t *testing.T) {
	if os.Getenv("AIOPS_OPSMANUAL_REAL_LLM") != "1" {
		t.Skip("set AIOPS_OPSMANUAL_REAL_LLM=1 to run real LLM integration tests")
	}
	t.Run("LLM-01 builtin probes keeps structured fields stable", func(t *testing.T) {
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
		noLLM, err := GenerateWorkflowManualCandidate(context.Background(), req, NoopWorkflowManualLLMSummarizer{})
		if err != nil {
			t.Fatalf("GenerateWorkflowManualCandidate(no llm) error = %v", err)
		}
		req.Options.UseLLMSummary = true
		realLLM, err := GenerateWorkflowManualCandidate(context.Background(), req, EnvWorkflowManualLLMSummarizer{})
		if err != nil {
			t.Fatalf("GenerateWorkflowManualCandidate(real llm) error = %v", err)
		}
		if workflowManualStructuredHash(noLLM.Candidate) != workflowManualStructuredHash(realLLM.Candidate) {
			t.Fatalf("structured field hash changed: no llm=%s real=%s", workflowManualStructuredHash(noLLM.Candidate), workflowManualStructuredHash(realLLM.Candidate))
		}
		if !summaryHasChinese(realLLM.UserSummary) {
			t.Fatalf("UserSummary = %#v, want Chinese summary", realLLM.UserSummary)
		}
		if strings.TrimSpace(realLLM.Candidate.ProposedManual.DocumentMarkdown) == "" {
			t.Fatalf("real LLM markdown is empty")
		}
		if validationHasCode(realLLM.ValidationReport.Warnings, "llm_summary_failed") || validationHasCode(realLLM.ValidationReport.Warnings, "llm_output_rejected") {
			t.Fatalf("real LLM output was not accepted: %#v", realLLM.ValidationReport.Warnings)
		}
	})

	t.Run("LLM-02 http chatops secrets remain filtered", func(t *testing.T) {
		req := WorkflowManualGenerationRequest{
			WorkflowID: "http-chatops-ticket",
			RawYAML:    loadWorkflowReverseFixture(t, "http_chatops_ticket.yaml"),
			ActionSpecs: []ActionSpecSummary{{
				Action:       "http.request",
				Risk:         "medium",
				RequiredArgs: []string{"url"},
			}},
			Options: WorkflowManualGenerationOptions{UseLLMSummary: true},
		}
		result, err := GenerateWorkflowManualCandidate(context.Background(), req, EnvWorkflowManualLLMSummarizer{})
		if err != nil {
			t.Fatalf("GenerateWorkflowManualCandidate(real llm) error = %v", err)
		}
		if containsForbiddenLLMText(result.Candidate.ProposedManual.DocumentMarkdown + " " + strings.Join(result.UserSummary.Understood, " ")) {
			t.Fatalf("LLM output leaks forbidden text")
		}
		if riskLevelRank(result.Candidate.ProposedManual.Operation.RiskLevel) < riskLevelRank("medium") {
			t.Fatalf("RiskLevel = %q, want medium or higher", result.Candidate.ProposedManual.Operation.RiskLevel)
		}
		if validationHasCode(result.ValidationReport.Blocking, "sensitive_default_value") {
			t.Fatalf("ValidationReport = %#v, want no sensitive default blocking", result.ValidationReport)
		}
	})

	t.Run("LLM-03 pg restore keeps high-risk approval", func(t *testing.T) {
		req := WorkflowManualGenerationRequest{
			WorkflowID: "pg-restore",
			RawYAML:    loadWorkflowReverseFixture(t, "pg_restore.yaml"),
			ActionSpecs: []ActionSpecSummary{{
				Action: "script.shell",
				Risk:   "high",
			}},
			Options: WorkflowManualGenerationOptions{UseLLMSummary: true},
		}
		result, err := GenerateWorkflowManualCandidate(context.Background(), req, EnvWorkflowManualLLMSummarizer{})
		if err != nil {
			t.Fatalf("GenerateWorkflowManualCandidate(real llm) error = %v", err)
		}
		markdown := result.Candidate.ProposedManual.DocumentMarkdown
		for _, text := range []string{"适用范围", "前置检查", "执行步骤", "验证"} {
			if !strings.Contains(markdown, text) {
				t.Fatalf("markdown missing %q:\n%s", text, markdown)
			}
		}
		if !strings.Contains(markdown, "不能使用") && !strings.Contains(markdown, "禁止使用") {
			t.Fatalf("markdown missing cannot-use guidance:\n%s", markdown)
		}
		if result.Candidate.ProposedManual.Operation.RiskLevel != "high" || !result.Candidate.ProposedManual.RunnableConditions.RequiresApproval {
			t.Fatalf("risk/approval = %#v / %#v, want high and approval", result.Candidate.ProposedManual.Operation, result.Candidate.ProposedManual.RunnableConditions)
		}
		if strings.Contains(markdown, "无需审批") || strings.Contains(markdown, "直接执行生产恢复") {
			t.Fatalf("markdown weakens approval requirement:\n%s", markdown)
		}
	})

	t.Run("LLM-04 kubelet structured fields stable across runs", func(t *testing.T) {
		req := WorkflowManualGenerationRequest{
			WorkflowID: "self-healing-kubelet",
			RawYAML:    loadWorkflowReverseFixture(t, "self_healing_kubelet.yaml"),
			ActionSpecs: []ActionSpecSummary{
				{Action: "builtin.http_check", Risk: "read_only"},
				{Action: "script.shell", Risk: "high"},
			},
			Options: WorkflowManualGenerationOptions{UseLLMSummary: true},
		}
		var baseline ManualCandidate
		for i := 0; i < 3; i++ {
			result, err := GenerateWorkflowManualCandidate(context.Background(), req, EnvWorkflowManualLLMSummarizer{})
			if err != nil {
				t.Fatalf("GenerateWorkflowManualCandidate(real llm #%d) error = %v", i+1, err)
			}
			if err := ValidateWorkflowManualLLMOutput(WorkflowManualLLMSummaryResult{DocumentMarkdown: result.Candidate.ProposedManual.DocumentMarkdown, UserSummary: result.UserSummary}, result.Candidate.ProposedManual); err != nil {
				t.Fatalf("LLM output #%d failed safety validation: %v", i+1, err)
			}
			if i == 0 {
				baseline = result.Candidate
				continue
			}
			if baseline.ProposedManual.WorkflowRef.WorkflowDigest != result.Candidate.ProposedManual.WorkflowRef.WorkflowDigest ||
				!reflect.DeepEqual(baseline.ProposedManual.Operation, result.Candidate.ProposedManual.Operation) ||
				!reflect.DeepEqual(baseline.ProposedManual.RiskPolicy, result.Candidate.ProposedManual.RiskPolicy) ||
				!reflect.DeepEqual(baseline.ProposedManual.ParameterRules, result.Candidate.ProposedManual.ParameterRules) {
				t.Fatalf("structured fields changed between real LLM runs")
			}
		}
	})
}

func workflowManualStructuredHash(candidate ManualCandidate) string {
	value := struct {
		WorkflowRef    WorkflowRef              `json:"workflow_ref"`
		Operation      OperationProfile         `json:"operation"`
		RiskPolicy     RiskPolicy               `json:"risk_policy"`
		ParameterRules map[string]ParameterRule `json:"parameter_rules"`
	}{
		WorkflowRef:    candidate.ProposedManual.WorkflowRef,
		Operation:      candidate.ProposedManual.Operation,
		RiskPolicy:     candidate.ProposedManual.RiskPolicy,
		ParameterRules: candidate.ProposedManual.ParameterRules,
	}
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func summaryHasChinese(summary ManualGenerationUserSummary) bool {
	text := strings.Join(append(append([]string{}, summary.Understood...), append(summary.Missing, summary.NextSteps...)...), "")
	for _, r := range text {
		if r >= '\u4e00' && r <= '\u9fff' {
			return true
		}
	}
	return false
}

func containsForbiddenLLMText(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range []string{"authorization", "itsm/api-token", "chatops/bot-token", "sk-"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
