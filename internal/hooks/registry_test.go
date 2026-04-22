package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aiops-v2/internal/settings"
	"aiops-v2/internal/tooling"
)

type stubTool struct {
	meta tooling.ToolMetadata
}

func (t stubTool) Metadata() tooling.ToolMetadata                              { return t.meta }
func (t stubTool) InputSchema() json.RawMessage                                { return nil }
func (t stubTool) OutputSchema() json.RawMessage                               { return nil }
func (t stubTool) Description(json.RawMessage, tooling.DescribeContext) string { return "" }
func (t stubTool) Prompt(tooling.PromptContext) string                         { return "" }
func (t stubTool) IsEnabled(tooling.ToolContext) bool                          { return true }
func (t stubTool) IsReadOnly(json.RawMessage) bool                             { return false }
func (t stubTool) IsDestructive(json.RawMessage) bool                          { return false }
func (t stubTool) IsConcurrencySafe(json.RawMessage) bool                      { return true }
func (t stubTool) ValidateInput(context.Context, json.RawMessage) error        { return nil }
func (t stubTool) CheckPermissions(context.Context, json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
}
func (t stubTool) Execute(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	return tooling.ToolResult{}, nil
}

func TestRegistryRegisterValidation(t *testing.T) {
	r := NewRegistry()

	if err := r.RegisterTool(ToolRegistration{Hook: func(context.Context, *ToolEvent) error { return nil }}); err == nil {
		t.Fatal("expected error for empty name and stage")
	}
	if err := r.RegisterTool(ToolRegistration{Name: "tool"}); err == nil {
		t.Fatal("expected error for missing stage and hook")
	}
	if err := r.RegisterTurn(TurnRegistration{Name: "turn"}); err == nil {
		t.Fatal("expected error for missing stage and hook")
	}
}

