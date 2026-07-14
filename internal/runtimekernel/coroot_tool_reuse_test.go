package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/tooling"
)

func TestMaybeReuseCorootListServicesStatusFilterFromPriorBroadResult(t *testing.T) {
	tools := []promptcompiler.Tool{
		&tooling.StaticTool{
			Meta: tooling.ToolMetadata{Name: "coroot.list_services", Description: "List Coroot services"},
			ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
				t.Fatal("executor should not run for a status filter covered by the prior broad result")
				return tooling.ToolResult{}, nil
			},
		},
	}
	snapshot := &TurnSnapshot{
		ID:        "turn-coroot",
		SessionID: "session-coroot",
		Iterations: []IterationState{{
			ToolCalls: []ToolCall{{
				ID:        "call-list-all",
				Name:      "coroot_list_services",
				Arguments: json.RawMessage(`{}`),
			}},
			ToolResults: []ToolResult{{
				ToolCallID: "call-list-all",
				Content:    `{"schemaVersion":"aiops.coroot/v1","tool":"coroot.list_services","status":"ok","totalServices":3,"statusCounts":{"warning":1,"critical":1},"problemServices":[{"name":"checkout","status":"warning"}]}`,
			}},
		}},
	}
	current := ToolCall{
		ID:        "call-list-warning",
		Name:      "coroot_list_services",
		Arguments: json.RawMessage(`{"status":"warning"}`),
	}

	result, ok := maybeReuseCoveredReadResult(snapshot, tools, current)
	if !ok {
		t.Fatal("maybeReuseCoveredReadResult() = false, want reuse")
	}
	if result.Result.ToolCallID != "call-list-warning" || result.Error != "" {
		t.Fatalf("reuse result = %#v, want successful current tool result", result)
	}
	if result.Outcome != "tool_reused" || result.Source != "runtime" {
		t.Fatalf("reuse outcome/source = %q/%q, want tool_reused/runtime", result.Outcome, result.Source)
	}
	if !strings.Contains(result.Result.Content, corootReuseSkipReason) || !strings.Contains(result.Result.Content, "call-list-all") {
		t.Fatalf("reuse content missing skip reason or prior call id: %s", result.Result.Content)
	}
}

func TestMaybeReuseCorootListServicesStatusFilterFromExternalizedPriorResult(t *testing.T) {
	tools := []promptcompiler.Tool{
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.list_services"}},
	}
	snapshot := &TurnSnapshot{
		ID:        "turn-coroot",
		SessionID: "session-coroot",
		Iterations: []IterationState{{
			ToolCalls: []ToolCall{{
				ID:        "call-list-all",
				Name:      "coroot_list_services",
				Arguments: json.RawMessage(`{}`),
			}},
			ToolResults: []ToolResult{{
				ToolCallID: "call-list-all",
				Content:    `Summary: {"categoryCounts":{"application":25},"evidenceRefs":["ev-1"],"modelGuidance":"Use problemServices/statusCounts from this result before calling coroot.list_services again","problemServices":[{"name":"checkout","status":"warning"}],"statusCounts":{"warning":1}}\nExternal ref: store://tool-spills/spill-turn-coroot-0-call-list-all.`,
				Spilled:    true,
				ExternalReferences: []ExternalReference{{
					ID:    "spill-turn-coroot-0-call-list-all",
					URI:   "store://tool-spills/spill-turn-coroot-0-call-list-all",
					Title: "coroot.list_services",
				}},
			}},
		}},
	}
	current := ToolCall{
		ID:        "call-list-warning",
		Name:      "coroot_list_services",
		Arguments: json.RawMessage(`{"status":"warning"}`),
	}

	result, ok := maybeReuseCoveredReadResult(snapshot, tools, current)
	if !ok {
		t.Fatal("maybeReuseCoveredReadResult() = false, want reuse from externalized prior result")
	}
	if !strings.Contains(result.Result.Content, corootReuseSkipReason) || !strings.Contains(result.Result.Content, "call-list-all") {
		t.Fatalf("reuse content missing skip reason or prior call id: %s", result.Result.Content)
	}
}

func TestMaybeReuseCorootListServicesKeepsDistinctNamespaceFilterExecutable(t *testing.T) {
	tools := []promptcompiler.Tool{
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.list_services"}},
	}
	snapshot := &TurnSnapshot{
		ID:        "turn-coroot",
		SessionID: "session-coroot",
		Iterations: []IterationState{{
			ToolCalls: []ToolCall{{
				ID:        "call-list-all",
				Name:      "coroot_list_services",
				Arguments: json.RawMessage(`{}`),
			}},
			ToolResults: []ToolResult{{
				ToolCallID: "call-list-all",
				Content:    `{"schemaVersion":"aiops.coroot/v1","tool":"coroot.list_services","status":"ok","totalServices":3}`,
			}},
		}},
	}
	current := ToolCall{
		ID:        "call-list-namespace",
		Name:      "coroot_list_services",
		Arguments: json.RawMessage(`{"namespace":"smecloud","status":"warning"}`),
	}

	if result, ok := maybeReuseCoveredReadResult(snapshot, tools, current); ok {
		t.Fatalf("maybeReuseCoveredReadResult() = %#v,true, want no reuse for namespace filter", result)
	}
}

