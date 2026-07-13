package runtimekernel

import (
	"strings"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestRuntimeToolRouterSnapshotFromPolicyBuildsCanonicalValidProjection(t *testing.T) {
	snapshot := RuntimeToolRouterSnapshotFromPolicy(
		[]string{"synthetic.read", "synthetic.write"},
		tooling.ToolSurfacePolicySnapshot{
			Hash: "policy-1",
			SurfaceDecisions: []tooling.SurfaceDecision{
				{Name: "synthetic.read", Visible: true, DispatchAction: tooling.SurfaceDispatchAllow, Reason: "policy_allowed"},
				{Name: "synthetic.write", Visible: true, SummaryOnly: true, DispatchAction: tooling.SurfaceDispatchNeedApproval, Reason: "approval_required_schema_hidden"},
			},
		},
		[]string{"synthetic.read", "synthetic.write"},
		nil,
		"surface-1",
	)

	if got := strings.Join(snapshot.ModelVisibleTools, ","); got != "synthetic.read,synthetic.write" {
		t.Fatalf("model visible tools = %q, want both tools visible", got)
	}
	if got := strings.Join(snapshot.DispatchableTools, ","); got != "synthetic.read,synthetic.write" {
		t.Fatalf("dispatchable tools = %q, want the model-visible callable set", got)
	}
	if snapshot.PolicyHash != "policy-1" || snapshot.Fingerprint == "" || snapshot.Fingerprint == "surface-1" {
		t.Fatalf("snapshot identity = %#v", snapshot)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRuntimeToolRouterSnapshotFromTurnSnapshotRestoresFrozenRouter(t *testing.T) {
	router, err := BuildStepToolRouter(StepToolRouterInput{
		Registered: []string{"host_read", "hidden_exec"}, ModelVisible: []string{"host_read"},
		Dispatchable: []string{"host_read"}, HiddenReasons: map[string][]string{"hidden_exec": {"profile_denied"}},
		PolicyHash: "policy-1", SourceFingerprint: "source-1",
	})
	if err != nil {
		t.Fatalf("BuildStepToolRouter() error = %v", err)
	}
	snapshot := &TurnSnapshot{ToolSurfaceSnapshot: &ToolSurfaceSnapshotRef{
		Fingerprint: router.Fingerprint,
		ToolNames:   []string{"host_read"},
		StepRouter:  &router,
	}}
	restored := runtimeToolRouterSnapshotFromTurnSnapshot(snapshot)
	if restored.Fingerprint != router.Fingerprint || !stepToolListContains(restored.RegisteredTools, "hidden_exec") {
		t.Fatalf("restored router = %#v, want frozen router %#v", restored, router)
	}
	result := NewToolDispatcher(&stepToolRouterCountingLookup{}, nil, &testMockEventEmitter{}).
		WithStepToolRouter(restored).
		Dispatch(t.Context(), "session-1", "turn-1", ToolCall{ID: "call-hidden", Name: "hidden_exec"}, SessionTypeHost, ModeInspect)
	if result.Reason != "tool_not_visible_in_step" || strings.Contains(result.Error, "tool_not_found") {
		t.Fatalf("hidden resume result = %#v", result)
	}
}
