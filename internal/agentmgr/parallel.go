package agentmgr

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// ParallelAgent — parallel execution of Worker Agents within a mission.
//
// Requirement 13.9: THE AgentManager SHALL 使用 adk.NewParallelAgent 并行执行
// 同一 mission 下的多个 Worker Agent.
//
// This integrates with the PlanExecuteAgent's Executor phase: the Executor
// spawns a ParallelAgent that runs multiple Worker ChatModelAgents concurrently,
// each bound to a specific host. Results are collected and returned to the
// Replanner for evaluation.
// ---------------------------------------------------------------------------

// ParallelWorkerRequest describes a single worker to be executed in parallel.
type ParallelWorkerRequest struct {
	// AgentID is the unique identifier for this worker agent instance.
	AgentID string

	// HostID is the host this worker is bound to.
	HostID string

	// Config is the assembled agent configuration for this worker.
	Config *AgentConfig

	// ResourceLocks declares planned generic resource scopes for this worker.
	ResourceLocks []ResourceLockKey
}

// ParallelExecutionResult holds the aggregated results of parallel worker execution.
type ParallelExecutionResult struct {
	// Results contains the outcome of each worker, keyed by AgentID.
	Results map[string]*AgentResult

	// Succeeded lists agent IDs that completed successfully.
	Succeeded []string

	// Failed lists agent IDs that failed.
	Failed []string

	// ResourceLocks records lock acquire decisions by AgentID.
	ResourceLocks map[string][]ResourceLockResult

	// TotalDuration is the wall-clock time for the entire parallel execution.
	TotalDuration time.Duration
}

// ParallelAgent orchestrates parallel execution of multiple Worker Agents
// within a single mission. It respects the BudgetController's concurrency
// limits and handles partial failures gracefully.
//
// In production, this represents the behavior of adk.NewParallelAgent:
// spawning multiple ChatModelAgent workers concurrently and collecting their
// results for the Replanner to evaluate.
type ParallelAgent struct {
	manager *AgentManager
	budget  *AgentBudgetController
	locks   *ResourceLockManager
}

// NewParallelAgent creates a ParallelAgent that uses the given AgentManager
// for execution and AgentBudgetController for concurrency control.
func NewParallelAgent(manager *AgentManager, budget *AgentBudgetController) (*ParallelAgent, error) {
	if manager == nil {
		return nil, fmt.Errorf("AgentManager is required")
	}
	if budget == nil {
		return nil, fmt.Errorf("AgentBudgetController is required")
	}
	return &ParallelAgent{
		manager: manager,
		budget:  budget,
	}, nil
}

func (pa *ParallelAgent) WithResourceLockManager(locks *ResourceLockManager) *ParallelAgent {
	if pa == nil {
		return nil
	}
	pa.locks = locks
	return pa
}

