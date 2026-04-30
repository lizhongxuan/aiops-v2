package tooling

import (
	"context"
	"encoding/json"
	"io"
	"time"
)

// ToolOrigin describes where a tool came from.
// It is retained as a compatibility/display field and should not be treated as
// the primary runtime classification axis for tool selection.
type ToolOrigin string

const (
	ToolOriginBuiltin ToolOrigin = "builtin"
	ToolOriginMCP     ToolOrigin = "mcp"
	ToolOriginMeta    ToolOrigin = "meta"
)

// ToolSource captures Claude-like source traits without relying on Origin as the primary axis.
type ToolSource string

const (
	ToolSourceBuiltin ToolSource = "builtin"
	ToolSourceMCP     ToolSource = "mcp"
	ToolSourceLSP     ToolSource = "lsp"
	ToolSourceMeta    ToolSource = "meta"
)

// MCPInfo stores MCP-specific metadata for tools that originated from an MCP server.
type MCPInfo struct {
	ServerID   string          `json:"serverId,omitempty"`
	ServerName string          `json:"serverName,omitempty"`
	ToolName   string          `json:"toolName,omitempty"`
	Raw        json.RawMessage `json:"raw,omitempty"`
}

// ProviderNativeToolInfo describes an equivalent provider-hosted tool.
// It keeps the unified Tool abstraction intact while allowing adapters/tools
// to delegate to model-native capabilities such as OpenAI web_search.
type ProviderNativeToolInfo struct {
	Provider string `json:"provider,omitempty"`
	Type     string `json:"type,omitempty"`
	Prefer   bool   `json:"prefer,omitempty"`
}

// ToolRiskLevel classifies operational risk for governance and prompting.
type ToolRiskLevel string

const (
	ToolRiskLow      ToolRiskLevel = "low"
	ToolRiskMedium   ToolRiskLevel = "medium"
	ToolRiskHigh     ToolRiskLevel = "high"
	ToolRiskCritical ToolRiskLevel = "critical"
)

// Normalize fills missing or unknown risk levels with the conservative default.
func (r ToolRiskLevel) Normalize() ToolRiskLevel {
	switch r {
	case ToolRiskLow, ToolRiskMedium, ToolRiskHigh, ToolRiskCritical:
		return r
	default:
		return ToolRiskMedium
	}
}

// RequiresApproval reports whether this risk level must cross an approval gate.
func (r ToolRiskLevel) RequiresApproval() bool {
	switch r.Normalize() {
	case ToolRiskHigh, ToolRiskCritical:
		return true
	default:
		return false
	}
}

// ToolFailurePolicy controls how runtime handles tool execution failure.
type ToolFailurePolicy string

const (
	ToolFailurePolicyFeedBackToModel ToolFailurePolicy = "feed_back_to_model"
	ToolFailurePolicyFailTurn        ToolFailurePolicy = "fail_turn"
)

// Normalize fills missing or unknown failure policies with the compatible
// default: feed the failure back to the model as a tool result.
func (p ToolFailurePolicy) Normalize() ToolFailurePolicy {
	switch p {
	case ToolFailurePolicyFeedBackToModel, ToolFailurePolicyFailTurn:
		return p
	default:
		return ToolFailurePolicyFeedBackToModel
	}
}

// ResultSpillPolicy controls how large tool results are externalized.
type ResultSpillPolicy string

const (
	ResultSpillPolicyInline        ResultSpillPolicy = "inline"
	ResultSpillPolicySummaryInline ResultSpillPolicy = "summary_inline"
	ResultSpillPolicyExternalize   ResultSpillPolicy = "externalize"
)

// ResultBudget captures the runtime budget for a tool result.
type ResultBudget struct {
	MaxInlineResultBytes int               `json:"maxInlineResultBytes,omitempty"`
	SpillPolicy          ResultSpillPolicy `json:"spillPolicy,omitempty"`
	SummarizeLargeResult bool              `json:"summarizeLargeResult,omitempty"`
}

// Normalize fills zero values with runtime defaults.
func (b ResultBudget) Normalize(defaultInlineBytes int) ResultBudget {
	if b.MaxInlineResultBytes <= 0 {
		b.MaxInlineResultBytes = defaultInlineBytes
	}
	if b.SpillPolicy == "" {
		b.SpillPolicy = ResultSpillPolicySummaryInline
	}
	if !b.SummarizeLargeResult && b.SpillPolicy != ResultSpillPolicyInline {
		b.SummarizeLargeResult = true
	}
	return b
}

