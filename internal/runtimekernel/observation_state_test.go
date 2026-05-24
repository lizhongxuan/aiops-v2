package runtimekernel

import (
	"strings"
	"testing"
	"time"
)

func TestObservationStateUnchangedReturnsStub(t *testing.T) {
	state := NewObservationState()
	record := ObservationRecord{
		Key:            "logs.search|host=host-1|window=10m",
		Digest:         "sha256:abc",
		SourceRef:      "ev-1",
		Summary:        "10 errors from nginx",
		LastObservedAt: time.Unix(100, 0).UTC(),
	}
	state.Upsert(record)
	result := state.Check(ObservationRecord{
		Key:       record.Key,
		Digest:    record.Digest,
		SourceRef: "ev-1",
		Summary:   "10 errors from nginx",
	})
	if !result.Hit {
		t.Fatal("expected dedupe hit")
	}
	if !strings.Contains(result.ModelVisibleContent, "Observation unchanged") {
		t.Fatalf("stub = %q", result.ModelVisibleContent)
	}
	if !strings.Contains(result.ModelVisibleContent, "ev-1") {
		t.Fatalf("stub missing evidence ref: %q", result.ModelVisibleContent)
	}
}

func TestObservationStateDigestChangeReturnsDelta(t *testing.T) {
	state := NewObservationState()
	state.Upsert(ObservationRecord{
		Key:       "metrics.query|host=host-1|window=10m",
		Digest:    "sha256:old",
		SourceRef: "ev-old",
		Summary:   "cpu normal",
	})
	result := state.Check(ObservationRecord{
		Key:       "metrics.query|host=host-1|window=10m",
		Digest:    "sha256:new",
		SourceRef: "ev-new",
		Summary:   "cpu saturated",
	})
	if result.Hit {
		t.Fatal("digest change should not be a hit")
	}
	if !result.Changed {
		t.Fatal("expected changed result")
	}
	if !strings.Contains(result.ModelVisibleContent, "Observation changed") || !strings.Contains(result.ModelVisibleContent, "ev-new") {
		t.Fatalf("delta stub = %q", result.ModelVisibleContent)
	}
}

func TestObservationStateWindowOrSourceVersionChangeMisses(t *testing.T) {
	state := NewObservationState()
	state.Upsert(ObservationRecord{
		ToolName:      "logs.search",
		Target:        "host-1",
		Window:        "10m",
		SourceVersion: "v1",
		Digest:        "sha256:same",
		SourceRef:     "ev-1",
		Summary:       "same summary",
	})
	result := state.Check(ObservationRecord{
		ToolName:      "logs.search",
		Target:        "host-1",
		Window:        "30m",
		SourceVersion: "v1",
		Digest:        "sha256:same",
		SourceRef:     "ev-2",
		Summary:       "same summary",
	})
	if result.Hit {
		t.Fatal("time window change should miss")
	}
	result = state.Check(ObservationRecord{
		ToolName:      "logs.search",
		Target:        "host-1",
		Window:        "10m",
		SourceVersion: "v2",
		Digest:        "sha256:same",
		SourceRef:     "ev-3",
		Summary:       "same summary",
	})
	if result.Hit {
		t.Fatal("source version change should miss")
	}
}
