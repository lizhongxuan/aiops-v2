package runtimekernel

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestToolDiscoveryStateApplySelection(t *testing.T) {
	now := time.Unix(100, 0)
	var state ToolDiscoverySessionState
	state.ApplySelection(ToolSelectionDelta{
		LoadedTools: []LoadedToolRef{
			{Name: "synthetic.read", Pack: "synthetic_pack", Fingerprint: "fp-read", Source: "tool_search.select", Reason: "read evidence"},
			{Name: "synthetic.read", Pack: "synthetic_pack", Fingerprint: "fp-read", Source: "tool_search.select", Reason: "duplicate"},
			{Name: "synthetic.search", Pack: "synthetic_pack", Fingerprint: "fp-search", Source: "tool_search.select", Reason: "search evidence"},
		},
		LoadedPacks: []LoadedPackRef{
			{Name: "synthetic_pack", Fingerprint: "fp-pack", Source: "tool_search.select", Reason: "need pack"},
			{Name: "synthetic_pack", Fingerprint: "fp-pack", Source: "tool_search.select", Reason: "duplicate"},
		},
	}, now)

	if got := state.EnabledTools(); !reflect.DeepEqual(got, []string{"synthetic.read", "synthetic.search"}) {
		t.Fatalf("EnabledTools() = %#v", got)
	}
	if got := state.EnabledPacks(); !reflect.DeepEqual(got, []string{"synthetic_pack"}) {
		t.Fatalf("EnabledPacks() = %#v", got)
	}
	if state.UpdatedAt != now {
		t.Fatalf("UpdatedAt = %v, want %v", state.UpdatedAt, now)
	}
}

func TestToolDiscoveryStateRecordsNotLoadedSelectionReasons(t *testing.T) {
	now := time.Unix(120, 0)
	var state ToolDiscoverySessionState
	state.ApplySelection(ToolSelectionDelta{
		LoadedTools: []LoadedToolRef{{Name: "synthetic.ready"}},
		NotLoaded:   []string{"synthetic.unavailable"},
		NotLoadedReasons: map[string]string{
			"synthetic.unavailable": "mcp_unavailable",
		},
		Reason: "need live evidence",
	}, now)

	if state.LastSelection == nil {
		t.Fatal("LastSelection = nil, want selection delta")
	}
	if got := state.LastSelection.NotLoaded; !reflect.DeepEqual(got, []string{"synthetic.unavailable"}) {
		t.Fatalf("LastSelection.NotLoaded = %#v", got)
	}
	if got := state.LastSelection.NotLoadedReasons["synthetic.unavailable"]; got != "mcp_unavailable" {
		t.Fatalf("NotLoadedReasons[synthetic.unavailable] = %q, want mcp_unavailable", got)
	}

	events := toolSelectionTraceEventsFromDiscovery(state)
	if len(events) != 1 {
		t.Fatalf("trace events = %d, want 1", len(events))
	}
	if got := events[0].NotLoaded; !reflect.DeepEqual(got, []string{"synthetic.unavailable"}) {
		t.Fatalf("trace NotLoaded = %#v", got)
	}
	if got := events[0].NotLoadedReasons["synthetic.unavailable"]; got != "mcp_unavailable" {
		t.Fatalf("trace NotLoadedReasons[synthetic.unavailable] = %q, want mcp_unavailable", got)
	}
}

