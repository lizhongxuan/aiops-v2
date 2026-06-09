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

func TestSpawnAgentRejectsLazyDelegation(t *testing.T) {
	tool := NewSpawnAgentTool(&fakeAgentManager{})
	input := json.RawMessage(`{
		"agentType":"logs_investigator",
		"assignment":{"objective":"collect evidence"}
	}`)
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected lazy assignment to be rejected")
	}
	for _, want := range []string{"agent_assignment_lint_failed", "background", "scope", "expectedOutput", "stopCondition"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

func TestSpawnAgentAcceptsSelfContainedAssignment(t *testing.T) {
	mgr := &fakeAgentManager{}
	tool := NewSpawnAgentTool(mgr)
	input := json.RawMessage(`{
		"agentType":"logs_investigator",
		"assignment":{
			"objective":"collect independent log evidence",
			"background":"manager observed a synthetic symptom",
			"knownFacts":["synthetic symptom is bounded to requested window"],
			"scope":{"resourceRefs":["synthetic.resource/service-a"],"timeRange":"last_30m"},
			"expectedOutput":"bounded summary with evidence refs",
			"evidenceRequirement":{"minEvidenceRefs":1,"requiredKinds":["log"]},
			"stopCondition":"stop after collecting one evidence ref or finding a blocker"
		}
	}`)
	if _, err := tool.Execute(context.Background(), input); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if mgr.spawned.Assignment.Objective != "collect independent log evidence" {
		t.Fatalf("assignment = %#v", mgr.spawned.Assignment)
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
	for _, want := range []string{`"agentId":"agent-1"`, `"evidenceRefs":["ev-1"]`, `"confidence":"medium"`, `"nextQuestions"`, `"errors"`, `"notifications"`, `"status":"completed"`} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("result missing %s: %s", want, result.Content)
		}
	}
}
