package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/permissions"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/tooling"
)

func TestToolDispatcher_DoesNotEmitStartedBeforeApproval(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "should-not-run"}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"restart_service": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name:   "restart_service",
						Origin: tooling.ToolOriginBuiltin,
					},
				},
				executor: executor,
			},
		},
	}
	dispatcher := NewToolDispatcher(lookup, &policyengine.Engine{
		ModePolicy: map[string]policyengine.ModePolicy{},
	}, emitter).WithPermissions(permissions.NewEngine([]permissions.Rule{
		{
			Name:   "ask-before-restart",
			Action: permissions.ActionAsk,
			Reason: "service restart needs explicit approval",
			Matcher: permissions.Matcher{
				ToolNames: []string{"restart_service"},
			},
		},
	}))

	result := dispatcher.Dispatch(
		context.Background(),
		"sess-approval",
		"turn-approval",
		ToolCall{
			ID:        "tool-restart-1",
			Name:      "restart_service",
			Arguments: json.RawMessage(`{"service":"redis"}`),
		},
		SessionTypeHost,
		ModeExecute,
	)

	if !result.Blocked || result.Outcome != "approval_needed" {
		t.Fatalf("dispatch result = %#v, want blocked approval_needed", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0 before approval", executor.calls)
	}
	for _, event := range emitter.events {
		if event.Type == EventToolStarted {
			t.Fatalf("emitted %s before approval was resolved", EventToolStarted)
		}
	}
}

func TestToolDispatcher_DispatchApprovedEmitsStartedAfterApprovalGate(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "restarted"}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"restart_service": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name:   "restart_service",
						Origin: tooling.ToolOriginBuiltin,
					},
				},
				executor: executor,
			},
		},
	}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).WithPermissions(permissions.NewEngine([]permissions.Rule{
		{
			Name:   "ask-before-restart",
			Action: permissions.ActionAsk,
			Reason: "service restart needs explicit approval",
			Matcher: permissions.Matcher{
				ToolNames: []string{"restart_service"},
			},
		},
	}))

	result := dispatcher.DispatchApproved(
		context.Background(),
		"sess-approval",
		"turn-approval",
		ToolCall{
			ID:        "tool-restart-1",
			Name:      "restart_service",
			Arguments: json.RawMessage(`{"service":"redis"}`),
		},
		SessionTypeHost,
		ModeExecute,
	)

	if result.Content != "restarted" {
		t.Fatalf("dispatch result content = %q, want restarted", result.Content)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1 after approval", executor.calls)
	}
	if len(emitter.events) != 2 {
		t.Fatalf("events = %d, want started and completed", len(emitter.events))
	}
	if emitter.events[0].Type != EventToolStarted || emitter.events[1].Type != EventToolCompleted {
		t.Fatalf("events = %q then %q, want tool.started then tool.completed", emitter.events[0].Type, emitter.events[1].Type)
	}
}

func TestToolDispatcher_UsesToolPermissionGateBeforePolicyAndExecution(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &permissionCheckingExecutor{
		decision: tooling.PermissionDecision{
			Action: tooling.PermissionActionNeedEvidence,
			Reason: "missing action token",
		},
	}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"exec_command": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name:   "exec_command",
						Origin: tooling.ToolOriginBuiltin,
					},
					InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
				},
				executor: executor,
			},
		},
	}
	dispatcher := NewToolDispatcher(lookup, &policyengine.Engine{
		ModePolicy: map[string]policyengine.ModePolicy{},
	}, emitter)

	result := dispatcher.Dispatch(
		context.Background(),
		"sess-tool-permission",
		"turn-tool-permission",
		ToolCall{
			ID:        "tool-exec-1",
			Name:      "exec_command",
			Arguments: json.RawMessage(`{"command":"systemctl","args":["restart","erp-report.service"]}`),
		},
		SessionTypeHost,
		ModeExecute,
	)

	if !result.Blocked || result.Outcome != "evidence_needed" || result.Source != "tool" {
		t.Fatalf("dispatch result = %#v, want tool evidence_needed", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0 before tool permission gate is resolved", executor.calls)
	}
	for _, event := range emitter.events {
		if event.Type == EventToolStarted {
			t.Fatalf("emitted %s before tool permission gate was resolved", EventToolStarted)
		}
	}
}

