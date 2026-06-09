package agentmgr

import (
	"fmt"
	"time"

	"aiops-v2/internal/agents"
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

// AgentDefinition defines a template for creating agent instances of a given kind.
// It maps to adk.ChatModelAgentConfig when the AgentFactory creates an agent.
type AgentDefinition struct {
	// Kind identifies the agent type (planner/worker).
	Kind AgentKind

	// Name is a human-readable name for this agent definition.
	Name string

	// Description is the human-readable summary of the agent's role.
	Description string

	// Role controls generic runtime policy for tool access and mutation gates.
	Role AgentRole

	// Prompt is the raw prompt text for the agent definition.
	Prompt string

	// Discovery contains short, prompt-safe routing metadata.
	Discovery agents.AgentDiscoveryMetadata

	// Budget contains generic scheduling budget hints.
	Budget agents.AgentBudgetMetadata

	// PromptTemplate is the PromptCompiler template key used for this agent type.
	PromptTemplate string

	// Tools lists the tool names this agent definition expects to use.
	Tools []string

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

// ToAgentTool projects the definition into the orchestration-facing AgentTool view.
func (d AgentDefinition) ToAgentTool() agents.AgentTool {
	return d.ToRegistryDefinition().ToAgentTool()
}

// ToRegistryDefinition converts the agent manager definition into the shared agents registry definition.
func (d AgentDefinition) ToRegistryDefinition() agents.Definition {
	prompt := d.Prompt
	if prompt == "" {
		prompt = d.PromptTemplate
	}

	return agents.Definition{
		Kind:          string(d.Kind),
		Name:          d.Name,
		Source:        string(agents.SourceBuiltin),
		Description:   d.Description,
		Prompt:        prompt,
		Discovery:     d.Discovery,
		Budget:        d.Budget,
		Tools:         append([]string(nil), d.Tools...),
		Model:         d.Model,
		Hooks:         append([]string(nil), d.Hooks...),
		MCPServers:    append([]string(nil), d.MCPServers...),
		MaxIterations: d.MaxIterations,
	}
}

// FromRegistryDefinition converts the shared registry definition into the agent manager definition.
func FromRegistryDefinition(def agents.Definition) AgentDefinition {
	return AgentDefinition{
		Kind:           AgentKind(def.Kind),
		Name:           def.Name,
		Description:    def.Description,
		Role:           AgentRoleExplore,
		Prompt:         def.Prompt,
		PromptTemplate: def.Prompt,
		Discovery:      def.Discovery,
		Budget:         def.Budget,
		Tools:          append([]string(nil), def.Tools...),
		MaxIterations:  def.MaxIterations,
		Model:          def.Model,
		Hooks:          append([]string(nil), def.Hooks...),
		MCPServers:     append([]string(nil), def.MCPServers...),
	}
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

	// ResultRefs points to bounded artifacts or evidence references returned by the agent.
	ResultRefs []string

	// Usage records bounded execution usage for notifications and traces.
	Usage AgentUsage
}

// EvidenceReport is the standardized output contract for spawned operations
// investigation agents. Child agents report evidence, not arbitrary prose or
// code changes.
type EvidenceReport struct {
	AgentID       string   `json:"agentId"`
	Summary       string   `json:"summary"`
	EvidenceRefs  []string `json:"evidenceRefs"`
	Confidence    string   `json:"confidence"`
	NextQuestions []string `json:"nextQuestions"`
	Errors        []string `json:"errors"`
}

// Normalize fills optional slices so JSON output keeps the report contract
// stable even when a child agent has no follow-up questions or errors.
func (r EvidenceReport) Normalize() EvidenceReport {
	r.EvidenceRefs = append([]string(nil), r.EvidenceRefs...)
	r.NextQuestions = append([]string(nil), r.NextQuestions...)
	r.Errors = append([]string(nil), r.Errors...)
	if r.Confidence == "" {
		r.Confidence = "unknown"
	}
	return r
}

// Validate checks the minimum fields needed for downstream evidence use.
func (r EvidenceReport) Validate() error {
	if r.AgentID == "" {
		return fmt.Errorf("evidence report agent id is required")
	}
	if r.Summary == "" && len(r.Errors) == 0 {
		return fmt.Errorf("evidence report summary or errors are required")
	}
	return nil
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

	// AssignmentSummary is a bounded self-contained assignment summary.
	AssignmentSummary string

	// EvidenceRequirement records the minimum evidence contract for this worker.
	EvidenceRequirement EvidenceRequirement

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
