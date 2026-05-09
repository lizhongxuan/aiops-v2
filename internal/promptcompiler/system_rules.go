package promptcompiler

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Layer 1: System Prompt — environment and role definition
// ---------------------------------------------------------------------------

// buildSystemPrompt compiles Layer 1: the system prompt containing role and
// environment context for the agent.
func (c *PromptCompilerImpl) buildSystemPrompt(ctx CompileContext) (SystemPrompt, error) {
	var parts []string

	// Role based on agent kind
	role := c.resolveRole(ctx)
	parts = append(parts, fmt.Sprintf("# Role\n%s", role))
	parts = append(parts, "You are expected to be precise, safe, and helpful.")

	// Environment context based on session type
	env := c.resolveEnvironment(ctx)
	if env != "" {
		parts = append(parts, fmt.Sprintf("# Environment\n%s", env))
	}

	content := strings.Join(parts, "\n\n")
	return SystemPrompt{
		Content:     content,
		Role:        role,
		Environment: env,
	}, nil
}

// resolveRole determines the agent's role text based on AgentKind and SessionType.
func (c *PromptCompilerImpl) resolveRole(ctx CompileContext) string {
	switch ctx.AgentKind {
	case AgentKindPlanner:
		return "You are a planning agent responsible for analyzing user intent and creating multi-step execution plans for cross-host operations."
	case AgentKindWorker:
		return "You are a worker agent responsible for executing specific tasks on a designated host."
	default:
		// Default role based on session type
		switch ctx.SessionType {
		case "host":
			return "You are an AIOps assistant for host-level operations including inspection, monitoring, and controlled mutations."
		case "workspace":
			return "You are an AIOps workspace assistant coordinating multi-host operations and complex tasks."
		default:
			return "You are an AIOps assistant."
		}
	}
}

// resolveEnvironment builds the environment context string from host/workspace context.
func (c *PromptCompilerImpl) resolveEnvironment(ctx CompileContext) string {
	var envParts []string

	if ctx.HostContext != "" {
		envParts = append(envParts, fmt.Sprintf("Host: %s", ctx.HostContext))
	}
	if ctx.WorkspaceContext != "" {
		envParts = append(envParts, fmt.Sprintf("Workspace: %s", ctx.WorkspaceContext))
	}

	envParts = append(envParts, fmt.Sprintf("Session: %s", ctx.SessionType))
	envParts = append(envParts, fmt.Sprintf("Mode: %s", ctx.Mode))

	return strings.Join(envParts, "\n")
}
