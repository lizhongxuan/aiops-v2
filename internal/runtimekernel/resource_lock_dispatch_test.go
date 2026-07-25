package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/runtimekernel/toolfailure"
	"aiops-v2/internal/tooling"
)

type recordingToolResourceLockGate struct {
	decision ToolResourceLockDecision
	requests []ToolResourceLockRequest
	released int
}

type approvalResourceLockExecutor struct {
	calls int
}

func (e *approvalResourceLockExecutor) CheckPermissions(context.Context, json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionNeedApproval, Reason: "review exact workflow"}
}

func (e *approvalResourceLockExecutor) Execute(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	e.calls++
	return tooling.ToolResult{Content: "should-not-run"}, nil
}

func (g *recordingToolResourceLockGate) AcquireToolResourceLocks(_ context.Context, req ToolResourceLockRequest) (ToolResourceLockDecision, func(), error) {
	g.requests = append(g.requests, req)
	decision := g.decision
	if decision.Action == "" {
		decision.Action = "acquired"
	}
	return decision, func() { g.released++ }, nil
}

func TestToolDispatcherAcquiresAndReleasesResourceLockForMutatingTool(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "changed"}
	gate := &recordingToolResourceLockGate{}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"write_config": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name:             "write_config",
				Mutating:         true,
				RequiresApproval: true,
				ResourceLocks: []tooling.ToolResourceLockKey{{
					ResourceType:  "file",
					ResourceID:    "config://service-a",
					OperationKind: "write",
				}},
				Idempotency: tooling.ToolIdempotencyMetadata{
					Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
					PostCheckRefs: []string{"cat config://service-a"},
				},
			}},
			executor: executor,
		},
	}}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).
		WithPermissionBinding("sha256:test", "sha256:test").
		WithResourceLockGate(gate)

	result := dispatcher.Dispatch(context.Background(), "sess-lock", "turn-lock", ToolCall{
		ID:        "call-lock",
		Name:      "write_config",
		Arguments: json.RawMessage(`{"path":"/etc/example.conf"}`),
	}, SessionTypeHost, ModeExecute)

	if result.Content != "changed" || result.Error != "" {
		t.Fatalf("dispatch result = %#v, want successful tool result", result)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1 after lock acquired", executor.calls)
	}
	if len(gate.requests) != 1 {
		t.Fatalf("lock requests = %d, want 1", len(gate.requests))
	}
	if gate.released != 1 {
		t.Fatalf("lock releases = %d, want 1", gate.released)
	}
	req := gate.requests[0]
	if req.OwnerID == "" || req.ToolCall.ID != "call-lock" || len(req.Keys) != 1 {
		t.Fatalf("lock request = %#v, want owner, tool call, and one key", req)
	}
	if len(result.ResourceLocks) != 1 || result.ResourceLocks[0].Action != "acquired" {
		t.Fatalf("resource lock trace = %#v, want acquired trace", result.ResourceLocks)
	}
}

func TestToolDispatcherResolvesDynamicResourceLockFromTrustedContext(t *testing.T) {
	executor := &mockToolExecutor{result: "changed"}
	gate := &recordingToolResourceLockGate{}
	resolver := func(args json.RawMessage, execution tooling.ToolExecutionContext) ([]tooling.ToolResourceLockKey, error) {
		var input struct {
			WorkflowID string `json:"workflowId"`
		}
		if json.Unmarshal(args, &input) != nil || execution.TenantID == "" || input.WorkflowID == "" {
			return nil, errors.New("trusted workflow scope is unavailable")
		}
		return []tooling.ToolResourceLockKey{{ResourceType: "workflow", ResourceID: execution.TenantID + ":" + input.WorkflowID, OperationKind: "apply"}}, nil
	}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"workflow.apply_patch": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name: "workflow.apply_patch", Mutating: true, RequiresApproval: true, ResourceLockResolver: resolver,
				Idempotency: tooling.ToolIdempotencyMetadata{Strategy: tooling.ToolIdempotencyStrategyExplicitKey, KeyFields: []string{"idempotencyKey"}, PostCheckRefs: []string{"workflow receipt"}},
			}},
			executor: executor,
		},
	}}
	dispatcher := NewToolDispatcher(lookup, nil, &testMockEventEmitter{}).
		WithPermissionBinding("sha256:test", "sha256:test").
		WithResourceLockGate(gate)
	ctx := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{TenantID: "tenant-a", UserID: "user-a"})
	result := dispatcher.Dispatch(ctx, "sess-lock", "turn-lock", ToolCall{ID: "call-lock", Name: "workflow.apply_patch", Arguments: json.RawMessage(`{"workflowId":"workflow-a","idempotencyKey":"idem-a"}`)}, SessionTypeHost, ModeExecute)

	if result.Error != "" || executor.calls != 1 || len(gate.requests) != 1 {
		t.Fatalf("dispatch = %#v, calls=%d locks=%d", result, executor.calls, len(gate.requests))
	}
	keys := gate.requests[0].Keys
	if len(keys) != 1 || keys[0].ResourceType != "workflow" || keys[0].ResourceID != "tenant-a:workflow-a" || keys[0].OperationKind != "apply" {
		t.Fatalf("resolved keys = %#v", keys)
	}
}

