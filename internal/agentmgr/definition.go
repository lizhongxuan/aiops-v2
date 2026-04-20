package agentmgr

import (
	"fmt"
	"time"

	"aiops-v2/internal/agents"
	"aiops-v2/internal/capability"
)

// ---------------------------------------------------------------------------
// AgentKind identifies the type of agent (planner or worker).
// ---------------------------------------------------------------------------

// AgentKind identifies the agent type for multi-agent orchestration.
type AgentKind string

const (
	AgentKindPlanner AgentKind = "planner"
	AgentKindWorker  AgentKind = "worker"
)

// IsValid reports whether the value is one of the canonical agent kinds.
func (k AgentKind) IsValid() bool {
	switch k {
	case AgentKindPlanner, AgentKindWorker:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// CapabilityScope defines the capability visibility for an agent type.
// ---------------------------------------------------------------------------

// CapabilityScope defines the capability kinds and host access an agent type
// is allowed to use. An empty HostIDs slice means unrestricted host access.
type CapabilityScope struct {
	// Kinds lists the allowed capability kinds for this agent type.
	Kinds []capability.Kind

	// HostIDs lists the allowed host IDs. Empty means unrestricted.
	HostIDs []string
}

// ---------------------------------------------------------------------------
// AgentDefinition defines a template for creating agent instances.
// Maps to adk.ChatModelAgentConfig at creation time.
// ---------------------------------------------------------------------------

// AgentDefinition defines a template for creating agent instances of a given kind.
// It maps to adk.ChatModelAgentConfig when the AgentFactory creates an agent.
type AgentDefinition struct {
	// Kind identifies the agent type (planner/worker).
	Kind AgentKind

	// Name is a human-readable name for this agent definition.
	Name string

	// Description is the human-readable summary of the agent's role.
	Description string

	// Prompt is the raw prompt text for the agent definition.
	Prompt string

	// PromptTemplate is the PromptCompiler template key used for this agent type.
	PromptTemplate string

	// Tools lists the tool names this agent definition expects to use.
	Tools []string

	// CapabilityScope defines the capability visibility for agents of this kind.
	CapabilityScope CapabilityScope

	// MaxIterations is the maximum number of ReAct iterations for the ChatModelAgent.
	MaxIterations int

	// Model specifies the LLM model to use (worker agents may use cheaper models).
	Model string

	// Hooks lists lifecycle hook names associated with this definition.
	Hooks []string

	// MCPServers lists MCP server names associated with this definition.
	MCPServers []string
}

// Validate checks that the AgentDefinition has required fields and valid kind.
func (d AgentDefinition) Validate() error {
	if !d.Kind.IsValid() {
		return fmt.Errorf("invalid agent kind %q", d.Kind)
	}
	if d.Name == "" {
		return fmt.Errorf("agent definition name is required")
	}
	if d.MaxIterations < 0 {
		return fmt.Errorf("max iterations must be non-negative, got %d", d.MaxIterations)
	}
	return nil
}

// ToRegistryDefinition converts the agent manager definition into the shared agents registry definition.
func (d AgentDefinition) ToRegistryDefinition() agents.Definition {
	prompt := d.Prompt
	if prompt == "" {
		prompt = d.PromptTemplate
	}

	return agents.Definition{
		Kind:            string(d.Kind),
		Name:            d.Name,
		Description:     d.Description,
		Prompt:          prompt,
		Tools:           append([]string(nil), d.Tools...),
		Model:           d.Model,
		Hooks:           append([]string(nil), d.Hooks...),
		MCPServers:      append([]string(nil), d.MCPServers...),
		MaxIterations:   d.MaxIterations,
		CapabilityKinds: capabilityKindsToStrings(d.CapabilityScope.Kinds),
		CapabilityHosts: append([]string(nil), d.CapabilityScope.HostIDs...),
	}
}

// FromRegistryDefinition converts the shared registry definition into the agent manager definition.
func FromRegistryDefinition(def agents.Definition) AgentDefinition {
	return AgentDefinition{
		Kind:           AgentKind(def.Kind),
		Name:           def.Name,
		Description:    def.Description,
		Prompt:         def.Prompt,
		PromptTemplate: def.Prompt,
		Tools:          append([]string(nil), def.Tools...),
		MaxIterations:  def.MaxIterations,
		Model:          def.Model,
		Hooks:          append([]string(nil), def.Hooks...),
		MCPServers:     append([]string(nil), def.MCPServers...),
		CapabilityScope: CapabilityScope{
			Kinds:   stringsToCapabilityKinds(def.CapabilityKinds),
			HostIDs: append([]string(nil), def.CapabilityHosts...),
		},
	}
}

func capabilityKindsToStrings(kinds []capability.Kind) []string {
	if len(kinds) == 0 {
		return nil
	}
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, string(k))
	}
	return out
}

