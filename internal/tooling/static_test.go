package tooling

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestVisibilityAllowsConfiguredSessionsAndModes(t *testing.T) {
	t.Parallel()

	visible := Visibility{
		SessionTypes: []string{"workspace"},
		Modes:        []string{"execute"},
	}

	if visible.Allows("host", "execute") {
		t.Fatal("Allows(host, execute) = true, want false")
	}
	if visible.Allows("workspace", "chat") {
		t.Fatal("Allows(workspace, chat) = true, want false")
	}
	if !visible.Allows("workspace", "execute") {
		t.Fatal("Allows(workspace, execute) = false, want true")
	}
}

func TestVisibilityZeroValueAllowsEveryContext(t *testing.T) {
	t.Parallel()

	if !(Visibility{}).Allows("host", "chat") {
		t.Fatal("zero Visibility should allow any session/mode")
	}
}

func TestStaticToolDefaultsToMetadataAndVisibility(t *testing.T) {
	t.Parallel()

	tool := &StaticTool{
		Meta: ToolMetadata{Name: "workspace_read", Description: "Read workspace state."},
		Visibility: Visibility{
			SessionTypes: []string{"workspace"},
			Modes:        []string{"chat"},
		},
		InputSchemaData: json.RawMessage(`{"type":"object"}`),
		ExecuteFunc: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "ok"}, nil
		},
	}

	if tool.Metadata().Name != "workspace_read" {
		t.Fatalf("Metadata().Name = %q, want workspace_read", tool.Metadata().Name)
	}
	if tool.Description(nil, DescribeContext{}) != "Read workspace state." {
		t.Fatalf("Description() = %q, want metadata description", tool.Description(nil, DescribeContext{}))
	}
	if tool.Prompt(PromptContext{}) != "Read workspace state." {
		t.Fatalf("Prompt() = %q, want metadata description", tool.Prompt(PromptContext{}))
	}
	if tool.IsEnabled(ToolContext{SessionType: "host", Mode: "chat"}) {
		t.Fatal("IsEnabled(host/chat) = true, want false")
	}
	if !tool.IsEnabled(ToolContext{SessionType: "workspace", Mode: "chat"}) {
		t.Fatal("IsEnabled(workspace/chat) = false, want true")
	}
	if err := tool.ValidateInput(context.Background(), nil); err != nil {
		t.Fatalf("ValidateInput() error = %v, want nil", err)
	}
	decision := tool.CheckPermissions(context.Background(), nil)
	if decision.Action != PermissionActionAllow {
		t.Fatalf("CheckPermissions().Action = %q, want %q", decision.Action, PermissionActionAllow)
	}
	if tool.IsReadOnly(nil) {
		t.Fatal("IsReadOnly() = true, want false by default")
	}
	if tool.IsDestructive(nil) {
		t.Fatal("IsDestructive() = true, want false by default")
	}
	if tool.IsConcurrencySafe(nil) {
		t.Fatal("IsConcurrencySafe() = true, want false by default")
	}
}

func TestStaticToolDelegatesOptionalHooks(t *testing.T) {
	t.Parallel()

	validateErr := errors.New("bad input")
	permission := PermissionDecision{Action: PermissionActionNeedApproval, Reason: "confirm"}
	tool := &StaticTool{
		Meta:             ToolMetadata{Name: "custom"},
		OutputSchemaData: json.RawMessage(`{"type":"string"}`),
		DescriptionFunc: func(input json.RawMessage, ctx DescribeContext) string {
			if string(input) != `{"x":1}` {
				t.Fatalf("Description input = %s, want {\"x\":1}", string(input))
			}
			if ctx.Metadata.Name != "custom" {
				t.Fatalf("Description ctx.Metadata.Name = %q, want custom", ctx.Metadata.Name)
			}
			return "custom description"
		},
		PromptFunc: func(ctx PromptContext) string {
			if ctx.Metadata.Name != "custom" {
				t.Fatalf("Prompt ctx.Metadata.Name = %q, want custom", ctx.Metadata.Name)
			}
			return "custom prompt"
		},
		EnabledFunc: func(ctx ToolContext) bool {
			return ctx.SessionType == "host" && ctx.Mode == "execute"
		},
		ReadOnlyFunc:         func(json.RawMessage) bool { return true },
		DestructiveFunc:      func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc:  func(json.RawMessage) bool { return true },
		ValidateInputFunc:    func(context.Context, json.RawMessage) error { return validateErr },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) PermissionDecision { return permission },
		ExecuteFunc: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "done"}, nil
		},
	}

	if got := tool.Description(json.RawMessage(`{"x":1}`), DescribeContext{Metadata: tool.Metadata()}); got != "custom description" {
		t.Fatalf("Description() = %q, want custom description", got)
	}
	if got := tool.Prompt(PromptContext{Metadata: tool.Metadata()}); got != "custom prompt" {
		t.Fatalf("Prompt() = %q, want custom prompt", got)
	}
	if !tool.IsEnabled(ToolContext{SessionType: "host", Mode: "execute"}) {
		t.Fatal("IsEnabled(host/execute) = false, want true")
	}
	if tool.IsEnabled(ToolContext{SessionType: "host", Mode: "chat"}) {
		t.Fatal("IsEnabled(host/chat) = true, want false")
	}
	if !tool.IsReadOnly(nil) {
		t.Fatal("IsReadOnly() = false, want true")
	}
	if !tool.IsDestructive(nil) {
		t.Fatal("IsDestructive() = false, want true")
	}
	if !tool.IsConcurrencySafe(nil) {
		t.Fatal("IsConcurrencySafe() = false, want true")
	}
	if err := tool.ValidateInput(context.Background(), nil); !errors.Is(err, validateErr) {
		t.Fatalf("ValidateInput() error = %v, want %v", err, validateErr)
	}
	if got := tool.CheckPermissions(context.Background(), nil); got != permission {
		t.Fatalf("CheckPermissions() = %#v, want %#v", got, permission)
	}
	if got := tool.OutputSchema(); string(got) != `{"type":"string"}` {
		t.Fatalf("OutputSchema() = %s, want string schema", string(got))
	}
}

func TestStaticToolExecuteWithoutHandlerReturnsError(t *testing.T) {
	t.Parallel()

	tool := &StaticTool{Meta: ToolMetadata{Name: "missing_handler"}}
	if _, err := tool.Execute(context.Background(), nil); err == nil {
		t.Fatal("Execute() error = nil, want missing handler error")
	}
}
