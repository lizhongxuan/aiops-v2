package tooling

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

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
		Name:        ProviderSafeToolName(meta.Name),
		Desc:        desc,
		ParamsOneOf: paramsOneOf,
	}
}

var (
	_ tool.BaseTool      = (*einoToolAdapter)(nil)
	_ tool.InvokableTool = (*einoToolAdapter)(nil)
)
