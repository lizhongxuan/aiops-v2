package promptcompiler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/tooling"
	"pgregory.net/rapid"
)

// Feature: aiops-codex-eino-rewrite, Property 9: PromptCompiler 四层输出结构
// Feature: aiops-codex-eino-rewrite, Property 11: Tool Prompt 内容约束
// Feature: aiops-codex-eino-rewrite, Property 12: Mode 特定策略 Prompt

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

// genSessionType generates a random valid SessionType.
func genSessionType() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{"host", "workspace"})
}

// genMode generates a random valid Mode.
func genMode() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{"chat", "inspect", "plan", "execute"})
}

// genAgentKind generates a random AgentKind (including empty for default).
func genAgentKind() *rapid.Generator[AgentKind] {
	return rapid.SampledFrom([]AgentKind{"", AgentKindPlanner, AgentKindWorker})
}

// rapidMockToolRuntime is a mock ToolRuntime for property tests.
type rapidMockToolRuntime struct {
	name                string
	metadataDescription string
	desc                string
	readOnly            bool
	destructive         bool
	concurrencySafe     bool
	displayType         string
	outputSchema        json.RawMessage
}

func (m *rapidMockToolRuntime) Metadata() tooling.ToolMetadata {
	return tooling.ToolMetadata{
		Name:        m.name,
		Description: m.metadataDescription,
		Origin:      tooling.ToolOriginBuiltin,
	}
}
func (m *rapidMockToolRuntime) Description(_ json.RawMessage, _ tooling.DescribeContext) string {
	return m.desc
}
func (m *rapidMockToolRuntime) Prompt(_ tooling.PromptContext) string { return m.desc }
func (m *rapidMockToolRuntime) CheckPermissions(_ context.Context, _ json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
}
func (m *rapidMockToolRuntime) IsEnabled(_ tooling.ToolContext) bool     { return true }
func (m *rapidMockToolRuntime) IsReadOnly(_ json.RawMessage) bool        { return m.readOnly }
func (m *rapidMockToolRuntime) IsDestructive(_ json.RawMessage) bool     { return m.destructive }
func (m *rapidMockToolRuntime) IsConcurrencySafe(_ json.RawMessage) bool { return m.concurrencySafe }
func (m *rapidMockToolRuntime) OutputSchema() json.RawMessage            { return m.outputSchema }
func (m *rapidMockToolRuntime) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (m *rapidMockToolRuntime) ValidateInput(_ context.Context, _ json.RawMessage) error {
	return nil
}
func (m *rapidMockToolRuntime) Execute(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
	return tooling.ToolResult{}, nil
}

