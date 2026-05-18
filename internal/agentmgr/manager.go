package agentmgr

import (
	"context"
	"fmt"
	"sync"
	"time"

	"aiops-v2/internal/projection"
)

// ---------------------------------------------------------------------------
// SpawnRequest contains the parameters for spawning a new agent instance.
// ---------------------------------------------------------------------------

// SpawnRequest contains the parameters for creating a new agent instance.
type SpawnRequest struct {
	// ID is the unique identifier for the agent instance.
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

	// Task describes what this agent should do.
	Task string
}

// ---------------------------------------------------------------------------
// AgentRunner is the interface for executing an agent. In production this
// wraps adk.Runner; in tests it can be replaced with a mock.
// ---------------------------------------------------------------------------

// AgentRunner abstracts the execution of an agent via adk.Runner.
// Production implementations use adk.NewRunner with EnableStreaming and
// CheckPointStore. Test implementations can simulate execution.
type AgentRunner interface {
	// Run executes the agent and returns the output text or an error.
	Run(ctx context.Context, config *AgentConfig) (output string, err error)
}

// ---------------------------------------------------------------------------
// AgentManager manages agent instance lifecycles using adk.Runner.
//
// Requirement 13.4: WHEN Worker_Agent 完成任务时，THE AgentManager SHALL 将
// Worker_Agent 的执行结果汇报给 Replanner Agent.
// Requirement 13.5: IF Worker_Agent 执行失败或其绑定的 Host Agent 离线，THEN
// THE AgentManager SHALL 将该 Worker_Agent 标记为 failed.
// ---------------------------------------------------------------------------

// AgentManager manages agent instance lifecycles. It uses AgentRunner (backed
// by adk.Runner in production) to execute agents, tracks instances via a
// thread-safe map, and collects results for mission-level reporting.
type AgentManager struct {
	mu        sync.RWMutex
	instances map[string]*AgentInstance // agentID → instance

	factory   *AgentFactory
	runner    AgentRunner
	projector *projection.Projector
}

// NewAgentManager creates a new AgentManager with the given dependencies.
func NewAgentManager(factory *AgentFactory, runner AgentRunner, projector *projection.Projector) *AgentManager {
	return &AgentManager{
		instances: make(map[string]*AgentInstance),
		factory:   factory,
		runner:    runner,
		projector: projector,
	}
}

// ---------------------------------------------------------------------------
// Spawn creates and registers a new AgentInstance with status idle.
// ---------------------------------------------------------------------------

// Spawn creates and registers a new AgentInstance. The instance starts in
// idle status and must be explicitly run via RunAgent.
func (m *AgentManager) Spawn(ctx context.Context, req SpawnRequest) (*AgentInstance, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("spawn: agent ID is required")
	}
	if !req.Kind.IsValid() {
		return nil, fmt.Errorf("spawn: invalid agent kind %q", req.Kind)
	}
	if req.MissionID == "" {
		return nil, fmt.Errorf("spawn: mission ID is required")
	}
	if req.SessionID == "" {
		return nil, fmt.Errorf("spawn: session ID is required")
	}

	now := time.Now()
	instance := &AgentInstance{
		ID:        req.ID,
		Kind:      req.Kind,
		MissionID: req.MissionID,
		ParentID:  req.ParentID,
		HostID:    req.HostID,
		SessionID: req.SessionID,
		Status:    AgentStatusIdle,
		Task:      req.Task,
		CreatedAt: now,
		UpdatedAt: now,
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.instances[req.ID]; exists {
		return nil, fmt.Errorf("spawn: agent %q already exists", req.ID)
	}

	m.instances[req.ID] = instance
	return instance, nil
}

// ---------------------------------------------------------------------------
// RunAgent transitions an agent to running, executes it via AgentRunner,
// and records the result. This is the core execution path that in production
// uses adk.NewRunner with EnableStreaming:true and CheckPointStore.
// ---------------------------------------------------------------------------

