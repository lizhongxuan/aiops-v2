package modelrouter

import (
	"time"

	"aiops-v2/internal/promptinput"
)

type ProviderUsage struct {
	PromptTokens     int `json:"promptTokens,omitempty"`
	CompletionTokens int `json:"completionTokens,omitempty"`
	TotalTokens      int `json:"totalTokens,omitempty"`
}

type ProviderStreamMetrics struct {
	FirstDeltaMs int `json:"firstDeltaMs,omitempty"`
	StreamMs     int `json:"streamMs,omitempty"`
	DeltaCount   int `json:"deltaCount,omitempty"`
	OutputChars  int `json:"outputChars,omitempty"`
}

type ProviderResponse struct {
	RequestID             string                           `json:"requestId,omitempty"`
	Output                string                           `json:"output,omitempty"`
	ToolCalls             []promptinput.ModelInputToolCall `json:"toolCalls,omitempty"`
	FinishReason          string                           `json:"finishReason,omitempty"`
	Usage                 ProviderUsage                    `json:"usage,omitempty"`
	StreamMetrics         ProviderStreamMetrics            `json:"streamMetrics,omitempty"`
	NativeWebSearchEvents []ProviderNativeWebSearchEvent   `json:"nativeWebSearchEvents,omitempty"`
	StartedAt             time.Time                        `json:"startedAt,omitempty"`
	FinishedAt            time.Time                        `json:"finishedAt,omitempty"`
}
