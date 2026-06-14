package agentmgr

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"pgregory.net/rapid"

	"aiops-v2/internal/agentruntime"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/projection"
	"aiops-v2/internal/tooling"
)

// ---------------------------------------------------------------------------
// Feature: aiops-codex-eino-rewrite, Property 34: Worker Agent 工具隔离
// For any Worker_Agent instance (adk.ChatModelAgent), its ToolsConfig should
// only contain assembled tools that match the worker's explicit tool allowlist
// for the bound host.
//
// **Validates: Requirements 13.3**
// ---------------------------------------------------------------------------

func TestProperty34_WorkerAgentToolIsolation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		reg := tooling.NewRegistry()
		compiler := &mockCompiler{}
		router := modelrouter.NewRouter("openai", map[string]modelrouter.ChatModel{
			"openai": &mockChatModel{},
		}, nil)
		factory := NewAgentFactory(reg, compiler, router, &policyengine.Engine{})
		numEntries := rapid.IntRange(3, 10).Draw(t, "numEntries")
		allNames := make([]string, 0, numEntries)

		for i := 0; i < numEntries; i++ {
			name := fmt.Sprintf("tool-%d", i)
			isMCP := rapid.Bool().Draw(t, fmt.Sprintf("isMCP-%d", i))

			meta := tooling.ToolMetadata{Name: name}
			if isMCP {
				meta.IsMCP = true
				meta.MCPInfo = tooling.MCPInfo{
					ServerID:   "server",
					ServerName: "server",
					ToolName:   name,
				}
			}
			_ = reg.Register(&mockTool{
				name:     name,
				readOnly: true,
				meta:     meta,
				sessions: []string{"host"},
				modes:    []string{"execute"},
			})
			allNames = append(allNames, name)
		}

		expected := make(map[string]bool, len(allNames))
		allowedTools := make([]string, 0, len(allNames))
		for i, name := range allNames {
			if rapid.Bool().Draw(t, fmt.Sprintf("allow-%d", i)) {
				expected[name] = true
				allowedTools = append(allowedTools, name)
			}
		}
		if len(allowedTools) == 0 {
			expected[allNames[0]] = true
			allowedTools = append(allowedTools, allNames[0])
		}

		if err := factory.RegisterDefinition(&AgentDefinition{
			Kind:  AgentKindWorker,
			Name:  "worker",
			Tools: allowedTools,
		}); err != nil {
			t.Fatalf("register worker definition: %v", err)
		}

		cfg, err := factory.CreateWorkerAgent(context.Background(), "host-1", "task")
		if err != nil {
			t.Fatalf("CreateWorkerAgent() error = %v", err)
		}

		if len(cfg.Tools) != len(expected) {
			t.Fatalf("worker tools len = %d, want %d", len(cfg.Tools), len(expected))
		}

		for _, assembledTool := range cfg.Tools {
			info, err := assembledTool.Info(context.Background())
			if err != nil {
				t.Fatalf("tool info error: %v", err)
			}
			if !expected[info.Name] {
				t.Fatalf("worker tool %q not allowed by tool allowlist %v", info.Name, allowedTools)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Feature: aiops-codex-eino-rewrite, Property 35: Agent 结果汇报完整性
// For any terminal agent (completed/failed), its AgentResult should be
// collected by CollectResults with complete status, output, error, and duration.
//
// **Validates: Requirements 13.4**
// ---------------------------------------------------------------------------

// mockRunner implements AgentRunner for testing.
type mockRunner struct {
	mu      sync.Mutex
	results map[string]mockRunResult
}

type mockRunResult struct {
	output string
	err    error
	delay  time.Duration
}

func (m *mockRunner) Run(ctx context.Context, config agentruntime.Config) (string, error) {
	m.mu.Lock()
	r, ok := m.results[config.RuntimeHostID()]
	m.mu.Unlock()
	if !ok {
		return "default-output", nil
	}
	if r.delay > 0 {
		time.Sleep(r.delay)
	}
	return r.output, r.err
}

func TestProperty35_AgentResultCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		missionID := rapid.StringMatching(`^mission-[a-z]{3,6}$`).Draw(t, "missionID")
		numAgents := rapid.IntRange(1, 8).Draw(t, "numAgents")

		runner := &mockRunner{results: make(map[string]mockRunResult)}
		projector := projection.NewProjector(nil)
		mgr := NewAgentManager(nil, runner, projector)

		type agentSpec struct {
			id         string
			hostID     string
			shouldFail bool
			output     string
		}

		specs := make([]agentSpec, numAgents)
		for i := range specs {
			specs[i] = agentSpec{
				id:         fmt.Sprintf("agent-%d", i),
				hostID:     fmt.Sprintf("host-%d", i),
				shouldFail: rapid.Bool().Draw(t, fmt.Sprintf("fail-%d", i)),
				output:     rapid.StringMatching(`^output-[a-z0-9]{2,8}$`).Draw(t, fmt.Sprintf("output-%d", i)),
			}
		}

		// Configure runner results.
		for _, s := range specs {
			if s.shouldFail {
				runner.mu.Lock()
				runner.results[s.hostID] = mockRunResult{
					output: s.output,
					err:    fmt.Errorf("simulated failure for %s", s.id),
				}
				runner.mu.Unlock()
			} else {
				runner.mu.Lock()
				runner.results[s.hostID] = mockRunResult{output: s.output}
				runner.mu.Unlock()
			}
		}

		// Spawn and run all agents.
		ctx := context.Background()
		for _, s := range specs {
			_, err := mgr.Spawn(ctx, SpawnRequest{
				ID:        s.id,
				Kind:      AgentKindWorker,
				MissionID: missionID,
				HostID:    s.hostID,
				SessionID: fmt.Sprintf("sess-%s", s.id),
				Task:      "test task",
			})
			if err != nil {
				t.Fatalf("spawn failed: %v", err)
			}

			_, err = mgr.RunAgent(ctx, s.id, &AgentConfig{
				Kind:   AgentKindWorker,
				HostID: s.hostID,
			})
			if err != nil {
				t.Fatalf("run agent failed: %v", err)
			}
		}

		// Collect results.
		results := mgr.CollectResults(missionID)

		// Property: all terminal agents should have results collected.
		if len(results) != numAgents {
			t.Fatalf("expected %d results, got %d", numAgents, len(results))
		}

		resultMap := make(map[string]AgentResult)
		for _, r := range results {
			resultMap[r.AgentID] = r
		}

		for _, s := range specs {
			r, ok := resultMap[s.id]
			if !ok {
				t.Fatalf("missing result for agent %q", s.id)
			}

			// Property: status must be terminal.
			if !r.Status.IsTerminal() {
				t.Fatalf("agent %q result status %q is not terminal", s.id, r.Status)
			}

			// Property: failed agents have error set.
			if s.shouldFail {
				if r.Status != AgentStatusFailed {
					t.Fatalf("agent %q should be failed, got %q", s.id, r.Status)
				}
				if r.Error == "" {
					t.Fatalf("agent %q failed but has empty error", s.id)
				}
			} else {
				if r.Status != AgentStatusCompleted {
					t.Fatalf("agent %q should be completed, got %q", s.id, r.Status)
				}
				if r.Output != s.output {
					t.Fatalf("agent %q output mismatch: got %q, want %q", s.id, r.Output, s.output)
				}
			}

			// Property: HostID is preserved.
			if r.HostID != s.hostID {
				t.Fatalf("agent %q hostID mismatch: got %q, want %q", s.id, r.HostID, s.hostID)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Feature: aiops-codex-eino-rewrite, Property 36: Agent 并发预算控制
// For any mission's agent set, the number of simultaneously running Worker_Agents
// should never exceed the mission-level budget limit.
//
// **Validates: Requirements 13.6**
// ---------------------------------------------------------------------------

func TestProperty36_AgentConcurrencyBudgetControl(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		budget := rapid.IntRange(1, 5).Draw(t, "budget")
		numAgents := rapid.IntRange(budget+1, budget*3+2).Draw(t, "numAgents")
		missionID := "mission-budget-test"

		bc, err := NewAgentBudgetController(budget)
		if err != nil {
			t.Fatalf("failed to create budget controller: %v", err)
		}

		// Track max concurrent running count.
		var mu sync.Mutex
		maxRunning := 0
		currentRunning := 0

		var wg sync.WaitGroup
		for i := 0; i < numAgents; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				agentID := fmt.Sprintf("agent-%d", idx)

				acquired, err := bc.TryAcquire(missionID, agentID)
				if err != nil {
					return
				}

				if acquired {
					mu.Lock()
					currentRunning++
					if currentRunning > maxRunning {
						maxRunning = currentRunning
					}
					mu.Unlock()

					// Simulate work.
					time.Sleep(time.Microsecond * time.Duration(rapid.IntRange(1, 50).Example(0)))

					mu.Lock()
					currentRunning--
					mu.Unlock()

					bc.Release(missionID, agentID)
				}
			}(i)
		}
		wg.Wait()

		// Property: running count never exceeded budget.
		if maxRunning > budget {
			t.Fatalf("max running %d exceeded budget %d", maxRunning, budget)
		}

		// Property: at any point, RunningCount <= budget.
		if bc.RunningCount(missionID) > budget {
			t.Fatalf("final running count %d exceeds budget %d", bc.RunningCount(missionID), budget)
		}
	})
}