// ToolGovernance is the normalized policy-facing governance view for a tool.
type ToolGovernance struct {
	RiskLevel        ToolRiskLevel     `json:"riskLevel"`
	Mutating         bool              `json:"mutating"`
	RequiresApproval bool              `json:"requiresApproval"`
	ResultBudget     ResultBudget      `json:"resultBudget"`
	FailurePolicy    ToolFailurePolicy `json:"failurePolicy"`
}

// ToolMetadata captures the registry-facing metadata for a tool.
type ToolMetadata struct {
	Name             string                  `json:"name"`
	Aliases          []string                `json:"aliases,omitempty"`
	Description      string                  `json:"description,omitempty"`
	Origin           ToolOrigin              `json:"origin,omitempty"`
	SearchHint       string                  `json:"searchHint,omitempty"`
	ShouldDefer      bool                    `json:"shouldDefer,omitempty"`
	AlwaysLoad       bool                    `json:"alwaysLoad,omitempty"`
	IsMCP            bool                    `json:"isMCP,omitempty"`
	IsLSP            bool                    `json:"isLSP,omitempty"`
	RiskLevel        ToolRiskLevel           `json:"riskLevel,omitempty"`
	Mutating         bool                    `json:"mutating,omitempty"`
	RequiresApproval bool                    `json:"requiresApproval,omitempty"`
	ResultBudget     ResultBudget            `json:"resultBudget,omitempty"`
	FailurePolicy    ToolFailurePolicy       `json:"failurePolicy,omitempty"`
	MCPInfo          MCPInfo                 `json:"mcpInfo,omitempty"`
	ProviderNative   *ProviderNativeToolInfo `json:"providerNative,omitempty"`
}

// HasMCPSource reports whether the metadata identifies an MCP-backed tool.
// This traits-first predicate is the preferred runtime check over Origin.
func (m ToolMetadata) HasMCPSource() bool {
	if m.IsMCP {
		return true
	}
	return m.MCPInfo.ServerID != "" || m.MCPInfo.ServerName != "" || m.MCPInfo.ToolName != ""
}

// HasSource reports whether the metadata matches the requested source trait.
func (m ToolMetadata) HasSource(source ToolSource) bool {
	switch source {
	case ToolSourceMCP:
		return m.HasMCPSource()
	case ToolSourceLSP:
		return m.IsLSP
	case ToolSourceMeta:
		return m.Origin == ToolOriginMeta
	case ToolSourceBuiltin:
		return !m.HasMCPSource() && !m.IsLSP && m.Origin != ToolOriginMeta
	default:
		return false
	}
}

// EffectiveResultBudget returns the configured result budget or runtime defaults.
func (m ToolMetadata) EffectiveResultBudget(defaultInlineBytes int) ResultBudget {
	return m.ResultBudget.Normalize(defaultInlineBytes)
}

// EffectiveGovernance returns the normalized governance metadata used by prompt
// rendering and dispatch policy.
func (m ToolMetadata) EffectiveGovernance(defaultInlineBytes int) ToolGovernance {
	risk := m.RiskLevel.Normalize()
	return ToolGovernance{
		RiskLevel:        risk,
		Mutating:         m.Mutating,
		RequiresApproval: m.RequiresApproval || risk.RequiresApproval(),
		ResultBudget:     m.EffectiveResultBudget(defaultInlineBytes),
		FailurePolicy:    m.FailurePolicy.Normalize(),
	}
}