func TestToolDispatcher_SessionApprovalGrantBypassesToolPermissionGate(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &permissionCheckingExecutor{
		decision: tooling.PermissionDecision{
			Action: tooling.PermissionActionNeedEvidence,
			Reason: "missing action token",
		},
	}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"exec_command": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name:   "exec_command",
						Origin: tooling.ToolOriginBuiltin,
					},
				},
				executor: executor,
			},
		},
	}
	input := json.RawMessage(`{"cmd":"systemctl restart erp-report.service"}`)
	hash, err := actionproposal.NormalizedInputHash(input)
	if err != nil {
		t.Fatalf("hash input: %v", err)
	}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).WithSessionApprovalGrants([]SessionApprovalGrant{{
		ToolName:  "exec_command",
		InputHash: hash,
		Command:   "systemctl restart erp-report.service",
	}})

	result := dispatcher.Dispatch(
		context.Background(),
		"sess-tool-grant",
		"turn-tool-grant",
		ToolCall{
			ID:        "tool-exec-1",
			Name:      "exec_command",
			Arguments: input,
		},
		SessionTypeHost,
		ModeExecute,
	)

	if result.Blocked || result.Error != "" || result.Content != "should-not-run" {
		t.Fatalf("dispatch result = %#v, want execution via session approval grant", result)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1 when session grant matches", executor.calls)
	}
}

func TestToolDispatcher_DeferredUnloadedToolReturnsRecoverableError(t *testing.T) {
	emitter := &testMockEventEmitter{}
	dispatcher := NewToolDispatcher(&mockToolLookup{tools: map[string]mockToolEntry{}}, nil, emitter).
		WithDeferredCatalogLookup(mockDeferredCatalogLookup{
			"synthetic.resource_reader": {
				Name:        "synthetic.resource_reader",
				Description: "Read bounded resources",
				Layer:       tooling.ToolLayerDeferred,
				Pack:        "synthetic_resources",
				Discovery: tooling.ToolDiscoveryMetadata{
					CapabilityKind: "read",
					ResourceTypes:  []string{"resource"},
					OperationKinds: []string{"read"},
				},
			},
		})

	result := dispatcher.Dispatch(
		context.Background(),
		"sess-deferred",
		"turn-deferred",
		ToolCall{
			ID:        "call-deferred",
			Name:      "synthetic.resource_reader",
			Arguments: json.RawMessage(`{"uri":"synthetic://resource"}`),
		},
		SessionTypeHost,
		ModeInspect,
	)

	if result.Error == "" || !strings.Contains(result.Error, `"errorType":"tool_unloaded"`) {
		t.Fatalf("dispatch error = %q, want structured tool_unloaded", result.Error)
	}
	if !strings.Contains(result.Error, `"requiredAction":"call tool_search with mode=search, then mode=select"`) {
		t.Fatalf("dispatch error missing recovery action: %s", result.Error)
	}
}

func TestToolDispatcher_UnknownToolReturnsStructuredNotFound(t *testing.T) {
	emitter := &testMockEventEmitter{}
	dispatcher := NewToolDispatcher(&mockToolLookup{tools: map[string]mockToolEntry{}}, nil, emitter)

	result := dispatcher.Dispatch(
		context.Background(),
		"sess-unknown",
		"turn-unknown",
		ToolCall{ID: "call-unknown", Name: "synthetic.missing", Arguments: json.RawMessage(`{}`)},
		SessionTypeHost,
		ModeInspect,
	)

	if result.Error == "" || !strings.Contains(result.Error, `"errorType":"tool_not_found"`) {
		t.Fatalf("dispatch error = %q, want structured tool_not_found", result.Error)
	}
}