// RunAgent executes the given agent configuration for the specified agent ID.
// It transitions the agent from idle to running, executes via AgentRunner
// (backed by adk.Runner with EnableStreaming:true and CheckPointStore in
// production), and records the result as completed or failed.
func (m *AgentManager) RunAgent(ctx context.Context, agentID string, config *AgentConfig) (*AgentResult, error) {
	m.mu.Lock()
	instance, exists := m.instances[agentID]
	if !exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("run agent: agent %q not found", agentID)
	}
	if instance.Status != AgentStatusIdle {
		m.mu.Unlock()
		return nil, fmt.Errorf("run agent: agent %q is in status %q, expected idle", agentID, instance.Status)
	}
	instance.Status = AgentStatusRunning
	instance.UpdatedAt = time.Now()
	m.mu.Unlock()

	// Execute via AgentRunner (adk.Runner in production).
	startTime := time.Now()
	output, err := m.runner.Run(ctx, config)
	duration := time.Since(startTime)

	// Build result and update instance.
	result := &AgentResult{
		AgentID:  agentID,
		HostID:   instance.HostID,
		Duration: duration,
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Re-fetch instance in case it was killed during execution.
	instance, exists = m.instances[agentID]
	if !exists {
		return nil, fmt.Errorf("run agent: agent %q was removed during execution", agentID)
	}

	// If killed during execution, don't overwrite the killed status.
	if instance.Status == AgentStatusKilled {
		result.Status = AgentStatusKilled
		result.Output = output
		return result, nil
	}

	if err != nil {
		instance.Status = AgentStatusFailed
		instance.Error = err.Error()
		instance.Duration = duration
		instance.UpdatedAt = time.Now()

		result.Status = AgentStatusFailed
		result.Error = err.Error()
		result.Output = output
	} else {
		instance.Status = AgentStatusCompleted
		instance.Output = output
		instance.Duration = duration
		instance.UpdatedAt = time.Now()

		result.Status = AgentStatusCompleted
		result.Output = output
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// KillAgent terminates an agent instance, setting its status to killed.
// ---------------------------------------------------------------------------

// KillAgent terminates the specified agent instance. It sets the status to
// killed regardless of current status (unless already in a terminal state).
func (m *AgentManager) KillAgent(ctx context.Context, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	instance, exists := m.instances[agentID]
	if !exists {
		return fmt.Errorf("kill agent: agent %q not found", agentID)
	}

	if instance.Status.IsTerminal() {
		return fmt.Errorf("kill agent: agent %q is already in terminal status %q", agentID, instance.Status)
	}

	instance.Status = AgentStatusKilled
	instance.UpdatedAt = time.Now()
	return nil
}

// MarkAgentFailed records a spawn-time or configuration failure for an agent
// that could not enter RunAgent.
func (m *AgentManager) MarkAgentFailed(agentID string, err error) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	instance, ok := m.instances[agentID]
	if !ok || instance.Status.IsTerminal() {
		return
	}
	instance.Status = AgentStatusFailed
	if err != nil {
		instance.Error = err.Error()
	}
	instance.UpdatedAt = time.Now()
}

// ---------------------------------------------------------------------------
// CollectResults gathers all results for a given mission.
// ---------------------------------------------------------------------------

// CollectResults returns AgentResult for all agents belonging to the given
// mission that are in a terminal state (completed, failed, or killed).
func (m *AgentManager) CollectResults(missionID string) []AgentResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []AgentResult
	for _, inst := range m.instances {
		if inst.MissionID != missionID {
			continue
		}
		if !inst.Status.IsTerminal() {
			continue
		}
		results = append(results, AgentResult{
			AgentID:  inst.ID,
			HostID:   inst.HostID,
			Status:   inst.Status,
			Output:   inst.Output,
			Error:    inst.Error,
			Duration: inst.Duration,
		})
	}
	return results
}

// ---------------------------------------------------------------------------
// Helper methods for querying agent state.
// ---------------------------------------------------------------------------

// GetInstance returns the agent instance for the given ID, or nil if not found.
func (m *AgentManager) GetInstance(agentID string) *AgentInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.instances[agentID]
}

// ListInstances returns all agent instances for the given mission.
func (m *AgentManager) ListInstances(missionID string) []*AgentInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*AgentInstance
	for _, inst := range m.instances {
		if inst.MissionID == missionID {
			result = append(result, inst)
		}
	}
	return result
}

// RunningCount returns the number of agents currently in running status
// for the given mission.
func (m *AgentManager) RunningCount(missionID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, inst := range m.instances {
		if inst.MissionID == missionID && inst.Status == AgentStatusRunning {
			count++
		}
	}
	return count
}
