package tooling

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNewEinoToolAdapterBuildsInfoFromToolDescription(t *testing.T) {
	t.Parallel()

	tool := &StaticTool{
		Meta: ToolMetadata{Name: "query_services"},
		InputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"service": {"type": "string"}
			}
		}`),
		DescriptionFunc: func(json.RawMessage, DescribeContext) string {
			return "Query service telemetry."
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "ok"}, nil
		},
	}

	adapter := NewEinoToolAdapter(tool)
	info, err := adapter.Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Name != "query_services" {
		t.Fatalf("Info().Name = %q, want query_services", info.Name)
	}
	if info.Desc != "Query service telemetry." {
		t.Fatalf("Info().Desc = %q, want Query service telemetry.", info.Desc)
	}
	if info.ParamsOneOf == nil {
		t.Fatal("Info().ParamsOneOf = nil, want JSON schema params")
	}
}

func TestEinoToolAdapterInvokableRunReturnsToolContent(t *testing.T) {
	t.Parallel()

	tool := &StaticTool{
		Meta: ToolMetadata{Name: "echo", Description: "Echo text."},
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: string(input)}, nil
		},
	}

	got, err := NewEinoToolAdapter(tool).InvokableRun(context.Background(), `{"message":"hi"}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}
	if got != `{"message":"hi"}` {
		t.Fatalf("InvokableRun() = %q, want raw content", got)
	}
}

func TestEinoToolAdapterInvokableRunWrapsToolFailures(t *testing.T) {
	t.Parallel()

	t.Run("execute error", func(t *testing.T) {
		t.Parallel()

		tool := &StaticTool{
			Meta: ToolMetadata{Name: "explode"},
			ExecuteFunc: func(context.Context, json.RawMessage) (ToolResult, error) {
				return ToolResult{}, errors.New("boom")
			},
		}

		_, err := NewEinoToolAdapter(tool).InvokableRun(context.Background(), `{}`)
		if err == nil {
			t.Fatal("InvokableRun() error = nil, want wrapped execute error")
		}
		if !strings.Contains(err.Error(), `tool "explode" execution failed: boom`) {
			t.Fatalf("InvokableRun() error = %q, want wrapped execute error", err.Error())
		}
	})

	t.Run("result error", func(t *testing.T) {
		t.Parallel()

		tool := &StaticTool{
			Meta: ToolMetadata{Name: "reject"},
			ExecuteFunc: func(context.Context, json.RawMessage) (ToolResult, error) {
				return ToolResult{Error: "denied"}, nil
			},
		}

		_, err := NewEinoToolAdapter(tool).InvokableRun(context.Background(), `{}`)
		if err == nil {
			t.Fatal("InvokableRun() error = nil, want tool result error")
		}
		if !strings.Contains(err.Error(), `tool "reject" returned error: denied`) {
			t.Fatalf("InvokableRun() error = %q, want tool result error", err.Error())
		}
	})
}

func TestAssembleEinoToolPoolAdaptsEveryTool(t *testing.T) {
	t.Parallel()

	pool := AssembleEinoToolPool([]Tool{
		&StaticTool{
			Meta: ToolMetadata{Name: "alpha", Description: "A"},
			ExecuteFunc: func(context.Context, json.RawMessage) (ToolResult, error) {
				return ToolResult{Content: "a"}, nil
			},
		},
		&StaticTool{
			Meta: ToolMetadata{Name: "beta", Description: "B"},
			ExecuteFunc: func(context.Context, json.RawMessage) (ToolResult, error) {
				return ToolResult{Content: "b"}, nil
			},
		},
	})

	if len(pool) != 2 {
		t.Fatalf("len(AssembleEinoToolPool()) = %d, want 2", len(pool))
	}
}
