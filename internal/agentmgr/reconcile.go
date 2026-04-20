package agentmgr

import (
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Reconcile logic — executed after service restart.
//
// Requirement 13.8: WHEN 服务重启后，THE AgentManager SHALL 执行 reconcile
// 逻辑，将所有非终态的 Worker_Agent 标记为 failed，不误恢复为 running.
//
// Design Property 38: Agent Reconcile 安全性
// For any 重启前的 agent 状态快照（包含 running 状态的 agent），reconcile 后
// 所有非终态 agent 应被标记为 failed，不误恢复为 running.
// ---------------------------------------------------------------------------

// AgentReconcileSummary contains the results of an agent reconcile operation.
type AgentReconcileSummary struct {
	// ReconciledAgents lists the agent IDs that were moved to failed state.
	ReconciledAgents []string
	// AlreadyTerminal lists the agent IDs that were already in a terminal state.
	AlreadyTerminal []string
	// TotalAgents is the total number of agent instances examined.
	TotalAgents int
}

// ReconcileAgents executes post-restart reconciliation on the AgentManager and
// AgentBudgetController. It marks all non-terminal agent instances (idle/running/waiting)
// as failed with a reconcile reason, resets the AgentBudgetController state, and
// returns a summary.
//
// Key invariant: already-terminal agents (completed/failed/killed) are NEVER
// restored to running.
func ReconcileAgents(mgr *AgentManager, bc *AgentBudgetController) (*AgentReconcileSummary, error) {
	if mgr == nil {
		return nil, fmt.Errorf("AgentManager is nil")
	}
	if bc == nil {
		return nil, fmt.Errorf("AgentBudgetController is nil")
	}

	// Clear budget controller state first — after restart, no agents should be
	// considered running or queued in the budget.
	bc.Reset()

	// Reconcile all agent instances in the AgentManager.
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	summary := &AgentReconcileSummary{
		TotalAgents: len(mgr.instances),
	}

	now := time.Now()
	reason := "reconciled after restart"

	for _, inst := range mgr.instances {
		if inst.Status.IsTerminal() {
			// Already in terminal state (completed/failed/killed) — leave untouched.
			summary.AlreadyTerminal = append(summary.AlreadyTerminal, inst.ID)
			continue
		}

		// Non-terminal (idle/running/waiting) — mark as failed.
		inst.Status = AgentStatusFailed
		inst.Error = reason
		inst.UpdatedAt = now
		summary.ReconciledAgents = append(summary.ReconciledAgents, inst.ID)
	}

	return summary, nil
}