func TestApplyToolSearchDiscoveryStatePersistsNotLoadedReasons(t *testing.T) {
	session := &SessionState{ID: "sess-tool-search-not-loaded"}
	applyToolSearchDiscoveryState(session, "tool_search", ToolResult{
		Content: `{"mode":"select","selection":{"loadedTools":["synthetic.ready"],"notLoaded":["synthetic.unavailable"],"notLoadedReasons":{"synthetic.unavailable":"mcp_unavailable"},"reason":"need live evidence"}}`,
	}, "turn-1")

	if got := session.ToolDiscovery.EnabledTools(); !reflect.DeepEqual(got, []string{"synthetic.ready"}) {
		t.Fatalf("EnabledTools = %#v", got)
	}
	if session.ToolDiscovery.LastSelection == nil {
		t.Fatal("LastSelection = nil, want selection delta")
	}
	if got := session.ToolDiscovery.LastSelection.NotLoadedReasons["synthetic.unavailable"]; got != "mcp_unavailable" {
		t.Fatalf("LastSelection.NotLoadedReasons[synthetic.unavailable] = %q, want mcp_unavailable", got)
	}
}

func TestApplyToolSearchDiscoveryStatePersistsV3SearchSnapshots(t *testing.T) {
	session := &SessionState{ID: "sess-tool-search-v3"}
	applyToolSearchDiscoveryState(session, "tool_search", ToolResult{
		Content: `{
			"mode":"search",
			"ranker":"bm25",
			"request":{
				"mode":"search",
				"query":"service metrics",
				"intent":"rca",
				"sessionType":"host",
				"runtimeMode":"chat",
				"limit":5,
				"mcpHealth":{"coroot":"unavailable"},
				"ranker":"bm25"
			},
			"matches":[{"kind":"tool","name":"local.service_logs","capabilityKind":"read","resourceTypes":["service"],"operationKinds":["read"],"riskLevel":"low","requiresSelect":false}],
			"rejected":[{"name":"coroot.service_metrics","reason":"mcp_unavailable","status":"unavailable","source":"mcp","mcpServerId":"coroot","healthStatus":"unavailable"}]
		}`,
	}, "turn-v3")

	if session.ToolDiscovery.LastSearchRequest == nil {
		t.Fatal("LastSearchRequest = nil, want v3 request snapshot")
	}
	if got := session.ToolDiscovery.LastSearchRequest.Query; got != "service metrics" {
		t.Fatalf("LastSearchRequest.Query = %q, want service metrics", got)
	}
	if got := session.ToolDiscovery.LastSearchRequest.Ranker; got != "bm25" {
		t.Fatalf("LastSearchRequest.Ranker = %q, want bm25", got)
	}
	if got := session.ToolDiscovery.LastSearchRequest.MCPHealth["coroot"]; got != "unavailable" {
		t.Fatalf("LastSearchRequest.MCPHealth[coroot] = %q, want unavailable", got)
	}
	if session.ToolDiscovery.LastSearchResponse == nil {
		t.Fatal("LastSearchResponse = nil, want v3 response snapshot")
	}
	if session.ToolDiscovery.LastSearchResponse.MatchCount != 1 || session.ToolDiscovery.LastSearchResponse.RejectedCount != 1 {
		t.Fatalf("LastSearchResponse = %#v, want match/rejected counts 1/1", session.ToolDiscovery.LastSearchResponse)
	}
	if len(session.ToolDiscovery.LastRejectedSearchResults) != 1 || session.ToolDiscovery.LastRejectedSearchResults[0].Reason != "mcp_unavailable" {
		t.Fatalf("LastRejectedSearchResults = %#v, want one mcp_unavailable rejection", session.ToolDiscovery.LastRejectedSearchResults)
	}

	events := toolSearchTraceEventsFromDiscovery(session.ToolDiscovery)
	if len(events) != 1 {
		t.Fatalf("toolSearchTraceEvents = %d, want 1", len(events))
	}
	if events[0].Query != "service metrics" || events[0].Ranker != "bm25" || events[0].RejectedCount != 1 {
		t.Fatalf("tool search trace event = %#v, want query/ranker/rejected count", events[0])
	}
}

