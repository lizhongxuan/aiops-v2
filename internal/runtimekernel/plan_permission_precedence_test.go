package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/tooling"
)

func TestToolDispatcherPlanActiveBlocksApprovedMutation(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "mutated"}
	input := json.RawMessage(`{"resourceId":"synthetic-resource","operation":"write"}`)
	hash, err := actionproposal.NormalizedInputHash(input)
	if err != nil {
		t.Fatalf("hash input: %v", err)
	}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"synthetic.write": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name:      "synthetic.write",
				Mutating:  true,
				RiskLevel: tooling.ToolRiskHigh,
				Discovery: tooling.ToolDiscoveryMetadata{
					ResourceTypes: []string{"synthetic_resource"},
				},
				ResourceLocks: []tooling.ToolResourceLockKey{{
					ResourceType:  "synthetic_resource",
					ResourceID:    "synthetic-resource",
					OperationKind: "write",
				}},
				Idempotency: tooling.ToolIdempotencyMetadata{
					Strategy: tooling.ToolIdempotencyStrategyArgumentsHash,
				},
			}},
			executor: executor,
		},
	}}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).
		WithSessionApprovalGrants([]SessionApprovalGrant{{ToolName: "synthetic.write", InputHash: hash}}).
		WithPlanApprovalContext(PlanModeState{State: PlanModeStateActive, PlanID: "plan-synthetic"}, nil)

	result := dispatcher.Dispatch(context.Background(), "sess-plan-active", "turn-plan-active", ToolCall{
		ID:        "call-write",
		Name:      "synthetic.write",
		Arguments: input,
	}, SessionTypeWorkspace, ModePlan)

	if !result.Blocked && !strings.Contains(result.Error, "plan_active") {
		t.Fatalf("result = %#v, want plan_active denial", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want blocked before execution", executor.calls)
	}
}

func TestToolDispatcherApprovedCallMustMatchPlanScope(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "mutated"}
	input := json.RawMessage(`{"resourceId":"synthetic-resource","operation":"write"}`)
	hash, err := actionproposal.NormalizedInputHash(input)
	if err != nil {
		t.Fatalf("hash input: %v", err)
	}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"synthetic.write": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name:      "synthetic.write",
				Mutating:  true,
				RiskLevel: tooling.ToolRiskMedium,
				Discovery: tooling.ToolDiscoveryMetadata{
					ResourceTypes: []string{"synthetic_resource"},
				},
				ResourceLocks: []tooling.ToolResourceLockKey{{
					ResourceType:  "synthetic_resource",
					ResourceID:    "synthetic-resource",
					OperationKind: "write",
				}},
				Idempotency: tooling.ToolIdempotencyMetadata{
					Strategy: tooling.ToolIdempotencyStrategyArgumentsHash,
				},
			}},
			executor: executor,
		},
	}}
	planMode := PlanModeState{State: PlanModeStateApproved, PlanID: "plan-synthetic", ApprovedPlanID: "plan-synthetic"}
	scope := PlanApprovalScope{
		PlanID:         "plan-synthetic",
		AllowedActions: []string{"synthetic.write"},
		ResourceScopes: []PlanApprovalResourceScope{{Type: "synthetic_resource", ID: "synthetic-resource"}},
		RiskCeiling:    "medium",
		InputHash:      "different-hash",
	}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).
		WithSessionApprovalGrants([]SessionApprovalGrant{{ToolName: "synthetic.write", InputHash: hash}}).
		WithPlanApprovalContext(planMode, []PlanApprovalScope{scope})

	result := dispatcher.Dispatch(context.Background(), "sess-plan-scope", "turn-plan-scope", ToolCall{
		ID:        "call-write",
		Name:      "synthetic.write",
		Arguments: input,
	}, SessionTypeWorkspace, ModeExecute)

	if !result.Blocked && !strings.Contains(result.Error, "approved call is outside") {
		t.Fatalf("result = %#v, want plan approval scope denial", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want blocked before execution", executor.calls)
	}
}

func TestToolDispatcherApprovedPlanScopeAllowsMatchingMutation(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "mutated"}
	input := json.RawMessage(`{"resourceId":"synthetic-resource","operation":"write"}`)
	hash, err := actionproposal.NormalizedInputHash(input)
	if err != nil {
		t.Fatalf("hash input: %v", err)
	}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"synthetic.write": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name:      "synthetic.write",
				Mutating:  true,
				RiskLevel: tooling.ToolRiskMedium,
				Discovery: tooling.ToolDiscoveryMetadata{
					ResourceTypes: []string{"synthetic_resource"},
				},
				ResourceLocks: []tooling.ToolResourceLockKey{{
					ResourceType:  "synthetic_resource",
					ResourceID:    "synthetic-resource",
					OperationKind: "write",
				}},
				Idempotency: tooling.ToolIdempotencyMetadata{
					Strategy: tooling.ToolIdempotencyStrategyArgumentsHash,
				},
			}},
			executor: executor,
		},
	}}
	planMode := PlanModeState{State: PlanModeStateApproved, PlanID: "plan-synthetic", ApprovedPlanID: "plan-synthetic"}
	scope := PlanApprovalScope{
		PlanID:         "plan-synthetic",
		AllowedActions: []string{"synthetic.write"},
		ResourceScopes: []PlanApprovalResourceScope{{Type: "synthetic_resource", ID: "synthetic-resource"}},
		RiskCeiling:    "medium",
		InputHash:      hash,
	}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).
		WithSessionApprovalGrants([]SessionApprovalGrant{{ToolName: "synthetic.write", InputHash: hash}}).
		WithPlanApprovalContext(planMode, []PlanApprovalScope{scope})

	result := dispatcher.Dispatch(context.Background(), "sess-plan-scope", "turn-plan-scope", ToolCall{
		ID:        "call-write",
		Name:      "synthetic.write",
		Arguments: input,
	}, SessionTypeWorkspace, ModeExecute)

	if result.Blocked || result.Error != "" || result.Content != "mutated" {
		t.Fatalf("result = %#v, want matching plan scope to execute", result)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want execution", executor.calls)
	}
}
