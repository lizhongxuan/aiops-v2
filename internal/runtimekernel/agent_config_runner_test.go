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
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
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

func TestAgentConfigRunnerPrefersTypedEnvelopeAndRebuildsCurrentStepFacts(t *testing.T) {
	staleTool := &tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "stale_tool_marker", Description: "stale tool surface"}}
	upstream, err := promptcompiler.NewCompiler().Compile(promptcompiler.CompileContext{
		SessionType:          "host",
		Mode:                 "execute",
		HostContext:          "stale-host-marker",
		RuntimePolicy:        "stale_runtime_marker",
		HostTaskPromptAssets: []string{"delegated_task_marker"},
		AssembledTools:       []tooling.Tool{staleTool},
	})
	if err != nil {
		t.Fatalf("upstream Compile() error = %v", err)
	}

	currentTool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "current_tool_marker", Description: "current tool surface"},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "current tool evidence"}, nil
		},
	}
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:       "call-current-tool",
			Type:     "function",
			Function: schema.FunctionCall{Name: "current_tool_marker", Arguments: `{}`},
		}}),
		schema.AssistantMessage("typed envelope ok", nil),
	}}
	runner := NewAgentConfigRunner(AgentConfigRunnerConfig{
		Policy:    &policyengine.Engine{ModePolicy: policyengine.NewDefaultModePolicies()},
		Projector: &testMockEventEmitter{},
	})
	output, err := runner.Run(context.Background(), agentConfigRunnerTestConfig{
		kind:             "host",
		model:            model,
		promptEnvelopeV2: upstream.EnvelopeV2,
		instructions:     []*schema.Message{{Role: schema.System, Content: "legacy_override_marker"}},
		assembledTools:   []tooling.Tool{currentTool},
		maxIterations:    1,
		hostID:           "current-host-marker",
		sessionID:        "typed-envelope-child-session",
		input:            "execute delegated task",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if output != "typed envelope ok" {
		t.Fatalf("Run() output = %q", output)
	}

	got := schemaMessagesText(flattenMessageBatches(model.inputs))
	for _, want := range []string{"delegated_task_marker", "current_tool_marker", "current-host-marker"} {
		if !strings.Contains(got, want) {
			t.Fatalf("current typed model input missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"legacy_override_marker", "stale_tool_marker", "stale_runtime_marker", "stale-host-marker"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("typed envelope allowed stale/legacy fact %q to override current step:\n%s", forbidden, got)
		}
	}
}

func TestAgentConfigRunnerRejectsInvalidTypedEnvelopeWithoutLegacyFallback(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("must not run", nil)}}
	runner := NewAgentConfigRunner(AgentConfigRunnerConfig{
		Policy:    &policyengine.Engine{ModePolicy: policyengine.NewDefaultModePolicies()},
		Projector: &testMockEventEmitter{},
	})
	_, err := runner.Run(context.Background(), agentConfigRunnerTestConfig{
		kind:             "host",
		model:            model,
		promptEnvelopeV2: promptcompiler.PromptEnvelopeV2{SchemaVersion: promptcompiler.PromptEnvelopeV2SchemaVersion},
		instructions:     []*schema.Message{{Role: schema.System, Content: "legacy fallback must not mask invalid typed input"}},
		hostID:           "host-a",
		sessionID:        "invalid-typed-envelope-session",
		input:            "inspect",
	})
	if err == nil || !strings.Contains(err.Error(), "prompt envelope v2") {
		t.Fatalf("Run() error = %v, want invalid typed envelope rejection", err)
	}
	if len(model.inputs) != 0 {
		t.Fatalf("model calls = %d, want fail closed before provider", len(model.inputs))
	}
}

func TestFixedAgentCompilerProducesSectionEnvelope(t *testing.T) {
	instructionContext := classifyFixedAgentInstructions([]*schema.Message{{Role: schema.System, Content: "host child instructions"}})
	compiler := fixedAgentCompiler{
		roleContent: instructionContext.Role, dynamicContent: instructionContext.Dynamic,
	}
	compiled, err := compiler.Compile(promptcompiler.CompileContext{
		Mode:          "execute",
		RuntimePolicy: "mode: execute",
		AssembledTools: []tooling.Tool{
			&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "read_host", Description: "read host"}},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compiled.Envelope.Sections) != 3 {
		t.Fatalf("sections = %#v, want base/tool/runtime", compiled.Envelope.Sections)
	}
	var layers []string
	for _, section := range compiled.Envelope.Sections {
		layers = append(layers, section.ID)
	}
	if got := strings.Join(layers, ","); got != "base.contract,tool.surface,runtime.state" {
		t.Fatalf("prompt layers = %q", got)
	}
	if strings.Contains(strings.Join(layers, ","), "system") {
		t.Fatalf("fixed child compiler should not use legacy prompt layers: %#v", layers)
	}
	if err := compiled.EnvelopeV2.Validate(); err != nil {
		t.Fatalf("EnvelopeV2.Validate() error = %v", err)
	}
	if strings.Contains(compiled.EnvelopeV2.Sections[1].Content, "host child instructions") {
		t.Fatalf("untyped child instructions leaked into L1: %#v", compiled.EnvelopeV2.Sections[1])
	}
	if !strings.Contains(promptEnvelopeV2LayerText(compiled, promptcompiler.LayerStepDynamicContext), "host child instructions") {
		t.Fatalf("child instructions missing from L5: %#v", compiled.EnvelopeV2.Sections)
	}
}

