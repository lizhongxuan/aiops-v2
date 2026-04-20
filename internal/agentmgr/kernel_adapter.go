package agentmgr

import (
	"context"
	"fmt"
	"time"

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
