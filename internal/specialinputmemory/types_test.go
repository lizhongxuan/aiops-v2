package specialinputmemory

import (
	"strings"
	"testing"
	"time"
)

func TestZeroStateNormalizeCloneAndSnapshotAreSafe(t *testing.T) {
	var state SessionSpecialInputState

	normalized := state.Normalize("sess-1", "task-1", time.Unix(100, 0))
	if normalized.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", normalized.SchemaVersion, SchemaVersion)
	}
	if normalized.SessionID != "sess-1" || normalized.TaskID != "task-1" {
		t.Fatalf("normalized ids = %q/%q", normalized.SessionID, normalized.TaskID)
	}

	clone := normalized.Clone()
	clone.Facts = append(clone.Facts, MentionFact{ID: "fact-extra"})
	if len(normalized.Facts) != 0 {
		t.Fatalf("Clone shared facts slice with source state")
	}

	snapshot := normalized.Snapshot()
	if snapshot.SchemaVersion != SchemaVersion {
		t.Fatalf("snapshot schema version = %q, want %q", snapshot.SchemaVersion, SchemaVersion)
	}
}

func TestSnapshotStableOrderingAndRedaction(t *testing.T) {
	now := time.Unix(200, 0)
	state := SessionSpecialInputState{
		SchemaVersion: SchemaVersion,
		SessionID:     "sess-1",
		TaskID:        "task-1",
		Facts: []MentionFact{
			{
				ID:               "fact-b",
				Kind:             FactKindHost,
				CanonicalKey:     "host:b",
				Display:          "host-b",
				Status:           FactStatusActive,
				RedactedPayload:  map[string]string{"token": "secret-value"},
				LastSeenTurnID:   "turn-2",
				FirstSeenTurnID:  "turn-2",
				LastUsedTurnID:   "turn-2",
				ExpiresAt:        now.Add(time.Hour),
				Weight:           1,
				ValidationHash:   "validation-b",
				ValidationSource: "host_repository",
			},
			{
				ID:              "fact-a",
				Kind:            FactKindHost,
				CanonicalKey:    "host:a",
				Display:         "host-a",
				Status:          FactStatusActive,
				RedactedPayload: map[string]string{"safe": "ok"},
				LastSeenTurnID:  "turn-1",
				FirstSeenTurnID: "turn-1",
				ExpiresAt:       now.Add(time.Hour),
				Weight:          1,
			},
		},
		Grants: []ExecutionScopeGrant{
			{ID: "grant-b", CanonicalKey: "host:b", Status: GrantStatusActive, CreatedTurnID: "turn-2"},
			{ID: "grant-a", CanonicalKey: "host:a", Status: GrantStatusActive, CreatedTurnID: "turn-1"},
		},
	}

	snapshot := state.Snapshot()
	if got := []string{snapshot.Facts[0].ID, snapshot.Facts[1].ID}; got[0] != "fact-a" || got[1] != "fact-b" {
		t.Fatalf("facts not sorted by canonical key/id: %#v", got)
	}
	if got := []string{snapshot.Grants[0].ID, snapshot.Grants[1].ID}; got[0] != "grant-a" || got[1] != "grant-b" {
		t.Fatalf("grants not sorted by canonical key/id: %#v", got)
	}
	for _, fact := range snapshot.Facts {
		for key, value := range fact.RedactedPayload {
			if strings.Contains(strings.ToLower(key), "token") || strings.Contains(value, "secret") {
				t.Fatalf("snapshot leaked sensitive payload: key=%q value=%q", key, value)
			}
		}
	}
}
