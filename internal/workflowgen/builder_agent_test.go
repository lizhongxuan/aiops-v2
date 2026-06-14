package workflowgen

import (
	"context"
	"strings"
	"testing"
	"time"
)

type recordingValidationProvider struct {
	result *ValidationResult
	req    ValidationRequest
}

func (p *recordingValidationProvider) Name() ValidationProvider {
	if p.result != nil && p.result.Provider != "" {
		return p.result.Provider
	}
	return ValidationProviderDocker
}

func (p *recordingValidationProvider) Validate(_ context.Context, req ValidationRequest) (*ValidationResult, error) {
	p.req = req
	if p.result != nil {
		return p.result, nil
	}
	now := time.Now().UTC()
	return &ValidationResult{
		ID:        "validation-test",
		Provider:  p.Name(),
		Status:    "passed",
		Scenario:  req.Scenario,
		Summary:   "validation passed",
		StartedAt: now,
		EndedAt:   now,
	}, nil
}

func TestWorkflowBuilderAgentGenerateDraftEmitsOrderedEvents(t *testing.T) {
	ctx := context.Background()
	store := NewMemorySessionStore()
	plan := testWorkflowGenerationPlan()
	created, err := store.Create(ctx, WorkflowGenerationSession{
		Status:      SessionStatusPlanReady,
		Requirement: "每天早上8点抓取 AI 新闻并直接返回三条摘要",
		Plan:        &plan,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	validator := &recordingValidationProvider{}
	agent := WorkflowBuilderAgent{
		Store:          store,
		GraphGenerator: RunnerGraphGenerator{},
		Validator:      validator,
	}

	result, err := agent.GenerateDraft(ctx, GenerateDraftRequest{SessionID: created.ID})
	if err != nil {
		t.Fatalf("GenerateDraft() error = %v", err)
	}
	if result.Session.Status != SessionStatusValidationPassed {
		t.Fatalf("session status = %s, want %s", result.Session.Status, SessionStatusValidationPassed)
	}
	if len(result.Graph.Nodes) == 0 || len(result.Graph.Edges) == 0 {
		t.Fatalf("graph was not generated: nodes=%d edges=%d", len(result.Graph.Nodes), len(result.Graph.Edges))
	}
	if validator.req.NetworkPolicy != "none" {
		t.Fatalf("validator network policy = %q, want none", validator.req.NetworkPolicy)
	}
	if validator.req.Scenario != "news-summary-return-only" {
		t.Fatalf("validator scenario = %q, want news-summary-return-only", validator.req.Scenario)
	}

	events, err := store.ListEvents(ctx, created.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	got := make([]EventType, 0, len(events))
	for _, event := range events {
		got = append(got, event.Type)
	}
	wantPrefix := []EventType{
		EventGenerationStarted,
		EventNodeGenerating,
		EventNodeGenerated,
		EventNodeGenerating,
		EventNodeGenerated,
		EventNodeGenerating,
		EventNodeGenerated,
		EventGraphPreviewReady,
		EventValidationStarted,
		EventValidationPassed,
	}
	if len(got) != len(wantPrefix) {
		t.Fatalf("events = %#v, want %#v", got, wantPrefix)
	}
	for i := range wantPrefix {
		if got[i] != wantPrefix[i] {
			t.Fatalf("event[%d] = %s, want %s; all=%#v", i, got[i], wantPrefix[i], got)
		}
	}
}

func TestWorkflowBuilderAgentMarksValidationFailure(t *testing.T) {
	ctx := context.Background()
	store := NewMemorySessionStore()
	plan := testWorkflowGenerationPlan()
	created, err := store.Create(ctx, WorkflowGenerationSession{
		Status:      SessionStatusPlanReady,
		Requirement: "生成一个测试工作流",
		Plan:        &plan,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	validator := &recordingValidationProvider{result: &ValidationResult{
		ID:            "validation-failed",
		Provider:      ValidationProviderDocker,
		Status:        "failed",
		Scenario:      "news-summary-return-only",
		Summary:       "Docker validation failed.",
		FailureNodeID: "extract-key-news",
	}}
	agent := WorkflowBuilderAgent{Store: store, Validator: validator}

	result, err := agent.GenerateDraft(ctx, GenerateDraftRequest{SessionID: created.ID})
	if err != nil {
		t.Fatalf("GenerateDraft() error = %v", err)
	}
	if result.Session.Status != SessionStatusValidationFailed {
		t.Fatalf("session status = %s, want %s", result.Session.Status, SessionStatusValidationFailed)
	}
	events, err := store.ListEvents(ctx, created.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	last := events[len(events)-1]
	if last.Type != EventValidationFailed || last.Status != "failed" {
		t.Fatalf("last event = %#v, want validation failed", last)
	}
}

func TestWorkflowBuilderAgentRequiresPlan(t *testing.T) {
	ctx := context.Background()
	store := NewMemorySessionStore()
	created, err := store.Create(ctx, WorkflowGenerationSession{Requirement: "缺少 plan"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	agent := WorkflowBuilderAgent{Store: store}

	_, err = agent.GenerateDraft(ctx, GenerateDraftRequest{SessionID: created.ID})
	if err == nil || !strings.Contains(err.Error(), "plan is required") {
		t.Fatalf("GenerateDraft() error = %v, want plan required", err)
	}
	updated, getErr := store.Get(ctx, created.ID)
	if getErr != nil {
		t.Fatalf("get session: %v", getErr)
	}
	if updated.Status != SessionStatusFailed {
		t.Fatalf("session status = %s, want failed", updated.Status)
	}
}

func TestWorkflowBuilderAgentRequiresSessionID(t *testing.T) {
	agent := WorkflowBuilderAgent{Store: NewMemorySessionStore()}
	_, err := agent.GenerateDraft(context.Background(), GenerateDraftRequest{})
	if err != nil && strings.Contains(err.Error(), "session_id is required") {
		return
	}
	t.Fatalf("GenerateDraft() error = %v, want session_id is required", err)
}

func testWorkflowGenerationPlan() WorkflowGenerationPlan {
	return WorkflowGenerationPlan{
		Version: 1,
		Title:   "AI 新闻摘要工作流",
		Intent:  "抓取 AI 行业新闻并提取三条关键内容",
		Trigger: WorkflowTrigger{
			Type:     TriggerTypeSchedule,
			Schedule: "0 8 * * *",
			Summary:  "每天早上 8 点运行",
		},
		Nodes: []WorkflowPlanNode{
			{
				ID:      "search-news",
				Kind:    NodeKindSearch,
				Title:   "搜索 AI 新闻",
				Action:  "script.python",
				Outputs: []WorkflowIO{{ID: "news_items", Type: "array"}},
			},
			{
				ID:     "extract-key-news",
				Kind:   NodeKindTransform,
				Title:  "提取关键新闻",
				Action: "script.python",
				Inputs: []WorkflowIO{{ID: "news_items", Type: "array"}},
				Outputs: []WorkflowIO{{
					ID:   "key_news",
					Type: "array",
				}},
			},
			{
				ID:     "deliver-result",
				Kind:   NodeKindOutput,
				Title:  "直接返回结果",
				Action: "script.python",
				Config: map[string]any{"target": "return"},
			},
		},
		Outputs: []WorkflowOutput{{ID: "chat_return", Target: OutputTargetReturn}},
		ValidationStrategy: ValidationStrategy{
			Enabled:  true,
			Provider: ValidationProviderDocker,
			Scenario: "news-summary-return-only",
			Network:  "none",
		},
	}
}
