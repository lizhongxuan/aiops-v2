package promptcompiler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/tooling"
)

// ---------------------------------------------------------------------------
// Mock ToolRuntime for testing
// ---------------------------------------------------------------------------

type mockToolRuntime struct {
	name                string
	metadataDescription string
	desc                string
	readOnly            bool
	destructive         bool
	concurrencySafe     bool
	displayType         string
	outputSchema        json.RawMessage
}

func (m *mockToolRuntime) Metadata() tooling.ToolMetadata {
	return tooling.ToolMetadata{
		Name:        m.name,
		Description: m.metadataDescription,
		Origin:      tooling.ToolOriginBuiltin,
	}
}

func (m *mockToolRuntime) Description(_ json.RawMessage, _ tooling.DescribeContext) string {
	return m.desc
}

func (m *mockToolRuntime) Prompt(_ tooling.PromptContext) string {
	return m.desc
}

func (m *mockToolRuntime) CheckPermissions(_ context.Context, _ json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
}

func (m *mockToolRuntime) IsEnabled(_ tooling.ToolContext) bool { return true }

func (m *mockToolRuntime) IsReadOnly(_ json.RawMessage) bool { return m.readOnly }

func (m *mockToolRuntime) IsDestructive(_ json.RawMessage) bool { return m.destructive }

func (m *mockToolRuntime) IsConcurrencySafe(_ json.RawMessage) bool { return m.concurrencySafe }

func (m *mockToolRuntime) OutputSchema() json.RawMessage { return m.outputSchema }

func (m *mockToolRuntime) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (m *mockToolRuntime) ValidateInput(_ context.Context, _ json.RawMessage) error { return nil }

func (m *mockToolRuntime) Execute(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
	return tooling.ToolResult{}, nil
}

// ---------------------------------------------------------------------------
// Test: Compile produces four-layer output
// ---------------------------------------------------------------------------

func TestCompile_FourLayerOutput(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType:   "host",
		Mode:          "inspect",
		RuntimePolicy: "",
		HostContext:   "server-01 (Ubuntu 22.04)",
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All four layers must have content
	if result.System.Content == "" {
		t.Error("Layer 1 (System) should not be empty")
	}
	if result.Developer.Content == "" {
		t.Error("Layer 2 (Developer) should not be empty")
	}
	if result.Tools.Content == "" {
		t.Error("Layer 3 (Tools) should not be empty")
	}
	if result.Policy.Content == "" {
		t.Error("Layer 4 (Policy) should not be empty")
	}
}

// ---------------------------------------------------------------------------
// Test: System Prompt contains role and environment
// ---------------------------------------------------------------------------

func TestCompile_SystemPrompt_HostSession(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "host",
		Mode:        "chat",
		HostContext: "prod-web-01",
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.System.Content, "Role") {
		t.Error("System prompt should contain role section")
	}
	if !strings.Contains(result.System.Content, "Environment") {
		t.Error("System prompt should contain environment section")
	}
	if !strings.Contains(result.System.Environment, "prod-web-01") {
		t.Error("System prompt environment should contain host context")
	}
}

func TestCompile_SystemPrompt_WorkspaceSession(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType:      "workspace",
		Mode:             "execute",
		WorkspaceContext: "mission-123",
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.System.Role, "workspace") {
		t.Error("Workspace session should have workspace-related role")
	}
	if !strings.Contains(result.System.Environment, "mission-123") {
		t.Error("System prompt environment should contain workspace context")
	}
}

func TestCompile_SystemPrompt_PlannerAgent(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "workspace",
		Mode:        "execute",
		AgentKind:   AgentKindPlanner,
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.System.Role, "planning") {
		t.Error("Planner agent should have planning-related role")
	}
}

func TestCompile_SystemPrompt_WorkerAgent(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "workspace",
		Mode:        "execute",
		AgentKind:   AgentKindWorker,
		HostContext: "worker-host-01",
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.System.Role, "worker") {
		t.Error("Worker agent should have worker-related role")
	}
}

// ---------------------------------------------------------------------------
// Test: Developer Instructions contain mode-specific constraints
// ---------------------------------------------------------------------------

func TestCompile_DeveloperInstructions_ChatMode(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "host",
		Mode:        "chat",
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Developer.Content, "read-only") {
		t.Error("Chat mode should mention read-only constraint")
	}
	if !strings.Contains(result.Developer.Content, "mutation") {
		t.Error("Chat mode should mention mutation prohibition")
	}
}

func TestCompile_DeveloperInstructions_ExecuteMode(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "host",
		Mode:        "execute",
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Developer.Content, "approval") {
		t.Error("Execute mode should mention approval requirement")
	}
	if !strings.Contains(result.Developer.Content, "evidence") {
		t.Error("Execute mode should mention evidence collection")
	}
}

func TestCompile_DeveloperInstructions_SkillAssets(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType:       "host",
		Mode:              "inspect",
		SkillPromptAssets: []string{"Use structured output for disk analysis."},
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Developer.Content, "structured output for disk analysis") {
		t.Error("Developer instructions should include skill prompt assets")
	}
}

