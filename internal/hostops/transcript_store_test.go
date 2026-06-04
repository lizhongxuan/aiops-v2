package hostops

import (
	"context"
	"testing"
)

func TestTranscriptStoreAppendsOrderedItems(t *testing.T) {
	store := NewInMemoryTranscriptStore()
	err := store.Append(context.Background(), "agent-1", TranscriptItem{Type: TranscriptItemManagerMessage, Content: "检查PG版本"})
	if err != nil {
		t.Fatalf("Append(manager) error = %v", err)
	}
	err = store.Append(context.Background(), "agent-1", TranscriptItem{Type: TranscriptItemAssistantMessage, Content: "PostgreSQL 15"})
	if err != nil {
		t.Fatalf("Append(assistant) error = %v", err)
	}
	items, err := store.List(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 2 || items[0].Type != TranscriptItemManagerMessage || items[1].Type != TranscriptItemAssistantMessage {
		t.Fatalf("items = %#v, want manager then assistant", items)
	}
	if items[0].ID == "" || items[0].CreatedAt.IsZero() {
		t.Fatalf("items[0] missing generated ID/CreatedAt: %#v", items[0])
	}
}

func TestTranscriptStoreCopiesItems(t *testing.T) {
	store := NewInMemoryTranscriptStore()
	item := TranscriptItem{ID: "item-1", Type: TranscriptItemManagerMessage, Content: "检查PG版本"}
	if err := store.Append(context.Background(), "agent-1", item); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	item.Content = "mutated"
	items, err := store.List(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if items[0].Content != "检查PG版本" {
		t.Fatalf("items[0].Content = %q, want original", items[0].Content)
	}
}
