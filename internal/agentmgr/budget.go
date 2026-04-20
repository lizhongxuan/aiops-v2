package agentmgr

import (
	"fmt"
	"sync"
)

// ---------------------------------------------------------------------------
// AgentBudgetController — enforces per-mission concurrency budget for Worker Agents.
//
// Requirement 13.6: THE AgentManager SHALL 实现并发预算控制，同时运行的
// Worker_Agent 数量不超过 mission 级预算上限，超出预算的 agent 进入队列等待.
//
// Design Property 36: Agent 并发预算控制
// For any mission 的 agent 集合，同时处于 running 状态的 Worker_Agent 数量
// 不应超过 mission 级预算上限.
// ---------------------------------------------------------------------------

// AgentBudgetController manages per-mission concurrency budget for Worker Agents.
// Each mission has an independent budget that limits how many Worker Agents can
// run concurrently. Agents exceeding the budget are queued and automatically
// promoted when running agents complete.
type AgentBudgetController struct {
	mu             sync.Mutex
	missionBudget  int                    // per-mission max concurrent agents
	missions       map[string]*missionBudgetState // missionID → state
}

// missionBudgetState tracks the running and queued agents for a single mission.
type missionBudgetState struct {
	running map[string]bool // agentID → true
	queue   []string        // ordered queue of pending agent IDs
}

// NewAgentBudgetController creates an AgentBudgetController with the given
// per-mission max concurrency budget. missionBudget must be >= 1.
func NewAgentBudgetController(missionBudget int) (*AgentBudgetController, error) {
	if missionBudget < 1 {
		return nil, fmt.Errorf("missionBudget must be >= 1, got %d", missionBudget)
	}
	return &AgentBudgetController{
		missionBudget: missionBudget,
		missions:      make(map[string]*missionBudgetState),
	}, nil
}

// ensureMission returns or creates the budget state for a mission.
// Caller must hold bc.mu.
func (bc *AgentBudgetController) ensureMission(missionID string) *missionBudgetState {
	state, ok := bc.missions[missionID]
	if !ok {
		state = &missionBudgetState{
			running: make(map[string]bool),
		}
		bc.missions[missionID] = state
	}
	return state
}

// TryAcquire attempts to acquire a budget slot for the given agent in the
// specified mission. If budget is available, the agent is marked as running
// and returns true. If budget is exhausted, the agent is enqueued and returns
// false. Returns an error if the agentID is already running or queued.
func (bc *AgentBudgetController) TryAcquire(missionID, agentID string) (acquired bool, err error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if missionID == "" {
		return false, fmt.Errorf("missionID is required")
	}
	if agentID == "" {
		return false, fmt.Errorf("agentID is required")
	}

	state := bc.ensureMission(missionID)

	// Check if already running.
	if state.running[agentID] {
		return false, fmt.Errorf("agent %q is already running in mission %q", agentID, missionID)
	}

	// Check if already queued.
	for _, qid := range state.queue {
		if qid == agentID {
			return false, fmt.Errorf("agent %q is already queued in mission %q", agentID, missionID)
		}
	}

	// Try to acquire budget.
	if len(state.running) < bc.missionBudget {
		state.running[agentID] = true
		return true, nil
	}

	// Budget exhausted — enqueue.
	state.queue = append(state.queue, agentID)
	return false, nil
}

// Release releases the budget slot for the given agent in the specified mission
// and triggers queue backfill. Returns the agentID that was promoted from the
// queue (empty string if none).
func (bc *AgentBudgetController) Release(missionID, agentID string) (promoted string, err error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if missionID == "" {
		return "", fmt.Errorf("missionID is required")
	}
	if agentID == "" {
		return "", fmt.Errorf("agentID is required")
	}

	state, ok := bc.missions[missionID]
	if !ok {
		return "", fmt.Errorf("mission %q not found", missionID)
	}

	if !state.running[agentID] {
		return "", fmt.Errorf("agent %q is not running in mission %q", agentID, missionID)
	}

	// Release the slot.
	delete(state.running, agentID)

	// Queue backfill: promote the next queued agent.
	if len(state.queue) > 0 {
		promoted = state.queue[0]
		state.queue = state.queue[1:]
		state.running[promoted] = true
	}

	return promoted, nil
}

// Remove removes an agent from either the running set or the queue for the
// specified mission. This is used when an agent is killed or fails before
// completion. Returns true if the agent was found and removed. If the agent
// was running, triggers queue backfill and returns the promoted agentID.
func (bc *AgentBudgetController) Remove(missionID, agentID string) (found bool, promoted string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	state, ok := bc.missions[missionID]
	if !ok {
		return false, ""
	}

	// Check running set.
	if state.running[agentID] {
		delete(state.running, agentID)
		// Backfill from queue.
		if len(state.queue) > 0 {
			promoted = state.queue[0]
			state.queue = state.queue[1:]
			state.running[promoted] = true
		}
		return true, promoted
	}

	// Check queue.
	for i, qid := range state.queue {
		if qid == agentID {
			state.queue = append(state.queue[:i], state.queue[i+1:]...)
			return true, ""
		}
	}

	return false, ""
}

// RunningCount returns the number of currently running agents for the given mission.
func (bc *AgentBudgetController) RunningCount(missionID string) int {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	state, ok := bc.missions[missionID]
	if !ok {
		return 0
	}
	return len(state.running)
}

// QueueLen returns the number of agents waiting in the queue for the given mission.
func (bc *AgentBudgetController) QueueLen(missionID string) int {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	state, ok := bc.missions[missionID]
	if !ok {
		return 0
	}
	return len(state.queue)
}

// MissionBudget returns the configured per-mission max concurrency budget.
func (bc *AgentBudgetController) MissionBudget() int {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.missionBudget
}

// IsRunning reports whether the given agent currently holds a budget slot
// in the specified mission.
func (bc *AgentBudgetController) IsRunning(missionID, agentID string) bool {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	state, ok := bc.missions[missionID]
	if !ok {
		return false
	}
	return state.running[agentID]
}

// IsQueued reports whether the given agent is currently in the wait queue
// for the specified mission.
func (bc *AgentBudgetController) IsQueued(missionID, agentID string) bool {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	state, ok := bc.missions[missionID]
	if !ok {
		return false
	}
	for _, qid := range state.queue {
		if qid == agentID {
			return true
		}
	}
	return false
}

// RunningAgents returns a copy of the currently running agent IDs for the
// given mission.
func (bc *AgentBudgetController) RunningAgents(missionID string) []string {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	state, ok := bc.missions[missionID]
	if !ok {
		return nil
	}
	result := make([]string, 0, len(state.running))
	for id := range state.running {
		result = append(result, id)
	}
	return result
}

// QueuedAgents returns a copy of the queued agent IDs in order for the
// given mission.
func (bc *AgentBudgetController) QueuedAgents(missionID string) []string {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	state, ok := bc.missions[missionID]
	if !ok {
		return nil
	}
	result := make([]string, len(state.queue))
	copy(result, state.queue)
	return result
}

// ResetMission clears all running agents and queued agents for the specified
// mission. This is used during reconcile after a restart.
func (bc *AgentBudgetController) ResetMission(missionID string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	delete(bc.missions, missionID)
}

// Reset clears all mission budget state. Used during full reconcile.
func (bc *AgentBudgetController) Reset() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.missions = make(map[string]*missionBudgetState)
}