func TestMaybeReuseCorootIncidentsLimitOnlyFromPriorBroadResult(t *testing.T) {
	tools := []promptcompiler.Tool{
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.incidents"}},
	}
	snapshot := &TurnSnapshot{
		ID:        "turn-coroot",
		SessionID: "session-coroot",
		Iterations: []IterationState{{
			ToolCalls: []ToolCall{{
				ID:        "call-incidents",
				Name:      "coroot_incidents",
				Arguments: json.RawMessage(`{}`),
			}},
			ToolResults: []ToolResult{{
				ToolCallID: "call-incidents",
				Content:    `{"schemaVersion":"aiops.coroot/v1","tool":"coroot.incidents","status":"ok","incidents":[]}`,
			}},
		}},
	}
	current := ToolCall{
		ID:        "call-incidents-limit",
		Name:      "coroot_incidents",
		Arguments: json.RawMessage(`{"limit":10}`),
	}

	result, ok := maybeReuseCoveredReadResult(snapshot, tools, current)
	if !ok {
		t.Fatal("maybeReuseCoveredReadResult() = false, want incidents limit-only reuse")
	}
	if !strings.Contains(result.Result.Content, corootReuseSkipReason) || !strings.Contains(result.Result.Content, "call-incidents") {
		t.Fatalf("reuse content missing skip reason or prior incident call id: %s", result.Result.Content)
	}
}

func TestMaybeReuseCorootIncidentsOpenStatusFromPriorBroadResult(t *testing.T) {
	tools := []promptcompiler.Tool{
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.incidents"}},
	}
	snapshot := &TurnSnapshot{
		ID:        "turn-coroot",
		SessionID: "session-coroot",
		Iterations: []IterationState{{
			ToolCalls: []ToolCall{{
				ID:        "call-incidents",
				Name:      "coroot_incidents",
				Arguments: json.RawMessage(`{}`),
			}},
			ToolResults: []ToolResult{{
				ToolCallID: "call-incidents",
				Content:    `Summary: {"incidents":[{"application":"rabbitmq-server","state":"open"}],"status":"ok"}\nExternal ref: store://tool-spills/spill-turn-coroot-0-call-incidents.`,
				Spilled:    true,
				ExternalReferences: []ExternalReference{{
					ID:    "spill-turn-coroot-0-call-incidents",
					URI:   "store://tool-spills/spill-turn-coroot-0-call-incidents",
					Title: "coroot.incidents",
				}},
			}},
		}},
	}
	current := ToolCall{
		ID:        "call-incidents-open",
		Name:      "coroot_incidents",
		Arguments: json.RawMessage(`{"status":"open"}`),
	}

	result, ok := maybeReuseCoveredReadResult(snapshot, tools, current)
	if !ok {
		t.Fatal("maybeReuseCoveredReadResult() = false, want incidents open-status reuse")
	}
	if !strings.Contains(result.Result.Content, corootReuseSkipReason) || !strings.Contains(result.Result.Content, "call-incidents") {
		t.Fatalf("reuse content missing skip reason or prior incident call id: %s", result.Result.Content)
	}
}

func TestCoveredReadReuseFromSameBatchResult(t *testing.T) {
	tools := []promptcompiler.Tool{
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.list_services"}},
	}
	batch := []ToolCall{
		{ID: "call-list-all", Name: "coroot_list_services", Arguments: json.RawMessage(`{}`)},
		{ID: "call-list-warning", Name: "coroot_list_services", Arguments: json.RawMessage(`{"status":"warning"}`)},
	}

	priorIndex, ok := coveredReadReusePriorIndex(tools, batch[:1], batch[1])
	if !ok || priorIndex != 0 {
		t.Fatalf("coveredReadReusePriorIndex() = %d,%v, want 0,true", priorIndex, ok)
	}
	results := []DispatchResult{
		{
			ToolCallID: "call-list-all",
			Result: tooling.ToolResult{
				ToolCallID: "call-list-all",
				Content:    `{"schemaVersion":"aiops.coroot/v1","tool":"coroot.list_services","status":"ok","statusCounts":{"warning":1},"problemServices":[{"name":"checkout","status":"warning"}]}`,
			},
		},
		{},
	}

	result, ok := coveredReadReuseFromBatchResult(tools, batch, results, coveredReadBatchReuse{index: 1, priorIndex: priorIndex})
	if !ok {
		t.Fatal("coveredReadReuseFromBatchResult() = false, want reuse")
	}
	if result.Outcome != "tool_reused" || !strings.Contains(result.Result.Content, "call-list-all") {
		t.Fatalf("reuse result = %#v", result)
	}
}

func TestBroadenCoveredReadBatchListServicesStatusFilters(t *testing.T) {
	tools := []promptcompiler.Tool{
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.list_services"}},
	}
	batch := []ToolCall{
		{ID: "call-list-warning", Name: "coroot_list_services", Arguments: json.RawMessage(`{"status":"warning"}`)},
		{ID: "call-list-critical", Name: "coroot_list_services", Arguments: json.RawMessage(`{"status":"critical"}`)},
		{ID: "call-incidents", Name: "coroot_incidents", Arguments: json.RawMessage(`{"limit":20}`)},
	}

	got := broadenCoveredReadBatch(tools, batch)
	if string(got[0].Arguments) != `{}` {
		t.Fatalf("first list_services args = %s, want broad args", got[0].Arguments)
	}
	if string(got[1].Arguments) != `{"status":"critical"}` {
		t.Fatalf("second list_services args = %s, want unchanged for reuse decision", got[1].Arguments)
	}
	priorIndex, ok := coveredReadReusePriorIndex(tools, got[:1], got[1])
	if !ok || priorIndex != 0 {
		t.Fatalf("coveredReadReusePriorIndex() after broaden = %d,%v, want 0,true", priorIndex, ok)
	}
}