// ToolDisplayPayload is a simple structured payload for human-facing tool output.
type ToolDisplayPayload struct {
	Type    string          `json:"type"`
	Title   string          `json:"title,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	CardRef string          `json:"cardRef,omitempty"`
}

// ResultReferenceKind describes the external carrier for a tool result.
type ResultReferenceKind string

const (
	ResultReferenceKindBlob ResultReferenceKind = "blob"
	ResultReferenceKindCard ResultReferenceKind = "card"
	ResultReferenceKindFile ResultReferenceKind = "file"
)

// ResultReference captures a structured external reference returned by a tool.
type ResultReference struct {
	Kind        ResultReferenceKind `json:"kind"`
	URI         string              `json:"uri,omitempty"`
	CardRef     string              `json:"cardRef,omitempty"`
	FilePath    string              `json:"filePath,omitempty"`
	Title       string              `json:"title,omitempty"`
	Summary     string              `json:"summary,omitempty"`
	ContentType string              `json:"contentType,omitempty"`
	Digest      string              `json:"digest,omitempty"`
	Bytes       int64               `json:"bytes,omitempty"`
}

// ResultSpill captures content that was externalized because it exceeded the inline budget.
type ResultSpill struct {
	ID          string    `json:"id"`
	ToolCallID  string    `json:"toolCallId,omitempty"`
	ToolName    string    `json:"toolName,omitempty"`
	SessionID   string    `json:"sessionId,omitempty"`
	TurnID      string    `json:"turnId,omitempty"`
	ContentType string    `json:"contentType,omitempty"`
	Summary     string    `json:"summary,omitempty"`
	Content     []byte    `json:"content,omitempty"`
	Bytes       int64     `json:"bytes,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// StreamingResult captures a stream-backed tool result that should be read
// incrementally by the runtime.
type StreamingResult struct {
	Reader      io.Reader `json:"-"`
	ChunkSize   int       `json:"-"`
	ContentType string    `json:"contentType,omitempty"`
}

// ToolResult is the output of a tool invocation.
type ToolResult struct {
	ToolCallID   string              `json:"toolCallId,omitempty"`
	Content      string              `json:"content,omitempty"`
	Display      *ToolDisplayPayload `json:"display,omitempty"`
	Error        string              `json:"error,omitempty"`
	References   []ResultReference   `json:"references,omitempty"`
	ResultBudget ResultBudget        `json:"resultBudget,omitempty"`
	Spill        *ResultSpill        `json:"spill,omitempty"`
	Stream       *StreamingResult    `json:"-"`
}

// EffectiveResultBudget returns the configured result budget or runtime defaults.
func (r ToolResult) EffectiveResultBudget(defaultInlineBytes int) ResultBudget {
	return r.ResultBudget.Normalize(defaultInlineBytes)
}

// HasSpill reports whether the result points to an externalized spill.
func (r ToolResult) HasSpill() bool {
	return r.Spill != nil && r.Spill.ID != ""
}

// HasReferences reports whether the result returns explicit external references.
func (r ToolResult) HasReferences() bool {
	return len(r.References) > 0
}

// HasStream reports whether the result should be consumed incrementally.
func (r ToolResult) HasStream() bool {
	return r.Stream != nil && r.Stream.Reader != nil
}

// PermissionAction classifies the outcome of a permission check.
type PermissionAction string

const (
	PermissionActionAllow        PermissionAction = "allow"
	PermissionActionDeny         PermissionAction = "deny"
	PermissionActionNeedApproval PermissionAction = "need_approval"
	PermissionActionNeedEvidence PermissionAction = "need_evidence"
)

// PermissionDecision is the result returned by a permission check.
type PermissionDecision struct {
	Action PermissionAction `json:"action"`
	Reason string           `json:"reason,omitempty"`
}

// DescribeContext carries contextual data for Description generation.
type DescribeContext struct {
	Context     context.Context
	SessionType string
	Mode        string
	Metadata    ToolMetadata
}

// PromptContext carries contextual data for Prompt generation.
type PromptContext struct {
	Context     context.Context
	SessionType string
	Mode        string
	Metadata    ToolMetadata
}

// ToolContext carries visibility/runtime context for IsEnabled.
type ToolContext struct {
	Context     context.Context
	SessionType string
	Mode        string
	Metadata    ToolMetadata
}

// Tool is the unified tool abstraction used by the new tooling registry.
type Tool interface {
	Metadata() ToolMetadata
	InputSchema() json.RawMessage
	OutputSchema() json.RawMessage
	Description(input json.RawMessage, ctx DescribeContext) string
	Prompt(ctx PromptContext) string
	IsEnabled(ctx ToolContext) bool
	IsReadOnly(input json.RawMessage) bool
	IsDestructive(input json.RawMessage) bool
	IsConcurrencySafe(input json.RawMessage) bool
	ValidateInput(ctx context.Context, input json.RawMessage) error
	CheckPermissions(ctx context.Context, input json.RawMessage) PermissionDecision
	Execute(ctx context.Context, input json.RawMessage) (ToolResult, error)
}