func TestDispatchUsesSamePolicySnapshot(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "hidden result"}
	meta := tooling.ToolMetadata{Name: "synthetic.hidden_read", RiskLevel: tooling.ToolRiskLow}
	dispatcher := NewToolDispatcher(&mockToolLookup{tools: map[string]mockToolEntry{
		"synthetic.hidden_read": {
			desc:     ToolDescriptor{Metadata: meta},
			executor: executor,
		},
	}}, nil, emitter).WithToolSurfacePolicySnapshot(&tooling.ToolSurfacePolicySnapshot{
		Hash: "policy-hash-1",
		HiddenTools: []tooling.ToolHiddenReason{{
			Name:   "synthetic.hidden_read",
			Reason: "hidden_from_prompt",
		}},
	})

	result := dispatcher.Dispatch(
		context.Background(),
		"sess-policy",
		"turn-policy",
		ToolCall{ID: "call-hidden", Name: "synthetic.hidden_read", Arguments: json.RawMessage(`{}`)},
		SessionTypeHost,
		ModeInspect,
	)

	if result.Error == "" || !strings.Contains(result.Error, `"errorType":"tool_hidden_by_policy"`) {
		t.Fatalf("dispatch error = %q, want tool_hidden_by_policy", result.Error)
	}
	if !strings.Contains(result.Error, `"policySnapshotHash":"policy-hash-1"`) {
		t.Fatalf("dispatch error missing policy snapshot hash: %s", result.Error)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestToolDispatcher_DedicatedToolPreferenceRejectsUnexplainedShellFallback(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "raw file"}
	dispatcher := NewToolDispatcher(&mockToolLookup{tools: map[string]mockToolEntry{
		"exec_command": {
			desc:     ToolDescriptor{Metadata: tooling.ToolMetadata{Name: "exec_command"}},
			executor: executor,
		},
	}}, nil, emitter).WithVisibleToolMetadata([]tooling.ToolMetadata{
		{
			Name: "synthetic.read_file",
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"file"},
				OperationKinds: []string{"read"},
			},
		},
	})

	result := dispatcher.Dispatch(
		context.Background(),
		"sess-dedicated",
		"turn-dedicated",
		ToolCall{ID: "call-shell", Name: "exec_command", Arguments: json.RawMessage(`{"command":"cat","args":["synthetic.log"]}`)},
		SessionTypeHost,
		ModeInspect,
	)

	if result.Error == "" || !strings.Contains(result.Error, `"errorType":"dedicated_tool_preferred"`) {
		t.Fatalf("dispatch error = %q, want dedicated_tool_preferred", result.Error)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestToolDispatcher_DedicatedToolPreferenceAllowsReasonedShellFallback(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "raw file"}
	dispatcher := NewToolDispatcher(&mockToolLookup{tools: map[string]mockToolEntry{
		"exec_command": {
			desc:     ToolDescriptor{Metadata: tooling.ToolMetadata{Name: "exec_command"}},
			executor: executor,
		},
	}}, nil, emitter).WithVisibleToolMetadata([]tooling.ToolMetadata{
		{
			Name: "synthetic.read_file",
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"file"},
				OperationKinds: []string{"read"},
			},
		},
	})

	result := dispatcher.Dispatch(
		context.Background(),
		"sess-dedicated",
		"turn-dedicated",
		ToolCall{ID: "call-shell", Name: "exec_command", Arguments: json.RawMessage(`{"command":"cat","args":["synthetic.log"],"fallbackReason":"need exact byte count from shell"}`)},
		SessionTypeHost,
		ModeInspect,
	)

	if result.Error != "" || result.Content != "raw file" {
		t.Fatalf("dispatch result = %#v, want allowed shell fallback", result)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
}

type mockDeferredCatalogLookup map[string]tooling.ToolMetadata

