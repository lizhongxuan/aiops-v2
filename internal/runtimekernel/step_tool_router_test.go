package runtimekernel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/tooling"
)

func TestStepToolRouterRejectsInvalidVisibilityAndDispatchSets(t *testing.T) {
	tests := []StepToolRouter{
		{
			RegisteredTools: []string{"registered"}, ModelVisibleTools: []string{"missing"},
			DispatchableTools: []string{"missing"}, Fingerprint: "fp-1",
		},
		{
			RegisteredTools: []string{"visible", "internal"}, ModelVisibleTools: []string{"visible"},
			DispatchableTools: []string{"visible", "internal"}, Fingerprint: "fp-1",
		},
		{
			RegisteredTools: []string{"visible"}, ModelVisibleTools: []string{"visible"},
			DispatchableTools: nil, Fingerprint: "fp-1",
		},
	}
	for index, router := range tests {
		if err := router.Validate(); err == nil {
			t.Fatalf("case %d Validate() error = nil", index)
		}
	}
}

func TestStepToolRouterBuildIsStableAndRejectsProviderSafeCollision(t *testing.T) {
	first, err := BuildStepToolRouter(StepToolRouterInput{
		Registered: []string{"tool_b", "tool_a"}, ModelVisible: []string{"tool_b", "tool_a"},
		Dispatchable: []string{"tool_a", "tool_b"}, HiddenReasons: map[string][]string{"hidden": {"policy_denied"}},
		PolicyHash: "policy-1", SourceFingerprint: "source-1",
	})
	if err != nil {
		t.Fatalf("BuildStepToolRouter(first) error = %v", err)
	}
	second, err := BuildStepToolRouter(StepToolRouterInput{
		Registered: []string{"tool_a", "tool_b"}, ModelVisible: []string{"tool_a", "tool_b"},
		Dispatchable: []string{"tool_b", "tool_a"}, HiddenReasons: map[string][]string{"hidden": {"policy_denied"}},
		PolicyHash: "policy-1", SourceFingerprint: "source-1",
	})
	if err != nil {
		t.Fatalf("BuildStepToolRouter(second) error = %v", err)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("fingerprints differ: %q != %q", first.Fingerprint, second.Fingerprint)
	}
	if _, err := BuildStepToolRouter(StepToolRouterInput{
		Registered: []string{"mcp.foo", "mcp_foo"}, ModelVisible: []string{"mcp.foo", "mcp_foo"},
		Dispatchable: []string{"mcp.foo", "mcp_foo"},
	}); err == nil {
		t.Fatal("BuildStepToolRouter() accepted provider-safe name collision")
	}
}

func TestStepToolRouterRequiresTypedHostInternalReason(t *testing.T) {
	router := StepToolRouter{
		RegisteredTools:   []string{"visible", "host_internal"},
		ModelVisibleTools: []string{"visible"},
		DispatchableTools: []string{"visible", "host_internal"},
		HostInternalDispatchReasons: map[string]HostInternalDispatchReason{
			"host_internal": {Code: HostInternalDispatchReasonRuntimeControl},
		},
		Fingerprint: "fp-internal",
	}
	if err := router.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if router.AllowsModelDispatch("host_internal") {
		t.Fatal("host internal tool became model-dispatchable")
	}
	if !router.AllowsHostInternalDispatch("host_internal") {
		t.Fatal("typed host internal tool is not internally dispatchable")
	}
}

