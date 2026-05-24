package opsmanual

import (
	"context"
	"testing"
	"time"
)

func TestSessionOpsContextStoreUpsertsPrunesAndOrdersFacts(t *testing.T) {
	store := NewMemorySessionOpsContextStore()
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)

	for _, fact := range []SessionOpsFact{
		{
			Key:        SessionOpsFactTargetHost,
			Value:      "server-a",
			Source:     "read_only_discovery",
			Confidence: 0.7,
			ExpiresAt:  now.Add(15 * time.Minute),
		},
		{
			Key:             SessionOpsFactTargetHost,
			Value:           "server-a",
			Source:          "user_form",
			Confidence:      1,
			ConfirmedByUser: true,
			ExpiresAt:       now.Add(2 * time.Hour),
		},
		{
			Key:        SessionOpsFactTargetInstance,
			Value:      "redis-old",
			Source:     "read_only_discovery",
			Confidence: 0.9,
			ExpiresAt:  now.Add(-time.Minute),
		},
		{
			Key:        SessionOpsFactTargetInstance,
			Value:      "redis-candidate",
			Source:     "read_only_discovery",
			Confidence: 0.9,
			ExpiresAt:  now.Add(15 * time.Minute),
		},
	} {
		if err := store.UpsertFact(context.Background(), "sess-1", fact); err != nil {
			t.Fatalf("UpsertFact() error = %v", err)
		}
	}

	facts, err := store.ListFacts(context.Background(), "sess-1", SessionOpsFactFilter{Now: now})
	if err != nil {
		t.Fatalf("ListFacts() error = %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("facts = %#v, want 2 non-expired de-duplicated facts", facts)
	}
	if facts[0].Key != SessionOpsFactTargetHost || !facts[0].ConfirmedByUser || facts[0].Source != "user_form" {
		t.Fatalf("first fact = %#v, want confirmed user_form target_host first", facts[0])
	}
	if facts[1].Key != SessionOpsFactTargetInstance || facts[1].Value != "redis-candidate" {
		t.Fatalf("second fact = %#v, want current target_instance candidate", facts[1])
	}

	if err := store.PruneExpired(context.Background(), now); err != nil {
		t.Fatalf("PruneExpired() error = %v", err)
	}
	all, err := store.ListFacts(context.Background(), "sess-1", SessionOpsFactFilter{})
	if err != nil {
		t.Fatalf("ListFacts() after prune error = %v", err)
	}
	for _, fact := range all {
		if fact.Value == "redis-old" {
			t.Fatalf("expired fact was not pruned: %#v", all)
		}
	}
}

func TestSessionOpsContextStoreDoesNotReturnSensitivePlaintext(t *testing.T) {
	store := NewMemorySessionOpsContextStore()
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	err := store.UpsertFact(context.Background(), "sess-1", SessionOpsFact{
		Key:             "db_password",
		Value:           "raw-password",
		Source:          "user_form",
		Confidence:      1,
		ConfirmedByUser: true,
		Sensitive:       true,
		SecretRef:       "secret://pg/password",
		ExpiresAt:       now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("UpsertFact() error = %v", err)
	}
	facts, err := store.ListFacts(context.Background(), "sess-1", SessionOpsFactFilter{Now: now})
	if err != nil {
		t.Fatalf("ListFacts() error = %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("facts = %#v, want one", facts)
	}
	if facts[0].Value == "raw-password" {
		t.Fatalf("sensitive plaintext returned: %#v", facts[0])
	}
	if facts[0].SecretRef != "secret://pg/password" {
		t.Fatalf("secret ref = %q, want preserved SecretRef", facts[0].SecretRef)
	}
}

func TestOpsManualSuppressionFactMatchesScope(t *testing.T) {
	fact := NewOpsManualSuppressionFact(OpsManualSuppression{
		ManualID:    "manual-redis-rca",
		ObjectType:  "redis",
		Action:      "rca_or_repair",
		TargetScope: "host:server-a/container:redis-a",
		Reason:      "user_opt_out",
	}, time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC))
	if fact.Key != SessionOpsFactOpsManualSuppression || fact.Source != "user_opt_out" || !fact.ConfirmedByUser {
		t.Fatalf("suppression fact = %#v, want confirmed user opt-out fact", fact)
	}
	suppression, ok := OpsManualSuppressionFromFact(fact)
	if !ok {
		t.Fatalf("failed to decode suppression fact: %#v", fact)
	}
	if !suppression.Matches(OpsManualSuppression{
		ManualID:    "manual-redis-rca",
		ObjectType:  "redis",
		Action:      "rca_or_repair",
		TargetScope: "host:server-a/container:redis-a",
	}) {
		t.Fatalf("suppression did not match same scope: %#v", suppression)
	}
	if suppression.Matches(OpsManualSuppression{
		ManualID:    "manual-redis-rca",
		ObjectType:  "redis",
		Action:      "status_check",
		TargetScope: "host:server-a/container:redis-a",
	}) {
		t.Fatalf("suppression matched different action: %#v", suppression)
	}
}
