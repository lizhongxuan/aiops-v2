package tooling

import (
	"sort"
	"strings"
)

// EffectiveDiscovery returns normalized generic discovery metadata.
func (m ToolMetadata) EffectiveDiscovery() ToolDiscoveryMetadata {
	d := m.Discovery
	d.DiscoveryGroup = strings.TrimSpace(d.DiscoveryGroup)
	d.CapabilityKind = normalizeDiscoveryToken(d.CapabilityKind)
	d.DiscoveryTags = normalizeDiscoveryList(d.DiscoveryTags)
	d.Capabilities = normalizeDiscoveryList(d.Capabilities)
	d.ResourceTypes = normalizeDiscoveryList(d.ResourceTypes)
	d.TargetKinds = normalizeDiscoveryList(d.TargetKinds)
	d.OperationKinds = normalizeDiscoveryList(d.OperationKinds)
	if d.RiskLevel != "" {
		d.RiskLevel = d.RiskLevel.Normalize()
	} else if m.RiskLevel != "" {
		d.RiskLevel = m.RiskLevel.Normalize()
	}
	d.LoadingPolicy = m.effectiveLoadingPolicy(d.LoadingPolicy)
	d.AgentProfiles = mergeDiscoveryLists(d.AgentProfiles, m.Profiles)
	d.ToolPackIDs = mergeDiscoveryLists(d.ToolPackIDs, singletonIfNotEmpty(m.Pack))
	d.MCPServerID = normalizeDiscoveryToken(firstNonEmpty(d.MCPServerID, m.MCPInfo.ServerID, m.MCPInfo.ServerName))
	d.EvidenceKind = normalizeDiscoveryToken(d.EvidenceKind)
	d.PermissionScope = normalizeDiscoveryToken(d.PermissionScope)
	d.PromptBudgetClass = normalizeDiscoveryToken(d.PromptBudgetClass)
	d.SchemaBudgetClass = normalizeDiscoveryToken(d.SchemaBudgetClass)
	d.SupersedesShellHints = normalizeDiscoveryList(d.SupersedesShellHints)
	d.TargetKinds = mergeDiscoveryLists(d.TargetKinds, d.ResourceTypes)
	d.ResourceTypes = mergeDiscoveryLists(d.ResourceTypes, d.TargetKinds)

	if m.HasMCPSource() && !m.AlwaysLoad {
		d.RequiresHealthyMCP = true
	}
	if d.CapabilityKind == "" {
		d.CapabilityKind = discoveryCapabilityFromCapabilities(d.Capabilities)
	}
	if d.CapabilityKind == "" {
		switch {
		case m.Mutating:
			d.CapabilityKind = "write"
		case m.RiskLevel.Normalize().RequiresApproval():
			d.CapabilityKind = "execute"
		default:
			for _, op := range d.OperationKinds {
				switch op {
				case "read", "list", "search", "inspect", "query", "summarize":
					d.CapabilityKind = "read"
				case "write", "delete", "modify", "create", "update":
					d.CapabilityKind = "write"
				case "run", "execute":
					d.CapabilityKind = "execute"
				}
				if d.CapabilityKind != "" {
					break
				}
			}
		}
	}
	if d.CapabilityKind == "" {
		d.CapabilityKind = "read"
	}
	d.Capabilities = mergeDiscoveryLists(d.Capabilities, singletonIfNotEmpty(d.CapabilityKind))
	if m.Layer == ToolLayerInternal {
		d.HiddenFromDiscovery = true
		d.HiddenFromPrompt = true
	}
	return d
}

// EffectiveLoadingPolicy returns the normalized loading policy without forcing
// callers to inspect both legacy layer flags and MCP source traits.
func (m ToolMetadata) EffectiveLoadingPolicy() ToolLoadingPolicy {
	return m.EffectiveDiscovery().LoadingPolicy
}

// ToolHiddenFromDiscovery reports whether a tool should be omitted from
// discovery listings such as tool_search.
func ToolHiddenFromDiscovery(meta ToolMetadata) bool {
	return meta.EffectiveDiscovery().HiddenFromDiscovery
}

// ToolExcludedFromDeferredDiscovery reports whether a tool belongs to a
// first-class runtime surface that must not be discovered through tool_search.
func ToolExcludedFromDeferredDiscovery(meta ToolMetadata) bool {
	name := strings.ToLower(strings.TrimSpace(meta.Name))
	pack := strings.ToLower(strings.TrimSpace(meta.Pack))
	switch name {
	case "tool_search", "web_search", "browse_url":
		return true
	}
	return pack == "public_web"
}

// ExplicitToolSearchDiscoveryRequested reports whether the user explicitly asks
// to use tool_search for deferred tool discovery. Mentions while debugging,
// forbidding, or discussing tool_search should not expose the tool.
func ExplicitToolSearchDiscoveryRequested(input string) bool {
	text := strings.ToLower(strings.TrimSpace(input))
	if text == "" || !strings.Contains(text, "tool_search") {
		return false
	}
	if containsAnyDiscoveryPhrase(text,
		"不要", "别用", "不要用", "不应该", "不要再", "禁止",
		"do not", "don't", "dont", "should not", "must not", "without tool_search",
	) {
		return false
	}
	if containsAnyDiscoveryPhrase(text, "@tool_search") {
		return true
	}
	action := containsAnyDiscoveryPhrase(text,
		"use tool_search", "call tool_search", "enable tool_search", "invoke tool_search", "select tool_search",
		"使用 tool_search", "使用tool_search", "用 tool_search", "用tool_search",
		"调用 tool_search", "调用tool_search", "启用 tool_search", "启用tool_search",
		"选择 tool_search", "选择tool_search",
	)
	if !action {
		return false
	}
	return containsAnyDiscoveryPhrase(text,
		"deferred", "defer", "discover", "discovery", "select", "pack", "tool family",
		"发现", "延迟", "未暴露", "还没暴露", "工具包", "工具家族", "选择工具",
	)
}

