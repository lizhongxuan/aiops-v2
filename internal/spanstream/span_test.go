package spanstream

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func newTestRoot() *Span {
	return &Span{
		ID:        "root-1",
		Type:      SpanTypeTurn,
		Status:    SpanStatusRunning,
		Name:      "test turn",
		StartTime: time.Now(),
	}
}

func TestNewSpanTree(t *testing.T) {
	root := newTestRoot()
	tree := NewSpanTree(root)
	if tree.RootSpan != root {
		t.Fatal("expected root span to be set")
	}
}

func TestAddChild(t *testing.T) {
	root := newTestRoot()
	tree := NewSpanTree(root)

	child := &Span{
		ID:        "child-1",
		Type:      SpanTypeToolCall,
		Status:    SpanStatusRunning,
		Name:      "tool call",
		StartTime: time.Now(),
	}

	ok := tree.AddChild("root-1", child)
	if !ok {
		t.Fatal("expected AddChild to succeed")
	}
	if child.ParentID != "root-1" {
		t.Fatalf("expected ParentID to be root-1, got %s", child.ParentID)
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}
}

func TestAddChildNestedMultiLevel(t *testing.T) {
	root := newTestRoot()
	tree := NewSpanTree(root)

	// turn → tool_call → sub_operation (3 levels)
	toolCall := &Span{
		ID:        "tool-1",
		Type:      SpanTypeToolCall,
		Status:    SpanStatusRunning,
		Name:      "host.disk_usage",
		StartTime: time.Now(),
	}
	tree.AddChild("root-1", toolCall)

	subOp := &Span{
		ID:        "sub-1",
		Type:      SpanTypeShellExec,
		Status:    SpanStatusRunning,
		Name:      "df -h",
		StartTime: time.Now(),
	}
	ok := tree.AddChild("tool-1", subOp)
	if !ok {
		t.Fatal("expected nested AddChild to succeed")
	}
	if subOp.ParentID != "tool-1" {
		t.Fatalf("expected ParentID tool-1, got %s", subOp.ParentID)
	}

	// Verify we can find the deeply nested span
	found := tree.FindSpan("sub-1")
	if found == nil {
		t.Fatal("expected to find nested span")
	}
	if found.Name != "df -h" {
		t.Fatalf("expected name 'df -h', got %s", found.Name)
	}
}

func TestAddChildParentNotFound(t *testing.T) {
	root := newTestRoot()
	tree := NewSpanTree(root)

	child := &Span{ID: "orphan", Type: SpanTypeSearch, Status: SpanStatusRunning, Name: "search", StartTime: time.Now()}
	ok := tree.AddChild("nonexistent", child)
	if ok {
		t.Fatal("expected AddChild to fail for nonexistent parent")
	}
}

func TestFindSpan(t *testing.T) {
	root := newTestRoot()
	tree := NewSpanTree(root)

	child := &Span{ID: "find-me", Type: SpanTypeFileRead, Status: SpanStatusRunning, Name: "read file", StartTime: time.Now()}
	tree.AddChild("root-1", child)

	found := tree.FindSpan("find-me")
	if found == nil {
		t.Fatal("expected to find span")
	}
	if found.ID != "find-me" {
		t.Fatalf("expected ID find-me, got %s", found.ID)
	}

	notFound := tree.FindSpan("does-not-exist")
	if notFound != nil {
		t.Fatal("expected nil for nonexistent span")
	}
}

func TestCompleteSpan(t *testing.T) {
	root := newTestRoot()
	tree := NewSpanTree(root)

	child := &Span{ID: "c1", Type: SpanTypeToolCall, Status: SpanStatusRunning, Name: "tool", StartTime: time.Now()}
	tree.AddChild("root-1", child)

	ok := tree.CompleteSpan("c1", "done", "full detail")
	if !ok {
		t.Fatal("expected CompleteSpan to succeed")
	}

	found := tree.FindSpan("c1")
	if found.Status != SpanStatusCompleted {
		t.Fatalf("expected completed status, got %s", found.Status)
	}
	if found.Summary != "done" {
		t.Fatalf("expected summary 'done', got %s", found.Summary)
	}
	if found.Detail != "full detail" {
		t.Fatalf("expected detail 'full detail', got %s", found.Detail)
	}
	if found.EndTime == nil {
		t.Fatal("expected EndTime to be set")
	}
}

func TestCompleteSpanNotFound(t *testing.T) {
	tree := NewSpanTree(newTestRoot())
	ok := tree.CompleteSpan("nope", "s", "d")
	if ok {
		t.Fatal("expected CompleteSpan to fail for nonexistent span")
	}
}

func TestFailSpan(t *testing.T) {
	root := newTestRoot()
	tree := NewSpanTree(root)

	child := &Span{ID: "f1", Type: SpanTypeShellExec, Status: SpanStatusRunning, Name: "cmd", StartTime: time.Now()}
	tree.AddChild("root-1", child)

	ok := tree.FailSpan("f1", "connection timeout")
	if !ok {
		t.Fatal("expected FailSpan to succeed")
	}

	found := tree.FindSpan("f1")
	if found.Status != SpanStatusFailed {
		t.Fatalf("expected failed status, got %s", found.Status)
	}
	if found.Detail != "connection timeout" {
		t.Fatalf("expected detail 'connection timeout', got %s", found.Detail)
	}
	if found.EndTime == nil {
		t.Fatal("expected EndTime to be set")
	}
}