func TestToolDispatcherBindsDynamicResourceLockBeforeApprovalWithoutAcquiringIt(t *testing.T) {
	executor := &approvalResourceLockExecutor{}
	gate := &recordingToolResourceLockGate{}
	resolver := func(_ json.RawMessage, execution tooling.ToolExecutionContext) ([]tooling.ToolResourceLockKey, error) {
		if execution.TenantID != "tenant-a" {
			return nil, errors.New("trusted tenant is unavailable")
		}
		return []tooling.ToolResourceLockKey{{ResourceType: "workflow", ResourceID: "tenant-a:workflow-a", OperationKind: "apply"}}, nil
	}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"workflow.apply_patch": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name: "workflow.apply_patch", Mutating: true, RequiresApproval: true,
				ResourceLockResolver: resolver,
				Discovery:            tooling.ToolDiscoveryMetadata{ResourceTypes: []string{"workflow", "workflow_candidate"}},
				Idempotency: tooling.ToolIdempotencyMetadata{
					Strategy: tooling.ToolIdempotencyStrategyExplicitKey, KeyFields: []string{"idempotencyKey"}, PostCheckRefs: []string{"workflow receipt"},
				},
			}},
			executor: executor,
		},
	}}
	dispatcher := NewToolDispatcher(lookup, nil, &testMockEventEmitter{}).
		WithPermissionBinding("sha256:test", "sha256:test").
		WithResourceLockGate(gate)
	ctx := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{TenantID: "tenant-a"})
	result := dispatcher.Dispatch(ctx, "sess-lock", "turn-lock", ToolCall{
		ID: "call-lock", Name: "workflow.apply_patch", Arguments: json.RawMessage(`{"idempotencyKey":"idem-a"}`),
	}, SessionTypeHost, ModeExecute)

	if !result.Blocked || result.Outcome != "approval_needed" || result.Source != "tool" {
		t.Fatalf("dispatch = %#v, want exact approval boundary", result)
	}
	if executor.calls != 0 || len(gate.requests) != 0 {
		t.Fatalf("approval boundary executed/acquired early: calls=%d requests=%d", executor.calls, len(gate.requests))
	}
	if result.Metadata.ResourceLockResolver != nil {
		t.Fatal("approval metadata retained an unfrozen dynamic resolver")
	}
	if len(result.Metadata.ResourceLocks) != 1 || result.Metadata.ResourceLocks[0] != (tooling.ToolResourceLockKey{ResourceType: "workflow", ResourceID: "tenant-a:workflow-a", OperationKind: "apply"}) {
		t.Fatalf("approval resource locks = %#v", result.Metadata.ResourceLocks)
	}
	wantScopes := []string{"type=workflow", "type=workflow id=tenant-a:workflow-a op=apply", "type=workflow_candidate"}
	if got := pendingApprovalResourceScopes(result.Metadata); strings.Join(got, "\x00") != strings.Join(wantScopes, "\x00") {
		t.Fatalf("approval resource scopes = %#v, want %#v", got, wantScopes)
	}
}

