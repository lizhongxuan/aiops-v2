package spanstream

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// mockSummaryModel implements model.ChatModel for testing the compressor.
type mockSummaryModel struct {
	response string
	err      error
	delay    time.Duration
	calls    atomic.Int32
}

func (m *mockSummaryModel) Generate(_ context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.calls.Add(1)
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	if m.err != nil {
		return nil, m.err
	}
	return &schema.Message{Role: schema.Assistant, Content: m.response}, nil
}

func (m *mockSummaryModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *mockSummaryModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

type capturingSummaryModel struct {
	response string
	messages []*schema.Message
}

func (m *capturingSummaryModel) Generate(_ context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.messages = cloneCompressorSchemaMessages(msgs)
	return &schema.Message{Role: schema.Assistant, Content: m.response}, nil
}

func (m *capturingSummaryModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *capturingSummaryModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

func cloneCompressorSchemaMessages(messages []*schema.Message) []*schema.Message {
	out := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		cp := *msg
		out = append(out, &cp)
	}
	return out
}

func TestNewContextCompressor(t *testing.T) {
	mock := &mockSummaryModel{response: "test"}
	cc := NewContextCompressor(mock, 4)
	if cc == nil {
		t.Fatal("expected non-nil compressor")
	}
	if cc.summaryModel != mock {
		t.Fatal("expected model to be set")
	}
	if cap(cc.workerPool) != 4 {
		t.Fatalf("expected pool capacity 4, got %d", cap(cc.workerPool))
	}
}

func TestNewContextCompressorDefaultConcurrency(t *testing.T) {
	mock := &mockSummaryModel{response: "test"}
	cc := NewContextCompressor(mock, 0)
	if cap(cc.workerPool) != 4 {
		t.Fatalf("expected default pool capacity 4, got %d", cap(cc.workerPool))
	}
}

func TestCompressAsyncSuccess(t *testing.T) {
	mock := &mockSummaryModel{response: "Found 3 OOM errors in /var/log/syslog"}
	cc := NewContextCompressor(mock, 2)

	span := &Span{
		ID:   "span-1",
		Type: SpanTypeToolCall,
		Name: "host.log_tail",
	}
	messages := []Message{
		{Role: "tool", Content: "Jan 1 OOM killed process 1234\nJan 1 OOM killed process 5678\nJan 2 OOM killed process 9012"},
	}

	ch := cc.CompressAsync(context.Background(), span, messages)

	select {
	case summary := <-ch:
		if summary != "Found 3 OOM errors in /var/log/syslog" {
			t.Fatalf("unexpected summary: %s", summary)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for summary")
	}

	// Verify span summary was updated
	if span.Summary != "Found 3 OOM errors in /var/log/syslog" {
		t.Fatalf("expected span summary to be updated, got: %s", span.Summary)
	}
}

func TestCompressSyncSuccess(t *testing.T) {
	mock := &mockSummaryModel{response: "Disk usage at 95% on /dev/sda1"}
	cc := NewContextCompressor(mock, 2)

	span := &Span{
		ID:   "span-sync",
		Type: SpanTypeShellExec,
		Name: "host.disk_usage",
	}
	messages := []Message{
		{Role: "user", Content: "Check disk usage"},
		{Role: "tool", Content: "Filesystem      Size  Used Avail Use% Mounted on\n/dev/sda1       100G   95G    5G  95% /"},
	}

	summary, err := cc.Compress(context.Background(), span, messages)
	if err != nil {
		t.Fatalf("Compress returned error: %v", err)
	}
	if summary != "Disk usage at 95% on /dev/sda1" {
		t.Fatalf("unexpected summary: %s", summary)
	}
	if span.Summary != summary {
		t.Fatalf("expected span summary to be updated, got: %s", span.Summary)
	}
	if mock.calls.Load() != 1 {
		t.Fatalf("expected 1 model call, got %d", mock.calls.Load())
	}
}

func TestCompressorUsesAIOpsCompactionPrompt(t *testing.T) {
	capture := &capturingSummaryModel{response: "AIOps compact summary"}
	cc := NewContextCompressor(capture, 1)

	_, err := cc.Compress(context.Background(), &Span{ID: "compact", Type: SpanTypeSummary, Name: "compact"}, []Message{
		{Role: "tool", Content: "evidenceRefs: ev-1\npending approvals: approval-1"},
	})
	if err != nil {
		t.Fatalf("Compress returned error: %v", err)
	}
	if len(capture.messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(capture.messages))
	}
	system := capture.messages[0].Content
	for _, want := range []string{
		"Do NOT call tools",
		"用户当前目标",
		"当前事故",
		"已确认事实和 evidenceRefs",
		"pending approvals",
		"Runner / OpsManual / MCP / Skills",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, system)
		}
	}
	if !strings.Contains(capture.messages[1].Content, "transcript/ref") {
		t.Fatalf("user prompt missing transcript/ref hint:\n%s", capture.messages[1].Content)
	}
}

func TestCompressWritesModelInputTraceWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	mock := &mockSummaryModel{response: "summary"}
	cc := NewContextCompressor(mock, 1)
	span := &Span{ID: "span/trace", Type: SpanTypeSummary, Name: "summary"}

	_, err := cc.Compress(context.Background(), span, []Message{{Role: "tool", Content: "verbose output"}})
	if err != nil {
		t.Fatalf("Compress returned error: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "spanstream_compressor", "span-trace", "*.json"))
	if err != nil {
		t.Fatalf("glob trace output: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one compressor trace, got %d", len(matches))
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	if !strings.Contains(string(data), `"kind": "spanstream_compressor"`) || !strings.Contains(string(data), "Summarize the following conversation/output") {
		t.Fatalf("trace missing compressor model input:\n%s", string(data))
	}
	if _, err := os.Stat(strings.TrimSuffix(matches[0], filepath.Ext(matches[0])) + ".md"); err != nil {
		t.Fatalf("missing markdown trace: %v", err)
	}
}

func TestCompressAsyncEmptyMessages(t *testing.T) {
	mock := &mockSummaryModel{response: "should not be called"}
	cc := NewContextCompressor(mock, 2)

	span := &Span{ID: "span-2", Type: SpanTypeToolCall, Name: "empty"}
	ch := cc.CompressAsync(context.Background(), span, nil)

	select {
	case summary := <-ch:
		if summary != "" {
			t.Fatalf("expected empty summary for nil messages, got: %s", summary)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	if mock.calls.Load() != 0 {
		t.Fatal("expected no model calls for empty messages")
	}
}

func TestCompressAsyncModelError(t *testing.T) {
	mock := &mockSummaryModel{err: errors.New("model unavailable")}
	cc := NewContextCompressor(mock, 2)

	span := &Span{ID: "span-3", Type: SpanTypeToolCall, Name: "failing"}
	messages := []Message{{Role: "tool", Content: "some output"}}

	ch := cc.CompressAsync(context.Background(), span, messages)

	select {
	case summary := <-ch:
		if summary != "" {
			t.Fatalf("expected empty summary on error, got: %s", summary)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestCompressAsyncContextCancelled(t *testing.T) {
	mock := &mockSummaryModel{response: "summary", delay: 500 * time.Millisecond}
	cc := NewContextCompressor(mock, 1)

	// Drain the worker pool so the next call blocks on acquire
	<-cc.workerPool

	ctx, cancel := context.WithCancel(context.Background())
	span := &Span{ID: "span-4", Type: SpanTypeToolCall, Name: "cancelled"}
	messages := []Message{{Role: "tool", Content: "data"}}

	ch := cc.CompressAsync(ctx, span, messages)

	// Cancel immediately - the goroutine should detect ctx.Done while waiting for pool
	cancel()

	select {
	case summary := <-ch:
		if summary != "" {
			t.Fatalf("expected empty summary on cancel, got: %s", summary)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for cancelled result")
	}

	// Restore pool for cleanup
	cc.workerPool <- struct{}{}
}

func TestCompressAsyncNonBlocking(t *testing.T) {
	mock := &mockSummaryModel{response: "delayed summary", delay: 100 * time.Millisecond}
	cc := NewContextCompressor(mock, 2)

	span := &Span{ID: "span-5", Type: SpanTypeToolCall, Name: "non-blocking"}
	messages := []Message{{Role: "tool", Content: "verbose log output"}}

	start := time.Now()
	ch := cc.CompressAsync(context.Background(), span, messages)
	elapsed := time.Since(start)

	// CompressAsync should return immediately (non-blocking)
	if elapsed > 10*time.Millisecond {
		t.Fatalf("CompressAsync blocked for %v, expected non-blocking", elapsed)
	}

	// But the result should eventually arrive
	select {
	case summary := <-ch:
		if summary != "delayed summary" {
			t.Fatalf("unexpected summary: %s", summary)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestCompressAsyncConcurrencyControl(t *testing.T) {
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32

	mock := &mockSummaryModel{response: "summary", delay: 50 * time.Millisecond}
	// Wrap to track concurrency
	trackingModel := &concurrencyTrackingModel{
		inner:         mock,
		concurrent:    &concurrent,
		maxConcurrent: &maxConcurrent,
	}

	maxWorkers := 2
	cc := NewContextCompressor(trackingModel, maxWorkers)

	// Launch more compressions than the pool allows
	numTasks := 6
	channels := make([]<-chan string, numTasks)
	for i := 0; i < numTasks; i++ {
		span := &Span{ID: "span-conc", Type: SpanTypeToolCall, Name: "concurrent"}
		messages := []Message{{Role: "tool", Content: "data"}}
		channels[i] = cc.CompressAsync(context.Background(), span, messages)
	}

	// Wait for all to complete
	for i, ch := range channels {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for task %d", i)
		}
	}

	// Max concurrent should not exceed pool size
	if maxConcurrent.Load() > int32(maxWorkers) {
		t.Fatalf("max concurrent %d exceeded pool size %d", maxConcurrent.Load(), maxWorkers)
	}
}

// concurrencyTrackingModel wraps a model to track concurrent calls.
type concurrencyTrackingModel struct {
	inner         model.ChatModel
	concurrent    *atomic.Int32
	maxConcurrent *atomic.Int32
}

func (m *concurrencyTrackingModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	cur := m.concurrent.Add(1)
	defer m.concurrent.Add(-1)

	// Update max
	for {
		old := m.maxConcurrent.Load()
		if cur <= old {
			break
		}
		if m.maxConcurrent.CompareAndSwap(old, cur) {
			break
		}
	}

	return m.inner.Generate(ctx, msgs, opts...)
}

func (m *concurrencyTrackingModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return m.inner.Stream(ctx, msgs, opts...)
}

func (m *concurrencyTrackingModel) BindTools(tools []*schema.ToolInfo) error {
	return m.inner.BindTools(tools)
}

func TestCompressAsyncNilSpan(t *testing.T) {
	mock := &mockSummaryModel{response: "summary without span"}
	cc := NewContextCompressor(mock, 2)

	messages := []Message{{Role: "tool", Content: "some output"}}
	ch := cc.CompressAsync(context.Background(), nil, messages)

	select {
	case summary := <-ch:
		if summary != "summary without span" {
			t.Fatalf("unexpected summary: %s", summary)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestCompressAsyncMultipleMessages(t *testing.T) {
	mock := &mockSummaryModel{response: "Disk usage at 95% on /dev/sda1"}
	cc := NewContextCompressor(mock, 2)

	span := &Span{ID: "span-multi", Type: SpanTypeShellExec, Name: "host.disk_usage"}
	messages := []Message{
		{Role: "user", Content: "Check disk usage"},
		{Role: "assistant", Content: "Running df -h..."},
		{Role: "tool", Content: "Filesystem      Size  Used Avail Use% Mounted on\n/dev/sda1       100G   95G    5G  95% /"},
	}

	ch := cc.CompressAsync(context.Background(), span, messages)

	select {
	case summary := <-ch:
		if summary != "Disk usage at 95% on /dev/sda1" {
			t.Fatalf("unexpected summary: %s", summary)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestCompressAsyncChannelClosedAfterResult(t *testing.T) {
	mock := &mockSummaryModel{response: "done"}
	cc := NewContextCompressor(mock, 2)

	span := &Span{ID: "span-close", Type: SpanTypeToolCall, Name: "test"}
	messages := []Message{{Role: "tool", Content: "output"}}

	ch := cc.CompressAsync(context.Background(), span, messages)

	// First read gets the result
	result := <-ch
	if result != "done" {
		t.Fatalf("expected 'done', got %s", result)
	}

	// Second read should get zero value (channel closed)
	result2, ok := <-ch
	if ok {
		t.Fatalf("expected channel to be closed, got: %s", result2)
	}
}

func TestCompressAsyncParallelSafety(t *testing.T) {
	mock := &mockSummaryModel{response: "safe", delay: 10 * time.Millisecond}
	cc := NewContextCompressor(mock, 4)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			span := &Span{ID: "span-safe", Type: SpanTypeToolCall, Name: "parallel"}
			messages := []Message{{Role: "tool", Content: "data"}}
			ch := cc.CompressAsync(context.Background(), span, messages)
			<-ch
		}()
	}
	wg.Wait()
}
