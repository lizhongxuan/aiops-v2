package opsmanual

import (
	"context"
	"strings"
	"sync"
	"testing"

	"aiops-v2/internal/modelrouter"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func resetWorkflowManualLLMDefaultsForTest() {
	SetDefaultWorkflowManualLLMSummarizer(nil)
}

func TestWorkflowManualDefaultLLMSummarizerUsesConfiguredProvider(t *testing.T) {
	t.Cleanup(resetWorkflowManualLLMDefaultsForTest)
	SetDefaultWorkflowManualLLMSummarizer(fakeWorkflowManualLLMSummarizer{
		result: WorkflowManualLLMSummaryResult{
			DocumentMarkdown: "modelrouter polished markdown",
			UserSummary: ManualGenerationUserSummary{
				Understood: []string{"默认 provider 已接入。"},
				NextSteps:  []string{"继续审核。"},
			},
		},
	})
	t.Setenv("AIOPS_LLM_BASE_URL", "")
	t.Setenv("AIOPS_LLM_API_KEY", "")
	t.Setenv("AIOPS_LLM_MODEL", "")
	t.Setenv("AIOPS_LLM_CONFIG_FILE", "")

	req := WorkflowManualGenerationRequest{
		WorkflowID: "builtin-probes",
		RawYAML:    loadWorkflowReverseFixture(t, "builtin_probes.yaml"),
		ActionSpecs: []ActionSpecSummary{
			{Action: "builtin.dns_resolve", Risk: "read_only"},
			{Action: "builtin.tcp_ping", Risk: "read_only"},
			{Action: "builtin.http_check", Risk: "read_only"},
			{Action: "builtin.ssl_expiry_check", Risk: "read_only"},
		},
		Options: WorkflowManualGenerationOptions{UseLLMSummary: true},
	}

	result, err := GenerateWorkflowManualCandidate(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("GenerateWorkflowManualCandidate() error = %v", err)
	}
	if result.Candidate.ProposedManual.DocumentMarkdown != "modelrouter polished markdown" {
		t.Fatalf("DocumentMarkdown = %q, want configured default provider output", result.Candidate.ProposedManual.DocumentMarkdown)
	}
	if validationHasCode(result.ValidationReport.Warnings, "llm_summary_failed") {
		t.Fatalf("Warnings = %#v, want no env config fallback failure", result.ValidationReport.Warnings)
	}
}

func TestModelRouterWorkflowManualLLMSummarizerUsesSkillPromptTemplate(t *testing.T) {
	model := &capturingWorkflowManualChatModel{
		response: `{"document_markdown":"template polished markdown","user_summary":{"understood":["skill template used"],"missing":[],"next_steps":["review"]}}`,
	}
	router := modelrouter.NewRouter("opsmanual", map[string]modelrouter.ChatModel{"opsmanual": model}, nil)
	summarizer := ModelRouterWorkflowManualLLMSummarizer{
		Router: router,
		PromptTemplate: strings.Join([]string{
			"SKILL TEMPLATE",
			"language={{language}}",
			"payload={{workflow_manual_payload}}",
		}, "\n"),
	}

	manualReq := sampleWorkflowManualLLMSummaryRequest(t)
	result, err := summarizer.SummarizeWorkflowManual(context.Background(), manualReq)
	if err != nil {
		t.Fatalf("SummarizeWorkflowManual() error = %v", err)
	}
	if result.DocumentMarkdown != "template polished markdown" {
		t.Fatalf("DocumentMarkdown = %q, want model response", result.DocumentMarkdown)
	}
	if len(model.messages) != 2 {
		t.Fatalf("captured messages = %d, want system and user", len(model.messages))
	}
	userPrompt := model.messages[1].Content
	if !strings.Contains(userPrompt, "SKILL TEMPLATE") || !strings.Contains(userPrompt, `"workflow_ref"`) {
		t.Fatalf("user prompt missing skill template or payload:\n%s", userPrompt)
	}
	if strings.Contains(userPrompt, "{{workflow_manual_payload}}") || strings.Contains(userPrompt, "{{language}}") {
		t.Fatalf("user prompt contains unreplaced template placeholders:\n%s", userPrompt)
	}
}

func TestModelRouterWorkflowManualLLMSummarizerFailureKeepsDeterministicCandidate(t *testing.T) {
	t.Cleanup(resetWorkflowManualLLMDefaultsForTest)
	router := modelrouter.NewRouter("missing", nil, nil)
	SetDefaultWorkflowManualLLMSummarizer(ModelRouterWorkflowManualLLMSummarizer{Router: router})
	t.Setenv("AIOPS_LLM_BASE_URL", "")
	t.Setenv("AIOPS_LLM_API_KEY", "")
	t.Setenv("AIOPS_LLM_MODEL", "")
	t.Setenv("AIOPS_LLM_CONFIG_FILE", "")

	req := WorkflowManualGenerationRequest{
		WorkflowID: "pg-restore",
		RawYAML:    loadWorkflowReverseFixture(t, "pg_restore.yaml"),
		ActionSpecs: []ActionSpecSummary{{
			Action: "script.shell",
			Risk:   "high",
		}},
	}
	deterministic, err := GenerateWorkflowManualCandidate(context.Background(), req, NoopWorkflowManualLLMSummarizer{})
	if err != nil {
		t.Fatalf("GenerateWorkflowManualCandidate(noop) error = %v", err)
	}

	req.Options.UseLLMSummary = true
	result, err := GenerateWorkflowManualCandidate(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("GenerateWorkflowManualCandidate(modelrouter failure) error = %v", err)
	}
	if result.Candidate.ProposedManual.DocumentMarkdown != deterministic.Candidate.ProposedManual.DocumentMarkdown {
		t.Fatalf("DocumentMarkdown changed after modelrouter failure")
	}
	if !validationHasCode(result.ValidationReport.Warnings, "llm_summary_failed") {
		t.Fatalf("Warnings = %#v, want llm_summary_failed", result.ValidationReport.Warnings)
	}
}

func sampleWorkflowManualLLMSummaryRequest(t *testing.T) WorkflowManualLLMSummaryRequest {
	t.Helper()
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
	analysis, err := AnalyzeWorkflowForManual(req)
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual() error = %v", err)
	}
	candidate, err := BuildWorkflowManualCandidate(analysis)
	if err != nil {
		t.Fatalf("BuildWorkflowManualCandidate() error = %v", err)
	}
	return WorkflowManualLLMSummaryRequest{
		Analysis:    analysis,
		Manual:      candidate.ProposedManual,
		Validation:  candidate.StructuredValidationReport,
		UserSummary: candidate.UserSummary,
		Language:    "zh-CN",
	}
}

type capturingWorkflowManualChatModel struct {
	mu       sync.Mutex
	response string
	messages []*schema.Message
}

func (m *capturingWorkflowManualChatModel) Generate(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append([]*schema.Message(nil), messages...)
	return &schema.Message{Role: schema.Assistant, Content: m.response}, nil
}

func (m *capturingWorkflowManualChatModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *capturingWorkflowManualChatModel) BindTools([]*schema.ToolInfo) error {
	return nil
}