func TestToolDispatcherDynamicResourceLockResolverFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		resolver tooling.ToolResourceLockResolver
	}{
		{
			name: "empty scope",
			resolver: func(json.RawMessage, tooling.ToolExecutionContext) ([]tooling.ToolResourceLockKey, error) {
				return nil, nil
			},
		},
		{
			name: "noncanonical whitespace",
			resolver: func(json.RawMessage, tooling.ToolExecutionContext) ([]tooling.ToolResourceLockKey, error) {
				return []tooling.ToolResourceLockKey{{ResourceType: " workflow", ResourceID: "tenant-a:workflow-a", OperationKind: "apply"}}, nil
			},
		},
		{
			name: "missing resource id",
			resolver: func(json.RawMessage, tooling.ToolExecutionContext) ([]tooling.ToolResourceLockKey, error) {
				return []tooling.ToolResourceLockKey{{ResourceType: "workflow", OperationKind: "apply"}}, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockToolExecutor{result: "should-not-run"}
			gate := &recordingToolResourceLockGate{}
			lookup := &mockToolLookup{tools: map[string]mockToolEntry{
				"workflow.apply_patch": {
					desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
						Name: "workflow.apply_patch", Mutating: true, RequiresApproval: true,
						ResourceLockResolver: tt.resolver,
						Idempotency: tooling.ToolIdempotencyMetadata{
							Strategy: tooling.ToolIdempotencyStrategyExplicitKey, KeyFields: []string{"idempotencyKey"}, PostCheckRefs: []string{"workflow receipt"},
						},
					}},
					executor: executor,
				},
			}}
			dispatcher := NewToolDispatcher(lookup, nil, &testMockEventEmitter{}).
				WithPermissionBinding("sha256:test", "sha256:test").
				WithResourceLockGate(gate)
			result := dispatcher.Dispatch(context.Background(), "sess-lock", "turn-lock", ToolCall{
				ID: "call-lock", Name: "workflow.apply_patch", Arguments: json.RawMessage(`{"idempotencyKey":"idem-a"}`),
			}, SessionTypeWorkspace, ModeExecute)

			if result.Outcome != "tool_failed" || result.Source != "runtime" || executor.calls != 0 || len(gate.requests) != 0 {
				t.Fatalf("dispatch = %#v, calls=%d requests=%d; want runtime fail-closed before execution/acquisition", result, executor.calls, len(gate.requests))
			}
			if !strings.Contains(result.Error, "resource lock") && !strings.Contains(result.Error, "non-canonical") {
				t.Fatalf("dispatch error = %q, want resource lock scope failure", result.Error)
			}
		})
	}
}

func TestToolDispatcherBlocksMutatingToolOnResourceLockConflict(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "should-not-run"}
	gate := &recordingToolResourceLockGate{decision: ToolResourceLockDecision{
		Action: "denied",
		Reason: "resource_lock_conflict",
		Holder: "worker-a",
	}}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"write_config": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name:             "write_config",
				Mutating:         true,
				RequiresApproval: true,
				ResourceLocks: []tooling.ToolResourceLockKey{{
					ResourceType:  "file",
					ResourceID:    "config://service-a",
					OperationKind: "write",
				}},
				Idempotency: tooling.ToolIdempotencyMetadata{
					Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
					PostCheckRefs: []string{"cat config://service-a"},
				},
			}},
			executor: executor,
		},
	}}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).
		WithPermissionBinding("sha256:test", "sha256:test").
		WithResourceLockGate(gate)

	result := dispatcher.Dispatch(context.Background(), "sess-lock", "turn-lock", ToolCall{
		ID:        "call-lock",
		Name:      "write_config",
		Arguments: json.RawMessage(`{"path":"/etc/example.conf"}`),
	}, SessionTypeHost, ModeExecute)

	if result.Outcome != "tool_failed" || result.Source != "runtime" {
		t.Fatalf("dispatch result = %#v, want runtime tool_failed", result)
	}
	if !strings.Contains(result.Error, "resource_lock_conflict") || !strings.Contains(result.Error, "worker-a") {
		t.Fatalf("dispatch error = %q, want structured resource lock conflict with holder", result.Error)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0 when lock conflicts", executor.calls)
	}
	if gate.released != 0 {
		t.Fatalf("lock releases = %d, want 0 for unacquired lock", gate.released)
	}
	for _, event := range emitter.events {
		if event.Type == EventToolStarted {
			t.Fatalf("emitted %s despite resource lock conflict", EventToolStarted)
		}
	}
	if len(result.ResourceLocks) != 1 || result.ResourceLocks[0].Action != "denied" || result.ResourceLocks[0].Holder != "worker-a" {
		t.Fatalf("resource lock trace = %#v, want denied holder trace", result.ResourceLocks)
	}
}

