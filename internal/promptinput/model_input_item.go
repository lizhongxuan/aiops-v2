package promptinput

import "encoding/json"

type ProviderRole string

const (
	ProviderRoleSystem    ProviderRole = "system"
	ProviderRoleDeveloper ProviderRole = "developer"
	ProviderRoleUser      ProviderRole = "user"
	ProviderRoleAssistant ProviderRole = "assistant"
	ProviderRoleTool      ProviderRole = "tool"
)

type ModelInputSource struct {
	Layer     string `json:"layer,omitempty"`
	SectionID string `json:"sectionId,omitempty"`
	MessageID string `json:"messageId,omitempty"`
	Origin    string `json:"origin,omitempty"`
}

type ModelInputContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type ModelInputToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type ModelInputToolResult struct {
	ToolCallID string `json:"toolCallId"`
	Content    string `json:"content,omitempty"`
}

type ModelInputItem struct {
	ID           string                  `json:"id"`
	ProviderRole ProviderRole            `json:"providerRole"`
	SemanticRole string                  `json:"semanticRole,omitempty"`
	Content      string                  `json:"content,omitempty"`
	ContentParts []ModelInputContentPart `json:"contentParts,omitempty"`
	Name         string                  `json:"name,omitempty"`
	ToolCalls    []ModelInputToolCall    `json:"toolCalls,omitempty"`
	ToolCallID   string                  `json:"toolCallId,omitempty"`
	ToolResult   *ModelInputToolResult   `json:"toolResult,omitempty"`
	Source       ModelInputSource        `json:"source,omitempty"`
	Phase        string                  `json:"phase,omitempty"`
	CacheGroup   string                  `json:"cacheGroup,omitempty"`
	Metadata     map[string]string       `json:"metadata,omitempty"`
}

func (i ModelInputItem) ToolResultToolCallID() string {
	if i.ToolResult == nil {
		return ""
	}
	return i.ToolResult.ToolCallID
}
