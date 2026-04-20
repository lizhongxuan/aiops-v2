package runtimekernel

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Tests for Business Logic Baseline compliance validation (Req 12.1, 12.4, 12.6)
// ---------------------------------------------------------------------------

func TestValidateFourLayerSeparation_Clean(t *testing.T) {
	items := []SemanticContent{
		{Layer: LayerFinal, Content: "Here is the disk usage report for your server."},
		{Layer: LayerProcess, Content: "[tool_started] disk_usage"},
		{Layer: LayerBlocking, Content: "[approval_pending] rm -rf /tmp"},
		{Layer: LayerDeepSurface, Content: "[card_data] {\"type\":\"metrics\"}"},
	}
	if err := ValidateFourLayerSeparation(items); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateFourLayerSeparation_ProcessLeakIntoFinal(t *testing.T) {
	items := []SemanticContent{
		{Layer: LayerFinal, Content: "The result is ready. [tool_started] checking disk"},
	}
	err := ValidateFourLayerSeparation(items)
	if err == nil {
		t.Fatal("expected error for process marker in final layer")
	}
}

func TestValidateFourLayerSeparation_BlockingLeakIntoFinal(t *testing.T) {
	items := []SemanticContent{
		{Layer: LayerFinal, Content: "Please wait [approval_pending] for admin"},
	}
	err := ValidateFourLayerSeparation(items)
	if err == nil {
		t.Fatal("expected error for blocking marker in final layer")
	}
}

func TestValidateFourLayerSeparation_DeepSurfaceLeakIntoFinal(t *testing.T) {
	items := []SemanticContent{
		{Layer: LayerFinal, Content: "Here is the data [card_data] embedded"},
	}
	err := ValidateFourLayerSeparation(items)
	if err == nil {
		t.Fatal("expected error for deep surface marker in final layer")
	}
}

func TestValidateFourLayerSeparation_InvalidLayer(t *testing.T) {
	items := []SemanticContent{
		{Layer: "unknown", Content: "some content"},
	}
	err := ValidateFourLayerSeparation(items)
	if err == nil {
		t.Fatal("expected error for invalid layer")
	}
}

func TestValidateWorkspaceStateIsolation_Clean(t *testing.T) {
	body := "I've completed the disk cleanup across all three servers. Total freed: 45GB."
	if err := ValidateWorkspaceStateIsolation(body); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateWorkspaceStateIsolation_AgentStateLeak(t *testing.T) {
	body := "Working on it [agent_status: running] checking server A"
	err := ValidateWorkspaceStateIsolation(body)
	if err == nil {
		t.Fatal("expected error for agent state leak")
	}
}

func TestValidateWorkspaceStateIsolation_PlanStateLeak(t *testing.T) {
	body := "Step 2 complete [plan_step: 3/5] moving to next"
	err := ValidateWorkspaceStateIsolation(body)
	if err == nil {
		t.Fatal("expected error for plan state leak")
	}
}

func TestValidateWorkspaceStateIsolation_ChoiceStateLeak(t *testing.T) {
	body := "Waiting for input [choice_pending] select option"
	err := ValidateWorkspaceStateIsolation(body)
	if err == nil {
		t.Fatal("expected error for choice state leak")
	}
}

func TestValidateToolStateSource_Lifecycle(t *testing.T) {
	if err := ValidateToolStateSource(ToolStateFromLifecycle); err != nil {
		t.Fatalf("lifecycle source should be valid: %v", err)
	}
}

func TestValidateToolStateSource_Display(t *testing.T) {
	if err := ValidateToolStateSource(ToolStateFromDisplay); err != nil {
		t.Fatalf("display source should be valid: %v", err)
	}
}

func TestValidateToolStateSource_AssistantText(t *testing.T) {
	err := ValidateToolStateSource(ToolStateFromAssistantText)
	if err == nil {
		t.Fatal("assistant text source should be forbidden")
	}
}

func TestValidateToolLifecycleRecords_AllFromLifecycle(t *testing.T) {
	records := []ToolLifecycleRecord{
		{ToolCallID: "tc-1", ToolName: "disk_usage", Status: "completed", Source: ToolStateFromLifecycle},
		{ToolCallID: "tc-2", ToolName: "file_read", Status: "started", Source: ToolStateFromLifecycle},
	}
	if err := ValidateToolLifecycleRecords(records); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateToolLifecycleRecords_FromAssistantText(t *testing.T) {
	records := []ToolLifecycleRecord{
		{ToolCallID: "tc-1", ToolName: "disk_usage", Status: "completed", Source: ToolStateFromLifecycle},
		{ToolCallID: "tc-2", ToolName: "file_read", Status: "started", Source: ToolStateFromAssistantText},
	}
	err := ValidateToolLifecycleRecords(records)
	if err == nil {
		t.Fatal("expected error for record derived from assistant text")
	}
}

func TestValidateBaseline_FullyCompliant(t *testing.T) {
	content := []SemanticContent{
		{Layer: LayerFinal, Content: "Disk cleanup complete."},
		{Layer: LayerProcess, Content: "[tool_completed] disk_cleanup"},
	}
	body := "All servers are healthy now."
	records := []ToolLifecycleRecord{
		{ToolCallID: "tc-1", ToolName: "disk_cleanup", Status: "completed", Source: ToolStateFromLifecycle},
	}

	report := ValidateBaseline(content, body, records)
	if !report.Compliant {
		t.Fatalf("expected compliant, got violations: %v", report.Violations)
	}
}

func TestValidateBaseline_MultipleViolations(t *testing.T) {
	content := []SemanticContent{
		{Layer: LayerFinal, Content: "Result [tool_started] leaked"},
	}
	body := "Status [agent_status: running] leaked"
	records := []ToolLifecycleRecord{
		{ToolCallID: "tc-1", ToolName: "x", Status: "completed", Source: ToolStateFromAssistantText},
	}

	report := ValidateBaseline(content, body, records)
	if report.Compliant {
		t.Fatal("expected non-compliant")
	}
	if len(report.Violations) != 3 {
		t.Fatalf("expected 3 violations, got %d: %v", len(report.Violations), report.Violations)
	}
}