// Execute runs all workers in parallel, respecting budget constraints.
// Workers that cannot immediately acquire budget are queued and executed
// as slots become available. The method blocks until all workers complete
// (or the context is cancelled).
//
// Partial failures are handled: if some workers fail, the remaining workers
// continue execution. The caller receives results for all workers.
func (pa *ParallelAgent) Execute(ctx context.Context, missionID string, workers []ParallelWorkerRequest) (*ParallelExecutionResult, error) {
	if missionID == "" {
		return nil, fmt.Errorf("missionID is required")
	}
	if len(workers) == 0 {
		return &ParallelExecutionResult{
			Results:       make(map[string]*AgentResult),
			ResourceLocks: make(map[string][]ResourceLockResult),
		}, nil
	}

	// Validate all workers have unique IDs and required fields.
	seen := make(map[string]bool, len(workers))
	for _, w := range workers {
		if w.AgentID == "" {
			return nil, fmt.Errorf("worker AgentID is required")
		}
		if w.Config == nil {
			return nil, fmt.Errorf("worker %q Config is required", w.AgentID)
		}
		if seen[w.AgentID] {
			return nil, fmt.Errorf("duplicate worker AgentID %q", w.AgentID)
		}
		seen[w.AgentID] = true
	}

	startTime := time.Now()

	var (
		mu            sync.Mutex
		results       = make(map[string]*AgentResult, len(workers))
		resourceLocks = make(map[string][]ResourceLockResult, len(workers))
		wg            sync.WaitGroup
	)

	// Channel to signal when a budget slot is released, allowing queued
	// workers to proceed.
	slotReleased := make(chan struct{}, len(workers))

	// executeWorker runs a single worker: acquires budget, executes via
	// AgentManager, releases budget, and records the result.
	executeWorker := func(w ParallelWorkerRequest) {
		defer wg.Done()

		// Wait for budget acquisition.
		acquired, err := pa.budget.TryAcquire(missionID, w.AgentID)
		if err != nil {
			// Agent already running/queued — record as failed.
			mu.Lock()
			results[w.AgentID] = &AgentResult{
				AgentID: w.AgentID,
				HostID:  w.HostID,
				Status:  AgentStatusFailed,
				Error:   fmt.Sprintf("budget acquire error: %v", err),
			}
			mu.Unlock()
			return
		}

		if !acquired {
			// Queued — wait for a slot to open or context cancellation.
			for {
				select {
				case <-ctx.Done():
					// Context cancelled while waiting in queue.
					pa.budget.Remove(missionID, w.AgentID)
					mu.Lock()
					results[w.AgentID] = &AgentResult{
						AgentID: w.AgentID,
						HostID:  w.HostID,
						Status:  AgentStatusFailed,
						Error:   fmt.Sprintf("context cancelled while queued: %v", ctx.Err()),
					}
					mu.Unlock()
					return
				case <-slotReleased:
					// A slot was released — check if we were promoted.
					if pa.budget.IsRunning(missionID, w.AgentID) {
						goto execute
					}
					// Not promoted yet, keep waiting.
				}
			}
		}

	execute:
		releaseBudget := func() {
			_, _ = pa.budget.Release(missionID, w.AgentID)
			select {
			case slotReleased <- struct{}{}:
			default:
			}
		}

		lockResults, releaseLocks, lockErr := pa.acquireWorkerResourceLocks(w)
		if len(lockResults) > 0 {
			mu.Lock()
			resourceLocks[w.AgentID] = append(resourceLocks[w.AgentID], lockResults...)
			mu.Unlock()
		}
		if lockErr != nil {
			mu.Lock()
			results[w.AgentID] = &AgentResult{
				AgentID: w.AgentID,
				HostID:  w.HostID,
				Status:  AgentStatusFailed,
				Error:   lockErr.Error(),
			}
			mu.Unlock()
			releaseBudget()
			return
		}
		if releaseLocks != nil {
			defer releaseLocks()
		}

		// Execute the worker via AgentManager.RunAgent.
		result, runErr := pa.manager.RunAgent(ctx, w.AgentID, w.Config)
		if runErr != nil {
			// RunAgent itself failed (e.g., agent not found).
			mu.Lock()
			results[w.AgentID] = &AgentResult{
				AgentID: w.AgentID,
				HostID:  w.HostID,
				Status:  AgentStatusFailed,
				Error:   runErr.Error(),
			}
			mu.Unlock()
		} else {
			mu.Lock()
			results[w.AgentID] = result
			mu.Unlock()
		}

		releaseBudget()
	}

	// Spawn all agents first via AgentManager.Spawn, then launch goroutines.
	for _, w := range workers {
		_, spawnErr := pa.manager.Spawn(ctx, SpawnRequest{
			ID:        w.AgentID,
			Kind:      AgentKindWorker,
			MissionID: missionID,
			HostID:    w.HostID,
			SessionID: fmt.Sprintf("parallel-%s-%s", missionID, w.AgentID),
			Task:      fmt.Sprintf("parallel worker for host %s", w.HostID),
		})
		if spawnErr != nil {
			// If spawn fails, record immediately and skip execution.
			results[w.AgentID] = &AgentResult{
				AgentID: w.AgentID,
				HostID:  w.HostID,
				Status:  AgentStatusFailed,
				Error:   fmt.Sprintf("spawn error: %v", spawnErr),
			}
			continue
		}

		wg.Add(1)
		go executeWorker(w)
	}

	// Wait for all workers to complete.
	wg.Wait()
	close(slotReleased)

	// Build the aggregated result.
	execResult := &ParallelExecutionResult{
		Results:       results,
		ResourceLocks: resourceLocks,
		TotalDuration: time.Since(startTime),
	}

	for agentID, r := range results {
		if r.Status == AgentStatusCompleted {
			execResult.Succeeded = append(execResult.Succeeded, agentID)
		} else {
			execResult.Failed = append(execResult.Failed, agentID)
		}
	}

	return execResult, nil
}

func (pa *ParallelAgent) acquireWorkerResourceLocks(w ParallelWorkerRequest) ([]ResourceLockResult, func(), error) {
	if pa == nil || pa.locks == nil || len(w.ResourceLocks) == 0 {
		return nil, nil, nil
	}
	results := make([]ResourceLockResult, 0, len(w.ResourceLocks))
	acquired := make([]ResourceLockKey, 0, len(w.ResourceLocks))
	release := func() {
		for i := len(acquired) - 1; i >= 0; i-- {
			pa.locks.Release(acquired[i], w.AgentID)
		}
	}
	for _, key := range w.ResourceLocks {
		result, err := pa.locks.TryAcquire(key, w.AgentID)
		if err != nil {
			release()
			return append(results, result), nil, fmt.Errorf("resource lock acquire error: %w", err)
		}
		results = append(results, result)
		if !result.Acquired {
			release()
			reason := result.Reason
			if reason == "" {
				reason = "resource_lock_conflict"
			}
			if result.BlockingAgentID != "" {
				return results, nil, fmt.Errorf("%s: holder=%s", reason, result.BlockingAgentID)
			}
			return results, nil, fmt.Errorf("%s", reason)
		}
		acquired = append(acquired, result.Key)
	}
	return results, release, nil
}