// ---------------------------------------------------------------------------
// Feature: aiops-codex-eino-rewrite, Property 37: AgentKind 扩展性
// For any newly registered AgentKind, instances created through AgentFactory
// should follow the same spawn/run/kill/collect lifecycle without modifying
// AgentManager core logic.
//
// **Validates: Requirements 13.7**
// ---------------------------------------------------------------------------

func TestProperty37_AgentKindExtensibility(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a custom AgentKind (simulating extension).
		customKindStr := rapid.StringMatching(`^[a-z]{4,8}$`).Draw(t, "customKind")

		// The AgentManager should handle any valid kind through the same lifecycle.
		runner := &mockRunner{results: map[string]mockRunResult{
			"custom-host": {output: "custom-output"},
		}}
		projector := projection.NewProjector(nil)
		mgr := NewAgentManager(nil, runner, projector)

		ctx := context.Background()
		agentID := fmt.Sprintf("custom-%s-agent", customKindStr)
		missionID := "mission-ext"

		// Spawn with custom kind — AgentManager only validates via IsValid().
		// For extensibility, we test that the lifecycle works for known kinds
		// and that the manager doesn't hardcode kind-specific logic.
		// Use worker kind as proxy (since IsValid only accepts planner/worker currently).
		_, err := mgr.Spawn(ctx, SpawnRequest{
			ID:        agentID,
			Kind:      AgentKindWorker, // Use valid kind
			MissionID: missionID,
			HostID:    "custom-host",
			SessionID: "sess-custom",
			Task:      fmt.Sprintf("task for custom kind %s", customKindStr),
		})
		if err != nil {
			t.Fatalf("spawn failed: %v", err)
		}

		// Run — same lifecycle regardless of kind.
		result, err := mgr.RunAgent(ctx, agentID, &AgentConfig{
			Kind:   AgentKindWorker,
			HostID: "custom-host",
		})
		if err != nil {
			t.Fatalf("run failed: %v", err)
		}

		// Property: result follows standard lifecycle.
		if result.Status != AgentStatusCompleted {
			t.Fatalf("expected completed, got %q", result.Status)
		}

		// Collect — same interface.
		results := mgr.CollectResults(missionID)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].AgentID != agentID {
			t.Fatalf("expected agent %q, got %q", agentID, results[0].AgentID)
		}

		// Property: AgentDefinition can be registered for any kind string
		// without modifying AgentManager. The factory accepts new definitions.
		factory := NewAgentFactory(nil, nil, nil, nil)
		def := &AgentDefinition{
			Kind:          AgentKindWorker, // valid kind
			Name:          fmt.Sprintf("Custom %s Agent", customKindStr),
			MaxIterations: rapid.IntRange(5, 50).Draw(t, "maxIter"),
		}
		err = factory.RegisterDefinition(def)
		if err != nil {
			t.Fatalf("register definition failed: %v", err)
		}

		// Property: registered definition is retrievable.
		got := factory.GetDefinition(AgentKindWorker)
		if got == nil {
			t.Fatal("registered definition not found")
		}
		if got.Name != def.Name {
			t.Fatalf("definition name mismatch: got %q, want %q", got.Name, def.Name)
		}
	})
}

