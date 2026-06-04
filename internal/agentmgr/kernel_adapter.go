package agentmgr

import (
	"context"
	"fmt"
	"time"

	"aiops-v2/internal/hostops"
	"aiops-v2/internal/runtimekernel"
)

// ---------------------------------------------------------------------------
// KernelAdapter adapts *AgentManager to the runtimekernel.AgentManagerSource
// interface, breaking the circular import between runtimekernel and agentmgr.
// ---------------------------------------------------------------------------

// KernelAdapter wraps an AgentManager and its AgentFactory to implement
// runtimekernel.AgentManagerSource. This is the bridge used at wiring time
// (e.g., in main.go) to inject AgentManager into EinoKernel.
type KernelAdapter struct {
	manager *AgentManager
	factory *AgentFactory
}

// NewKernelAdapter creates a KernelAdapter for the given AgentManager and Factory.
func NewKernelAdapter(manager *AgentManager, factory *AgentFactory) *KernelAdapter {
	return &KernelAdapter{
		manager: manager,
		factory: factory,
	}
}

// CreateWorkspaceAgent creates a workspace PlanExecuteAgent config via AgentFactory.
// This validates that the factory can produce a valid workspace agent configuration.
func (a *KernelAdapter) CreateWorkspaceAgent(ctx context.Context, missionID string) error {
	if a.factory == nil {
		return fmt.Errorf("agent factory not available")
	}
	_, err := a.factory.CreateWorkspaceAgent(ctx, missionID)
	if err != nil {
		return fmt.Errorf("create workspace agent: %w", err)
	}
	return nil
}

// SpawnAndRunPlanner spawns a planner agent, runs it via AgentManager, and
// returns the output. This encapsulates the spawn → run lifecycle for the
// planner component of a PlanExecuteAgent.
func (a *KernelAdapter) SpawnAndRunPlanner(ctx context.Context, missionID, sessionID, task string) (string, error) {
	if a.factory == nil {
		return "", fmt.Errorf("agent factory not available")
	}

	// Create workspace agent config to get the planner configuration.
	wsCfg, err := a.factory.CreateWorkspaceAgent(ctx, missionID)
	if err != nil {
		return "", fmt.Errorf("create workspace agent config: %w", err)
	}

	// Spawn the planner agent.
	plannerID := fmt.Sprintf("planner-%s-%d", missionID, time.Now().UnixNano())
	_, spawnErr := a.manager.Spawn(ctx, SpawnRequest{
		ID:        plannerID,
		Kind:      AgentKindPlanner,
		MissionID: missionID,
		SessionID: sessionID,
		Task:      task,
	})
	if spawnErr != nil {
		return "", fmt.Errorf("spawn planner agent: %w", spawnErr)
	}

	// Run the planner agent.
	result, runErr := a.manager.RunAgent(ctx, plannerID, &wsCfg.Planner)
	if runErr != nil {
		return "", fmt.Errorf("run planner agent: %w", runErr)
	}

	if result.Status == AgentStatusFailed {
		return "", fmt.Errorf("planner agent failed: %s", result.Error)
	}

	return result.Output, nil
}

