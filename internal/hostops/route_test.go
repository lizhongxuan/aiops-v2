package hostops

import "testing"

func TestDetectRouteForMultiHostForcesPlan(t *testing.T) {
	mentions := []HostMention{
		{Raw: "@1.1.1.1", HostID: "host-a", Resolved: true},
		{Raw: "@1.1.1.2", HostID: "host-b", Resolved: true},
	}
	decision := DetectRoute("在多台主机上完成通用运维变更", mentions)
	if decision.Kind != RouteKindHostOps {
		t.Fatalf("Kind = %q, want host_ops", decision.Kind)
	}
	if !decision.PlanRequired {
		t.Fatalf("PlanRequired = false, want true")
	}
}

func TestDetectRouteForSingleHostDoesNotForcePlan(t *testing.T) {
	mentions := []HostMention{{Raw: "@1.1.1.1", HostID: "host-a", Resolved: true}}
	decision := DetectRoute("检查主机状态", mentions)
	if decision.Kind != RouteKindHostOps {
		t.Fatalf("Kind = %q, want host_ops", decision.Kind)
	}
	if decision.PlanRequired {
		t.Fatalf("PlanRequired = true, want false for single host read operation")
	}
}

func TestDetectRouteWithoutMentionsUsesNormalChat(t *testing.T) {
	decision := DetectRoute("解释一下多主机运维计划的原则", nil)
	if decision.Kind != RouteKindNormalChat {
		t.Fatalf("Kind = %q, want normal_chat", decision.Kind)
	}
}
