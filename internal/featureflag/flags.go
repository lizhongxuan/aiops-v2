package featureflag

import (
	"os"
	"strconv"
	"strings"
	"time"

	"aiops-v2/internal/tooling"
)

const (
	envUnifiedToolModel      = "AIOPS_FLAG_UNIFIED_TOOL_MODEL"
	envToolSearch            = "AIOPS_FLAG_TOOL_SEARCH"
	envMCPServerRegistry     = "AIOPS_FLAG_MCP_SERVER_REGISTRY"
	envSkillRegistry         = "AIOPS_FLAG_SKILL_REGISTRY"
	envAgentRegistry         = "AIOPS_FLAG_AGENT_REGISTRY"
	envHooksV2               = "AIOPS_FLAG_HOOKS_V2"
	envDiagnosticProtocol    = "AIOPS_DIAGNOSTIC_PROTOCOL"
	envDisabledTools         = "AIOPS_DISABLED_TOOLS"
	envDeferredTools         = "AIOPS_DEFERRED_TOOLS"
	envExperimentalMetaTools = "AIOPS_EXPERIMENTAL_META_TOOLS"
	envReadOnlyToolRetry     = "AIOPS_FLAG_READONLY_TOOL_RETRY"

	envReadOnlyToolRetryMaxPerCall    = "AIOPS_READONLY_TOOL_RETRY_MAX_PER_CALL"
	envReadOnlyToolRetryMaxPerTurn    = "AIOPS_READONLY_TOOL_RETRY_MAX_PER_TURN"
	envReadOnlyToolRetryBackoffBaseMS = "AIOPS_READONLY_TOOL_RETRY_BACKOFF_BASE_MS"
	envReadOnlyToolRetryBackoffMaxMS  = "AIOPS_READONLY_TOOL_RETRY_BACKOFF_MAX_MS"
)

// Flags controls unified tool metadata exposure and related experiments.
type Flags struct {
	UnifiedToolModel      bool
	ToolSearch            bool
	MCPServerRegistry     bool
	SkillRegistry         bool
	AgentRegistry         bool
	HooksV2               bool
	DiagnosticProtocol    bool
	DisabledTools         []string
	DeferredTools         []string
	ExperimentalMetaTools []string

	ReadOnlyToolRetryEnabled     bool
	ReadOnlyToolRetryMaxPerCall  int
	ReadOnlyToolRetryMaxPerTurn  int
	ReadOnlyToolRetryBackoffBase time.Duration
	ReadOnlyToolRetryBackoffMax  time.Duration
}

// Default returns the zero-value flag set.
func Default() Flags {
	return Flags{
		DiagnosticProtocol:           true,
		ReadOnlyToolRetryMaxPerCall:  1,
		ReadOnlyToolRetryMaxPerTurn:  3,
		ReadOnlyToolRetryBackoffBase: 300 * time.Millisecond,
		ReadOnlyToolRetryBackoffMax:  2 * time.Second,
	}
}

// FromEnv builds a flag set from environment variables using the provided lookup.
func FromEnv(lookup func(string) string) Flags {
	f := Default()
	if lookup == nil {
		return f
	}

	f.UnifiedToolModel = parseBool(lookup(envUnifiedToolModel))
	f.ToolSearch = parseBool(lookup(envToolSearch))
	f.MCPServerRegistry = parseBool(lookup(envMCPServerRegistry))
	f.SkillRegistry = parseBool(lookup(envSkillRegistry))
	f.AgentRegistry = parseBool(lookup(envAgentRegistry))
	f.HooksV2 = parseBool(lookup(envHooksV2))
	f.DiagnosticProtocol = parseBoolDefault(lookup(envDiagnosticProtocol), true)
	f.DisabledTools = parseList(lookup(envDisabledTools))
	f.DeferredTools = parseList(lookup(envDeferredTools))
	f.ExperimentalMetaTools = parseList(lookup(envExperimentalMetaTools))
	f.ReadOnlyToolRetryEnabled = parseBool(lookup(envReadOnlyToolRetry))
	f.ReadOnlyToolRetryMaxPerCall = parsePositiveInt(lookup(envReadOnlyToolRetryMaxPerCall), f.ReadOnlyToolRetryMaxPerCall)
	f.ReadOnlyToolRetryMaxPerTurn = parsePositiveInt(lookup(envReadOnlyToolRetryMaxPerTurn), f.ReadOnlyToolRetryMaxPerTurn)
	f.ReadOnlyToolRetryBackoffBase = parseMillisDuration(lookup(envReadOnlyToolRetryBackoffBaseMS), f.ReadOnlyToolRetryBackoffBase)
	f.ReadOnlyToolRetryBackoffMax = parseMillisDuration(lookup(envReadOnlyToolRetryBackoffMaxMS), f.ReadOnlyToolRetryBackoffMax)
	return f
}

// Clone returns a copy of the flags with slices deep copied.
func (f Flags) Clone() Flags {
	return Flags{
		UnifiedToolModel:      f.UnifiedToolModel,
		ToolSearch:            f.ToolSearch,
		MCPServerRegistry:     f.MCPServerRegistry,
		SkillRegistry:         f.SkillRegistry,
		AgentRegistry:         f.AgentRegistry,
		HooksV2:               f.HooksV2,
		DiagnosticProtocol:    f.DiagnosticProtocol,
		DisabledTools:         cloneStrings(f.DisabledTools),
		DeferredTools:         cloneStrings(f.DeferredTools),
		ExperimentalMetaTools: cloneStrings(f.ExperimentalMetaTools),

		ReadOnlyToolRetryEnabled:     f.ReadOnlyToolRetryEnabled,
		ReadOnlyToolRetryMaxPerCall:  f.ReadOnlyToolRetryMaxPerCall,
		ReadOnlyToolRetryMaxPerTurn:  f.ReadOnlyToolRetryMaxPerTurn,
		ReadOnlyToolRetryBackoffBase: f.ReadOnlyToolRetryBackoffBase,
		ReadOnlyToolRetryBackoffMax:  f.ReadOnlyToolRetryBackoffMax,
	}
}

// IsToolVisible reports whether a tool should be exposed to the registry.
func (f Flags) IsToolVisible(meta tooling.ToolMetadata) bool {
	if containsString(f.DisabledTools, meta.Name) {
		return false
	}
	if containsString(f.ExperimentalMetaTools, meta.Name) && !f.ToolSearch {
		return false
	}
	return true
}

// ApplyToolMetadata returns a copy of meta with feature-flag-driven metadata applied.
func (f Flags) ApplyToolMetadata(meta tooling.ToolMetadata) tooling.ToolMetadata {
	out := cloneToolMetadata(meta)
	if containsString(f.DeferredTools, meta.Name) {
		out.ShouldDefer = true
	}
	return out
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseBoolDefault(value string, fallback bool) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, item := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == rune(os.PathListSeparator)
	}) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func parsePositiveInt(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseMillisDuration(value string, fallback time.Duration) time.Duration {
	millis := parsePositiveInt(value, -1)
	if millis <= 0 {
		return fallback
	}
	return time.Duration(millis) * time.Millisecond
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func cloneToolMetadata(meta tooling.ToolMetadata) tooling.ToolMetadata {
	out := meta
	out.Aliases = cloneStrings(meta.Aliases)
	if len(meta.MCPInfo.Raw) > 0 {
		out.MCPInfo.Raw = append([]byte(nil), meta.MCPInfo.Raw...)
	}
	return out
}