func TestStepToolRouterProviderAndDispatcherShareFingerprint(t *testing.T) {
	router := StepToolRouter{
		RegisteredTools: []string{"host_read"}, ModelVisibleTools: []string{"host_read"},
		DispatchableTools: []string{"host_read"}, PolicyHash: "policy-1", Fingerprint: "router-fingerprint-1",
	}
	if err := router.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	specs := providerToolSpecsFromStepToolRouter(router)
	if len(specs) != 1 || specs[0].Name != "host_read" || specs[0].Hash != router.Fingerprint {
		t.Fatalf("provider specs = %#v", specs)
	}

	executor := &mockToolExecutor{result: "ok"}
	dispatcher := NewToolDispatcher(&mockToolLookup{tools: map[string]mockToolEntry{
		"host_read": {desc: ToolDescriptor{Metadata: tooling.ToolMetadata{Name: "host_read"}}, executor: executor},
	}}, nil, &testMockEventEmitter{}).WithStepToolRouter(router)
	result := dispatcher.Dispatch(context.Background(), "session-1", "turn-1", ToolCall{
		ID: "call-1", Name: "host_read", Arguments: json.RawMessage(`{}`),
	}, SessionTypeHost, ModeInspect)
	if result.DecisionTrace.ToolSurfaceFingerprint != router.Fingerprint || executor.calls != 1 {
		t.Fatalf("dispatch result = %#v calls=%d", result, executor.calls)
	}
}

func TestRunTurnProviderRequestAndDispatcherDecisionShareToolSurfaceFingerprint(t *testing.T) {
	const (
		toolName  = "write_file"
		sessionID = "session-step-tool-router-contract"
		turnID    = "turn-step-tool-router-contract"
	)
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("request controlled write", []schema.ToolCall{{
			ID: "call-contract-write", Type: "function",
			Function: schema.FunctionCall{Name: toolName, Arguments: `{"path":"/tmp/router-contract","content":"synthetic"}`},
		}}),
	}}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name: toolName, Description: "Controlled write for router contract",
			Mutating: true, RequiresApproval: true,
			Discovery: tooling.ToolDiscoveryMetadata{PermissionScope: "argument_scoped"},
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType: "synthetic_file", ResourceID: "/tmp/router-contract", OperationKind: "write",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy: tooling.ToolIdempotencyStrategyArgumentsHash, PostCheckRefs: []string{"verify synthetic"},
			},
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeExecute)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return false },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{
				Action: tooling.PermissionActionNeedApproval, Reason: "contract write requires approval",
				Approval: &tooling.PermissionApprovalPayload{
					Risk: string(tooling.ToolRiskHigh), Source: "step_tool_router_contract_test",
					ExpectedEffect: "write synthetic", Rollback: "remove synthetic", Validation: "verify synthetic",
				},
			}
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "unexpected execution before approval"}, nil
		},
	}

	sink := &recordingReplayArtifactSink{}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, map[policyengine.Mode]policyengine.ModePolicy{})
	kernel.replayArtifactSink = sink
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: sessionID, SessionType: SessionTypeHost, Mode: ModeExecute,
		TurnID: turnID, Input: "perform the controlled write",
		Metadata: map[string]string{
			"aiops.userEvidence.present": "true", "aiops.userEvidence.kinds": "pre_change_snapshot",
			"aiops.userEvidence.signals": "file_absent", "aiops.userEvidence.rawExcerpt": "/tmp/router-contract absent before write",
		},
	})
	if err != nil || result.Status != "blocked" {
		t.Fatalf("RunTurn() = %#v, %v; want approval-blocked turn", result, err)
	}

	providerFingerprint := ""
	for _, artifact := range sink.artifacts {
		if artifact.Kind != ReplayArtifactKindStepContext || artifact.StepContext == nil {
			continue
		}
		for _, spec := range artifact.StepContext.ProviderRequest.Tools {
			if spec.Name == toolName {
				providerFingerprint = spec.Hash
				break
			}
		}
	}
	if providerFingerprint == "" {
		t.Fatalf("production RuntimeStepContext omitted provider fingerprint for %q: %#v", toolName, sink.artifacts)
	}

	session := kernel.sessions.Get(sessionID)
	if session == nil || len(session.PendingApprovals) != 1 {
		t.Fatalf("RunTurn pending approvals = %#v, want one dispatcher decision", session)
	}
	// markTurnBlocked copies this value directly from DispatchResult.DecisionTrace.ToolSurfaceFingerprint.
	dispatcherFingerprint := session.PendingApprovals[0].ToolSurfaceFingerprint
	if err := stepToolRouterFingerprintParityError(providerFingerprint, dispatcherFingerprint); err != nil {
		t.Fatal(err)
	}
	if err := stepToolRouterFingerprintParityError(providerFingerprint, dispatcherFingerprint+"-deliberate-divergence"); err == nil {
		t.Fatal("fingerprint parity assertion accepted a deliberate provider/dispatcher divergence")
	}
}

