package runtimekernel

import (
	"strings"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestRuntimeToolRouterSnapshotFromPolicySeparatesVisibleAndDispatchableTools(t *testing.T) {
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
	if got := strings.Join(snapshot.DispatchableTools, ","); got != "synthetic.read" {
		t.Fatalf("dispatchable tools = %q, want only allow-dispatch tool", got)
	}
	if snapshot.PolicyHash != "policy-1" || snapshot.Fingerprint != "surface-1" {
		t.Fatalf("snapshot identity = %#v", snapshot)
	}
}
