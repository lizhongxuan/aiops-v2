package spanstream

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// StreamingToolReader tests
// ---------------------------------------------------------------------------

func TestStreamingToolReader_ReadAll(t *testing.T) {
	content := "hello world, this is streaming content for LLM injection"
	reader := NewStreamingToolReader(strings.NewReader(content), 8, nil)

	result, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(result) != content {
		t.Fatalf("expected %q, got %q", content, string(result))
	}
	if !reader.Done() {
		t.Fatal("expected Done() to be true after ReadAll")
	}
	if reader.TotalRead() != len(content) {
		t.Fatalf("expected TotalRead=%d, got %d", len(content), reader.TotalRead())
	}
}

func TestStreamingToolReader_ChunkHandlerCalled(t *testing.T) {
	content := "abcdefghijklmnop" // 16 bytes
	chunkSize := 4

	var mu sync.Mutex
	var chunks []string
	handler := func(chunk []byte, totalRead int) error {
		mu.Lock()
		defer mu.Unlock()
		chunks = append(chunks, string(chunk))
		return nil
	}

	reader := NewStreamingToolReader(strings.NewReader(content), chunkSize, handler)
	_, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	// Verify all chunks together form the original content
	joined := strings.Join(chunks, "")
	if joined != content {
		t.Fatalf("chunks joined = %q, want %q", joined, content)
	}
	// Verify we got multiple chunks (content is 16 bytes, chunk size is 4)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
}

func TestStreamingToolReader_ChunkHandlerError(t *testing.T) {
	content := "some content that should be partially read"
	expectedErr := errors.New("handler abort")

	callCount := 0
	handler := func(chunk []byte, totalRead int) error {
		callCount++
		if callCount >= 2 {
			return expectedErr
		}
		return nil
	}

	reader := NewStreamingToolReader(strings.NewReader(content), 4, handler)
	_, err := reader.ReadAll()
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected handler error, got: %v", err)
	}
	// Should have partial content
	if reader.TotalRead() == 0 {
		t.Fatal("expected some bytes to have been read")
	}
}

func TestStreamingToolReader_EmptyReader(t *testing.T) {
	reader := NewStreamingToolReader(strings.NewReader(""), 4, nil)
	result, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %q", string(result))
	}
	if !reader.Done() {
		t.Fatal("expected Done() to be true")
	}
}

func TestStreamingToolReader_DefaultChunkSize(t *testing.T) {
	reader := NewStreamingToolReader(strings.NewReader("test"), 0, nil)
	if reader.chunkSize != 4096 {
		t.Fatalf("expected default chunkSize=4096, got %d", reader.chunkSize)
	}
}

func TestStreamingToolReader_IncrementalRead(t *testing.T) {
	content := "streaming data"
	reader := NewStreamingToolReader(strings.NewReader(content), 4, nil)

	// Read first chunk
	buf := make([]byte, 4)
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("first Read error: %v", err)
	}
	if n == 0 {
		t.Fatal("expected non-zero first read")
	}
	if reader.Done() {
		t.Fatal("should not be done after first read")
	}
	if reader.TotalRead() != n {
		t.Fatalf("TotalRead=%d, want %d", reader.TotalRead(), n)
	}
}

// ---------------------------------------------------------------------------
// PartialContentInjector tests
// ---------------------------------------------------------------------------