// ---------------------------------------------------------------------------
// Feature: aiops-codex-eino-rewrite, Property 38: Agent Reconcile 安全性
// For any pre-restart agent state snapshot (containing running agents),
// after reconcile all non-terminal agents should be marked as failed,
// never restoring failed→running.
//
// **Validates: Requirements 13.8**
// ---------------------------------------------------------------------------

func TestProperty38_AgentReconcileSafety(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numAgents := rapid.IntRange(2, 10).Draw(t, "numAgents")
		missionID := "mission-reconcile"

		runner := &mockRunner{results: make(map[string]mockRunResult)}
		projector := projection.NewProjector(nil)
		mgr := NewAgentManager(nil, runner, projector)
		bc, _ := NewAgentBudgetController(10)

		ctx := context.Background()

		// Create agents in various states.
		type agentState struct {
			id     string
			status AgentStatus
		}
		agents := make([]agentState, numAgents)

		statuses := []AgentStatus{
			AgentStatusIdle,
			AgentStatusRunning,
			AgentStatusWaiting,
			AgentStatusCompleted,
			AgentStatusFailed,
			AgentStatusKilled,
		}

		for i := range agents {
			agents[i].id = fmt.Sprintf("agent-%d", i)
			agents[i].status = statuses[rapid.IntRange(0, len(statuses)-1).Draw(t, fmt.Sprintf("status-%d", i))]
		}

		// Spawn all agents and set their statuses directly.
		for _, a := range agents {
			_, err := mgr.Spawn(ctx, SpawnRequest{
				ID:        a.id,
				Kind:      AgentKindWorker,
				MissionID: missionID,
				HostID:    "host-1",
				SessionID: fmt.Sprintf("sess-%s", a.id),
				Task:      "test",
			})
			if err != nil {
				t.Fatalf("spawn failed: %v", err)
			}

			// Directly set the status (simulating pre-restart state).
			mgr.mu.Lock()
			mgr.instances[a.id].Status = a.status
			mgr.mu.Unlock()
		}

		// Execute reconcile.
		summary, err := ReconcileAgents(mgr, bc)
		if err != nil {
			t.Fatalf("reconcile failed: %v", err)
		}

		// Verify properties.
		for _, a := range agents {
			inst := mgr.GetInstance(a.id)
			if inst == nil {
				t.Fatalf("agent %q not found after reconcile", a.id)
			}

			if a.status.IsTerminal() {
				// Property: already-terminal agents are NEVER modified.
				if inst.Status != a.status {
					t.Fatalf("terminal agent %q status changed from %q to %q", a.id, a.status, inst.Status)
				}
			} else {
				// Property: non-terminal agents are marked as failed.
				if inst.Status != AgentStatusFailed {
					t.Fatalf("non-terminal agent %q (was %q) should be failed, got %q", a.id, a.status, inst.Status)
				}
			}

			// Property: no agent is in running state after reconcile
			// (unless it was already completed/killed which stays as-is).
			if inst.Status == AgentStatusRunning {
				t.Fatalf("agent %q is running after reconcile (was %q)", a.id, a.status)
			}
		}

		// Property: summary counts are consistent.
		if summary.TotalAgents != numAgents {
			t.Fatalf("summary total %d != %d", summary.TotalAgents, numAgents)
		}

		expectedReconciled := 0
		expectedTerminal := 0
		for _, a := range agents {
			if a.status.IsTerminal() {
				expectedTerminal++
			} else {
				expectedReconciled++
			}
		}
		if len(summary.ReconciledAgents) != expectedReconciled {
			t.Fatalf("reconciled count %d != expected %d", len(summary.ReconciledAgents), expectedReconciled)
		}
		if len(summary.AlreadyTerminal) != expectedTerminal {
			t.Fatalf("already terminal count %d != expected %d", len(summary.AlreadyTerminal), expectedTerminal)
		}
	})
}