func TestFailSpanNotFound(t *testing.T) {
	tree := NewSpanTree(newTestRoot())
	ok := tree.FailSpan("nope", "err")
	if ok {
		t.Fatal("expected FailSpan to fail for nonexistent span")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	root := &Span{
		ID:        "root-rt",
		Type:      SpanTypeTurn,
		Status:    SpanStatusRunning,
		Name:      "round trip turn",
		StartTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Metadata:  map[string]any{"key": "value"},
	}
	tree := NewSpanTree(root)

	child := &Span{
		ID:        "child-rt",
		Type:      SpanTypeToolCall,
		Status:    SpanStatusCompleted,
		Name:      "tool call",
		Summary:   "executed successfully",
		Detail:    "full output here",
		StartTime: time.Date(2025, 1, 1, 0, 0, 1, 0, time.UTC),
	}
	endTime := time.Date(2025, 1, 1, 0, 0, 2, 0, time.UTC)
	child.EndTime = &endTime
	tree.AddChild("root-rt", child)

	grandchild := &Span{
		ID:        "grandchild-rt",
		Type:      SpanTypeShellExec,
		Status:    SpanStatusFailed,
		Name:      "shell exec",
		Detail:    "permission denied",
		StartTime: time.Date(2025, 1, 1, 0, 0, 1, 500000000, time.UTC),
	}
	gcEnd := time.Date(2025, 1, 1, 0, 0, 1, 800000000, time.UTC)
	grandchild.EndTime = &gcEnd
	tree.AddChild("child-rt", grandchild)

	// Marshal
	data, err := json.Marshal(tree)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Unmarshal into new tree
	var restored SpanTree
	err = json.Unmarshal(data, &restored)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Verify root
	if restored.RootSpan == nil {
		t.Fatal("expected root span after unmarshal")
	}
	if restored.RootSpan.ID != "root-rt" {
		t.Fatalf("expected root ID root-rt, got %s", restored.RootSpan.ID)
	}
	if restored.RootSpan.Type != SpanTypeTurn {
		t.Fatalf("expected type turn, got %s", restored.RootSpan.Type)
	}
	if restored.RootSpan.Metadata["key"] != "value" {
		t.Fatal("expected metadata to be preserved")
	}

	// Verify child
	if len(restored.RootSpan.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(restored.RootSpan.Children))
	}
	restoredChild := restored.RootSpan.Children[0]
	if restoredChild.ID != "child-rt" {
		t.Fatalf("expected child ID child-rt, got %s", restoredChild.ID)
	}
	if restoredChild.Status != SpanStatusCompleted {
		t.Fatalf("expected completed, got %s", restoredChild.Status)
	}
	if restoredChild.Summary != "executed successfully" {
		t.Fatalf("expected summary preserved, got %s", restoredChild.Summary)
	}
	if restoredChild.EndTime == nil {
		t.Fatal("expected EndTime preserved")
	}

	// Verify grandchild (multi-level nesting)
	if len(restoredChild.Children) != 1 {
		t.Fatalf("expected 1 grandchild, got %d", len(restoredChild.Children))
	}
	restoredGC := restoredChild.Children[0]
	if restoredGC.ID != "grandchild-rt" {
		t.Fatalf("expected grandchild ID grandchild-rt, got %s", restoredGC.ID)
	}
	if restoredGC.Status != SpanStatusFailed {
		t.Fatalf("expected failed, got %s", restoredGC.Status)
	}
	if restoredGC.Detail != "permission denied" {
		t.Fatalf("expected detail preserved, got %s", restoredGC.Detail)
	}
}

func TestJSONRoundTripEmptyTree(t *testing.T) {
	root := &Span{
		ID:        "empty-root",
		Type:      SpanTypeTurn,
		Status:    SpanStatusRunning,
		Name:      "empty",
		StartTime: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
	}
	tree := NewSpanTree(root)

	data, err := json.Marshal(tree)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var restored SpanTree
	err = json.Unmarshal(data, &restored)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if restored.RootSpan.ID != "empty-root" {
		t.Fatalf("expected empty-root, got %s", restored.RootSpan.ID)
	}
	if len(restored.RootSpan.Children) != 0 {
		t.Fatalf("expected no children, got %d", len(restored.RootSpan.Children))
	}
}

func TestConcurrentAccess(t *testing.T) {
	root := newTestRoot()
	tree := NewSpanTree(root)

	var wg sync.WaitGroup
	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			child := &Span{
				ID:        "concurrent-" + string(rune('A'+idx%26)) + string(rune('0'+idx/26)),
				Type:      SpanTypeToolCall,
				Status:    SpanStatusRunning,
				Name:      "concurrent op",
				StartTime: time.Now(),
			}
			tree.AddChild("root-1", child)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tree.FindSpan("root-1")
		}()
	}

	wg.Wait()

	// Verify tree is still consistent
	found := tree.FindSpan("root-1")
	if found == nil {
		t.Fatal("root should still be findable after concurrent access")
	}
}

func TestSpanTypes(t *testing.T) {
	types := []SpanType{
		SpanTypeTurn, SpanTypeToolCall, SpanTypeSearch,
		SpanTypeFileRead, SpanTypeShellExec, SpanTypeSummary,
	}
	expected := []string{"turn", "tool_call", "search", "file_read", "shell_exec", "summary"}

	for i, st := range types {
		if string(st) != expected[i] {
			t.Fatalf("expected %s, got %s", expected[i], string(st))
		}
	}
}

func TestSpanStatuses(t *testing.T) {
	statuses := []SpanStatus{SpanStatusRunning, SpanStatusCompleted, SpanStatusFailed}
	expected := []string{"running", "completed", "failed"}

	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Fatalf("expected %s, got %s", expected[i], string(s))
		}
	}
}
