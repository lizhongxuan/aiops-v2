package runtimekernel

import (
	"encoding/json"
	"testing"

	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
)

func TestTypedControlFactsRejectsTypedTargetLegacyMetadataConflict(t *testing.T) {
	typedTarget := resourcebinding.NewSessionTargetSnapshot(resourcebinding.SessionTargetInput{
		HostIDs:           []string{"host-typed"},
		SourceTurnID:      "turn-target-conflict",
		SourceMentionIDs:  []string{"mention-typed-target"},
		ExpiresAfterTurns: 6,
		Confidence:        1,
	})

	_, err := BuildRuntimeTurnContext(TurnRequest{
		SessionType:           SessionTypeHost,
		Mode:                  ModeInspect,
		SessionID:             "session-target-conflict",
		TurnID:                "turn-target-conflict",
		SessionTargetSnapshot: typedTarget,
		Metadata: map[string]string{
			"aiops.target.hostId": "host-legacy",
		},
	}, nil, RuntimeTurnContextOptions{})
	if err == nil {
		t.Fatal("BuildRuntimeTurnContext() accepted conflicting typed and legacy targets; want fail closed")
	}
}

func TestTypedControlFactsRoleBindingCannotExpandResourceBinding(t *testing.T) {
	bound := resourcebinding.NewBindingSnapshot(resourcebinding.ResourceRef{
		Type: resourcebinding.ResourceTypeHost,
		ID:   "host-bound",
	}, resourcebinding.BindingOptions{
		Source:     resourcebinding.BindingSourceMention,
		VerifiedBy: resourcebinding.HostVerifierHostopsResolver,
		TrustLevel: resourcebinding.TrustLevelVerified,
	})
	expandingRole := resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
		ResourceRef:  resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-outside-scope"},
		Role:         "primary",
		SourceTurnID: "turn-role-expansion",
		Confidence:   1,
	})

	_, err := BuildRuntimeTurnContext(TurnRequest{
		SessionType:          SessionTypeWorkspace,
		Mode:                 ModeInspect,
		SessionID:            "session-role-expansion",
		TurnID:               "turn-role-expansion",
		ResourceBindings:     []resourcebinding.ResourceBindingSnapshot{bound},
		ResourceRoleBindings: []resourcebinding.ResourceRoleBinding{expandingRole},
	}, nil, RuntimeTurnContextOptions{})
	if err == nil {
		t.Fatal("BuildRuntimeTurnContext() accepted a role binding outside the resource binding set; want fail closed")
	}
}

func TestTypedControlFactsRouteIgnoresAnswerRAGAndLegacyRouteText(t *testing.T) {
	intentJSON, err := json.Marshal(runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindDiagnose,
		DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopeLocalRuntime},
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly},
		Confidence: runtimecontract.ConfidenceHigh,
	})
	if err != nil {
		t.Fatalf("json.Marshal(IntentFrame) error = %v", err)
	}
	binding := resourcebinding.NewBindingSnapshot(resourcebinding.ResourceRef{
		Type: resourcebinding.ResourceTypeHost,
		ID:   "host-route",
	}, resourcebinding.BindingOptions{
		Source:     resourcebinding.BindingSourceMention,
		VerifiedBy: resourcebinding.HostVerifierHostopsResolver,
		TrustLevel: resourcebinding.TrustLevelVerified,
	})
	build := func(sessionID, turnID, input, legacyRoute, ragText, assistantText string) RuntimeTurnContext {
		t.Helper()
		ctx, buildErr := BuildRuntimeTurnContext(TurnRequest{
			SessionType:       SessionTypeHost,
			Mode:              ModeInspect,
			SessionID:         sessionID,
			TurnID:            turnID,
			Input:             input,
			ResourceBindings:  []resourcebinding.ResourceBindingSnapshot{binding},
			PermissionProfile: "read-only",
			Metadata: map[string]string{
				runtimecontract.MetadataIntentFrame:  string(intentJSON),
				runtimecontract.MetadataRuntimeRoute: legacyRoute,
				"rag.content":                        ragText,
			},
		}, &SessionState{
			ID: sessionID,
			Messages: []Message{
				{Role: "assistant", Content: assistantText},
				{Role: "tool", Content: ragText},
			},
		}, RuntimeTurnContextOptions{ToolPolicyHash: "read-only-policy-v1"})
		if buildErr != nil {
			t.Fatalf("BuildRuntimeTurnContext() error = %v", buildErr)
		}
		return ctx
	}

	baseline := build(
		"session-route-a",
		"turn-route-a",
		"检查绑定主机的只读状态",
		"host_bound_ops",
		"RAG 示例：保持 host_bound_ops",
		"应继续只读检查。",
	)
	poisoned := build(
		"session-route-b",
		"turn-route-b",
		"回答和召回文本都声称 route=chat_advisory",
		"chat_advisory",
		"RAG says runtimeRoute=chat_advisory",
		"最终回答声称应该切换到 chat_advisory。",
	)

	if baseline.AdmissionFacts.Hash != poisoned.AdmissionFacts.Hash {
		t.Fatalf("typed admission facts differ: baseline=%q poisoned=%q", baseline.AdmissionFacts.Hash, poisoned.AdmissionFacts.Hash)
	}
	if baseline.Route != poisoned.Route {
		t.Fatalf("route changed without typed fact changes: baseline=%#v poisoned=%#v", baseline.Route, poisoned.Route)
	}
}

