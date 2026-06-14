package hostops

import (
	"testing"

	"aiops-v2/internal/opssemantic"
)

func TestCommandPolicyAllowsGlobalWhitelist(t *testing.T) {
	policy := NewCommandPolicy(CommandPolicyConfig{
		GlobalWhitelist: []CommandPolicyRule{{ID: "read-uptime", Pattern: "uptime", MaxRisk: opssemantic.RiskReadOnly}},
	})

	decision := policy.Evaluate(CommandPolicyContext{
		MissionID:    "mission-1",
		ChildAgentID: "child-1",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "uptime",
		RiskLevel:    opssemantic.RiskReadOnly,
	})

	if !decision.Allowed || decision.RequiresApproval {
		t.Fatalf("decision = %#v, want allowed without approval", decision)
	}
	if decision.MatchedRuleID != "read-uptime" {
		t.Fatalf("MatchedRuleID = %q, want read-uptime", decision.MatchedRuleID)
	}
}

func TestCommandPolicyAppliesHostAndEnvironmentOverride(t *testing.T) {
	policy := NewCommandPolicy(CommandPolicyConfig{
		Overrides: []CommandPolicyOverride{{
			HostID:      "host-a",
			Environment: "lab",
			Rules:       []CommandPolicyRule{{ID: "status-read", Pattern: "systemctl status *", MaxRisk: opssemantic.RiskReadOnly}},
		}},
	})

	allowed := policy.Evaluate(CommandPolicyContext{
		HostID: "host-a", Environment: "lab", Command: "systemctl status unit-name", RiskLevel: opssemantic.RiskReadOnly,
	})
	deniedWrongHost := policy.Evaluate(CommandPolicyContext{
		HostID: "host-b", Environment: "lab", Command: "systemctl status unit-name", RiskLevel: opssemantic.RiskReadOnly,
	})
	deniedWrongEnv := policy.Evaluate(CommandPolicyContext{
		HostID: "host-a", Environment: "prod", Command: "systemctl status unit-name", RiskLevel: opssemantic.RiskReadOnly,
	})

	if !allowed.Allowed {
		t.Fatalf("allowed decision = %#v, want allowed", allowed)
	}
	if deniedWrongHost.Allowed {
		t.Fatalf("wrong host decision = %#v, want denied", deniedWrongHost)
	}
	if deniedWrongEnv.Allowed {
		t.Fatalf("wrong env decision = %#v, want denied", deniedWrongEnv)
	}
}

func TestCommandPolicyTaskGrantCannotCrossTaskHostStepOrRisk(t *testing.T) {
	policy := NewCommandPolicy(CommandPolicyConfig{
		TaskGrants: []CommandPolicyGrant{{
			MissionID:    "mission-1",
			ChildAgentID: "child-1",
			PlanStepID:   "step-1",
			HostID:       "host-a",
			Command:      "touch /tmp/aiops-check",
			RiskLevel:    opssemantic.RiskLowWrite,
		}},
	})
	base := CommandPolicyContext{
		MissionID:    "mission-1",
		ChildAgentID: "child-1",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "touch /tmp/aiops-check",
		RiskLevel:    opssemantic.RiskLowWrite,
	}
	if got := policy.Evaluate(base); !got.Allowed || got.RequiresApproval {
		t.Fatalf("base grant decision = %#v, want allowed", got)
	}
	base.PlanStepID = "step-2"
	if got := policy.Evaluate(base); got.Allowed {
		t.Fatalf("cross-step decision = %#v, want denied", got)
	}
	base.PlanStepID = "step-1"
	base.HostID = "host-b"
	if got := policy.Evaluate(base); got.Allowed {
		t.Fatalf("cross-host decision = %#v, want denied", got)
	}
	base.HostID = "host-a"
	base.RiskLevel = opssemantic.RiskMediumWrite
	if got := policy.Evaluate(base); got.Allowed {
		t.Fatalf("cross-risk decision = %#v, want denied", got)
	}
}