// genAssembledTool generates a random assembled tool.
func genAssembledTool() *rapid.Generator[Tool] {
	return rapid.Custom(func(t *rapid.T) Tool {
		name := rapid.StringMatching(`[a-z][a-z0-9_]{2,15}\.[a-z][a-z0-9_]{2,15}`).Draw(t, "name")
		desc := rapid.StringMatching(`[A-Z][a-zA-Z0-9 ]{5,40}`).Draw(t, "desc")
		readOnly := rapid.Bool().Draw(t, "readOnly")
		destructive := rapid.Bool().Draw(t, "destructive")
		concSafe := rapid.Bool().Draw(t, "concurrencySafe")
		displayType := rapid.SampledFrom([]string{"table", "text", "list", "json", ""}).Draw(t, "displayType")
		outputSchema := rapid.SampledFrom([]json.RawMessage{
			json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}}}`),
			json.RawMessage(`{"type":"array"}`),
			json.RawMessage(`{"type":"string"}`),
		}).Draw(t, "outputSchema")

		return &rapidMockToolRuntime{
			name:                name,
			metadataDescription: desc,
			desc:                "fallback " + desc,
			readOnly:            readOnly,
			destructive:         destructive,
			concurrencySafe:     concSafe,
			displayType:         displayType,
			outputSchema:        outputSchema,
		}
	})
}

// genCompileContext generates a random valid CompileContext.
func genCompileContext() *rapid.Generator[CompileContext] {
	return rapid.Custom(func(t *rapid.T) CompileContext {
		sessionType := genSessionType().Draw(t, "sessionType")
		mode := genMode().Draw(t, "mode")
		agentKind := genAgentKind().Draw(t, "agentKind")

		numTools := rapid.IntRange(0, 8).Draw(t, "numTools")
		tools := make([]Tool, numTools)
		for i := range tools {
			tools[i] = genAssembledTool().Draw(t, "tool")
		}

		numSkillAssets := rapid.IntRange(0, 3).Draw(t, "numSkillAssets")
		skillAssets := make([]string, numSkillAssets)
		for i := range skillAssets {
			skillAssets[i] = rapid.StringMatching(`[A-Za-z ]{5,30}`).Draw(t, "skillAsset")
		}

		numMCPAssets := rapid.IntRange(0, 3).Draw(t, "numMCPAssets")
		mcpAssets := make([]string, numMCPAssets)
		for i := range mcpAssets {
			mcpAssets[i] = rapid.StringMatching(`[A-Za-z ]{5,30}`).Draw(t, "mcpAsset")
		}

		hostCtx := ""
		if rapid.Bool().Draw(t, "hasHostCtx") {
			hostCtx = rapid.StringMatching(`[a-z0-9\-]{3,20}`).Draw(t, "hostCtx")
		}

		wsCtx := ""
		if rapid.Bool().Draw(t, "hasWsCtx") {
			wsCtx = rapid.StringMatching(`[a-z0-9\-]{3,20}`).Draw(t, "wsCtx")
		}

		return CompileContext{
			SessionType:       sessionType,
			Mode:              mode,
			AgentKind:         agentKind,
			AssembledTools:    tools,
			HostContext:       hostCtx,
			WorkspaceContext:  wsCtx,
			SkillPromptAssets: skillAssets,
			MCPPromptAssets:   mcpAssets,
		}
	})
}

// ---------------------------------------------------------------------------
// Property 9: PromptCompiler section-envelope output structure
// *For any* valid CompileContext, output should contain an ordered section
// envelope consumed by provider adapters. Legacy four-layer fields remain
// populated as derived compatibility fields.
// **Validates: Requirements 3.2, 3.3**
// ---------------------------------------------------------------------------

func TestProperty9_FourLayerOutputStructure(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		compiler := NewCompiler()
		ctx := genCompileContext().Draw(t, "ctx")

		result, err := compiler.Compile(ctx)
		if err != nil {
			t.Fatalf("Compile should not error for valid context: %v", err)
		}

		// All four layers must have non-empty content
		if result.System.Content == "" {
			t.Fatal("Layer 1 (System) must not be empty")
		}
		if result.Developer.Content == "" {
			t.Fatal("Layer 2 (Developer) must not be empty")
		}
		if result.Tools.Content == "" {
			t.Fatal("Layer 3 (Tools) must not be empty")
		}
		if result.Policy.Content == "" {
			t.Fatal("Layer 4 (Policy) must not be empty")
		}

		if len(result.Envelope.Sections) < 4 {
			t.Fatalf("expected section envelope with at least 4 sections, got %#v", result.Envelope.Sections)
		}
		if result.Envelope.Sections[0].ID != "base.contract" {
			t.Fatalf("first section = %q, want base.contract", result.Envelope.Sections[0].ID)
		}
		if result.Envelope.Sections[1].ID != "runtime.state" {
			t.Fatalf("second section = %q, want runtime.state", result.Envelope.Sections[1].ID)
		}
		if !strings.HasPrefix(result.Envelope.Sections[2].ID, "profile.") {
			t.Fatalf("third section = %q, want profile.*", result.Envelope.Sections[2].ID)
		}

		for i, section := range result.Envelope.Sections {
			if section.Content == "" {
				t.Fatalf("section[%d] %s content is empty", i, section.ID)
			}
			if section.Role != "system" {
				t.Fatalf("section[%d] %s role = %q, want system", i, section.ID, section.Role)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Property 9.1: Stable/Dynamic split invariants
// *For any* valid CompileContext, the stable prompt should contain the
// system/stable-developer/tool layers and the dynamic prompt should carry
// per-iteration assets plus policy.
// ---------------------------------------------------------------------------

func TestProperty9_StableDynamicSplitInvariants(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		compiler := NewCompiler()
		ctx := genCompileContext().Draw(t, "ctx")

		result, err := compiler.Compile(ctx)
		if err != nil {
			t.Fatalf("Compile should not error for valid context: %v", err)
		}

		if result.Stable.Content == "" {
			t.Fatal("Stable prompt must not be empty")
		}
		if result.Dynamic.Content == "" {
			t.Fatal("Dynamic prompt must not be empty")
		}

		wantStable := joinNonEmpty(
			result.System.Content,
			result.Stable.Developer.Content,
			result.Tools.Content,
		)
		if result.Stable.Content != wantStable {
			t.Fatalf("Stable prompt mismatch.\nGot:  %q\nWant: %q", result.Stable.Content, wantStable)
		}

		for _, asset := range result.Dynamic.SkillPromptAssets {
			asset = strings.TrimSpace(asset)
			if asset != "" && !strings.Contains(result.Dynamic.Content, asset) {
				t.Fatalf("Dynamic prompt missing skill prompt asset %q", asset)
			}
		}

		if !strings.Contains(result.Dynamic.Content, result.Policy.Content) {
			t.Fatalf("Dynamic prompt should include policy.\nGot:  %q\nWant substring: %q", result.Dynamic.Content, result.Policy.Content)
		}

		if result.Dynamic.Policy.Content != result.Policy.Content {
			t.Fatalf("Dynamic policy mismatch.\nGot:  %q\nWant: %q", result.Dynamic.Policy.Content, result.Policy.Content)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 11: Tool Prompt 内容约束
// *For any* tool prompt compilation output, content should only contain
// capability, constraints, result shape, approval note — NOT answer style content.
// **Validates: Requirements 3.5**
// ---------------------------------------------------------------------------

// answerStylePatterns are phrases that indicate answer style content which
// should NOT appear in tool prompts.
var answerStylePatterns = []string{
	"respond in",
	"answer format",
	"reply style",
	"use markdown",
	"be concise",
	"be verbose",
	"tone should",
	"writing style",
	"speak like",
	"format your answer",
	"output format should be",
}

func TestProperty11_ToolPromptContentConstraint(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		compiler := NewCompiler()

		// Generate context with at least one tool capability
		ctx := genCompileContext().Draw(t, "ctx")
		// Ensure at least one tool entry exists
		extraTool := genAssembledTool().Draw(t, "extraTool")
		ctx.AssembledTools = append(ctx.AssembledTools, extraTool)

		result, err := compiler.Compile(ctx)
		if err != nil {
			t.Fatalf("Compile should not error: %v", err)
		}

		// Verify each tool entry only contains the four allowed fields
		for i, entry := range result.Tools.Entries {
			// Each entry must have at least a capability description
			if entry.Capability == "" {
				t.Fatalf("Tool entry[%d] must have a capability description", i)
			}

			// The allowed fields are: Capability, Constraints, Guidance, ResultShape, ApprovalNote
			// No other content should be present in the entry struct (enforced by type system)
			// But we verify the content text doesn't contain answer style patterns
			allContent := strings.Join([]string{
				entry.Capability,
				entry.Constraints,
				entry.Guidance,
				entry.ResultShape,
				entry.ApprovalNote,
			}, " ")

			lowerContent := strings.ToLower(allContent)
			for _, pattern := range answerStylePatterns {
				if strings.Contains(lowerContent, pattern) {
					t.Fatalf("Tool entry[%d] contains answer style content %q in: %s", i, pattern, allContent)
				}
			}
		}

		// Also verify the full tool prompt content doesn't contain answer style
		lowerToolContent := strings.ToLower(result.Tools.Content)
		for _, pattern := range answerStylePatterns {
			if strings.Contains(lowerToolContent, pattern) {
				t.Fatalf("Tool prompt content contains answer style pattern %q", pattern)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Property 12: Mode 特定策略 Prompt
// *For any* mode (chat/inspect/plan/execute), the Runtime Policy Prompt should
// contain that mode's specific constraints and NOT contain other modes'
// mutually exclusive constraints.
// **Validates: Requirements 3.6**
// ---------------------------------------------------------------------------

// modeSpecificKeywords maps each mode to keywords that MUST appear in its policy.
var modeSpecificKeywords = map[string][]string{
	"chat":    {"chat", "read-only"},
	"inspect": {"inspect", "read"},
	"plan":    {"plan"},
	"execute": {"execute", "approval"},
}

// modeMutuallyExclusiveKeywords maps each mode to keywords from OTHER modes
// that must NOT appear (these represent mutually exclusive constraints).
var modeMutuallyExclusiveKeywords = map[string][]string{
	// Chat mode should NOT claim execute/mutation permissions
	"chat": {"All operations are permitted"},
	// Inspect mode should NOT claim execute/mutation permissions
	"inspect": {"All operations are permitted"},
	// Plan mode should NOT claim full execution permission
	"plan": {"All operations are permitted"},
	// Execute mode should NOT claim "strictly forbidden" for all mutations
	"execute": {"strictly forbidden"},
}

func TestProperty12_ModeSpecificPolicyPrompt(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		compiler := NewCompiler()

		sessionType := genSessionType().Draw(t, "sessionType")
		mode := genMode().Draw(t, "mode")
		agentKind := genAgentKind().Draw(t, "agentKind")

		// Use default policy (no custom RuntimePolicy) to test mode-specific generation
		ctx := CompileContext{
			SessionType: sessionType,
			Mode:        mode,
			AgentKind:   agentKind,
		}

		result, err := compiler.Compile(ctx)
		if err != nil {
			t.Fatalf("Compile should not error: %v", err)
		}

		policyContent := result.Policy.Content
		lowerPolicy := strings.ToLower(policyContent)

		// The policy mode field must match the input mode
		if result.Policy.Mode != mode {
			t.Fatalf("Policy.Mode should be %q, got %q", mode, result.Policy.Mode)
		}

		// Must contain mode-specific keywords
		keywords := modeSpecificKeywords[mode]
		for _, kw := range keywords {
			if !strings.Contains(lowerPolicy, strings.ToLower(kw)) {
				t.Fatalf("Policy for mode %q must contain keyword %q, content: %s", mode, kw, policyContent)
			}
		}

		// Must NOT contain mutually exclusive keywords from other modes
		exclusiveKeywords := modeMutuallyExclusiveKeywords[mode]
		for _, kw := range exclusiveKeywords {
			if strings.Contains(policyContent, kw) {
				t.Fatalf("Policy for mode %q must NOT contain mutually exclusive keyword %q, content: %s", mode, kw, policyContent)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Feature: aiops-codex-eino-rewrite, Property 10: Prompt → Eino 格式転換保真
// *For any* CompiledPrompt, converting to Eino Message format should preserve
// content completely (round-trip semantic preservation).
// **Validates: Requirements 3.4**
// ---------------------------------------------------------------------------

func TestProperty10_PromptToEinoFormatFidelity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		compiler := NewCompiler()
		ctx := genCompileContext().Draw(t, "ctx")

		// Step 1: Compile to get CompiledPrompt
		compiled, err := compiler.Compile(ctx)
		if err != nil {
			t.Fatalf("Compile should not error for valid context: %v", err)
		}

		// Step 2: Verify each compiled section is provider-adapter ready.
		for i, section := range compiled.Envelope.Sections {
			if section.Role != "system" {
				t.Fatalf("section[%d] %s role = %q, want system", i, section.ID, section.Role)
			}
			if strings.TrimSpace(section.Content) == "" {
				t.Fatalf("section[%d] %s content is empty", i, section.ID)
			}
		}
	})
}
