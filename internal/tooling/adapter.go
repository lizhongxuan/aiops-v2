package tooling

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

type legacyToolAdapter struct {
	runtime LegacyToolRuntime
	meta    ToolMetadata
}

// NewLegacyToolAdapter adapts a legacy runtime contract to the unified Tool interface.
func NewLegacyToolAdapter(runtime LegacyToolRuntime, meta ToolMetadata) Tool {
	return &legacyToolAdapter{runtime: runtime, meta: meta}
}

func (a *legacyToolAdapter) Metadata() ToolMetadata {
	meta := a.meta
	if meta.Name == "" {
		meta.Name = "legacy-tool"
	}
	if meta.Description == "" {
		meta.Description = a.runtime.Description()
	}
	return meta
}

func (a *legacyToolAdapter) InputSchema() json.RawMessage { return a.runtime.InputSchema() }

func (a *legacyToolAdapter) OutputSchema() json.RawMessage { return nil }

func (a *legacyToolAdapter) Description(_ json.RawMessage, _ DescribeContext) string {
	if a.meta.Description != "" {
		return a.meta.Description
	}
	return a.runtime.Description()
}

func (a *legacyToolAdapter) Prompt(ctx PromptContext) string {
	if a.meta.Description != "" {
		return a.meta.Description
	}
	return a.runtime.Description()
}

func (a *legacyToolAdapter) IsEnabled(ctx ToolContext) bool { return a.runtime.IsEnabled(ctx) }

func (a *legacyToolAdapter) IsReadOnly(_ json.RawMessage) bool { return a.runtime.IsReadOnly() }

func (a *legacyToolAdapter) IsDestructive(_ json.RawMessage) bool { return a.runtime.IsDestructive() }

func (a *legacyToolAdapter) IsConcurrencySafe(_ json.RawMessage) bool {
	return a.runtime.IsConcurrencySafe()
}

func (a *legacyToolAdapter) ValidateInput(_ context.Context, _ json.RawMessage) error { return nil }

func (a *legacyToolAdapter) CheckPermissions(ctx context.Context, _ json.RawMessage) PermissionDecision {
	if err := a.runtime.CheckPermissions(ctx); err != nil {
		return PermissionDecision{Action: PermissionActionDeny, Reason: err.Error()}
	}
	return PermissionDecision{Action: PermissionActionAllow}
}

func (a *legacyToolAdapter) Execute(ctx context.Context, input json.RawMessage) (ToolResult, error) {
	return a.runtime.Execute(ctx, input)
}

type einoToolAdapter struct {
	tool Tool
	info *schema.ToolInfo
}

// AssembleEinoToolPool adapts unified tools into Eino BaseTool instances.
func AssembleEinoToolPool(tools []Tool) []tool.BaseTool {
	pool := make([]tool.BaseTool, 0, len(tools))
	for _, t := range tools {
		pool = append(pool, NewEinoToolAdapter(t))
	}
	return pool
}

// NewEinoToolAdapter adapts a unified Tool to Eino's BaseTool/InvokableTool interfaces.
func NewEinoToolAdapter(t Tool) *einoToolAdapter {
	return &einoToolAdapter{tool: t, info: buildToolInfo(t)}
}

func (a *einoToolAdapter) Info(context.Context) (*schema.ToolInfo, error) { return a.info, nil }

func (a *einoToolAdapter) InvokableRun(ctx context.Context, args string, _ ...tool.Option) (string, error) {
	result, err := a.tool.Execute(ctx, json.RawMessage(args))
	if err != nil {
		return "", fmt.Errorf("tool %q execution failed: %w", a.tool.Metadata().Name, err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("tool %q returned error: %s", a.tool.Metadata().Name, result.Error)
	}
	return result.Content, nil
}

func buildToolInfo(t Tool) *schema.ToolInfo {
	inputSchema := t.InputSchema()
	var paramsOneOf *schema.ParamsOneOf
	if len(inputSchema) > 0 {
		var js jsonschema.Schema
		if err := json.Unmarshal(inputSchema, &js); err == nil {
			paramsOneOf = schema.NewParamsOneOfByJSONSchema(&js)
		}
	}

	meta := t.Metadata()
	desc := meta.Description
	if desc == "" {
		desc = t.Description(nil, DescribeContext{Metadata: meta})
	}

	return &schema.ToolInfo{
		Name:        meta.Name,
		Desc:        desc,
		ParamsOneOf: paramsOneOf,
	}
}

var (
	_ tool.BaseTool      = (*einoToolAdapter)(nil)
	_ tool.InvokableTool = (*einoToolAdapter)(nil)
	_ Tool               = (*legacyToolAdapter)(nil)
)