func TestCompile_DeveloperInstructions_IgnoresLegacyMCPAssets(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType:     "host",
		Mode:            "inspect",
		MCPPromptAssets: []string{"Coroot tools require service name parameter."},
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(result.Developer.Content, "Coroot tools require service name") {
		t.Error("Developer instructions should ignore legacy MCP prompt assets")
	}
}

func TestCompile_DeveloperInstructions_ExtraSections(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "host",
		Mode:        "inspect",
		ExtraSections: []PromptSection{
			{Title: "Hook Context", Content: "Runtime injected extra context."},
		},
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Developer.Content, "Hook Context") {
		t.Error("Developer instructions should include extra section title")
	}
	if !strings.Contains(result.Developer.Content, "Runtime injected extra context.") {
		t.Error("Developer instructions should include extra section content")
	}
}

// ---------------------------------------------------------------------------
// Test: Tool Prompt Set
// ---------------------------------------------------------------------------

func TestCompile_ToolPromptSet_WithTools(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "host",
		Mode:        "execute",
		AssembledTools: []Tool{
			&mockToolRuntime{
				name:                "host.disk_usage",
				metadataDescription: "Check disk usage on the host",
				desc:                "Fallback description should not win",
				readOnly:            true,
				destructive:         false,
				concurrencySafe:     true,
				displayType:         "table",
				outputSchema:        json.RawMessage(`{"type":"object","properties":{"used":{"type":"number"}}}`),
			},
			&mockToolRuntime{
				name:                "host.file_write",
				metadataDescription: "Write content to a file on the host",
				desc:                "Fallback description should not win",
				readOnly:            false,
				destructive:         true,
				concurrencySafe:     false,
				displayType:         "text",
				outputSchema:        json.RawMessage(`{"type":"string"}`),
			},
		},
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain tool names
	if !strings.Contains(result.Tools.Content, "host.disk_usage") {
		t.Error("Tool prompt should contain disk_usage tool")
	}
	if !strings.Contains(result.Tools.Content, "host.file_write") {
		t.Error("Tool prompt should contain file_write tool")
	}

	// Should have correct number of entries
	if len(result.Tools.Entries) != 2 {
		t.Errorf("expected 2 tool entries, got %d", len(result.Tools.Entries))
	}

	// Verify entry content: only capability, constraints, result shape, approval note
	diskEntry := result.Tools.Entries[0]
	if diskEntry.Capability != "Check disk usage on the host" {
		t.Errorf("metadata description should win, got %q", diskEntry.Capability)
	}
	if !strings.Contains(diskEntry.Constraints, "read-only") {
		t.Error("Read-only tool should have read-only constraint")
	}
	if !strings.Contains(diskEntry.ResultShape, "JSON schema") {
		t.Error("Read-only tool should include result shape from OutputSchema")
	}

	writeEntry := result.Tools.Entries[1]
	if !strings.Contains(writeEntry.Constraints, "destructive") {
		t.Error("Destructive tool should have destructive constraint")
	}
	if writeEntry.ApprovalNote == "" {
		t.Error("Destructive tool should have approval note")
	}
}

func TestCompile_ToolPromptSet_NoTools(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "host",
		Mode:        "chat",
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Tools.Content, "No tools available") {
		t.Error("Empty tool set should indicate no tools available")
	}
	if len(result.Tools.Entries) != 0 {
		t.Errorf("expected 0 tool entries, got %d", len(result.Tools.Entries))
	}
}

func TestCompile_ToolPromptSet_UsesMetadataDescriptionBeforeDescription(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "host",
		Mode:        "inspect",
		AssembledTools: []Tool{
			&mockToolRuntime{
				name:                "coroot.list_services",
				metadataDescription: "List services from Coroot",
				desc:                "Fallback description should not win",
				readOnly:            true,
				concurrencySafe:     true,
				displayType:         "list",
				outputSchema:        json.RawMessage(`{"type":"array"}`),
			},
		},
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Tools.Entries) != 1 {
		t.Errorf("expected 1 tool entry, got %d", len(result.Tools.Entries))
	}
	if !strings.Contains(result.Tools.Content, "coroot.list_services") {
		t.Error("Should include assembled tool in tool prompt")
	}
	if result.Tools.Entries[0].Capability != "List services from Coroot" {
		t.Errorf("metadata description should be used, got %q", result.Tools.Entries[0].Capability)
	}
}

func TestCompile_ToolPromptSet_UsesPromptOverrideAsConstraintGuidance(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "host",
		Mode:        "inspect",
		AssembledTools: []Tool{
			&mockToolRuntime{
				name:                "tool.search",
				metadataDescription: "Discover relevant tools for the task",
				desc:                "Use this orchestration tool to discover relevant tools before acting. It does not directly mutate files or services.",
				readOnly:            true,
				concurrencySafe:     true,
				outputSchema:        json.RawMessage(`{"type":"array"}`),
			},
		},
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Tools.Entries) != 1 {
		t.Fatalf("expected 1 tool entry, got %d", len(result.Tools.Entries))
	}
	if !strings.Contains(result.Tools.Entries[0].Constraints, "discover relevant tools before acting") {
		t.Fatalf("expected orchestration guidance in constraints, got %q", result.Tools.Entries[0].Constraints)
	}
}

