package runtimekernel

import (
	"strings"
	"testing"
	"time"
)

func TestPlanApprovalScopeAllowsOnlyApprovedActionsAndResources(t *testing.T) {
	now := time.Date(2026, 6, 7, 13, 0, 0, 0, time.UTC)
	expires := now.Add(time.Hour)
	scope := PlanApprovalScope{
		PlanID:         "plan-1",
		AllowedActions: []string{"restart_service"},
		ResourceScopes: []PlanApprovalResourceScope{{Type: "service", ID: "svc-a"}},
		RiskCeiling:    "high",
		ExpiresAt:      &expires,
		InputHash:      "hash-1",
	}
	allowed := scope.Match(PlanScopedToolCall{PlanID: "plan-1", Action: "restart_service", ResourceType: "service", ResourceID: "svc-a", Risk: "medium", InputHash: "hash-1"}, now)
	if !allowed.Allowed || allowed.NeedsApproval {
		t.Fatalf("allowed match = %#v, want allowed", allowed)
	}

	for _, tc := range []struct {
		name string
		call PlanScopedToolCall
		want string
		at   time.Time
	}{
		{name: "action", call: PlanScopedToolCall{PlanID: "plan-1", Action: "delete_file", ResourceType: "service", ResourceID: "svc-a", Risk: "medium", InputHash: "hash-1"}, want: "action", at: now},
		{name: "resource", call: PlanScopedToolCall{PlanID: "plan-1", Action: "restart_service", ResourceType: "service", ResourceID: "svc-b", Risk: "medium", InputHash: "hash-1"}, want: "resource", at: now},
		{name: "expired", call: PlanScopedToolCall{PlanID: "plan-1", Action: "restart_service", ResourceType: "service", ResourceID: "svc-a", Risk: "medium", InputHash: "hash-1"}, want: "expired", at: expires},
		{name: "plan", call: PlanScopedToolCall{PlanID: "plan-2", Action: "restart_service", ResourceType: "service", ResourceID: "svc-a", Risk: "medium", InputHash: "hash-1"}, want: "plan id", at: now},
		{name: "risk", call: PlanScopedToolCall{PlanID: "plan-1", Action: "restart_service", ResourceType: "service", ResourceID: "svc-a", Risk: "destructive", InputHash: "hash-1"}, want: "risk", at: now},
		{name: "input_hash", call: PlanScopedToolCall{PlanID: "plan-1", Action: "restart_service", ResourceType: "service", ResourceID: "svc-a", Risk: "medium", InputHash: "hash-2"}, want: "input hash", at: now},
	} {
		t.Run(tc.name, func(t *testing.T) {
			match := scope.Match(tc.call, tc.at)
			if match.Allowed || !match.NeedsApproval || !strings.Contains(match.Reason, tc.want) {
				t.Fatalf("match = %#v, want needs approval reason containing %q", match, tc.want)
			}
		})
	}
}
