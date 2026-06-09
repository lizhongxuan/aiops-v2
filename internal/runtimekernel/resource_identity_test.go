package runtimekernel

import (
	"strings"
	"testing"

	"aiops-v2/internal/resourceio"
)

func TestResourceIdentityRepeatedReadReturnsUnchangedStub(t *testing.T) {
	state := NewObservationState()
	first := ResourceReadRecord{
		Identity: ResourceIdentity{
			Scheme:  "store",
			URI:     "store://artifacts/resource-1",
			Version: "v1",
			Digest:  "sha256:same",
			Range:   ResourceRange{Offset: 0, Limit: 20},
		},
		SourceRef: "artifact-1",
		Summary:   "bounded previous summary",
		Content:   "full content should not repeat",
	}
	state.CheckResource(first)

	result := state.CheckResource(first)
	if !result.Unchanged || result.Changed || result.Miss {
		t.Fatalf("result = %#v, want unchanged hit", result)
	}
	if strings.Contains(result.ModelVisibleContent, first.Content) {
		t.Fatalf("unchanged stub repeated full content: %q", result.ModelVisibleContent)
	}
	if !strings.Contains(result.ModelVisibleContent, "Resource unchanged") || !strings.Contains(result.ModelVisibleContent, "artifact-1") {
		t.Fatalf("stub = %q", result.ModelVisibleContent)
	}
	if result.Event.Kind != "resource.dedupe.hit" {
		t.Fatalf("event kind = %q", result.Event.Kind)
	}
}

func TestResourceIdentityDigestChangeReturnsChangedStub(t *testing.T) {
	state := NewObservationState()
	state.CheckResource(ResourceReadRecord{
		Identity: ResourceIdentity{
			Scheme:  "store",
			URI:     "store://artifacts/resource-2",
			Version: "v1",
			Digest:  "sha256:old",
			Range:   ResourceRange{Offset: 0, Limit: 20},
		},
		SourceRef: "artifact-old",
		Summary:   "old summary",
	})

	result := state.CheckResource(ResourceReadRecord{
		Identity: ResourceIdentity{
			Scheme:  "store",
			URI:     "store://artifacts/resource-2",
			Version: "v1",
			Digest:  "sha256:new",
			Range:   ResourceRange{Offset: 0, Limit: 20},
		},
		SourceRef: "artifact-new",
		Summary:   "new summary",
		Content:   "new full content should not be injected in changed stub",
	})
	if !result.Changed || result.Unchanged || result.Miss {
		t.Fatalf("result = %#v, want changed", result)
	}
	if !strings.Contains(result.ModelVisibleContent, "Resource changed") || !strings.Contains(result.ModelVisibleContent, "artifact-new") {
		t.Fatalf("changed stub = %q", result.ModelVisibleContent)
	}
	if strings.Contains(result.ModelVisibleContent, "new full content") {
		t.Fatalf("changed stub repeated full content: %q", result.ModelVisibleContent)
	}
	if result.Event.Kind != "resource.dedupe.changed" {
		t.Fatalf("event kind = %q", result.Event.Kind)
	}
}

func TestResourceIdentitySameDigestDifferentVersionReturnsUnchanged(t *testing.T) {
	state := NewObservationState()
	state.CheckResource(ResourceReadRecord{
		Identity: ResourceIdentity{
			URI:     "resource://generic",
			Version: "v1",
			Digest:  "sha256:same",
			Range:   ResourceRange{Offset: 0, Limit: 20},
		},
		SourceRef: "ref-v1",
		Summary:   "same content",
		Content:   "full content should not repeat",
	})

	result := state.CheckResource(ResourceReadRecord{
		Identity: ResourceIdentity{
			URI:     "resource://generic",
			Version: "v2",
			Digest:  "sha256:same",
			Range:   ResourceRange{Offset: 0, Limit: 20},
		},
		SourceRef: "ref-v2",
		Summary:   "same content new version",
		Content:   "full content should not repeat",
	})
	if !result.Unchanged || result.Miss || result.Changed {
		t.Fatalf("result = %#v, want unchanged for same digest despite version change", result)
	}
	if strings.Contains(result.ModelVisibleContent, "full content should not repeat") {
		t.Fatalf("unchanged result repeated content: %q", result.ModelVisibleContent)
	}
}

