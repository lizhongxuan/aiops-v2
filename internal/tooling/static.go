package tooling

import (
	"context"
	"encoding/json"
	"fmt"
)

// Visibility constrains the session types and modes where a tool is enabled.
type Visibility struct {
	SessionTypes []string `json:"sessionTypes,omitempty"`
	Modes        []string `json:"modes,omitempty"`
}

// Allows reports whether the provided session/mode matches the visibility rule.
func (v Visibility) Allows(sessionType, mode string) bool {
	return visibilityMatch(v.SessionTypes, sessionType) && visibilityMatch(v.Modes, mode)
}

func visibilityMatch(allowed []string, actual string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if candidate == actual {
			return true
		}
	}
	return false
}

// StaticTool is a thin helper that wires a native Tool from metadata plus
// optional behavior hooks. It does not introduce a second registry abstraction.
type StaticTool struct {
	Meta             ToolMetadata
	Visibility       Visibility
	InputSchemaData  json.RawMessage
	OutputSchemaData json.RawMessage

	DescriptionFunc      func(input json.RawMessage, ctx DescribeContext) string
	PromptFunc           func(ctx PromptContext) string
	EnabledFunc          func(ctx ToolContext) bool
	ReadOnlyFunc         func(input json.RawMessage) bool
	DestructiveFunc      func(input json.RawMessage) bool
	ConcurrencySafeFunc  func(input json.RawMessage) bool
	ValidateInputFunc    func(ctx context.Context, input json.RawMessage) error
	CheckPermissionsFunc func(ctx context.Context, input json.RawMessage) PermissionDecision
	ExecuteFunc          func(ctx context.Context, input json.RawMessage) (ToolResult, error)
}

func (t *StaticTool) Metadata() ToolMetadata { return t.Meta }

func (t *StaticTool) InputSchema() json.RawMessage { return t.InputSchemaData }

func (t *StaticTool) OutputSchema() json.RawMessage { return t.OutputSchemaData }

func (t *StaticTool) Description(input json.RawMessage, ctx DescribeContext) string {
	ctx = t.describeContext(ctx)
	if t.DescriptionFunc != nil {
		if desc := t.DescriptionFunc(input, ctx); desc != "" {
			return desc
		}
	}
	return t.Meta.Description
}

func (t *StaticTool) Prompt(ctx PromptContext) string {
	ctx = t.promptContext(ctx)
	if t.PromptFunc != nil {
		if prompt := t.PromptFunc(ctx); prompt != "" {
			return prompt
		}
	}
	return t.Description(nil, DescribeContext{
		Context:     ctx.Context,
		SessionType: ctx.SessionType,
		Mode:        ctx.Mode,
		Metadata:    ctx.Metadata,
	})
}

func (t *StaticTool) IsEnabled(ctx ToolContext) bool {
	ctx = t.toolContext(ctx)
	if t.EnabledFunc != nil {
		return t.EnabledFunc(ctx)
	}
	return t.Visibility.Allows(ctx.SessionType, ctx.Mode)
}

func (t *StaticTool) IsReadOnly(input json.RawMessage) bool {
	if t.ReadOnlyFunc != nil {
		return t.ReadOnlyFunc(input)
	}
	return false
}

func (t *StaticTool) IsDestructive(input json.RawMessage) bool {
	if t.DestructiveFunc != nil {
		return t.DestructiveFunc(input)
	}
	return false
}

func (t *StaticTool) IsConcurrencySafe(input json.RawMessage) bool {
	if t.ConcurrencySafeFunc != nil {
		return t.ConcurrencySafeFunc(input)
	}
	return false
}

func (t *StaticTool) ValidateInput(ctx context.Context, input json.RawMessage) error {
	if t.ValidateInputFunc != nil {
		return t.ValidateInputFunc(ctx, input)
	}
	return nil
}

func (t *StaticTool) CheckPermissions(ctx context.Context, input json.RawMessage) PermissionDecision {
	if t.CheckPermissionsFunc != nil {
		return t.CheckPermissionsFunc(ctx, input)
	}
	return PermissionDecision{Action: PermissionActionAllow}
}

func (t *StaticTool) Execute(ctx context.Context, input json.RawMessage) (ToolResult, error) {
	if t.ExecuteFunc == nil {
		name := t.Meta.Name
		if name == "" {
			name = "static tool"
		}
		return ToolResult{}, fmt.Errorf("%s has no ExecuteFunc", name)
	}
	return t.ExecuteFunc(ctx, input)
}

func (t *StaticTool) describeContext(ctx DescribeContext) DescribeContext {
	if ctx.Metadata.Name == "" {
		ctx.Metadata = t.Meta
	}
	return ctx
}

func (t *StaticTool) promptContext(ctx PromptContext) PromptContext {
	if ctx.Metadata.Name == "" {
		ctx.Metadata = t.Meta
	}
	return ctx
}

func (t *StaticTool) toolContext(ctx ToolContext) ToolContext {
	if ctx.Metadata.Name == "" {
		ctx.Metadata = t.Meta
	}
	return ctx
}

var _ Tool = (*StaticTool)(nil)
