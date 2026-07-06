package specialinputmemory

import (
	"testing"
	"time"
)

func TestApplyGCExpiresRawTypedCandidateAndRetainsTombstone(t *testing.T) {
	now := time.Unix(1400, 0)
	state := SessionSpecialInputState{
		SchemaVersion: SchemaVersion,
		SessionID:     "sess-1",
		TaskID:        "task-1",
		Facts: []MentionFact{{
			ID:              "fact-raw",
			Kind:            FactKindHost,
			CanonicalKey:    "host:addr:1.1.1.1",
			Status:          FactStatusActive,
			TrustLevel:      TrustLevelRawTyped,
			LastSeenTurnID:  "turn-1",
			FirstSeenTurnID: "turn-1",
			ExpiresAt:       now.Add(-time.Minute),
		}},
		Tombstones: []MemoryTombstone{{
			ID:           "tomb-1",
			CanonicalKey: "host:old",
			Reason:       "correction",
			CreatedAt:    now.Add(-10 * time.Minute),
			ExpiresAt:    now.Add(20 * time.Minute),
		}},
	}

	next, events := ApplyGC(state, GCInput{Now: now, TurnID: "turn-3"})
	if next.Facts[0].Status != FactStatusExpired {
		t.Fatalf("raw fact status = %q, want expired", next.Facts[0].Status)
	}
	if len(next.Tombstones) != 1 {
		t.Fatalf("tombstones len = %d, want retained tombstone", len(next.Tombstones))
	}
	if len(events) == 0 {
		t.Fatalf("expected gc event")
	}
}