func TestToolDispatcherResourceLockMutationRetryGuardDeniesMutationWithoutSafetyMetadata(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "should-not-run"}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"write_config": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name:             "write_config",
				Mutating:         true,
				RequiresApproval: true,
			}},
			executor: executor,
		},
	}}
	input := json.RawMessage(`{"path":"/etc/example.conf","content":"changed"}`)
	hash, err := actionproposal.NormalizedInputHash(input)
	if err != nil {
		t.Fatalf("hash input: %v", err)
	}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).
		WithPermissionBinding("sha256:test", "sha256:test").
		WithSessionApprovalGrants([]SessionApprovalGrant{{
			ToolName:  "write_config",
			InputHash: hash,
		}})

	result := dispatcher.Dispatch(context.Background(), "sess-lock", "turn-lock", ToolCall{
		ID:        "call-lock",
		Name:      "write_config",
		Arguments: input,
	}, SessionTypeHost, ModeExecute)

	if result.Outcome != "tool_denied" || result.Source != "runtime" {
		t.Fatalf("dispatch result = %#v, want runtime tool_denied", result)
	}
	for _, want := range []string{"mutation_safety_guard", "resourceLocks", "idempotency"} {
		if !strings.Contains(result.Error, want) {
			t.Fatalf("dispatch error = %q, want %q", result.Error, want)
		}
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0 when mutation safety metadata is missing", executor.calls)
	}
	for _, event := range emitter.events {
		if event.Type == EventToolStarted {
			t.Fatalf("emitted %s despite mutation safety guard denial", EventToolStarted)
		}
	}
}

func TestToolDispatcherDynamicMCPAnnotationsCannotBypassMutationSafety(t *testing.T) {
	executor := &mockToolExecutor{result: "should-not-run"}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"apply_change": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name:             "apply_change",
				Origin:           tooling.ToolOriginMCP,
				IsMCP:            true,
				Mutating:         true,
				RequiresApproval: true,
				MCPInfo: tooling.MCPInfo{
					ServerID: "dynamic-safety", ToolName: "apply_change",
					Raw: json.RawMessage(`{"annotations":{"readOnlyHint":true,"destructiveHint":false},"resourceLocks":[{"resourceType":"workflow","resourceId":"forged"}],"idempotency":{"strategy":"explicit_key"}}`),
				},
			}},
			executor: executor,
		},
	}}
	dispatcher := NewToolDispatcher(lookup, nil, &testMockEventEmitter{}).
		WithPermissionBinding("sha256:test", "sha256:test")
	result := dispatcher.Dispatch(context.Background(), "sess-mcp", "turn-mcp", ToolCall{
		ID: "call-mcp", Name: "apply_change", Arguments: json.RawMessage(`{"target":"workflow-a"}`),
	}, SessionTypeWorkspace, ModeExecute)

	if result.Outcome != "tool_denied" || result.Source != "runtime" || executor.calls != 0 {
		t.Fatalf("dispatch = %#v, calls=%d; want mutation guard denial", result, executor.calls)
	}
	for _, want := range []string{"mutation_safety_guard", "resourceLocks", "idempotency"} {
		if !strings.Contains(result.Error, want) {
			t.Fatalf("dispatch error = %q, want %q", result.Error, want)
		}
	}
}

func TestMutationRetryGuardFailureRequiresPostCheck(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{err: errors.New("context deadline exceeded after package manager started")}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"install_package": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name:             "install_package",
				Mutating:         true,
				RequiresApproval: true,
				ResourceLocks: []tooling.ToolResourceLockKey{{
					ResourceType:  "host",
					ResourceID:    "host-a",
					OperationKind: "package_install",
				}},
				Idempotency: tooling.ToolIdempotencyMetadata{
					Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
					PostCheckRefs: []string{"psql --version"},
				},
			}},
			executor: executor,
		},
	}}
	input := json.RawMessage(`{"package":"postgresql"}`)
	hash, err := actionproposal.NormalizedInputHash(input)
	if err != nil {
		t.Fatalf("hash input: %v", err)
	}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).
		WithPermissionBinding("sha256:test", "sha256:test").
		WithResourceLockGate(&recordingToolResourceLockGate{}).
		WithReadOnlyRetryConfig(ReadOnlyRetryConfig{Enabled: true, MaxPerCall: 1, MaxPerTurn: 1}).
		WithSessionApprovalGrants([]SessionApprovalGrant{{
			ToolName:  "install_package",
			InputHash: hash,
		}})

	result := dispatcher.Dispatch(context.Background(), "sess-mutation", "turn-mutation", ToolCall{
		ID:        "call-install",
		Name:      "install_package",
		Arguments: input,
	}, SessionTypeHost, ModeExecute)

	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want no automatic retry for mutation timeout", executor.calls)
	}
	if len(result.Attempts) != 0 {
		t.Fatalf("retry attempts = %#v, want none for mutation failure", result.Attempts)
	}
	if failureKindForDispatchResult(result) != string(toolfailure.KindSideEffectUnknown) {
		t.Fatalf("failure kind = %q, want side_effect_unknown", failureKindForDispatchResult(result))
	}
	toolResult := failedToolResultForModel(ToolCall{ID: "call-install", Name: "install_package"}, result)
	for _, want := range []string{"side_effect_unknown", "postCheckRequired", "psql --version"} {
		if !strings.Contains(toolResult.Content, want) {
			t.Fatalf("failed tool result = %s, want %q", toolResult.Content, want)
		}
	}
}

