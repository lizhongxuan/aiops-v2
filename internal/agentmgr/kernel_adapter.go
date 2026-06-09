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
	manager    *AgentManager
	factory    *AgentFactory
	store      hostops.MissionStore
	transcript hostops.TranscriptStore
}

// NewKernelAdapter creates a KernelAdapter for the given AgentManager and Factory.
func NewKernelAdapter(manager *AgentManager, factory *AgentFactory) *KernelAdapter {
	return &KernelAdapter{
		manager: manager,
		factory: factory,
	}
}

// WithHostOpsSinks wires optional stores for asynchronous child status and
// transcript updates. The adapter still works without sinks, but UI drawers
// only see completed child output when these sinks are configured.
func (a *KernelAdapter) WithHostOpsSinks(store hostops.MissionStore, transcript hostops.TranscriptStore) *KernelAdapter {
	if a == nil {
		return a
	}
	a.store = store
	a.transcript = transcript
	return a
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
	config, err := a.factory.CreateHostChildAgent(ctx, req)
	if err != nil {
		return hostops.HostChildAgent{}, err
	}
	_, err = a.manager.Spawn(ctx, SpawnRequest{
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
	child := hostops.HostChildAgent{
		ID:               req.ChildAgentID,
		MissionID:        req.MissionID,
		ParentAgentID:    req.ParentAgentID,
		SessionID:        req.SessionID,
		HostID:           req.HostID,
		HostAddress:      req.HostAddress,
		Role:             req.Role,
		Task:             req.Task,
		Status:           hostops.HostChildAgentStatusRunning,
		LastInputPreview: req.Task,
		StartedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		PlanStepIDs:      nil,
	}
	a.startHostChildRun(req.ChildAgentID, child, config, false)
	return child, nil
}

func (a *KernelAdapter) SendMessage(ctx context.Context, childAgentID, content string) (hostops.HostChildAgent, error) {
	if a == nil || a.manager == nil {
		return hostops.HostChildAgent{}, fmt.Errorf("agent manager adapter is not available")
	}
	inst := a.manager.GetInstance(childAgentID)
	if inst == nil {
		return hostops.HostChildAgent{}, fmt.Errorf("host child agent %q not found", childAgentID)
	}
	config, err := a.factory.CreateHostChildAgent(ctx, hostops.SpawnHostChildRequest{
		ChildAgentID:  inst.ID,
		MissionID:     inst.MissionID,
		ParentAgentID: inst.ParentID,
		SessionID:     inst.SessionID,
		HostID:        inst.HostID,
		Task:          content,
	})
	if err != nil {
		return hostops.HostChildAgent{}, err
	}
	child := hostops.HostChildAgent{
		ID:                inst.ID,
		MissionID:         inst.MissionID,
		ParentAgentID:     inst.ParentID,
		SessionID:         inst.SessionID,
		HostID:            inst.HostID,
		Task:              inst.Task,
		Status:            hostops.HostChildAgentStatusRunning,
		LastInputPreview:  content,
		LastOutputPreview: inst.Output,
		Error:             inst.Error,
		StartedAt:         inst.CreatedAt,
		UpdatedAt:         time.Now().UTC(),
	}
	a.startHostChildRun(inst.ID, child, config, true)
	return child, nil
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

func (a *KernelAdapter) startHostChildRun(agentID string, child hostops.HostChildAgent, config *AgentConfig, followup bool) {
	if a == nil || a.manager == nil {
		return
	}
	go func() {
		runCtx := context.Background()
		var result *AgentResult
		var err error
		if followup {
			result, err = a.manager.RunAgentTurn(runCtx, agentID, config)
		} else {
			result, err = a.manager.RunAgent(runCtx, agentID, config)
		}
		a.recordHostChildRunResult(runCtx, child, result, err)
	}()
}

func (a *KernelAdapter) recordHostChildRunResult(ctx context.Context, child hostops.HostChildAgent, result *AgentResult, runErr error) {
	if result != nil {
		child.LastOutputPreview = result.Output
		child.Error = result.Error
		child.Status = hostChildStatusFromAgentStatus(result.Status)
	}
	if runErr != nil {
		child.Status = hostops.HostChildAgentStatusFailed
		child.Error = runErr.Error()
	}
	if child.Status == "" {
		child.Status = hostops.HostChildAgentStatusCompleted
	}
	child.UpdatedAt = time.Now().UTC()
	if isTerminalHostChildStatus(child.Status) {
		completedAt := child.UpdatedAt
		child.CompletedAt = &completedAt
	}
	a.saveHostChildUpdate(ctx, child)
	a.appendHostChildTranscript(ctx, child)
}

func (a *KernelAdapter) saveHostChildUpdate(ctx context.Context, child hostops.HostChildAgent) {
	if a == nil || a.store == nil || child.ID == "" {
		return
	}
	for attempt := 0; attempt < 8; attempt++ {
		current, err := a.store.GetChildAgent(ctx, child.ID)
		if err == nil {
			child = mergeAdapterChildUpdate(current, child)
			_ = a.store.SaveChildAgent(ctx, child)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = a.store.SaveChildAgent(ctx, child)
}

func (a *KernelAdapter) appendHostChildTranscript(ctx context.Context, child hostops.HostChildAgent) {
	if a == nil || a.transcript == nil || child.ID == "" {
		return
	}
	if child.Error != "" {
		_ = a.transcript.Append(ctx, child.ID, hostops.TranscriptItem{
			Type:    hostops.TranscriptItemError,
			Content: child.Error,
			Status:  string(child.Status),
		})
		return
	}
	if child.LastOutputPreview != "" {
		_ = a.transcript.Append(ctx, child.ID, hostops.TranscriptItem{
			Type:    hostops.TranscriptItemAssistantMessage,
			Content: child.LastOutputPreview,
			Status:  string(child.Status),
		})
	}
}

func mergeAdapterChildUpdate(current, update hostops.HostChildAgent) hostops.HostChildAgent {
	if update.MissionID == "" {
		update.MissionID = current.MissionID
	}
	if update.ParentAgentID == "" {
		update.ParentAgentID = current.ParentAgentID
	}
	if update.SessionID == "" {
		update.SessionID = current.SessionID
	}
	if update.HostID == "" {
		update.HostID = current.HostID
	}
	if update.HostAddress == "" {
		update.HostAddress = current.HostAddress
	}
	if update.Role == "" {
		update.Role = current.Role
	}
	if update.Task == "" {
		update.Task = current.Task
	}
	if update.LastInputPreview == "" {
		update.LastInputPreview = current.LastInputPreview
	}
	if update.StartedAt.IsZero() {
		update.StartedAt = current.StartedAt
	}
	if len(update.PlanStepIDs) == 0 {
		update.PlanStepIDs = current.PlanStepIDs
	}
	return update
}

func hostChildStatusFromAgentStatus(status AgentStatus) hostops.HostChildAgentStatus {
	switch status {
	case AgentStatusCompleted:
		return hostops.HostChildAgentStatusCompleted
	case AgentStatusFailed:
		return hostops.HostChildAgentStatusFailed
	case AgentStatusKilled:
		return hostops.HostChildAgentStatusCancelled
	case AgentStatusRunning:
		return hostops.HostChildAgentStatusRunning
	case AgentStatusWaiting:
		return hostops.HostChildAgentStatusWaiting
	default:
		return hostops.HostChildAgentStatusFailed
	}
}

func isTerminalHostChildStatus(status hostops.HostChildAgentStatus) bool {
	switch status {
	case hostops.HostChildAgentStatusCompleted, hostops.HostChildAgentStatusFailed, hostops.HostChildAgentStatusCancelled:
		return true
	default:
		return false
	}
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
