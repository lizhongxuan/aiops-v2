package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	core "aiops-v2/internal/evidence"
	"aiops-v2/internal/tooling"

	"github.com/cloudwego/eino/schema"
)

func TestRunTurn_AutoRecordsEvidenceForRawRefToolResult(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-coroot-metrics",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "coroot.slo_status",
						Arguments: `{"service":"checkout"}`,
					},
				},
			}),
			schema.AssistantMessage("metrics recorded", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "coroot.slo_status",
			Description: "Coroot metrics",
			Domain:      "coroot",
			RiskLevel:   tooling.ToolRiskLow,
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			content := `{"schemaVersion":"aiops.coroot/v1","tool":"coroot.slo_status","status":"ok","source":"coroot","summary":"checkout p95 latency is 450ms","rawRef":{"uri":"coroot://raw/latency","digest":"abc123"}}`
			return tooling.ToolResult{Content: content}, nil
		},
	}
	service := core.NewService(core.NewInMemoryStore(), func() time.Time {
		return time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	})
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	kernel.evidenceService = service

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-auto-evidence",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-auto-evidence",
		Input:       "检查 checkout 延迟",
		Metadata: map[string]string{
			"aiops.coroot.explicitMention": "true",
			"aiops.tool.corootRCAAllowed":  "true",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	var toolContent string
	for _, msg := range model.inputs[1] {
		if msg.Role == schema.Tool && msg.ToolCallID == "call-coroot-metrics" {
			toolContent = msg.Content
			break
		}
	}
	if toolContent == "" {
		t.Fatalf("second model input missing tool result: %#v", model.inputs[1])
	}
	var payload struct {
		EvidenceRefs []string `json:"evidenceRefs"`
	}
	if err := json.Unmarshal([]byte(toolContent), &payload); err != nil {
		t.Fatalf("tool result content is not json: %v\n%s", err, toolContent)
	}
	if len(payload.EvidenceRefs) != 1 {
		t.Fatalf("evidenceRefs = %#v, want one auto-recorded ref in %s", payload.EvidenceRefs, toolContent)
	}
	rec, ok := service.Get(context.Background(), payload.EvidenceRefs[0])
	if !ok {
		t.Fatalf("evidence ref %q not found", payload.EvidenceRefs[0])
	}
	if rec.SourceTool != "coroot.slo_status" || rec.Source != "coroot" || rec.ToolCallID != "call-coroot-metrics" {
		t.Fatalf("record = %#v, want coroot tool context", rec)
	}
	if !strings.Contains(rec.Summary, "checkout p95 latency") {
		t.Fatalf("record summary = %q, want tool summary", rec.Summary)
	}
}
