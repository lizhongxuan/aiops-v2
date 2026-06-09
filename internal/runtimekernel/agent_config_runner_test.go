package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/tooling"
)

func TestAgentConfigRunnerExecutesConfiguredChildTurn(t *testing.T) {
	toolCalls := 0
	assembledTool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_disk_usage",
			Description: "read disk usage from the target host",
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			toolCalls++
			return tooling.ToolResult{Content: `{"usage":"12%"}`}, nil
		},
	}
	runtimeTools := tooling.AssembleEinoToolPool([]tooling.Tool{assembledTool})

	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:       "call-disk",
					Type:     "function",
					Function: schema.FunctionCall{Name: "read_disk_usage", Arguments: `{}`},
				},
			}),
			schema.AssistantMessage("disk usage checked", nil),
		},
	}

	runner := NewAgentConfigRunner(AgentConfigRunnerConfig{
		Policy:    &policyengine.Engine{ModePolicy: policyengine.NewDefaultModePolicies()},
		Projector: &testMockEventEmitter{},
	})

	output, err := runner.Run(context.Background(), agentConfigRunnerTestConfig{
		kind:           "host",
		model:          model,
		instructions:   []*schema.Message{{Role: schema.System, Content: "You are a host-agent. Use only host tools."}},
		tools:          runtimeTools,
		assembledTools: []tooling.Tool{assembledTool},
		maxIterations:  3,
		hostID:         "host-120-77-239-90",
		sessionID:      "host-agent-session",
		input:          "check disk usage",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if output != "disk usage checked" {
		t.Fatalf("Run() output = %q, want disk usage checked", output)
	}
	if toolCalls != 1 {
		t.Fatalf("tool calls = %d, want 1", toolCalls)
	}
	if got := schemaMessagesText(flattenMessageBatches(model.inputs)); !strings.Contains(got, "You are a host-agent") || !strings.Contains(got, "check disk usage") || !strings.Contains(got, "12%") {
		t.Fatalf("model inputs did not include child instructions, user task, and tool result:\n%s", got)
	}
}

func TestAgentConfigRunnerBindsOnlyConfiguredChildTools(t *testing.T) {
	childCalls := 0
	childTool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "read_host", Description: "read host"},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			childCalls++
			return tooling.ToolResult{Content: "child ok"}, nil
		},
	}
	managerTool := &tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "spawn_host_agent", Description: "spawn child"}}
	runtimeTools := tooling.AssembleEinoToolPool([]tooling.Tool{childTool})

	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{
			{
				ID:       "call-child",
				Type:     "function",
				Function: schema.FunctionCall{Name: "read_host", Arguments: `{}`},
			},
		}),
		schema.AssistantMessage("ok", nil),
	}}
	runner := NewAgentConfigRunner(AgentConfigRunnerConfig{
		Policy:    &policyengine.Engine{ModePolicy: policyengine.NewDefaultModePolicies()},
		Projector: &testMockEventEmitter{},
	})

	_, err := runner.Run(context.Background(), agentConfigRunnerTestConfig{
		kind:           "host",
		model:          model,
		instructions:   []*schema.Message{{Role: schema.System, Content: "host child"}},
		tools:          runtimeTools,
		assembledTools: []tooling.Tool{childTool},
		maxIterations:  2,
		hostID:         "host-a",
		sessionID:      "host-agent-toolset-session",
		input:          "inspect",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if childCalls != 1 {
		t.Fatalf("child tool calls = %d, want 1", childCalls)
	}

	managerModel := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{
			{
				ID:       "call-manager",
				Type:     "function",
				Function: schema.FunctionCall{Name: managerTool.Metadata().Name, Arguments: `{}`},
			},
		}),
		schema.AssistantMessage("manager tool unavailable", nil),
	}}
	_, err = runner.Run(context.Background(), agentConfigRunnerTestConfig{
		kind:           "host",
		model:          managerModel,
		instructions:   []*schema.Message{{Role: schema.System, Content: "host child"}},
		tools:          runtimeTools,
		assembledTools: []tooling.Tool{childTool},
		maxIterations:  2,
		hostID:         "host-a",
		sessionID:      "host-agent-manager-tool-session",
		input:          "inspect",
	})
	if err != nil {
		t.Fatalf("Run() manager tool call error = %v", err)
	}
	if got := schemaMessagesText(flattenMessageBatches(managerModel.inputs)); !strings.Contains(got, managerTool.Metadata().Name) || !strings.Contains(strings.ToLower(got), "not found") {
		t.Fatalf("manager tool call was not surfaced as unavailable tool result:\n%s", got)
	}
}

func flattenMessageBatches(batches [][]*schema.Message) []*schema.Message {
	var out []*schema.Message
	for _, batch := range batches {
		out = append(out, batch...)
	}
	return out
}

type agentConfigRunnerTestConfig struct {
	kind           string
	model          modelrouter.ChatModel
	instructions   []*schema.Message
	tools          []tool.BaseTool
	assembledTools []tooling.Tool
	maxIterations  int
	hostID         string
	missionID      string
	sessionID      string
	input          string
	metadata       map[string]string
}

func (c agentConfigRunnerTestConfig) RuntimeKind() string {
	return c.kind
}

func (c agentConfigRunnerTestConfig) RuntimeModel() modelrouter.ChatModel {
	return c.model
}

func (c agentConfigRunnerTestConfig) RuntimeInstructions() []*schema.Message {
	return c.instructions
}

func (c agentConfigRunnerTestConfig) RuntimeTools() []tool.BaseTool {
	return c.tools
}

func (c agentConfigRunnerTestConfig) RuntimeAssembledTools() []tooling.Tool {
	return c.assembledTools
}

func (c agentConfigRunnerTestConfig) RuntimeMaxIterations() int {
	return c.maxIterations
}

func (c agentConfigRunnerTestConfig) RuntimeHostID() string {
	return c.hostID
}

func (c agentConfigRunnerTestConfig) RuntimeMissionID() string {
	return c.missionID
}

func (c agentConfigRunnerTestConfig) RuntimeSessionID() string {
	return c.sessionID
}

func (c agentConfigRunnerTestConfig) RuntimeInput() string {
	return c.input
}

func (c agentConfigRunnerTestConfig) RuntimeMetadata() map[string]string {
	return c.metadata
}