func TestToolDiscoveryStateInvalidatesOnFingerprintChange(t *testing.T) {
	now := time.Unix(100, 0)
	state := ToolDiscoverySessionState{}
	state.ApplySelection(ToolSelectionDelta{
		LoadedTools: []LoadedToolRef{{Name: "synthetic.read", Fingerprint: "old"}},
		LoadedPacks: []LoadedPackRef{{Name: "synthetic_pack", Fingerprint: "old-pack"}},
	}, now)

	report := state.InvalidateMissing(ToolCatalogSnapshot{
		Tools: map[string]string{
			"synthetic.read": "new",
		},
		Packs: map[string]string{},
	}, now.Add(time.Second))

	if len(report.InvalidatedTools) != 1 || report.InvalidatedTools[0] != "synthetic.read" {
		t.Fatalf("InvalidatedTools = %#v", report.InvalidatedTools)
	}
	if len(report.InvalidatedPacks) != 1 || report.InvalidatedPacks[0] != "synthetic_pack" {
		t.Fatalf("InvalidatedPacks = %#v", report.InvalidatedPacks)
	}
	if got := state.EnabledTools(); len(got) != 0 {
		t.Fatalf("EnabledTools after invalidation = %#v, want empty", got)
	}
}

func TestSessionValidateToolDiscoveryState(t *testing.T) {
	valid := SessionState{
		ID:   "sess-valid",
		Type: SessionTypeHost,
		Mode: ModeChat,
		ToolDiscovery: ToolDiscoverySessionState{
			LoadedTools: map[string]LoadedToolRef{
				"synthetic.read": {Name: "synthetic.read"},
			},
			LoadedPacks: map[string]LoadedPackRef{
				"synthetic_pack": {Name: "synthetic_pack"},
			},
			RejectedCalls: []DeferredToolRejectedCall{
				{ToolName: "synthetic.read", ErrorType: "tool_unloaded", Reason: "not loaded"},
			},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid session Validate() error = %v", err)
	}

	invalid := valid
	invalid.ID = "sess-invalid"
	invalid.ToolDiscovery.LoadedTools = map[string]LoadedToolRef{
		"bad": {},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid session Validate() error = nil, want error")
	}
}

func TestRecordRejectedToolCallFromDispatch(t *testing.T) {
	now := time.Unix(200, 0)
	session := &SessionState{ID: "sess-rejected"}
	payload, _ := json.Marshal(map[string]string{
		"errorType":            "tool_unloaded",
		"toolName":             "synthetic.read",
		"reason":               "tool exists in deferred catalog but is not loaded in current tool surface",
		"requiredAction":       "call tool_search with mode=search, then mode=select",
		"suggestedSearchQuery": "read resource",
	})

	recordRejectedToolCallFromDispatch(session, "turn-1", ToolCall{
		ID:   "call-1",
		Name: "synthetic.read",
	}, DispatchResult{Error: string(payload)}, now)

	if len(session.ToolDiscovery.RejectedCalls) != 1 {
		t.Fatalf("RejectedCalls = %d, want 1", len(session.ToolDiscovery.RejectedCalls))
	}
	call := session.ToolDiscovery.RejectedCalls[0]
	if call.ToolName != "synthetic.read" || call.ErrorType != "tool_unloaded" || call.TurnID != "turn-1" || call.ToolCallID != "call-1" {
		t.Fatalf("RejectedCalls[0] = %#v", call)
	}
	if call.SuggestedSearchQuery != "read resource" {
		t.Fatalf("SuggestedSearchQuery = %q, want read resource", call.SuggestedSearchQuery)
	}
	if !call.RejectedAt.Equal(now) {
		t.Fatalf("RejectedAt = %v, want %v", call.RejectedAt, now)
	}

	recordRejectedToolCallFromDispatch(session, "turn-1", ToolCall{
		ID:   "call-2",
		Name: "synthetic.write",
	}, DispatchResult{Error: "plain execution failure"}, now.Add(time.Second))

	if len(session.ToolDiscovery.RejectedCalls) != 1 {
		t.Fatalf("RejectedCalls after plain failure = %d, want 1", len(session.ToolDiscovery.RejectedCalls))
	}
}
