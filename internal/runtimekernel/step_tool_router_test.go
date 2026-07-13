package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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
