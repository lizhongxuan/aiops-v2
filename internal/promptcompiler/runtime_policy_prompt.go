package promptcompiler

import (
	"strings"
)

// ---------------------------------------------------------------------------
// Layer 4: Runtime Policy Prompt — mode-specific policy constraints
// ---------------------------------------------------------------------------

// buildRuntimePolicyPrompt compiles Layer 4: the runtime policy prompt
// containing mode-specific constraints and any custom policy overrides.
func (c *PromptCompilerImpl) buildRuntimePolicyPrompt(ctx CompileContext) (RuntimePolicyPrompt, error) {
	var parts []string
	parts = append(parts, "# Runtime Policy")

	// If explicit runtime policy is provided, use it
	if ctx.RuntimePolicy != "" {
		parts = append(parts, ctx.RuntimePolicy)
	} else {
		// Generate default policy based on mode
		policyText := c.defaultPolicyForMode(ctx.Mode)
		parts = append(parts, policyText)
	}

	content := strings.Join(parts, "\n")
	return RuntimePolicyPrompt{
		Content: content,
		Mode:    ctx.Mode,
	}, nil
}

// defaultPolicyForMode returns the default policy text for the given mode.
// Each mode has distinct constraints that are mutually exclusive.
func (c *PromptCompilerImpl) defaultPolicyForMode(mode Mode) string {
	switch mode {
	case "chat":
		return "Policy: Chat mode. Only lightweight read-only operations and web search are permitted. All mutation operations are strictly forbidden."
	case "inspect":
		return "Policy: Inspect mode. Read, list, search, and readonly shell operations are permitted. All mutation operations are strictly forbidden."
	case "plan":
		return "Policy: Plan mode. Inspection and planning operations are permitted. Direct mutation execution is forbidden. Generate plans for review."
	case "execute":
		return "Policy: Execute mode. All operations are permitted subject to approval constraints. Mutations require explicit approval through the runtime tool approval gate. Do not ask for approval in prose when a tool call can trigger the approval gate; call the scoped tool and let runtime pause for the user. Collect evidence before and after changes."
	default:
		return "Policy: Unknown mode. Default to read-only operations only."
	}
}