func (m mockDeferredCatalogLookup) LookupDeferredTool(name string) (tooling.ToolMetadata, bool) {
	meta, ok := m[name]
	return meta, ok
}

func TestToolDispatcher_CompletedPayloadFitsBudgetForLargeResult(t *testing.T) {
	emitter := &testMockEventEmitter{}
	largeResult := strings.Repeat("x", 20*1024)
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"read_log": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name:   "read_log",
						Origin: tooling.ToolOriginBuiltin,
					},
				},
				executor: &mockToolExecutor{result: largeResult},
			},
		},
	}
	dispatcher := NewToolDispatcher(lookup, nil, emitter)

	result := dispatcher.Dispatch(
		context.Background(),
		"sess-budget",
		"turn-budget",
		ToolCall{
			ID:        "tool-large-1",
			Name:      "read_log",
			Arguments: json.RawMessage(`{"path":"/var/log/app.log"}`),
		},
		SessionTypeHost,
		ModeInspect,
	)

	if result.Content != largeResult {
		t.Fatal("dispatch result should keep full content for model context")
	}
	var completed *LifecycleEvent
	for i := range emitter.events {
		if emitter.events[i].Type == EventToolCompleted {
			completed = &emitter.events[i]
			break
		}
	}
	if completed == nil {
		t.Fatal("expected tool.completed event")
	}
	encoded, err := json.Marshal(completed)
	if err != nil {
		t.Fatalf("marshal completed lifecycle event: %v", err)
	}
	if len(encoded) > 8192 {
		t.Fatalf("tool event payload = %d bytes, want <= 8192", len(encoded))
	}
	var payload map[string]any
	if err := json.Unmarshal(completed.Payload, &payload); err != nil {
		t.Fatalf("payload decode error = %v", err)
	}
	if payload["result"] == largeResult {
		t.Fatal("tool.completed payload should not include the full large result")
	}
	if payload["rawRef"] == "" {
		t.Fatal("tool.completed payload should include rawRef for large results")
	}
	if payload["outputPreview"] == nil {
		t.Fatal("large tool result should include a bounded outputPreview")
	}
}

func TestToolDispatcher_HighRiskMetadataRequiresApproval(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "secret"}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"read_secret": {
				desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
					Name:      "read_secret",
					RiskLevel: tooling.ToolRiskHigh,
				}},
				executor: executor,
			},
		},
	}
	dispatcher := NewToolDispatcher(lookup, &policyengine.Engine{ModePolicy: policyengine.NewDefaultModePolicies()}, emitter)

	result := dispatcher.Dispatch(context.Background(), "sess-risk", "turn-risk", ToolCall{
		ID:   "call-risk",
		Name: "read_secret",
	}, SessionTypeHost, ModeExecute)

	if !result.Blocked || result.Outcome != "approval_needed" {
		t.Fatalf("dispatch result = %#v, want approval_needed", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0 before approval", executor.calls)
	}
	for _, event := range emitter.events {
		if event.Type == EventToolStarted {
			t.Fatalf("emitted %s before high-risk approval", EventToolStarted)
		}
	}
}

func TestToolDispatcher_MutatingMetadataDeniedInInspectMode(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "mutated"}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"custom_probe": {
				desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
					Name:     "custom_probe",
					Mutating: true,
				}},
				executor: executor,
			},
		},
	}
	dispatcher := NewToolDispatcher(lookup, &policyengine.Engine{ModePolicy: policyengine.NewDefaultModePolicies()}, emitter)

	result := dispatcher.Dispatch(context.Background(), "sess-mut", "turn-mut", ToolCall{
		ID:   "call-mut",
		Name: "custom_probe",
	}, SessionTypeHost, ModeInspect)

	if result.Outcome != "tool_denied" || !strings.Contains(result.Error, "mutating") {
		t.Fatalf("dispatch result = %#v, want metadata mutation denial", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0 for denied mutation", executor.calls)
	}
}