func (a *KernelAdapter) SpawnHostChild(ctx context.Context, req hostops.SpawnHostChildRequest) (hostops.HostChildAgent, error) {
	if a == nil || a.factory == nil || a.manager == nil {
		return hostops.HostChildAgent{}, fmt.Errorf("agent manager adapter is not available")
	}
	if req.HostID == "" {
		return hostops.HostChildAgent{}, fmt.Errorf("hostId is required")
	}
	if req.MissionID == "" {
		return hostops.HostChildAgent{}, fmt.Errorf("missionId is required")
	}
	if req.SessionID == "" {
		req.SessionID = fmt.Sprintf("host-child:%s:%s", req.MissionID, req.HostID)
	}
	if req.ChildAgentID == "" {
		req.ChildAgentID = fmt.Sprintf("host-child-%s-%d", req.HostID, time.Now().UnixNano())
	}
	if _, err := a.factory.CreateHostChildAgent(ctx, req); err != nil {
		return hostops.HostChildAgent{}, err
	}
	_, err := a.manager.Spawn(ctx, SpawnRequest{
		ID:        req.ChildAgentID,
		Kind:      AgentKindWorker,
		MissionID: req.MissionID,
		ParentID:  req.ParentAgentID,
		HostID:    req.HostID,
		SessionID: req.SessionID,
		Task:      req.Task,
	})
	if err != nil {
		return hostops.HostChildAgent{}, err
	}
	return hostops.HostChildAgent{
		ID:               req.ChildAgentID,
		MissionID:        req.MissionID,
		ParentAgentID:    req.ParentAgentID,
		SessionID:        req.SessionID,
		HostID:           req.HostID,
		HostAddress:      req.HostAddress,
		Role:             req.Role,
		Task:             req.Task,
		Status:           hostops.HostChildAgentStatusSpawning,
		LastInputPreview: req.Task,
		StartedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PlanStepIDs:      nil,
	}, nil
}

func (a *KernelAdapter) SendMessage(_ context.Context, childAgentID, content string) (hostops.HostChildAgent, error) {
	if a == nil || a.manager == nil {
		return hostops.HostChildAgent{}, fmt.Errorf("agent manager adapter is not available")
	}
	inst := a.manager.GetInstance(childAgentID)
	if inst == nil {
		return hostops.HostChildAgent{}, fmt.Errorf("host child agent %q not found", childAgentID)
	}
	return hostops.HostChildAgent{
		ID:                inst.ID,
		MissionID:         inst.MissionID,
		ParentAgentID:     inst.ParentID,
		SessionID:         inst.SessionID,
		HostID:            inst.HostID,
		Task:              inst.Task,
		Status:            hostops.HostChildAgentStatusWaiting,
		LastInputPreview:  content,
		LastOutputPreview: inst.Output,
		Error:             inst.Error,
		StartedAt:         inst.CreatedAt,
		UpdatedAt:         time.Now().UTC(),
	}, nil
}

func (a *KernelAdapter) Stop(ctx context.Context, childAgentID string) (hostops.HostChildAgent, error) {
	if a == nil || a.manager == nil {
		return hostops.HostChildAgent{}, fmt.Errorf("agent manager adapter is not available")
	}
	if err := a.manager.KillAgent(ctx, childAgentID); err != nil {
		return hostops.HostChildAgent{}, err
	}
	inst := a.manager.GetInstance(childAgentID)
	if inst == nil {
		return hostops.HostChildAgent{}, fmt.Errorf("host child agent %q not found", childAgentID)
	}
	now := time.Now().UTC()
	return hostops.HostChildAgent{
		ID:            inst.ID,
		MissionID:     inst.MissionID,
		ParentAgentID: inst.ParentID,
		SessionID:     inst.SessionID,
		HostID:        inst.HostID,
		Task:          inst.Task,
		Status:        hostops.HostChildAgentStatusCancelled,
		Error:         inst.Error,
		StartedAt:     inst.CreatedAt,
		UpdatedAt:     now,
		CompletedAt:   &now,
	}, nil
}

// CollectResults returns all terminal agent results for the given mission,
// converted to the runtimekernel.AgentResult type for projection.
func (a *KernelAdapter) CollectResults(missionID string) []runtimekernel.AgentResult {
	results := a.manager.CollectResults(missionID)
	out := make([]runtimekernel.AgentResult, 0, len(results))
	for _, r := range results {
		out = append(out, runtimekernel.AgentResult{
			AgentID:    r.AgentID,
			HostID:     r.HostID,
			Status:     string(r.Status),
			Output:     r.Output,
			Error:      r.Error,
			DurationMs: r.Duration.Milliseconds(),
		})
	}
	return out
}
