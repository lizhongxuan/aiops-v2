package runtimekernel

import (
	"strings"
	"testing"

	"aiops-v2/internal/promptinput"
)

func TestAgentFinalGateBlocksPendingWorkerClaim(t *testing.T) {
	decision := EvaluateRuntimeAgentFinalGate(
		"synthetic-worker-1 confirmed bounded summary",
		[]promptinput.AgentNotificationTrace{{
			AgentID: "synthetic-worker-1",
			Status:  "running",
			Summary: "bounded summary",
		}},
	)
	if decision.Action != "require_wait" {
		t.Fatalf("action = %q, want require_wait: %#v", decision.Action, decision)
	}
}

func TestBuildDeterministicIncompleteFinal(t *testing.T) {
	got := buildDeterministicIncompleteFinal(incompleteFinalInput{
		ConfirmedFacts: []string{
			"用户描述主机A由 pgBackRest 恢复",
			"用户描述主机B作为从节点加入失败",
		},
		MissingEvidence: []string{
			"主机B完整错误输出",
			"主机A和主机B的 pg_control timeline",
		},
		LikelyDirection: "timeline 或恢复残留配置不一致",
		Confidence:      "low",
	})
	for _, want := range []string{"还不能给最终结论", "已确认", "仍缺少", "下一步只读检查"} {
		if !strings.Contains(got, want) {
			t.Fatalf("incomplete final missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "置信度") || strings.Contains(got, "confidence") {
		t.Fatalf("incomplete final must not expose confidence labels: %s", got)
	}
}

func TestEvaluateFinalMessageBoundaryConstrainDoesNotRequestRewriteForConfidenceOnly(t *testing.T) {
	decision := evaluateFinalMessageBoundary(finalMessageBoundaryInput{
		Text:                   "基于当前证据，更可能是 timeline 分叉。",
		FinalEvidenceAction:    string(FinalEvidenceActionDowngrade),
		EvidenceCoverageAction: "continue_gathering",
		RequiresEvidence:       true,
	})
	if decision.Action != FinalMessageBoundaryConstrain {
		t.Fatalf("action=%q, want constrain", decision.Action)
	}
	if decision.EvidenceBoundary != "limited" {
		t.Fatalf("boundary=%q, want limited", decision.EvidenceBoundary)
	}
	if decision.Retry {
		t.Fatal("confidence/evidence boundary downgrade must not force model rewrite")
	}
}

func TestAgentFinalGateAllowsPendingStatusDisclosure(t *testing.T) {
	decision := EvaluateRuntimeAgentFinalGate(
		"synthetic-worker-1 is still running and not confirmed",
		[]promptinput.AgentNotificationTrace{{
			AgentID: "synthetic-worker-1",
			Status:  "running",
		}},
	)
	if decision.Action != "allow" {
		t.Fatalf("action = %q, want allow: %#v", decision.Action, decision)
	}
}
