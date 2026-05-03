package runtimekernel

import (
	"strings"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
)

type ModelInputDebugTraceRequest struct {
	SessionID        string
	TurnID           string
	Iteration        int
	Metadata         map[string]string
	Compiled         promptcompiler.CompiledPrompt
	ModelInput       []*schema.Message
	VisibleTools     []string
	PromptInputTrace promptinput.PromptInputTrace
	PromptInputDiff  *promptinput.TraceDiff
}

func buildModelInput(history []Message, compiled promptcompiler.CompiledPrompt) ([]*schema.Message, error) {
	result, err := buildPromptInput(history, compiled)
	if err != nil {
		return nil, err
	}
	return result.Messages, nil
}

func buildPromptInput(history []Message, compiled promptcompiler.CompiledPrompt) (promptinput.BuildResult, error) {
	result, err := promptinput.Builder{}.Build(promptinput.BuildRequest{
		History:  promptInputMessagesFromRuntime(history),
		Compiled: compiled,
	})
	if err != nil {
		return promptinput.BuildResult{}, err
	}
	return result, nil
}

func messagesForCurrentTurnModelInput(history []Message) []Message {
	filtered := promptinput.MessagesForCurrentTurnModelInput(promptInputMessagesFromRuntime(history))
	return runtimeMessagesFromPromptInput(filtered)
}

func promptInputMessagesFromRuntime(history []Message) []promptinput.Message {
	out := make([]promptinput.Message, 0, len(history))
	for _, msg := range history {
		out = append(out, promptinput.Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCalls:  promptInputToolCallsFromRuntime(msg.ToolCalls),
			ToolResult: promptInputToolResultFromRuntime(msg.ToolResult),
		})
	}
	return out
}

func promptInputToolCallsFromRuntime(toolCalls []ToolCall) []promptinput.ToolCall {
	out := make([]promptinput.ToolCall, 0, len(toolCalls))
	for _, call := range toolCalls {
		out = append(out, promptinput.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}
	return out
}

func promptInputToolResultFromRuntime(result *ToolResult) *promptinput.ToolResult {
	if result == nil {
		return nil
	}
	return &promptinput.ToolResult{
		ToolCallID: result.ToolCallID,
		Content:    result.Content,
	}
}

func runtimeMessagesFromPromptInput(messages []promptinput.Message) []Message {
	out := make([]Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCalls:  runtimeToolCallsFromPromptInput(msg.ToolCalls),
			ToolResult: runtimeToolResultFromPromptInput(msg.ToolResult),
		})
	}
	return out
}

func runtimeToolCallsFromPromptInput(toolCalls []promptinput.ToolCall) []ToolCall {
	out := make([]ToolCall, 0, len(toolCalls))
	for _, call := range toolCalls {
		out = append(out, ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}
	return out
}

func runtimeToolResultFromPromptInput(result *promptinput.ToolResult) *ToolResult {
	if result == nil {
		return nil
	}
	return &ToolResult{
		ToolCallID: result.ToolCallID,
		Content:    result.Content,
	}
}

func writeModelInputDebugTrace(req ModelInputDebugTraceRequest) (string, error) {
	return modeltrace.Write(modeltrace.Request{
		Kind:              "runtime_model_input",
		SessionID:         req.SessionID,
		TurnID:            req.TurnID,
		Iteration:         req.Iteration,
		Metadata:          req.Metadata,
		VisibleTools:      req.VisibleTools,
		PromptFingerprint: promptFingerprintMap(req.Compiled.Fingerprint),
		Prompt: modeltrace.Prompt{
			StableHash: promptContentHash(req.Compiled.Stable.Content),
			Stable:     req.Compiled.Stable.Content,
			Dynamic:    req.Compiled.Dynamic.Content,
			System:     effectiveSystemPrompt(req.Compiled).Content,
			Developer:  effectiveDeveloperInstructions(req.Compiled).Content,
			Tools:      effectiveToolPromptSet(req.Compiled).Content,
			Policy:     effectiveRuntimePolicyPrompt(req.Compiled).Content,
		},
		ModelInput:       req.ModelInput,
		PromptInputTrace: req.PromptInputTrace,
		PromptInputDiff:  req.PromptInputDiff,
	})
}

func promptFingerprintMap(fp promptcompiler.PromptFingerprint) map[string]string {
	out := map[string]string{}
	add := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			out[key] = value
		}
	}
	add("version", fp.Version)
	add("compilerVersion", fp.CompilerVersion)
	add("stableHash", fp.StableHash)
	add("systemHash", fp.SystemHash)
	add("developerHash", fp.DeveloperHash)
	add("toolRegistryHash", fp.ToolRegistryHash)
	add("runtimePolicyHash", fp.RuntimePolicyHash)
	add("protocolStateHash", fp.ProtocolStateHash)
	if len(out) == 0 {
		return nil
	}
	return out
}

func effectiveSystemPrompt(compiled promptcompiler.CompiledPrompt) promptcompiler.SystemPrompt {
	if compiled.System.Content != "" || compiled.System.Role != "" || compiled.System.Environment != "" {
		return compiled.System
	}
	return compiled.Stable.System
}

func effectiveDeveloperInstructions(compiled promptcompiler.CompiledPrompt) promptcompiler.DeveloperInstructions {
	if compiled.Developer.Content != "" || len(compiled.Developer.Constraints) > 0 {
		return compiled.Developer
	}
	return compiled.Stable.Developer
}

func effectiveToolPromptSet(compiled promptcompiler.CompiledPrompt) promptcompiler.ToolPromptSet {
	if compiled.Tools.Content != "" || len(compiled.Tools.Entries) > 0 {
		return compiled.Tools
	}
	return compiled.Stable.Tools
}

func effectiveRuntimePolicyPrompt(compiled promptcompiler.CompiledPrompt) promptcompiler.RuntimePolicyPrompt {
	if compiled.Policy.Content != "" || compiled.Policy.Mode != "" {
		return compiled.Policy
	}
	return compiled.Dynamic.Policy
}
