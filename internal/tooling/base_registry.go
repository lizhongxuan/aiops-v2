package tooling

import (
	"fmt"
	"strings"
)

const (
	BaseRegistryRuntimeFunctionName = "getAllBaseTools"
	ExecCommandToolName             = "exec_command"
)

var mandatoryInitialToolNames = []string{
	ExecCommandToolName,
	"web_search",
	"list_mcp_resources",
	"read_mcp_resource",
	"grep",
	"skill_search",
	"skill_read",
}

// MandatoryInitialToolNames returns generic base tools that every agent should
// keep initially visible even when an agent definition narrows its allowlist.
func MandatoryInitialToolNames() []string {
	return append([]string(nil), mandatoryInitialToolNames...)
}

// IsRuntimeRegisteredTool reports tools that may stay registered with the
// runtime even when they are not exposed in the current model-visible surface.
func IsRuntimeRegisteredTool(meta ToolMetadata) bool {
	return matchesName(meta, ExecCommandToolName)
}

// IsModelVisibleToolForProfile reports whether a registered tool is allowed to
// appear in the model-facing surface for the selected profile. Runtime
// registration does not override this check.
func IsModelVisibleToolForProfile(meta ToolMetadata, profile string) bool {
	return profileAllowsTool(meta, strings.TrimSpace(profile))
}

// IsReservedRuntimeToolName reports names reserved for runtime/helper
// functions that must not become model-callable tools.
func IsReservedRuntimeToolName(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), BaseRegistryRuntimeFunctionName)
}

// ValidateBaseRegistryTools checks generic invariants for a Claude-like base
// registry before the tools are registered into the model-facing registry.
func ValidateBaseRegistryTools(tools []Tool) error {
	if len(tools) == 0 {
		return fmt.Errorf("base registry must contain at least one tool")
	}
	for _, tool := range tools {
		if tool == nil {
			return fmt.Errorf("base registry contains nil tool")
		}
		meta := tool.Metadata()
		if strings.TrimSpace(meta.Name) == "" {
			return fmt.Errorf("base registry contains tool with empty name")
		}
		if IsReservedRuntimeToolName(meta.Name) {
			return fmt.Errorf("%s is a runtime function and must not be registered as an LLM tool", BaseRegistryRuntimeFunctionName)
		}
	}
	return nil
}