func TestToolDispatcher_FailurePolicyFailTurnDoesNotFeedFailureBackToModel(t *testing.T) {
	emitter := &testMockEventEmitter{}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"fragile_mutation": {
				desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
					Name:          "fragile_mutation",
					FailurePolicy: tooling.ToolFailurePolicyFailTurn,
				}},
				executor: &mockToolExecutor{err: assertErr("boom")},
			},
		},
	}
	dispatcher := NewToolDispatcher(lookup, nil, emitter)

	result := dispatcher.Dispatch(context.Background(), "sess-fail", "turn-fail", ToolCall{
		ID:   "call-fail",
		Name: "fragile_mutation",
	}, SessionTypeHost, ModeExecute)

	if result.Error == "" {
		t.Fatalf("dispatch result = %#v, want tool error", result)
	}
	if shouldFeedToolFailureBackToModel(result) {
		t.Fatalf("failure policy should fail the turn, got feed-back result %#v", result)
	}
}

func TestToolDispatcher_RejectsArgumentsMissingRequiredSchemaFieldBeforeExecution(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "should-not-run"}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"read_metrics": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name:   "read_metrics",
						Origin: tooling.ToolOriginBuiltin,
					},
					InputSchema: json.RawMessage(`{
						"type":"object",
						"required":["namespace"],
						"properties":{
							"namespace":{"type":"string"},
							"service":{"type":"string"}
						}
					}`),
				},
				executor: executor,
			},
		},
	}
	dispatcher := NewToolDispatcher(lookup, nil, emitter)

	result := dispatcher.Dispatch(context.Background(), "sess-schema", "turn-schema", ToolCall{
		ID:        "call-schema",
		Name:      "read_metrics",
		Arguments: json.RawMessage(`{"service":"api"}`),
	}, SessionTypeHost, ModeInspect)

	if result.Outcome != "tool_failed" || result.Source != "runtime" {
		t.Fatalf("dispatch result = %#v, want runtime tool_failed", result)
	}
	if !strings.Contains(result.Error, "invalid arguments") || !strings.Contains(result.Error, "namespace") {
		t.Fatalf("dispatch error = %q, want invalid arguments mentioning namespace", result.Error)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0 when schema validation fails", executor.calls)
	}
	for _, event := range emitter.events {
		if event.Type == EventToolStarted {
			t.Fatalf("emitted %s before schema validation passed", EventToolStarted)
		}
	}
}

func TestToolDispatcher_RejectsMalformedJSONArgumentsBeforeExecution(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "should-not-run"}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"read_metrics": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name:   "read_metrics",
						Origin: tooling.ToolOriginBuiltin,
					},
					InputSchema: json.RawMessage(`{"type":"object"}`),
				},
				executor: executor,
			},
		},
	}
	dispatcher := NewToolDispatcher(lookup, nil, emitter)

	result := dispatcher.Dispatch(context.Background(), "sess-schema", "turn-schema", ToolCall{
		ID:        "call-schema",
		Name:      "read_metrics",
		Arguments: json.RawMessage(`{`),
	}, SessionTypeHost, ModeInspect)

	if result.Outcome != "tool_failed" || result.Source != "runtime" {
		t.Fatalf("dispatch result = %#v, want runtime tool_failed", result)
	}
	if !strings.Contains(result.Error, "invalid arguments") {
		t.Fatalf("dispatch error = %q, want invalid arguments", result.Error)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0 when JSON validation fails", executor.calls)
	}
}

type permissionCheckingExecutor struct {
	decision tooling.PermissionDecision
	calls    int
}

func (e *permissionCheckingExecutor) CheckPermissions(context.Context, json.RawMessage) tooling.PermissionDecision {
	return e.decision
}

func (e *permissionCheckingExecutor) Execute(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	e.calls++
	return tooling.ToolResult{Content: "should-not-run"}, nil
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
