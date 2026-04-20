package spanstream

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

func newTestStream() (*MultiplexedStream, *SpanTree) {
	root := &Span{
		ID:        "root-1",
		Type:      SpanTypeTurn,
		Status:    SpanStatusRunning,
		Name:      "test turn",
		StartTime: time.Now(),
	}
	tree := NewSpanTree(root)
	ms := NewMultiplexedStream(tree, 64)
	return ms, tree
}

func TestNewMultiplexedStream(t *testing.T) {
	ms, tree := newTestStream()
	if ms == nil {
		t.Fatal("expected non-nil stream")
	}
	if ms.tree != tree {
		t.Fatal("expected tree to be set")
	}
	if ms.chunks == nil {
		t.Fatal("expected chunks channel to be initialized")
	}
}

func TestNewMultiplexedStreamDefaultBuffer(t *testing.T) {
	root := &Span{ID: "r", Type: SpanTypeTurn, Status: SpanStatusRunning, Name: "t", StartTime: time.Now()}
	tree := NewSpanTree(root)
	ms := NewMultiplexedStream(tree, 0)
	if cap(ms.chunks) != 256 {
		t.Fatalf("expected default buffer 256, got %d", cap(ms.chunks))
	}
}

func TestEmitText(t *testing.T) {
	ms, _ := newTestStream()

	ms.EmitText("Hello, world!")

	select {
	case chunk := <-ms.Chunks():
		if chunk.Type != ChunkTypeText {
			t.Fatalf("expected text type, got %s", chunk.Type)
		}
		var text string
		if err := json.Unmarshal(chunk.Data, &text); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if text != "Hello, world!" {
			t.Fatalf("expected 'Hello, world!', got %s", text)
		}
		if chunk.Timestamp.IsZero() {
			t.Fatal("expected non-zero timestamp")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for chunk")
	}
}

func TestEmitTextOrdering(t *testing.T) {
	ms, _ := newTestStream()

	texts := []string{"first", "second", "third", "fourth"}
	for _, txt := range texts {
		ms.EmitText(txt)
	}

	for i, expected := range texts {
		select {
		case chunk := <-ms.Chunks():
			var got string
			json.Unmarshal(chunk.Data, &got)
			if got != expected {
				t.Fatalf("chunk %d: expected %q, got %q", i, expected, got)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for chunk %d", i)
		}
	}
}

func TestStartSpan(t *testing.T) {
	ms, tree := newTestStream()

	spanID := ms.StartSpan("root-1", SpanTypeToolCall, "host.disk_usage")

	if spanID == "" {
		t.Fatal("expected non-empty span ID")
	}

	// Verify span is in the map
	val, ok := ms.spans.Load(spanID)
	if !ok {
		t.Fatal("expected span to be stored in map")
	}
	span := val.(*Span)
	if span.Type != SpanTypeToolCall {
		t.Fatalf("expected tool_call type, got %s", span.Type)
	}
	if span.Name != "host.disk_usage" {
		t.Fatalf("expected name host.disk_usage, got %s", span.Name)
	}
	if span.Status != SpanStatusRunning {
		t.Fatalf("expected running status, got %s", span.Status)
	}
	if span.ParentID != "root-1" {
		t.Fatalf("expected parentID root-1, got %s", span.ParentID)
	}

	// Verify span is in the tree
	found := tree.FindSpan(spanID)
	if found == nil {
		t.Fatal("expected span to be in tree")
	}

	// Verify event was emitted
	select {
	case chunk := <-ms.Chunks():
		if chunk.Type != ChunkTypeSpanStart {
			t.Fatalf("expected span_start, got %s", chunk.Type)
		}
		if chunk.SpanID != spanID {
			t.Fatalf("expected spanID %s, got %s", spanID, chunk.SpanID)
		}
		if chunk.ParentID != "root-1" {
			t.Fatalf("expected parentID root-1, got %s", chunk.ParentID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for span_start chunk")
	}
}

func TestStreamCompleteSpan(t *testing.T) {
	ms, tree := newTestStream()

	spanID := ms.StartSpan("root-1", SpanTypeToolCall, "host.log_tail")
	<-ms.Chunks() // drain start event

	ms.CompleteSpan(spanID, "logs retrieved", "full log output here")

	// Verify span status in map
	val, _ := ms.spans.Load(spanID)
	span := val.(*Span)
	if span.Status != SpanStatusCompleted {
		t.Fatalf("expected completed, got %s", span.Status)
	}
	if span.Summary != "logs retrieved" {
		t.Fatalf("expected summary 'logs retrieved', got %s", span.Summary)
	}
	if span.EndTime == nil {
		t.Fatal("expected EndTime to be set")
	}

	// Verify tree is updated
	found := tree.FindSpan(spanID)
	if found.Status != SpanStatusCompleted {
		t.Fatalf("tree span expected completed, got %s", found.Status)
	}

	// Verify event
	select {
	case chunk := <-ms.Chunks():
		if chunk.Type != ChunkTypeSpanComplete {
			t.Fatalf("expected span_complete, got %s", chunk.Type)
		}
		if chunk.SpanID != spanID {
			t.Fatalf("expected spanID %s, got %s", spanID, chunk.SpanID)
		}
		var data struct {
			Summary string `json:"summary"`
			Detail  string `json:"detail"`
		}
		json.Unmarshal(chunk.Data, &data)
		if data.Summary != "logs retrieved" {
			t.Fatalf("expected summary in data, got %s", data.Summary)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for span_complete chunk")
	}
}

func TestStreamFailSpan(t *testing.T) {
	ms, tree := newTestStream()

	spanID := ms.StartSpan("root-1", SpanTypeShellExec, "rm -rf /tmp/old")
	<-ms.Chunks() // drain start event

	ms.FailSpan(spanID, errors.New("permission denied"))

	// Verify span status in map
	val, _ := ms.spans.Load(spanID)
	span := val.(*Span)
	if span.Status != SpanStatusFailed {
		t.Fatalf("expected failed, got %s", span.Status)
	}
	if span.Detail != "permission denied" {
		t.Fatalf("expected detail 'permission denied', got %s", span.Detail)
	}
	if span.EndTime == nil {
		t.Fatal("expected EndTime to be set")
	}

	// Verify tree is updated
	found := tree.FindSpan(spanID)
	if found.Status != SpanStatusFailed {
		t.Fatalf("tree span expected failed, got %s", found.Status)
	}

	// Verify event
	select {
	case chunk := <-ms.Chunks():
		if chunk.Type != ChunkTypeSpanComplete {
			t.Fatalf("expected span_complete for failure, got %s", chunk.Type)
		}
		var data struct {
			Error  string `json:"error"`
			Status string `json:"status"`
		}
		json.Unmarshal(chunk.Data, &data)
		if data.Error != "permission denied" {
			t.Fatalf("expected error in data, got %s", data.Error)
		}
		if data.Status != "failed" {
			t.Fatalf("expected status failed in data, got %s", data.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for fail chunk")
	}
}

func TestFailSpanNilError(t *testing.T) {
	ms, _ := newTestStream()

	spanID := ms.StartSpan("root-1", SpanTypeToolCall, "op")
	<-ms.Chunks()

	ms.FailSpan(spanID, nil)

	val, _ := ms.spans.Load(spanID)
	span := val.(*Span)
	if span.Status != SpanStatusFailed {
		t.Fatalf("expected failed, got %s", span.Status)
	}
	if span.Detail != "" {
		t.Fatalf("expected empty detail for nil error, got %s", span.Detail)
	}
}

func TestEventOrderSpanStartBeforeComplete(t *testing.T) {
	ms, _ := newTestStream()

	spanID := ms.StartSpan("root-1", SpanTypeSearch, "web search")
	ms.CompleteSpan(spanID, "found results", "3 results")

	// First event should be span_start
	chunk1 := <-ms.Chunks()
	if chunk1.Type != ChunkTypeSpanStart {
		t.Fatalf("expected first event to be span_start, got %s", chunk1.Type)
	}

	// Second event should be span_complete
	chunk2 := <-ms.Chunks()
	if chunk2.Type != ChunkTypeSpanComplete {
		t.Fatalf("expected second event to be span_complete, got %s", chunk2.Type)
	}

	// Timestamps should be ordered
	if chunk2.Timestamp.Before(chunk1.Timestamp) {
		t.Fatal("expected complete timestamp to be after start timestamp")
	}
}

func TestEmitProgress(t *testing.T) {
	ms, _ := newTestStream()

	spanID := ms.StartSpan("root-1", SpanTypeToolCall, "download")
	<-ms.Chunks()

	progress, _ := json.Marshal(map[string]any{"percent": 50})
	ms.EmitProgress(spanID, progress)

	select {
	case chunk := <-ms.Chunks():
		if chunk.Type != ChunkTypeSpanProgress {
			t.Fatalf("expected span_progress, got %s", chunk.Type)
		}
		if chunk.SpanID != spanID {
			t.Fatalf("expected spanID %s, got %s", spanID, chunk.SpanID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestEmitStatus(t *testing.T) {
	ms, _ := newTestStream()

	ms.EmitStatus("thinking")

	select {
	case chunk := <-ms.Chunks():
		if chunk.Type != ChunkTypeStatus {
			t.Fatalf("expected status, got %s", chunk.Type)
		}
		var status string
		json.Unmarshal(chunk.Data, &status)
		if status != "thinking" {
			t.Fatalf("expected 'thinking', got %s", status)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestEmitSummary(t *testing.T) {
	ms, _ := newTestStream()

	spanID := ms.StartSpan("root-1", SpanTypeToolCall, "analyze")
	<-ms.Chunks()

	ms.EmitSummary(spanID, "Found 3 OOM errors in syslog")

	select {
	case chunk := <-ms.Chunks():
		if chunk.Type != ChunkTypeSummary {
			t.Fatalf("expected summary, got %s", chunk.Type)
		}
		if chunk.SpanID != spanID {
			t.Fatalf("expected spanID %s, got %s", spanID, chunk.SpanID)
		}
		var data struct {
			Summary string `json:"summary"`
		}
		json.Unmarshal(chunk.Data, &data)
		if data.Summary != "Found 3 OOM errors in syslog" {
			t.Fatalf("expected summary text, got %s", data.Summary)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestTypedEventChunkSerialization(t *testing.T) {
	chunk := TypedEventChunk{
		Type:      ChunkTypeSpanStart,
		SpanID:    "span-123",
		ParentID:  "root-1",
		Data:      json.RawMessage(`{"spanType":"tool_call","name":"test"}`),
		Timestamp: time.Date(2025, 7, 1, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(chunk)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var restored TypedEventChunk
	err = json.Unmarshal(data, &restored)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if restored.Type != ChunkTypeSpanStart {
		t.Fatalf("expected span_start, got %s", restored.Type)
	}
	if restored.SpanID != "span-123" {
		t.Fatalf("expected span-123, got %s", restored.SpanID)
	}
	if restored.ParentID != "root-1" {
		t.Fatalf("expected root-1, got %s", restored.ParentID)
	}
	if !restored.Timestamp.Equal(chunk.Timestamp) {
		t.Fatal("timestamp mismatch")
	}
}

func TestConcurrentEmit(t *testing.T) {
	ms, _ := newTestStream()

	var wg sync.WaitGroup
	numGoroutines := 20

	// Concurrent text emissions
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ms.EmitText("concurrent text")
		}(i)
	}

	// Concurrent span operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			spanID := ms.StartSpan("root-1", SpanTypeToolCall, "concurrent op")
			ms.CompleteSpan(spanID, "done", "detail")
		}(i)
	}

	wg.Wait()

	// Drain and count events
	close(ms.chunks)
	count := 0
	for range ms.chunks {
		count++
	}

	// We expect at least some events (exact count depends on channel capacity)
	if count == 0 {
		t.Fatal("expected some events to be emitted")
	}
}

func TestMultipleSpansUniqueIDs(t *testing.T) {
	ms, _ := newTestStream()

	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := ms.StartSpan("root-1", SpanTypeToolCall, "op")
		if ids[id] {
			t.Fatalf("duplicate span ID: %s", id)
		}
		ids[id] = true
	}
}

func TestCompleteSpanUnknownID(t *testing.T) {
	ms, _ := newTestStream()

	// Should not panic on unknown span ID
	ms.CompleteSpan("nonexistent", "summary", "detail")

	select {
	case chunk := <-ms.Chunks():
		// Event is still emitted even for unknown spans
		if chunk.Type != ChunkTypeSpanComplete {
			t.Fatalf("expected span_complete, got %s", chunk.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestFailSpanUnknownID(t *testing.T) {
	ms, _ := newTestStream()

	// Should not panic on unknown span ID
	ms.FailSpan("nonexistent", errors.New("some error"))

	select {
	case chunk := <-ms.Chunks():
		if chunk.Type != ChunkTypeSpanComplete {
			t.Fatalf("expected span_complete, got %s", chunk.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestEventChunkTypes(t *testing.T) {
	types := []EventChunkType{
		ChunkTypeText, ChunkTypeSpanStart, ChunkTypeSpanProgress,
		ChunkTypeSpanComplete, ChunkTypeStatus, ChunkTypeSummary,
	}
	expected := []string{"text", "span_start", "span_progress", "span_complete", "status", "summary"}

	for i, ct := range types {
		if string(ct) != expected[i] {
			t.Fatalf("expected %s, got %s", expected[i], string(ct))
		}
	}
}