func stepToolRouterFingerprintParityError(providerFingerprint, dispatcherFingerprint string) error {
	if strings.TrimSpace(providerFingerprint) == "" || strings.TrimSpace(dispatcherFingerprint) == "" {
		return fmt.Errorf("provider/dispatcher tool surface fingerprint is empty: provider=%q dispatcher=%q", providerFingerprint, dispatcherFingerprint)
	}
	if providerFingerprint != dispatcherFingerprint {
		return fmt.Errorf("provider/dispatcher tool surface fingerprint diverged: provider=%q dispatcher=%q", providerFingerprint, dispatcherFingerprint)
	}
	return nil
}

func TestStepToolRouterHiddenModelToolHasStableFailure(t *testing.T) {
	router := StepToolRouter{
		RegisteredTools: []string{"host_read", "exec_command"}, ModelVisibleTools: []string{"host_read"},
		DispatchableTools: []string{"host_read"}, HiddenReasons: map[string][]string{"exec_command": {"profile_denied"}},
		PolicyHash: "policy-1", Fingerprint: "router-fingerprint-1",
	}
	executor := &mockToolExecutor{result: "should-not-run"}
	lookup := &stepToolRouterCountingLookup{tools: map[string]mockToolEntry{
		"exec_command": {desc: ToolDescriptor{Metadata: tooling.ToolMetadata{Name: "exec_command"}}, executor: executor},
	}}
	dispatcher := NewToolDispatcher(lookup, nil, &testMockEventEmitter{}).WithStepToolRouter(router)
	result := dispatcher.Dispatch(context.Background(), "session-1", "turn-1", ToolCall{
		ID: "call-hidden", Name: "exec_command", Arguments: json.RawMessage(`{}`),
	}, SessionTypeHost, ModeExecute)
	if result.Blocked || result.Reason != "tool_not_visible_in_step" || result.Outcome != "tool_unavailable" || !strings.Contains(result.Content, `"errorType":"tool_not_visible_in_step"`) {
		t.Fatalf("result = %#v", result)
	}
	if lookup.calls != 0 || executor.calls != 0 {
		t.Fatalf("lookup calls = %d executor calls = %d, want both 0", lookup.calls, executor.calls)
	}
}

func TestStepToolRouterUnregisteredToolPreservesMissingToolFailure(t *testing.T) {
	router := StepToolRouter{
		RegisteredTools: []string{"host_read"}, ModelVisibleTools: []string{"host_read"},
		DispatchableTools: []string{"host_read"}, Fingerprint: "router-fingerprint-1",
	}
	lookup := &stepToolRouterCountingLookup{}
	result := NewToolDispatcher(lookup, nil, &testMockEventEmitter{}).
		WithStepToolRouter(router).
		Dispatch(context.Background(), "session-1", "turn-1", ToolCall{
			ID: "call-missing", Name: "missing_tool", Arguments: json.RawMessage(`{}`),
		}, SessionTypeHost, ModeInspect)
	if !strings.Contains(result.Error, `"errorType":"tool_not_found"`) || result.Blocked {
		t.Fatalf("result = %#v", result)
	}
	if lookup.calls != 0 {
		t.Fatalf("lookup calls = %d, want router rejection before lookup", lookup.calls)
	}
}

type stepToolRouterCountingLookup struct {
	tools map[string]mockToolEntry
	calls int
}

func (lookup *stepToolRouterCountingLookup) LookupTool(name string) (ToolDescriptor, ToolExecutor, bool) {
	lookup.calls++
	entry, ok := lookup.tools[name]
	return entry.desc, entry.executor, ok
}