func containsAnyDiscoveryPhrase(text string, phrases ...string) bool {
	for _, phrase := range phrases {
		phrase = strings.ToLower(strings.TrimSpace(phrase))
		if phrase != "" && strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

// ToolHiddenFromPrompt reports whether a tool should be omitted from the
// prompt-visible tool index.
func ToolHiddenFromPrompt(meta ToolMetadata) bool {
	return meta.EffectiveDiscovery().HiddenFromPrompt
}

// ToolRequiresSelect reports whether the tool must be explicitly selected
// before full schema/call use is advertised to the model.
func ToolRequiresSelect(meta ToolMetadata) bool {
	if meta.AlwaysLoad {
		return false
	}
	d := meta.EffectiveDiscovery()
	if d.RequiresSelect {
		return true
	}
	switch d.LoadingPolicy {
	case ToolLoadingPolicyDeferred, ToolLoadingPolicyMCP, ToolLoadingPolicyConditional:
		return true
	}
	if meta.Layer == ToolLayerDeferred || meta.DeferByDefault || meta.Pack != "" {
		return !meta.AlwaysLoad
	}
	return false
}

// ToolDiscoverySearchText returns a normalized metadata-first search corpus.
func ToolDiscoverySearchText(meta ToolMetadata) string {
	d := meta.EffectiveDiscovery()
	parts := []string{
		meta.Name,
		strings.Join(meta.Aliases, " "),
		meta.Description,
		meta.Domain,
		meta.SearchHint,
		d.DiscoveryGroup,
		d.CapabilityKind,
		strings.Join(d.Capabilities, " "),
		strings.Join(d.DiscoveryTags, " "),
		strings.Join(d.ResourceTypes, " "),
		strings.Join(d.TargetKinds, " "),
		strings.Join(d.OperationKinds, " "),
		string(d.RiskLevel),
		string(d.LoadingPolicy),
		strings.Join(d.AgentProfiles, " "),
		strings.Join(d.ToolPackIDs, " "),
		d.MCPServerID,
		d.EvidenceKind,
		d.PermissionScope,
		d.PromptBudgetClass,
		d.SchemaBudgetClass,
		strings.Join(meta.Triggers, " "),
	}
	if d.RequiresExplicitTarget {
		parts = append(parts, "requires_explicit_target")
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func discoveryCapabilityFromCapabilities(capabilities []string) string {
	for _, preferred := range []string{"execute", "write", "read", "inspect", "search"} {
		for _, capability := range capabilities {
			if capability == preferred {
				return capability
			}
		}
	}
	if len(capabilities) == 0 {
		return ""
	}
	return capabilities[0]
}

func (m ToolMetadata) effectiveLoadingPolicy(explicit ToolLoadingPolicy) ToolLoadingPolicy {
	if m.AlwaysLoad {
		return ToolLoadingPolicyCore
	}
	switch normalizeDiscoveryToken(string(explicit)) {
	case string(ToolLoadingPolicyCore):
		return ToolLoadingPolicyCore
	case string(ToolLoadingPolicyDeferred):
		return ToolLoadingPolicyDeferred
	case string(ToolLoadingPolicyProfile):
		return ToolLoadingPolicyProfile
	case string(ToolLoadingPolicyMCP):
		return ToolLoadingPolicyMCP
	case string(ToolLoadingPolicyInternal):
		return ToolLoadingPolicyInternal
	case string(ToolLoadingPolicyConditional):
		return ToolLoadingPolicyConditional
	}
	switch m.Layer {
	case ToolLayerCore:
		return ToolLoadingPolicyCore
	case ToolLayerDeferred:
		return ToolLoadingPolicyDeferred
	case ToolLayerProfile:
		return ToolLoadingPolicyProfile
	case ToolLayerMCP:
		return ToolLoadingPolicyMCP
	case ToolLayerInternal:
		return ToolLoadingPolicyInternal
	case ToolLayerConditional:
		return ToolLoadingPolicyConditional
	case ToolLayerDebug, ToolLayerMutation:
		return ToolLoadingPolicyConditional
	}
	if m.HasMCPSource() {
		return ToolLoadingPolicyMCP
	}
	if m.ShouldDefer || m.DeferByDefault || m.Pack != "" {
		return ToolLoadingPolicyDeferred
	}
	return ToolLoadingPolicyCore
}

func mergeDiscoveryLists(values ...[]string) []string {
	var merged []string
	for _, list := range values {
		merged = append(merged, list...)
	}
	return normalizeDiscoveryList(merged)
}

func singletonIfNotEmpty(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{value}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeDiscoveryList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeDiscoveryToken(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func normalizeDiscoveryToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
