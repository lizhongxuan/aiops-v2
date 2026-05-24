package opsmanual

import (
	"path/filepath"
	"testing"
)

func TestParamResolutionEventStoreMemoryAndFileRoundTrip(t *testing.T) {
	event := ParamResolutionEvent{
		ID:        "evt-1",
		SessionID: "sess-1",
		TurnID:    "turn-1",
		OpsManualFlowID: "flow-redis-1",
		ManualID:  "manual-redis-rca",
		Result: ParamResolutionResult{
			Status:         ParamResolutionResolved,
			ResolvedParams: []ResolvedParam{{ID: "target_host", Value: "server-local"}},
			Graph: ParamResolutionGraph{Nodes: []ParamResolutionNode{{
				ID:          "target_host",
				ResolverLog: []ParamResolverLog{{Resolver: "selected_host", Status: "hit"}},
			}}},
		},
		CreatedAt: "2026-05-17T00:00:00Z",
	}

	mem := NewMemoryStore()
	if err := mem.SaveParamResolutionEvent(event); err != nil {
		t.Fatalf("memory save: %v", err)
	}
	memEvents, err := mem.ListParamResolutionEvents(ListParamResolutionEventsRequest{SessionID: "sess-1"})
	if err != nil || len(memEvents) != 1 || memEvents[0].Result.ResolvedParams[0].ID != "target_host" {
		t.Fatalf("memory events = %#v, err=%v", memEvents, err)
	}
	flowEvents, err := mem.ListParamResolutionEvents(ListParamResolutionEventsRequest{OpsManualFlowID: "flow-redis-1"})
	if err != nil || len(flowEvents) != 1 || flowEvents[0].SessionID != "sess-1" {
		t.Fatalf("flow events = %#v, err=%v", flowEvents, err)
	}

	path := filepath.Join(t.TempDir(), "library.json")
	fileStore, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileStore.SaveParamResolutionEvent(event); err != nil {
		t.Fatalf("file save: %v", err)
	}
	reloaded, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	fileEvents, err := reloaded.ListParamResolutionEvents(ListParamResolutionEventsRequest{SessionID: "sess-1"})
	if err != nil || len(fileEvents) != 1 {
		t.Fatalf("file events = %#v, err=%v", fileEvents, err)
	}
	if fileEvents[0].Result.Graph.Nodes[0].ResolverLog[0].Resolver != "selected_host" {
		t.Fatalf("file event lost resolver logs: %#v", fileEvents[0])
	}
}
