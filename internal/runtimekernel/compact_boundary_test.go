package runtimekernel

import (
	"testing"
	"time"
)

func TestCompactBoundaryMarkerMetadataRoundTrip(t *testing.T) {
	createdAt := time.Date(2026, 6, 5, 8, 30, 0, 0, time.UTC)
	msg := NewCompactBoundaryMessage(CompactBoundaryInput{
		SegmentID:          "cmp-turn-1-2",
		CompactedTurnStart: 0,
		CompactedTurnEnd:   7,
		PreservedTailCount: 4,
		CreatedAt:          createdAt,
	})

	if !IsCompactBoundaryMessage(msg) {
		t.Fatalf("IsCompactBoundaryMessage() = false for %#v", msg)
	}
	meta, ok := CompactBoundaryMetadataFromMessage(msg)
	if !ok {
		t.Fatal("CompactBoundaryMetadataFromMessage() ok = false")
	}
	if meta.Type != CompactBoundaryType {
		t.Fatalf("type = %q", meta.Type)
	}
	if meta.SegmentID != "cmp-turn-1-2" {
		t.Fatalf("segmentId = %q", meta.SegmentID)
	}
	if meta.SummarySchemaVersion != CompactSummarySchemaVersionV1 {
		t.Fatalf("summarySchemaVersion = %q", meta.SummarySchemaVersion)
	}
	if meta.CompactedTurnRange.Start != 0 || meta.CompactedTurnRange.End != 7 {
		t.Fatalf("compactedTurnRange = %#v", meta.CompactedTurnRange)
	}
	if meta.PreservedTailCount != 4 {
		t.Fatalf("preservedTailCount = %d", meta.PreservedTailCount)
	}
	if !meta.CreatedAt.Equal(createdAt) {
		t.Fatalf("createdAt = %s", meta.CreatedAt)
	}
}
