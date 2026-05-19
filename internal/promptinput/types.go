package promptinput

import (
	"encoding/json"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/promptcompiler"
)

// ToolCall is the promptinput-local tool call shape. Keeping it local avoids
// a dependency cycle between runtimekernel and promptinput.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolResult is the promptinput-local tool result shape.
type ToolResult struct {
	ToolCallID string `json:"toolCallId"`
	Content    string `json:"content,omitempty"`
}

// Message is the conversation message shape consumed by the prompt input
// builder.
type Message struct {
	Role       string      `json:"role"`
	Content    string      `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"toolCalls,omitempty"`
	ToolResult *ToolResult `json:"toolResult,omitempty"`
}

// BuildRequest contains all structured inputs needed to build provider model
// messages and a semantic trace for those messages.
type BuildRequest struct {
	History     []Message
	Compiled    promptcompiler.CompiledPrompt
	State       agentstate.AgentState
	Tools       []promptcompiler.Tool
	Memories    []MemoryItem
	MaxMemories int

	OpsContextCapsule     string
	SessionFactCount      int
	LettaHintCount        int
	DroppedContextReasons []string
}

// BuildResult is the provider-facing model input plus its explainable trace.
type BuildResult struct {
	Messages []*schema.Message
	Trace    PromptInputTrace
}

// PromptInputTrace records where each prompt input item came from.
type PromptInputTrace struct {
	Items                  []TraceItem `json:"items"`
	OpsContextCapsuleChars int         `json:"opsContextCapsuleChars,omitempty"`
	SessionFactCount       int         `json:"sessionFactCount,omitempty"`
	LettaHintCount         int         `json:"lettaHintCount,omitempty"`
	MemoryItemCount        int         `json:"memoryItemCount,omitempty"`
	VisibleOpsManualTools  []string    `json:"visibleOpsManualTools,omitempty"`
	DroppedContextReasons  []string    `json:"droppedContextReasons,omitempty"`
}

// TraceItem is one semantic prompt-input trace entry.
type TraceItem struct {
	Source       string `json:"source"`
	SemanticRole string `json:"semanticRole"`
	ProviderRole string `json:"providerRole,omitempty"`
	PromptLayer  string `json:"promptLayer,omitempty"`
	ID           string `json:"id,omitempty"`
	Status       string `json:"status,omitempty"`
	Content      string `json:"content,omitempty"`
}

type MemoryItem struct {
	ID    string `json:"id"`
	Scope string `json:"scope,omitempty"`
	Text  string `json:"text"`
}

// Builder builds provider input and trace from promptcompiler output plus
// filtered conversation state.
type Builder struct{}