// ---------------------------------------------------------------------------
// Test: Runtime Policy Prompt
// ---------------------------------------------------------------------------

func TestCompile_RuntimePolicy_ChatMode(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "host",
		Mode:        "chat",
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Policy.Mode != "chat" {
		t.Errorf("expected mode chat, got %s", result.Policy.Mode)
	}
	if !strings.Contains(result.Policy.Content, "Chat mode") {
		t.Error("Chat policy should mention chat mode")
	}
	if !strings.Contains(result.Policy.Content, "read-only") {
		t.Error("Chat policy should mention read-only")
	}
}

func TestCompile_RuntimePolicy_InspectMode(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "host",
		Mode:        "inspect",
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Policy.Mode != "inspect" {
		t.Errorf("expected mode inspect, got %s", result.Policy.Mode)
	}
	if !strings.Contains(result.Policy.Content, "Inspect mode") {
		t.Error("Inspect policy should mention inspect mode")
	}
}

func TestCompile_RuntimePolicy_PlanMode(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "host",
		Mode:        "plan",
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Policy.Mode != "plan" {
		t.Errorf("expected mode plan, got %s", result.Policy.Mode)
	}
	if !strings.Contains(result.Policy.Content, "Plan mode") {
		t.Error("Plan policy should mention plan mode")
	}
}

func TestCompile_RuntimePolicy_ExecuteMode(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "host",
		Mode:        "execute",
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Policy.Mode != "execute" {
		t.Errorf("expected mode execute, got %s", result.Policy.Mode)
	}
	if !strings.Contains(result.Policy.Content, "Execute mode") {
		t.Error("Execute policy should mention execute mode")
	}
	if !strings.Contains(result.Policy.Content, "approval") {
		t.Error("Execute policy should mention approval")
	}
}

func TestCompile_RuntimePolicy_CustomPolicy(t *testing.T) {
	compiler := NewCompiler()

	customPolicy := "Custom: Only allow disk inspection tools."
	ctx := CompileContext{
		SessionType:   "host",
		Mode:          "inspect",
		RuntimePolicy: customPolicy,
	}

	result, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Policy.Content, customPolicy) {
		t.Error("Should use custom runtime policy when provided")
	}
}

// ---------------------------------------------------------------------------
// Test: CompileForEino
// ---------------------------------------------------------------------------

func TestCompileForEino_ProducesSystemMessages(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType: "host",
		Mode:        "inspect",
		HostContext: "test-host",
		AssembledTools: []Tool{
			&mockToolRuntime{
				name:                "host.ps",
				metadataDescription: "List processes",
				desc:                "List processes fallback",
				readOnly:            true,
				concurrencySafe:     true,
				displayType:         "table",
				outputSchema:        json.RawMessage(`{"type":"array"}`),
			},
		},
	}

	messages, err := compiler.CompileForEino(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should produce 4 system messages (one per layer)
	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}

	// All messages should be system role
	for i, msg := range messages {
		if msg.Role != schema.System {
			t.Errorf("message[%d] should have role 'system', got %q", i, msg.Role)
		}
		if msg.Content == "" {
			t.Errorf("message[%d] should have non-empty content", i)
		}
	}

	// Verify layer order by content
	if !strings.Contains(messages[0].Content, "Role") {
		t.Error("First message should be system prompt (contains Role)")
	}
	if !strings.Contains(messages[1].Content, "Developer Instructions") {
		t.Error("Second message should be developer instructions")
	}
	if !strings.Contains(messages[2].Content, "Available Tools") {
		t.Error("Third message should be tool prompt set")
	}
	if !strings.Contains(messages[3].Content, "Runtime Policy") {
		t.Error("Fourth message should be runtime policy")
	}
}

func TestCompileForEino_ContentPreserved(t *testing.T) {
	compiler := NewCompiler()

	ctx := CompileContext{
		SessionType:   "host",
		Mode:          "execute",
		HostContext:   "important-host",
		RuntimePolicy: "Special policy: require dual approval.",
	}

	compiled, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	messages, err := compiler.CompileForEino(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify content is preserved in messages
	if messages[0].Content != compiled.System.Content {
		t.Error("System message content should match compiled system prompt")
	}
	if messages[1].Content != compiled.Developer.Content {
		t.Error("Developer message content should match compiled developer instructions")
	}
	if messages[2].Content != compiled.Tools.Content {
		t.Error("Tools message content should match compiled tool prompt set")
	}
	if messages[3].Content != compiled.Policy.Content {
		t.Error("Policy message content should match compiled runtime policy")
	}
}

// ---------------------------------------------------------------------------
// Test: Compiler interface compliance
// ---------------------------------------------------------------------------

func TestPromptCompilerImpl_ImplementsCompiler(t *testing.T) {
	var _ Compiler = (*PromptCompilerImpl)(nil)
}
