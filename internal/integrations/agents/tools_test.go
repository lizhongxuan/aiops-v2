package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/agentmgr"
)

type fakeAgentManager struct {
	spawned SpawnRequest
	result  agentmgr.EvidenceReport
}

func (m *fakeAgentManager) SpawnInvestigationAgent(ctx context.Context, req SpawnRequest) (SpawnResult, error) {
	m.spawned = req
	return SpawnResult{AgentID: "agent-1", AgentType: req.AgentType, Status: "running"}, nil
}

func (m *fakeAgentManager) WaitEvidenceReports(ctx context.Context, agentIDs []string) ([]agentmgr.EvidenceReport, error) {
	return []agentmgr.EvidenceReport{m.result}, nil
}

func TestSpawnAgentRejectsCodingAgentTypes(t *testing.T) {
	tool := NewSpawnAgentTool(&fakeAgentManager{})
	input := json.RawMessage(`{"agentType":"coder","task":"edit code"}`)
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("coding agent type should be rejected")
	}
}

func TestSpawnAgentAllowsOpsInvestigationTypes(t *testing.T) {
	tool := NewSpawnAgentTool(&fakeAgentManager{})
	input := json.RawMessage(`{"agentType":"logs_investigator","task":"collect redis error logs","evidenceGoal":"find evidence refs for timeout spike"}`)
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Content, `"agentId":"agent-1"`) {
		t.Fatalf("result = %s", result.Content)
	}
}

func TestWaitAgentReturnsEvidenceReport(t *testing.T) {
	mgr := &fakeAgentManager{result: agentmgr.EvidenceReport{
		AgentID:      "agent-1",
		Summary:      "RSS grew without used_memory growth",
		EvidenceRefs: []string{"ev-1"},
		Confidence:   "medium",
	}}
	tool := NewWaitAgentTool(mgr)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"agentIds":["agent-1"]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"agentId":"agent-1"`, `"evidenceRefs":["ev-1"]`, `"confidence":"medium"`, `"nextQuestions"`, `"errors"`} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("result missing %s: %s", want, result.Content)
		}
	}
}