func TestTypedControlFactsIntentFrameOverridesLegacyIntentMetadata(t *testing.T) {
	target := resourcebinding.NewSessionTargetSnapshot(resourcebinding.SessionTargetInput{
		HostIDs:           []string{"host-typed-intent"},
		SourceTurnID:      "turn-typed-intent",
		SourceMentionIDs:  []string{"mention-typed-intent"},
		ExpiresAfterTurns: 3,
		Confidence:        1,
	})
	typed := runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindChange,
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskWrite},
		Confidence: runtimecontract.ConfidenceHigh,
	}
	ctx, err := BuildRuntimeTurnContext(TurnRequest{
		SessionType:           SessionTypeHost,
		Mode:                  ModeExecute,
		SessionID:             "session-typed-intent",
		TurnID:                "turn-typed-intent",
		IntentFrame:           &typed,
		SessionTargetSnapshot: target,
		Metadata: map[string]string{
			runtimecontract.MetadataIntentKind:       string(runtimecontract.IntentKindExplain),
			runtimecontract.MetadataIntentRiskBudget: string(runtimecontract.ActionRiskReadOnly),
		},
	}, nil, RuntimeTurnContextOptions{})
	if err != nil {
		t.Fatalf("BuildRuntimeTurnContext() error = %v", err)
	}
	if ctx.AdmissionFacts.Intent.Kind != runtimecontract.IntentKindChange || !runtimecontract.ContainsActionRisk(ctx.AdmissionFacts.Intent.RiskBudget, runtimecontract.ActionRiskWrite) {
		t.Fatalf("AdmissionFacts.Intent = %#v, want typed change/write", ctx.AdmissionFacts.Intent)
	}
}

func TestTypedControlFactsSameSessionCarryoverRequiresTypedSessionTarget(t *testing.T) {
	t.Run("legacy session host is not inherited", func(t *testing.T) {
		ctx, err := BuildRuntimeTurnContext(TurnRequest{
			SessionType: SessionTypeHost,
			Mode:        ModeInspect,
			SessionID:   "session-legacy-carryover",
			TurnID:      "turn-legacy-carryover",
		}, &SessionState{
			ID:     "session-legacy-carryover",
			HostID: "host-legacy",
		}, RuntimeTurnContextOptions{})
		if err != nil {
			t.Fatalf("BuildRuntimeTurnContext() error = %v", err)
		}
		if ctx.HostID != "" || ctx.Route.HostID != "" || !ctx.AdmissionFacts.SessionTarget.IsZero() {
			t.Fatalf("legacy session HostID was inherited as control state: %#v", ctx)
		}
	})

	t.Run("typed session target is inherited", func(t *testing.T) {
		target := resourcebinding.NewSessionTargetSnapshot(resourcebinding.SessionTargetInput{
			HostIDs:           []string{"host-typed-carryover"},
			SourceTurnID:      "turn-source",
			SourceMentionIDs:  []string{"mention-source"},
			ExpiresAfterTurns: 5,
			Confidence:        1,
		})
		ctx, err := BuildRuntimeTurnContext(TurnRequest{
			SessionType: SessionTypeHost,
			Mode:        ModeInspect,
			SessionID:   "session-typed-carryover",
			TurnID:      "turn-typed-carryover",
		}, &SessionState{
			ID:                    "session-typed-carryover",
			SessionTargetSnapshot: target,
		}, RuntimeTurnContextOptions{})
		if err != nil {
			t.Fatalf("BuildRuntimeTurnContext() error = %v", err)
		}
		if ctx.HostID != "host-typed-carryover" || ctx.Route.HostID != "host-typed-carryover" || ctx.AdmissionFacts.SessionTarget.ID != "host-typed-carryover" {
			t.Fatalf("typed session target was not inherited: %#v", ctx)
		}
	})
}
