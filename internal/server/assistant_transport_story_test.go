package server

import (
	"encoding/json"
	"testing"
)

type assistantTransportStory struct {
	Name              string                  `json:"name"`
	Command           map[string]any          `json:"command"`
	ProviderResponses []storyProviderResponse `json:"providerResponses"`
	ToolOutcomes      []storyToolOutcome      `json:"toolOutcomes"`
	Want              storyTransportAssert    `json:"want"`
}

type storyProviderResponse struct {
	Role      string          `json:"role"`
	Content   string          `json:"content,omitempty"`
	ToolCalls []storyToolCall `json:"toolCalls,omitempty"`
}

type storyToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type storyToolOutcome struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	Content     string          `json:"content,omitempty"`
	Error       string          `json:"error,omitempty"`
	Risk        string          `json:"risk,omitempty"`
	Mutating    bool            `json:"mutating,omitempty"`
}

type storyTransportAssert struct {
	TurnStatus  string            `json:"turnStatus"`
	Messages    []storyMessage    `json:"messages"`
	Tools       []storyToolAssert `json:"tools"`
	Approvals   []string          `json:"approvals"`
	Evidence    []string          `json:"evidence"`
	TraceHashes map[string]string `json:"traceHashes"`
}

type storyMessage struct {
	Phase string `json:"phase"`
	Text  string `json:"text"`
}

type storyToolAssert struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func TestAssistantTransportStories(t *testing.T) {
	for _, story := range loadAssistantTransportStories(t) {
		story := story
		t.Run(story.Name, func(t *testing.T) {
			result := runAssistantTransportStory(t, story)
			assertAssistantTransportStory(t, story, result)
		})
	}
}
