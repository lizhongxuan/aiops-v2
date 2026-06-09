package agentmgr

import (
	"testing"

	"aiops-v2/internal/tooling"
)

func TestAgentRolePolicyBlocksMutatingTools(t *testing.T) {
	for _, role := range []AgentRole{AgentRoleExplore, AgentRolePlan, AgentRoleVerify} {
		policy := AgentRolePolicyFor(role)
		if policy.AllowMutatingTools {
			t.Fatalf("%s unexpectedly allows mutating tools", role)
		}
		allowed := policy.AllowsTool(tooling.ToolMetadata{
			Name:     "synthetic.write",
			Layer:    tooling.ToolLayerMutation,
			Mutating: true,
		})
		if allowed {
			t.Fatalf("%s allowed mutating tool", role)
		}
	}
}

func TestExecutorRoleRequiresApprovalAndResourceLockForMutation(t *testing.T) {
	policy := AgentRolePolicyFor(AgentRoleExecute)
	if !policy.AllowMutatingTools || !policy.RequiresPlanApproval || !policy.RequiresResourceLock {
		t.Fatalf("executor policy does not expose required mutation gates: %#v", policy)
	}
	if policy.AllowsTool(tooling.ToolMetadata{Mutating: true, RequiresApproval: false}) {
		t.Fatal("executor allowed mutating tool without approval")
	}
	if !policy.AllowsTool(tooling.ToolMetadata{Mutating: true, RequiresApproval: true, RiskLevel: tooling.ToolRiskHigh}) {
		t.Fatal("executor rejected approved mutating tool")
	}
}