func TestPartialContentInjector_IncrementalInjection(t *testing.T) {
	// Use a pipe to simulate streaming data
	pr, pw := io.Pipe()

	reader := NewStreamingToolReader(pr, 4, nil)
	injector := NewPartialContentInjector(reader)

	// Initially no content
	chunk, err := injector.NextChunk()
	if chunk != nil || err != nil {
		t.Fatalf("expected nil chunk initially, got chunk=%v, err=%v", chunk, err)
	}

	// Write some data in background
	go func() {
		pw.Write([]byte("hello"))
		pw.Write([]byte(" world"))
		pw.Close()
	}()

	// Read all content through the streaming reader
	go func() {
		reader.ReadAll()
	}()

	// Wait for content to be available and verify incremental injection
	var allInjected []byte
	for {
		chunk, err := injector.NextChunk()
		if err == io.EOF {
			break
		}
		if chunk != nil {
			allInjected = append(allInjected, chunk...)
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if string(allInjected) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", string(allInjected))
	}
}

func TestPartialContentInjector_AllInjected(t *testing.T) {
	content := "complete content"
	reader := NewStreamingToolReader(strings.NewReader(content), 4, nil)
	injector := NewPartialContentInjector(reader)

	if injector.AllInjected() {
		t.Fatal("should not be AllInjected before reading")
	}

	// Read all content
	reader.ReadAll()

	// Drain the injector
	for {
		chunk, err := injector.NextChunk()
		if err == io.EOF {
			break
		}
		if chunk == nil {
			break
		}
	}

	if !injector.AllInjected() {
		t.Fatal("should be AllInjected after draining")
	}
}

func TestPartialContentInjector_Cursor(t *testing.T) {
	content := "0123456789"
	reader := NewStreamingToolReader(strings.NewReader(content), 4, nil)
	injector := NewPartialContentInjector(reader)

	if injector.Cursor() != 0 {
		t.Fatalf("initial cursor should be 0, got %d", injector.Cursor())
	}

	// Read all content
	reader.ReadAll()

	// Get first chunk
	chunk, _ := injector.NextChunk()
	if injector.Cursor() != len(chunk) {
		t.Fatalf("cursor should be %d after first NextChunk, got %d", len(chunk), injector.Cursor())
	}
}

// ---------------------------------------------------------------------------
// StreamingToolOperation tests
// ---------------------------------------------------------------------------

func TestStreamingToolOperation_WebSearch(t *testing.T) {
	// Simulate a web search returning results incrementally
	searchResults := "Result 1: Server CPU at 95%\nResult 2: Memory leak detected\nResult 3: Disk full on /var"

	var chunks []string
	handler := func(chunk []byte, totalRead int) error {
		chunks = append(chunks, string(chunk))
		return nil
	}

	op := NewStreamingToolOperation("web_search", strings.NewReader(searchResults), 16, handler)

	if op.ToolName != "web_search" {
		t.Fatalf("expected ToolName='web_search', got %q", op.ToolName)
	}

	// Read all content
	result, err := op.Reader.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if string(result) != searchResults {
		t.Fatalf("expected full search results, got %q", string(result))
	}

	// Verify injector can provide the content
	allChunk, err := op.Injector.NextChunk()
	if err != nil {
		t.Fatalf("NextChunk error: %v", err)
	}
	if string(allChunk) != searchResults {
		t.Fatalf("injector content mismatch: got %q", string(allChunk))
	}
}

func TestStreamingToolOperation_FileRead(t *testing.T) {
	// Simulate reading a large file in chunks
	fileContent := bytes.Repeat([]byte("log line content\n"), 100)

	chunkCount := 0
	handler := func(chunk []byte, totalRead int) error {
		chunkCount++
		return nil
	}

	op := NewStreamingToolOperation("file_read", bytes.NewReader(fileContent), 64, handler)

	result, err := op.Reader.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if !bytes.Equal(result, fileContent) {
		t.Fatal("file content mismatch")
	}
	if chunkCount < 2 {
		t.Fatalf("expected multiple chunks for large file, got %d", chunkCount)
	}
}

func TestStreamingToolOperation_SpanID(t *testing.T) {
	op := NewStreamingToolOperation("web_search", strings.NewReader("data"), 4, nil)
	op.SpanID = "span-123"

	if op.SpanID != "span-123" {
		t.Fatalf("expected SpanID='span-123', got %q", op.SpanID)
	}
}

func TestStreamingToolOperation_ConcurrentReadAndInject(t *testing.T) {
	// Simulate concurrent reading and injection (real-world scenario)
	pr, pw := io.Pipe()

	op := NewStreamingToolOperation("web_search", pr, 8, nil)

	var wg sync.WaitGroup

	// Writer goroutine: simulates slow network data arrival
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer pw.Close()
		for i := 0; i < 5; i++ {
			pw.Write([]byte("chunk_"))
		}
	}()

	// Reader goroutine: reads from the streaming reader
	wg.Add(1)
	go func() {
		defer wg.Done()
		op.Reader.ReadAll()
	}()

	wg.Wait()

	// After everything is done, injector should provide all content
	var injected []byte
	for {
		chunk, err := op.Injector.NextChunk()
		if err == io.EOF {
			break
		}
		if chunk != nil {
			injected = append(injected, chunk...)
		}
		if chunk == nil && err == nil {
			break
		}
	}

	expected := "chunk_chunk_chunk_chunk_chunk_"
	if string(injected) != expected {
		t.Fatalf("expected %q, got %q", expected, string(injected))
	}
}