func TestRegistryRunToolStageMatchesAliasAndInputContains(t *testing.T) {
	r := NewRegistry()
	var calls []string

	if err := r.RegisterTool(ToolRegistration{
		Name:  "match",
		Stage: StagePreToolUse,
		Matcher: ToolMatcher{
			ToolNames:     []string{"canonical", "alias"},
			Sources:       []tooling.ToolSource{tooling.ToolSourceBuiltin},
			Modes:         []string{"mode-a"},
			SessionTypes:  []string{"session-a"},
			InputContains: []string{"needle"},
		},
		Hook: func(_ context.Context, _ *ToolEvent) error {
			calls = append(calls, "matched")
			return nil
		},
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	if err := r.RegisterTool(ToolRegistration{
		Name:  "miss",
		Stage: StagePreToolUse,
		Matcher: ToolMatcher{
			ToolNames: []string{"different"},
		},
		Hook: func(_ context.Context, _ *ToolEvent) error {
			calls = append(calls, "miss")
			return nil
		},
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	event := ToolEvent{
		SessionID:   "s-1",
		TurnID:      "t-1",
		SessionType: "session-a",
		Mode:        "mode-a",
		Tool:        tooling.ToolMetadata{Name: "canonical", Aliases: []string{"alias"}},
		Arguments:   json.RawMessage(`{"message":"contains the needle here"}`),
	}
	err := r.RunToolStage(context.Background(), StagePreToolUse, &event)
	if err != nil {
		t.Fatalf("run tool stage: %v", err)
	}

	if len(calls) != 1 || calls[0] != "matched" {
		t.Fatalf("expected only matching hook to run, got %v", calls)
	}
}

func TestToolMatcherMatchesSourceTraitsWithoutOrigin(t *testing.T) {
	matcher := ToolMatcher{
		Sources: []tooling.ToolSource{tooling.ToolSourceMCP},
	}

	if !matcher.Matches(ToolEvent{
		Tool: tooling.ToolMetadata{Name: "coroot.query", IsMCP: true},
	}) {
		t.Fatal("expected MCP trait matcher to work without origin")
	}
}

func TestRegistryRunToolStageIsolatedByStage(t *testing.T) {
	r := NewRegistry()
	var calls []string

	if err := r.RegisterTool(ToolRegistration{
		Name:  "pre",
		Stage: StagePreToolUse,
		Hook: func(_ context.Context, _ *ToolEvent) error {
			calls = append(calls, "pre")
			return nil
		},
	}); err != nil {
		t.Fatalf("register pre: %v", err)
	}
	if err := r.RegisterTool(ToolRegistration{
		Name:  "post",
		Stage: StagePostToolUse,
		Hook: func(_ context.Context, _ *ToolEvent) error {
			calls = append(calls, "post")
			return nil
		},
	}); err != nil {
		t.Fatalf("register post: %v", err)
	}

	event := ToolEvent{}
	if err := r.RunToolStage(context.Background(), StagePreToolUse, &event); err != nil {
		t.Fatalf("run pre stage: %v", err)
	}
	if len(calls) != 1 || calls[0] != "pre" {
		t.Fatalf("expected only pre hook to run, got %v", calls)
	}
}

func TestRegistryRunToolStageShortCircuitsOnError(t *testing.T) {
	r := NewRegistry()
	var secondCalled bool

	if err := r.RegisterTool(ToolRegistration{
		Name:  "first",
		Stage: StagePreToolUse,
		Hook: func(context.Context, *ToolEvent) error {
			return errors.New("boom")
		},
	}); err != nil {
		t.Fatalf("register first: %v", err)
	}
	if err := r.RegisterTool(ToolRegistration{
		Name:  "second",
		Stage: StagePreToolUse,
		Hook: func(context.Context, *ToolEvent) error {
			secondCalled = true
			return nil
		},
	}); err != nil {
		t.Fatalf("register second: %v", err)
	}

	event := ToolEvent{}
	err := r.RunToolStage(context.Background(), StagePreToolUse, &event)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected hook error, got %v", err)
	}
	if secondCalled {
		t.Fatal("expected execution to stop after first error")
	}
}

func TestRegistryRunTurnStage(t *testing.T) {
	r := NewRegistry()
	var got TurnEvent

	if err := r.RegisterTurn(TurnRegistration{
		Name:  "turn",
		Stage: StagePreTurn,
		Hook: func(_ context.Context, event *TurnEvent) error {
			got = *event
			return nil
		},
	}); err != nil {
		t.Fatalf("register turn: %v", err)
	}

	event := TurnEvent{
		SessionID:   "s-1",
		TurnID:      "t-1",
		SessionType: "agent",
		Mode:        "plan",
		Input:       "hello",
	}
	if err := r.RunTurnStage(context.Background(), StagePreTurn, &event); err != nil {
		t.Fatalf("run turn stage: %v", err)
	}

	if got.SessionID != "s-1" || got.TurnID != "t-1" || got.Input != "hello" {
		t.Fatalf("unexpected turn event: %+v", got)
	}
}

func TestRegistryRunToolStageAllowsHookOutputs(t *testing.T) {
	r := NewRegistry()

	if err := r.RegisterTool(ToolRegistration{
		Name:  "rewrite-input",
		Stage: StagePreToolUse,
		Hook: func(_ context.Context, event *ToolEvent) error {
			event.UpdatedInput = json.RawMessage(`{"path":"/tmp/after"}`)
			event.AdditionalContext = append(event.AdditionalContext, "input rewritten")
			event.UpdatedPermissions = &tooling.PermissionDecision{
				Action: tooling.PermissionActionNeedApproval,
				Reason: "approval required by hook",
			}
			event.WatchPaths = append(event.WatchPaths, "/tmp/after")
			return nil
		},
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	event := ToolEvent{
		Arguments: json.RawMessage(`{"path":"/tmp/before"}`),
	}
	if err := r.RunToolStage(context.Background(), StagePreToolUse, &event); err != nil {
		t.Fatalf("RunToolStage failed: %v", err)
	}

	if string(event.UpdatedInput) != `{"path":"/tmp/after"}` {
		t.Fatalf("UpdatedInput = %s", event.UpdatedInput)
	}
	if len(event.AdditionalContext) != 1 || event.AdditionalContext[0] != "input rewritten" {
		t.Fatalf("AdditionalContext = %v", event.AdditionalContext)
	}
	if event.UpdatedPermissions == nil || event.UpdatedPermissions.Action != tooling.PermissionActionNeedApproval {
		t.Fatalf("UpdatedPermissions = %#v", event.UpdatedPermissions)
	}
	if len(event.WatchPaths) != 1 || event.WatchPaths[0] != "/tmp/after" {
		t.Fatalf("WatchPaths = %v", event.WatchPaths)
	}
}

func TestRegistryRunToolStageAllowsOutputRewrite(t *testing.T) {
	r := NewRegistry()

	if err := r.RegisterTool(ToolRegistration{
		Name:  "rewrite-output",
		Stage: StagePostToolUse,
		Hook: func(_ context.Context, event *ToolEvent) error {
			event.UpdatedMCPToolOutput = &tooling.ToolResult{Content: "rewritten by hook"}
			return nil
		},
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	event := ToolEvent{
		Result: &tooling.ToolResult{Content: "original"},
	}
	if err := r.RunToolStage(context.Background(), StagePostToolUse, &event); err != nil {
		t.Fatalf("RunToolStage failed: %v", err)
	}

	if event.UpdatedMCPToolOutput == nil || event.UpdatedMCPToolOutput.Content != "rewritten by hook" {
		t.Fatalf("UpdatedMCPToolOutput = %#v", event.UpdatedMCPToolOutput)
	}
}

func TestRegistryRejectsCustomHooksWhenStrictPluginOnlyEnabled(t *testing.T) {
	governance := settings.NewGovernance()
	if err := governance.Register("managed", settings.GovernanceContribution{
		RestrictToPluginOnly: []settings.CustomizationSurface{settings.SurfaceHooks},
	}); err != nil {
		t.Fatalf("governance Register() error = %v", err)
	}

	r := NewRegistry()
	r.SetGovernance(governance)

	err := r.RegisterTool(ToolRegistration{
		Name:   "custom-hook",
		Source: string(settings.SourceUserSettings),
		Stage:  StagePreToolUse,
		Hook:   func(context.Context, *ToolEvent) error { return nil },
	})
	if err == nil {
		t.Fatal("expected strict plugin-only policy to reject userSettings hook")
	}
	if !strings.Contains(err.Error(), "strictPluginOnlyCustomization") {
		t.Fatalf("expected strict plugin-only error, got %v", err)
	}
}