func TestResourceIdentityRangeChangeMisses(t *testing.T) {
	state := NewObservationState()
	state.CheckResource(ResourceReadRecord{
		Identity: ResourceIdentity{
			Scheme:  "store",
			URI:     "store://artifacts/resource-3",
			Version: "v1",
			Digest:  "sha256:same",
			Range:   ResourceRange{Offset: 0, Limit: 20},
		},
		SourceRef: "artifact-a",
		Summary:   "first range",
		Content:   "first range content",
	})

	result := state.CheckResource(ResourceReadRecord{
		Identity: ResourceIdentity{
			Scheme:  "store",
			URI:     "store://artifacts/resource-3",
			Version: "v1",
			Digest:  "sha256:same",
			Range:   ResourceRange{Offset: 20, Limit: 20},
		},
		SourceRef: "artifact-b",
		Summary:   "second range",
		Content:   "second range content",
	})
	if !result.Miss || result.Unchanged || result.Changed {
		t.Fatalf("result = %#v, want miss for new range", result)
	}
	if result.ModelVisibleContent != "second range content" {
		t.Fatalf("model content = %q", result.ModelVisibleContent)
	}
	if result.Event.Kind != "resource.dedupe.miss" {
		t.Fatalf("event kind = %q", result.Event.Kind)
	}
}

func TestResourceIdentityDifferentRangesDoNotDedupe(t *testing.T) {
	state := NewObservationState()
	state.CheckResource(ResourceReadRecord{
		Identity: ResourceIdentity{
			URI:    "resource://generic",
			Digest: "sha256:same",
			Range:  ResourceRange{Offset: 0, Limit: 20},
		},
		SourceRef: "ref-a",
		Summary:   "first range",
		Content:   "first range content",
	})

	result := state.CheckResource(ResourceReadRecord{
		Identity: ResourceIdentity{
			URI:    "resource://generic",
			Digest: "sha256:same",
			Range:  ResourceRange{Offset: 20, Limit: 20},
		},
		SourceRef: "ref-b",
		Summary:   "second range",
		Content:   "second range content",
	})
	if !result.Miss || result.Unchanged {
		t.Fatalf("result = %#v, want miss for distinct range", result)
	}
}

func TestModelVisibleMessagesDedupeUsesExternalReferenceRange(t *testing.T) {
	session := &SessionState{ID: "sess-1"}
	history := []Message{
		resourceToolMessage("msg-1", "resource://generic", "sha256:same", resourceio.Range{Offset: 0, Limit: 20}, "first range content should be visible"),
		resourceToolMessage("msg-2", "resource://generic", "sha256:same", resourceio.Range{Offset: 20, Limit: 20}, "second range content should be visible"),
	}

	out, events := modelVisibleMessagesWithObservationDedupe(session, history)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if out[1].ToolResult == nil {
		t.Fatal("missing second tool result")
	}
	if strings.Contains(out[1].ToolResult.Content, "Resource unchanged") {
		t.Fatalf("second range was incorrectly deduped: %q", out[1].ToolResult.Content)
	}
	if !strings.Contains(out[1].ToolResult.Content, "second range content") {
		t.Fatalf("second range content = %q, want visible miss content", out[1].ToolResult.Content)
	}
}

func TestModelVisibleMessagesDedupeReportsDigestChange(t *testing.T) {
	session := &SessionState{ID: "sess-1"}
	history := []Message{
		resourceToolMessage("msg-1", "resource://generic", "sha256:old", resourceio.Range{Offset: 0, Limit: 20}, "old full content"),
		resourceToolMessage("msg-2", "resource://generic", "sha256:new", resourceio.Range{Offset: 0, Limit: 20}, "new full content should not repeat"),
	}

	out, events := modelVisibleMessagesWithObservationDedupe(session, history)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if out[1].ToolResult == nil {
		t.Fatal("missing second tool result")
	}
	content := out[1].ToolResult.Content
	if !strings.Contains(content, "Resource changed") {
		t.Fatalf("content = %q, want changed stub", content)
	}
	if strings.Contains(content, "new full content should not repeat") {
		t.Fatalf("changed stub repeated full content: %q", content)
	}
}

