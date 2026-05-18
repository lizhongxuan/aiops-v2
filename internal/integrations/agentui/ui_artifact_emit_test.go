package agentui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestRegisterBuiltinsAddsArtifactEmitTool(t *testing.T) {
	registry := tooling.NewRegistry()
	if err := RegisterBuiltins(registry); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}
	tools := registry.AssembleTools("host", "inspect")
	for _, tool := range tools {
		if tool.Metadata().Name == "aiops.ui_artifact_emit" {
			if !tool.IsReadOnly(nil) {
				t.Fatal("aiops.ui_artifact_emit should be read-only")
			}
			return
		}
	}
	t.Fatal("aiops.ui_artifact_emit not registered")
}

func TestUIArtifactEmitEmitsRCAReportDisplay(t *testing.T) {
	tool := NewUIArtifactEmitTool()
	input := json.RawMessage(`{
		"type":"rca_report",
		"titleZh":"checkout 根因分析",
		"summaryZh":"checkout 延迟升高最可能来自 catalog 依赖。",
		"status":"ok",
		"severity":"high",
		"source":"coroot",
		"permissionScope":"read",
		"redactionStatus":"redacted",
		"inlineData":{
			"schemaVersion":"aiops.rca_report/v1",
			"source":"coroot",
			"status":"ok",
			"target":{"service":"checkout"},
			"window":{"timeRange":"30m"},
			"conclusion":{"summaryZh":"checkout 延迟升高最可能来自 catalog 依赖。","confidence":0.72},
			"hypotheses":[],
			"sections":[],
			"evidenceRefs":["ev-coroot-latency"],
			"rawRefs":[]
		}
	}`)
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Display == nil || result.Display.Type != "rca_report" {
		t.Fatalf("Display = %#v, want rca_report", result.Display)
	}
	var payload map[string]any
	if err := json.Unmarshal(result.Display.Data, &payload); err != nil {
		t.Fatalf("display data is not json: %v", err)
	}
	if payload["type"] != "rca_report" || payload["permissionScope"] != "read" {
		t.Fatalf("payload = %#v, want rca_report read artifact", payload)
	}
}

func TestUIArtifactEmitRejectsUnknownType(t *testing.T) {
	_, err := NewUIArtifactEmitTool().Execute(context.Background(), json.RawMessage(`{"type":"free_html","inlineData":{"schemaVersion":"aiops.rca_report/v1"}}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported artifact type") {
		t.Fatalf("Execute() error = %v, want unsupported artifact type", err)
	}
}

func TestUIArtifactEmitRejectsForbiddenNestedKeys(t *testing.T) {
	_, err := NewUIArtifactEmitTool().Execute(context.Background(), json.RawMessage(`{
		"type":"rca_report",
		"inlineData":{"schemaVersion":"aiops.rca_report/v1","sections":[{"payload":{"script":"alert(1)"}}]}
	}`))
	if err == nil || !strings.Contains(err.Error(), "forbidden artifact key") {
		t.Fatalf("Execute() error = %v, want forbidden artifact key", err)
	}
}

func TestUIArtifactEmitRequiresEvidenceForNonInconclusiveReport(t *testing.T) {
	_, err := NewUIArtifactEmitTool().Execute(context.Background(), json.RawMessage(`{
		"type":"rca_report",
		"status":"ok",
		"inlineData":{
			"schemaVersion":"aiops.rca_report/v1",
			"status":"ok",
			"conclusion":{"summaryZh":"catalog is the root cause","confidence":0.8},
			"evidenceRefs":[],
			"rawRefs":[]
		}
	}`))
	if err == nil || !strings.Contains(err.Error(), "requires evidenceRefs or rawRefs") {
		t.Fatalf("Execute() error = %v, want evidence requirement", err)
	}
}

func TestUIArtifactEmitRejectsDirectActionHref(t *testing.T) {
	href := "java" + "scr" + "ipt:alert(1)"
	payload := strings.Replace(`{
		"type":"rca_report",
		"status":"ok",
		"inlineData":{
			"schemaVersion":"aiops.rca_report/v1",
			"status":"ok",
			"evidenceRefs":["ev-1"],
			"rawRefs":[]
		},
		"actions":[{"id":"unsafe","label":"open","href":"__href__"}]
	}`, "__href__", href, 1)
	_, err := NewUIArtifactEmitTool().Execute(context.Background(), json.RawMessage(payload))
	if err == nil || !strings.Contains(err.Error(), "direct action href is not allowed") {
		t.Fatalf("Execute() error = %v, want direct action href rejection", err)
	}
}