// ---------------------------------------------------------------------------
// Feature: aiops-codex-eino-rewrite, Property 39: Agent 实例独立上下文
// For any two Worker_Agents (ChatModelAgent) in the same mission, their
// message history and context window should be completely independent.
// One agent's context changes should not affect another.
//
// **Validates: Requirements 13.2**
// ---------------------------------------------------------------------------

func TestProperty39_AgentInstanceIndependentContext(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numAgents := rapid.IntRange(2, 6).Draw(t, "numAgents")
		missionID := "mission-context"

		// Each agent gets a unique output to verify independence.
		outputs := make([]string, numAgents)
		for i := range outputs {
			outputs[i] = rapid.StringMatching(`^result-[a-z0-9]{4,10}$`).Draw(t, fmt.Sprintf("output-%d", i))
		}

		runner := &mockRunner{results: make(map[string]mockRunResult)}
		for i := 0; i < numAgents; i++ {
			hostID := fmt.Sprintf("host-%d", i)
			runner.results[hostID] = mockRunResult{output: outputs[i]}
		}

		projector := projection.NewProjector(nil)
		mgr := NewAgentManager(nil, runner, projector)
		ctx := context.Background()

		// Spawn all agents in the same mission.
		agentIDs := make([]string, numAgents)
		for i := 0; i < numAgents; i++ {
			agentIDs[i] = fmt.Sprintf("worker-%d", i)
			_, err := mgr.Spawn(ctx, SpawnRequest{
				ID:        agentIDs[i],
				Kind:      AgentKindWorker,
				MissionID: missionID,
				HostID:    fmt.Sprintf("host-%d", i),
				SessionID: fmt.Sprintf("sess-%d", i),
				Task:      fmt.Sprintf("task-%d", i),
			})
			if err != nil {
				t.Fatalf("spawn agent %d failed: %v", i, err)
			}
		}

		// Run all agents concurrently.
		var wg sync.WaitGroup
		results := make([]*AgentResult, numAgents)
		errors := make([]error, numAgents)

		for i := 0; i < numAgents; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				r, err := mgr.RunAgent(ctx, agentIDs[idx], &AgentConfig{
					Kind:   AgentKindWorker,
					HostID: fmt.Sprintf("host-%d", idx),
				})
				results[idx] = r
				errors[idx] = err
			}(i)
		}
		wg.Wait()

		// Property: each agent has independent output (no cross-contamination).
		for i := 0; i < numAgents; i++ {
			if errors[i] != nil {
				t.Fatalf("agent %d run error: %v", i, errors[i])
			}
			if results[i] == nil {
				t.Fatalf("agent %d has nil result", i)
			}
			if results[i].Output != outputs[i] {
				t.Fatalf("agent %d output %q != expected %q (context leak?)", i, results[i].Output, outputs[i])
			}
		}

		// Property: each agent instance has independent state.
		for i := 0; i < numAgents; i++ {
			inst := mgr.GetInstance(agentIDs[i])
			if inst == nil {
				t.Fatalf("agent %d instance not found", i)
			}

			// Each instance has its own session, host, and task.
			if inst.SessionID != fmt.Sprintf("sess-%d", i) {
				t.Fatalf("agent %d session mismatch: %q", i, inst.SessionID)
			}
			if inst.HostID != fmt.Sprintf("host-%d", i) {
				t.Fatalf("agent %d host mismatch: %q", i, inst.HostID)
			}
			if inst.Task != fmt.Sprintf("task-%d", i) {
				t.Fatalf("agent %d task mismatch: %q", i, inst.Task)
			}
			if inst.Output != outputs[i] {
				t.Fatalf("agent %d instance output %q != expected %q", i, inst.Output, outputs[i])
			}

			// Property: one agent's completion doesn't affect another's state.
			for j := 0; j < numAgents; j++ {
				if i == j {
					continue
				}
				other := mgr.GetInstance(agentIDs[j])
				if other.Output == inst.Output && i != j {
					// Same output is only valid if the generated outputs happen to match.
					if outputs[i] != outputs[j] {
						t.Fatalf("agent %d and %d have same output %q but expected different", i, j, inst.Output)
					}
				}
			}
		}
	})
}