func TestModelVisibleMessagesWithMultipleExternalReferencesDoesNotDedupeFirstOnly(t *testing.T) {
	session := &SessionState{ID: "sess-1"}
	session.ObservationState.CheckResource(ResourceReadRecord{
		Identity: ResourceIdentity{
			URI:    "resource://first",
			Digest: "sha256:same",
			Range:  ResourceRange{Offset: 0, Limit: 20},
		},
		SourceRef: "first-ref",
		Summary:   "first content",
		Content:   "first content",
	})
	msg := resourceToolMessage("msg-1", "resource://first", "sha256:same", resourceio.Range{Offset: 0, Limit: 20}, "first content")
	msg.ToolResult.ExternalReferences = append(msg.ToolResult.ExternalReferences, ExternalReference{
		ID:          "second-ref",
		SessionID:   "sess-1",
		TurnID:      "turn-1",
		Iteration:   1,
		Kind:        string(ToolResultReferenceKindMCPResource),
		URI:         "resource://second",
		ContentType: "text/plain",
		Digest:      "sha256:new",
		Version:     "sha256:new",
		Bytes:       128,
		Range:       resourceio.Range{Offset: 20, Limit: 20},
	})
	msg.ToolResult.Content = "combined content with second resource"
	msg.Content = msg.ToolResult.Content

	out, events := modelVisibleMessagesWithObservationDedupe(session, []Message{msg})
	if len(events) != 0 {
		t.Fatalf("events = %d, want no single-resource dedupe for multi-reference result", len(events))
	}
	if out[0].ToolResult == nil || out[0].ToolResult.Content != "combined content with second resource" {
		t.Fatalf("tool result content = %#v", out[0].ToolResult)
	}
}

func TestModelInputTraceIncludesResourceDedupeRange(t *testing.T) {
	session := &SessionState{ID: "sess-1"}
	history := []Message{
		resourceToolMessage("msg-1", "resource://generic", "sha256:same", resourceio.Range{Offset: 6, Limit: 4}, "full content should not repeat"),
		resourceToolMessage("msg-2", "resource://generic", "sha256:same", resourceio.Range{Offset: 6, Limit: 4}, "full content should not repeat"),
	}

	_, events := modelVisibleMessagesWithObservationDedupe(session, history)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	traceItems := promptInputContextGovernanceFromRuntime(events)
	if len(traceItems) != 2 {
		t.Fatalf("trace items = %d, want 2", len(traceItems))
	}
	item := traceItems[1]
	if item.Kind != "resource.dedupe.hit" {
		t.Fatalf("kind = %q, want resource.dedupe.hit", item.Kind)
	}
	if item.Resource == nil {
		t.Fatalf("trace resource metadata is nil: %#v", item)
	}
	if item.Resource.Range.Offset != 6 || item.Resource.Range.Limit != 4 {
		t.Fatalf("resource range = %#v, want offset 6 limit 4", item.Resource.Range)
	}
	if item.Resource.URI != "resource://generic" || item.Resource.Digest != "sha256:same" {
		t.Fatalf("resource metadata = %#v", item.Resource)
	}
	if item.Resource.ContentType != "text/plain" || item.Resource.Bytes != 128 {
		t.Fatalf("resource metadata = %#v, want content type and bytes", item.Resource)
	}
}

func TestResourceIdentityWithoutDigestDoesNotReportUnchanged(t *testing.T) {
	state := NewObservationState()
	record := ResourceReadRecord{
		Identity: ResourceIdentity{
			Scheme: "store",
			URI:    "store://artifacts/resource-4",
			Range:  ResourceRange{Offset: 0, Limit: 20},
		},
		SourceRef: "artifact-no-digest",
		Summary:   "summary only",
		Content:   strings.Repeat("x", 200),
	}
	state.CheckResource(record)
	result := state.CheckResource(record)
	if !result.Miss || result.Unchanged || result.Changed {
		t.Fatalf("result = %#v, want conservative miss without digest", result)
	}
	if len(result.ModelVisibleContent) > 140 {
		t.Fatalf("model content was not bounded: len=%d", len(result.ModelVisibleContent))
	}
}

func resourceToolMessage(id, uri, digest string, rng resourceio.Range, content string) Message {
	return Message{
		ID:           id,
		ClientTurnID: "turn-1",
		Role:         "tool",
		Content:      content,
		ToolResult: &ToolResult{
			ToolCallID: id + "-call",
			Content:    content,
			ExternalReferences: []ExternalReference{{
				ID:          id + "-ref",
				SessionID:   "sess-1",
				TurnID:      "turn-1",
				Iteration:   1,
				Kind:        string(ToolResultReferenceKindMCPResource),
				URI:         uri,
				ContentType: "text/plain",
				Digest:      digest,
				Version:     digest,
				Bytes:       128,
				Range:       rng,
			}},
		},
	}
}