func TestWorkflowMutationResultBudgetPreservesReceiptAndErrorRecoveryFacts(t *testing.T) {
	tests := []struct {
		name       string
		marker     string
		structured json.RawMessage
		isError    bool
		errorText  string
	}{
		{
			name:       "receipt",
			marker:     "receipt status=completed transaction=txn-1",
			structured: json.RawMessage(`{"kind":"receipt","transactionId":"txn-1","status":"completed"}`),
		},
		{
			name:       "typed error",
			marker:     "STALE_REVISION refresh workflow context",
			structured: json.RawMessage(`{"kind":"error","code":"STALE_REVISION","retryHint":"refresh workflow context"}`),
			isError:    true,
			errorText:  "STALE_REVISION",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemoryToolResultSpillRepo()
			kernel := &RuntimeKernel{spillRepo: repo}
			session := &SessionState{ID: "sess-budget"}
			snapshot := &TurnSnapshot{ID: "turn-budget"}
			content := tt.marker + ": " + strings.Repeat("diagnostic-detail ", 32)
			materialized, err := kernel.materializeToolResult(session, snapshot, 1, ToolCall{ID: "call-apply", Name: "workflow.apply_patch"}, tooling.ToolMetadata{
				Name: "workflow.apply_patch",
				ResultBudget: tooling.ResultBudget{
					MaxInlineResultBytes: 64, SpillPolicy: tooling.ResultSpillPolicyExternalize, SummarizeLargeResult: true,
				},
			}, tooling.ToolResult{
				Content: content, StructuredContent: tt.structured, IsError: tt.isError, Error: tt.errorText,
				References: []tooling.ResultReference{{Kind: tooling.ResultReferenceKindMCPResource, URI: "workflow://resource/v2/receipt-txn-1", Title: "Workflow receipt"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !materialized.Spilled || !strings.Contains(materialized.Content, tt.marker) || !strings.Contains(materialized.Content, "External ref:") {
				t.Fatalf("materialized summary = %#v", materialized)
			}
			if string(materialized.StructuredContent) != string(tt.structured) || materialized.IsError != tt.isError || materialized.Error != tt.errorText {
				t.Fatalf("typed receipt/error facts changed: %#v", materialized)
			}
			if !toolResultHasReferenceURIForSafetyTest(materialized.References, "workflow://resource/v2/receipt-txn-1") {
				t.Fatalf("domain reference was lost: %#v", materialized.References)
			}
			spillID := toolResultBlobReferenceIDForSafetyTest(materialized.References)
			if spillID == "" {
				t.Fatalf("spill reference missing: %#v", materialized.References)
			}
			spill, err := repo.GetToolResultSpill(spillID)
			if err != nil || string(spill.Content) != content {
				t.Fatalf("persisted spill = %#v, %v", spill, err)
			}
		})
	}
}

func toolResultHasReferenceURIForSafetyTest(refs []ToolResultReference, uri string) bool {
	for _, ref := range refs {
		if ref.URI == uri {
			return true
		}
	}
	return false
}

func toolResultBlobReferenceIDForSafetyTest(refs []ToolResultReference) string {
	for _, ref := range refs {
		if ref.Kind == ToolResultReferenceKindBlob && strings.HasPrefix(ref.URI, "store://tool-spills/") {
			return strings.TrimPrefix(ref.URI, "store://tool-spills/")
		}
	}
	return ""
}