func TestFixedAgentCompilerReclassifiesLegacyAgentConfigInstructions(t *testing.T) {
	upstream, err := promptcompiler.NewCompiler().Compile(promptcompiler.CompileContext{
		SessionType: "host", Mode: "execute", HostContext: "stale-host",
		RuntimePolicy:        "stale_runtime_marker",
		HostTaskPromptAssets: []string{"delegated_host_task_marker"},
		AssembledTools: []tooling.Tool{
			&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "stale_tool", Description: "stale tool surface"}},
		},
	})
	if err != nil {
		t.Fatalf("upstream Compile() error = %v", err)
	}
	legacy, err := promptinput.Builder{}.Build(promptinput.BuildRequest{Compiled: upstream})
	if err != nil {
		t.Fatalf("legacy Builder.Build() error = %v", err)
	}
	instructions, _, err := modelrouter.ModelInputItemsToEinoMessages(legacy.Items)
	if err != nil {
		t.Fatalf("ModelInputItemsToEinoMessages() error = %v", err)
	}
	instructionContext := classifyFixedAgentInstructions(instructions)
	compiled, err := (fixedAgentCompiler{
		roleContent: instructionContext.Role, dynamicContent: instructionContext.Dynamic,
	}).Compile(promptcompiler.CompileContext{
		SessionType: "host", Mode: "execute", HostContext: "current-host",
		RuntimePolicy: "current_runtime_marker",
		AssembledTools: []tooling.Tool{
			&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "current_tool", Description: "current tool surface"}},
		},
	})
	if err != nil {
		t.Fatalf("fixed Compile() error = %v", err)
	}
	l1 := promptEnvelopeV2LayerText(compiled, promptcompiler.LayerRoleProfileCore)
	l5 := promptEnvelopeV2LayerText(compiled, promptcompiler.LayerStepDynamicContext)
	for _, forbidden := range []string{"stale_tool", "stale_runtime_marker", "delegated_host_task_marker", "current_tool", "current_runtime_marker", "stale-host", "current-host"} {
		if strings.Contains(l1, forbidden) {
			t.Fatalf("L1 leaked %q:\n%s", forbidden, l1)
		}
	}
	if strings.Contains(l5, "stale_tool") || strings.Contains(l5, "stale_runtime_marker") {
		t.Fatalf("L5 retained stale rebuilt facts:\n%s", l5)
	}
	for marker, want := range map[string]int{
		"delegated_host_task_marker": 1,
		"current_tool":               1,
		"current_runtime_marker":     1,
	} {
		if got := strings.Count(l5, marker); got != want {
			t.Fatalf("L5 marker %q count = %d, want %d:\n%s", marker, got, want, l5)
		}
	}
}

func promptEnvelopeV2LayerText(compiled promptcompiler.CompiledPrompt, layer promptcompiler.PromptLogicalLayer) string {
	var parts []string
	for _, section := range compiled.EnvelopeV2.Sections {
		if section.LogicalLayer == layer {
			parts = append(parts, section.Content)
		}
	}
	return strings.Join(parts, "\n")
}

func flattenMessageBatches(batches [][]*schema.Message) []*schema.Message {
	var out []*schema.Message
	for _, batch := range batches {
		out = append(out, batch...)
	}
	return out
}

type agentConfigRunnerTestConfig struct {
	kind             string
	model            modelrouter.ChatModel
	promptEnvelopeV2 promptcompiler.PromptEnvelopeV2
	instructions     []*schema.Message
	tools            []tool.BaseTool
	assembledTools   []tooling.Tool
	maxIterations    int
	hostID           string
	missionID        string
	sessionID        string
	input            string
	metadata         map[string]string
}

func (c agentConfigRunnerTestConfig) RuntimePromptEnvelopeV2() promptcompiler.PromptEnvelopeV2 {
	return c.promptEnvelopeV2
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
