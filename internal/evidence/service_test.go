package evidence

import (
	"context"
	"testing"
	"time"
)

func TestEvidenceServiceRecordGetAndLinkIncident(t *testing.T) {
	svc := NewService(NewInMemoryStore(), fixedClock())

	rec, err := svc.Record(context.Background(), RecordRequest{
		IncidentID:  "inc-redis-1",
		SourceTool:  "coroot.service_metrics",
		Source:      "coroot",
		Kind:        KindMetric,
		Service:     "redis-local-01",
		Environment: "prod",
		TimeRange:   "30m",
		Summary:     "RSS grows faster than used_memory",
		Data:        map[string]any{"rssBytes": 123},
		SessionID:   "sess-1",
		TurnID:      "turn-1",
		ToolCallID:  "call-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Ref == "" {
		t.Fatal("evidence ref is required")
	}
	if rec.CreatedAt.IsZero() {
		t.Fatal("created timestamp is required")
	}

	got, ok := svc.Get(context.Background(), rec.Ref)
	if !ok || got.Summary != rec.Summary {
		t.Fatalf("Get() = %#v, %v", got, ok)
	}
	if got.Data["rssBytes"] != 123 {
		t.Fatalf("record data = %#v, want rssBytes", got.Data)
	}

	if err := svc.LinkIncident(context.Background(), "inc-redis-1", []string{rec.Ref}, RelationSupports); err != nil {
		t.Fatal(err)
	}

	linked := svc.ListIncident(context.Background(), "inc-redis-1")
	if len(linked) != 1 || linked[0].Ref != rec.Ref {
		t.Fatalf("ListIncident() = %#v, want linked evidence", linked)
	}
}

func TestEvidenceServiceRejectsEmptySummary(t *testing.T) {
	svc := NewService(NewInMemoryStore(), fixedClock())

	if _, err := svc.Record(context.Background(), RecordRequest{Kind: KindLog}); err == nil {
		t.Fatal("Record() should reject empty summary")
	}
}

func fixedClock() func() time.Time {
	return func() time.Time {
		return time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)
	}
}