func stringsToCapabilityKinds(kinds []string) []capability.Kind {
	if len(kinds) == 0 {
		return nil
	}
	out := make([]capability.Kind, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, capability.Kind(k))
	}
	return out
}

// ---------------------------------------------------------------------------
// AgentStatus represents the lifecycle state of an agent instance.
// ---------------------------------------------------------------------------

// AgentStatus represents the lifecycle state of an agent instance.
type AgentStatus string

const (
	AgentStatusIdle      AgentStatus = "idle"
	AgentStatusRunning   AgentStatus = "running"
	AgentStatusWaiting   AgentStatus = "waiting"
	AgentStatusCompleted AgentStatus = "completed"
	AgentStatusFailed    AgentStatus = "failed"
	AgentStatusKilled    AgentStatus = "killed"
)

// IsValid reports whether the value is one of the canonical agent statuses.
func (s AgentStatus) IsValid() bool {
	switch s {
	case AgentStatusIdle, AgentStatusRunning, AgentStatusWaiting,
		AgentStatusCompleted, AgentStatusFailed, AgentStatusKilled:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether the status is a terminal state (no further transitions).
func (s AgentStatus) IsTerminal() bool {
	switch s {
	case AgentStatusCompleted, AgentStatusFailed, AgentStatusKilled:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// AgentResult represents the execution result of an agent.
// ---------------------------------------------------------------------------

// AgentResult represents the execution result of an agent instance.
type AgentResult struct {
	// AgentID is the unique identifier of the agent instance.
	AgentID string

	// HostID is the host this agent was bound to (empty for planner agents).
	HostID string

	// Status is the final status (completed or failed).
	Status AgentStatus

	// Output is the execution summary text.
	Output string

	// Error contains error information if the agent failed.
	Error string

	// Duration is the total execution time.
	Duration time.Duration
}

// ---------------------------------------------------------------------------
// AgentInstance represents a running agent instance in the system.
// ---------------------------------------------------------------------------

// AgentInstance represents a runtime agent instance managed by the AgentManager.
type AgentInstance struct {
	// ID is the unique identifier for this agent instance.
	ID string

	// Kind identifies the agent type (planner/worker).
	Kind AgentKind

	// MissionID is the workspace mission this agent belongs to.
	MissionID string

	// ParentID is the parent agent ID (empty for top-level agents).
	ParentID string

	// HostID is the host this agent is bound to (empty for planner agents).
	HostID string

	// SessionID is the session this agent operates within.
	SessionID string

	// Status is the current lifecycle status.
	Status AgentStatus

	// Task describes what this agent is doing.
	Task string

	// Output contains the execution output/summary.
	Output string

	// Error contains error information if the agent failed.
	Error string

	// Duration is the execution time so far.
	Duration time.Duration

	// CreatedAt is when this instance was created.
	CreatedAt time.Time

	// UpdatedAt is when this instance was last updated.
	UpdatedAt time.Time
}

// Validate checks that the AgentInstance has required fields and valid values.
func (i AgentInstance) Validate() error {
	if i.ID == "" {
		return fmt.Errorf("agent instance id is required")
	}
	if !i.Kind.IsValid() {
		return fmt.Errorf("invalid agent kind %q", i.Kind)
	}
	if i.MissionID == "" {
		return fmt.Errorf("mission id is required")
	}
	if i.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if !i.Status.IsValid() {
		return fmt.Errorf("invalid agent status %q", i.Status)
	}
	return nil
}
