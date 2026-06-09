package spanstream

import (
	"bytes"
	"io"
	"sync"
)

// ---------------------------------------------------------------------------
// StreamingToolReader wraps an io.Reader and emits partial content as it
// becomes available. This enables the LLM to begin reasoning on partial
// results from web search or file read operations without waiting for the
// complete content to download.
//
// Requirement 14.7: Utilize Go's io.Reader streaming to inject partial
// content into the LLM prompt immediately.
// ---------------------------------------------------------------------------

// ChunkHandler is called each time a new chunk of data is read from the
// underlying reader. It receives the chunk bytes and the total bytes read
// so far. Returning an error from the handler aborts the read.
type ChunkHandler func(chunk []byte, totalRead int) error

// StreamingToolReader wraps an io.Reader and invokes a ChunkHandler for
// each chunk read, enabling incremental content injection into the LLM prompt.
type StreamingToolReader struct {
	reader    io.Reader
	chunkSize int
	onChunk   ChunkHandler

	mu        sync.Mutex
	totalRead int
	buf       bytes.Buffer // accumulated content
	done      bool
	err       error
}

// StreamingReaderSnapshot captures the readable state of a streaming reader
// without advancing it.
type StreamingReaderSnapshot struct {
	Content   []byte
	TotalRead int
	Done      bool
	Err       error
}

// NewStreamingToolReader creates a StreamingToolReader that reads from r in
// chunks of chunkSize bytes. Each chunk triggers the onChunk callback.
// If chunkSize <= 0, defaults to 4096.
func NewStreamingToolReader(r io.Reader, chunkSize int, onChunk ChunkHandler) *StreamingToolReader {
	if chunkSize <= 0 {
		chunkSize = 4096
	}
	return &StreamingToolReader{
		reader:    r,
		chunkSize: chunkSize,
		onChunk:   onChunk,
	}
}

// Read implements io.Reader. Each call reads up to len(p) bytes from the
// underlying reader, accumulates the content, and invokes the chunk handler.
func (s *StreamingToolReader) Read(p []byte) (int, error) {
	n, err := s.reader.Read(p)
	if n > 0 {
		s.mu.Lock()
		s.buf.Write(p[:n])
		s.totalRead += n
		total := s.totalRead
		s.mu.Unlock()

		if s.onChunk != nil {
			if cbErr := s.onChunk(p[:n], total); cbErr != nil {
				return n, cbErr
			}
		}
	}
	if err != nil {
		s.mu.Lock()
		s.done = true
		s.err = err
		s.mu.Unlock()
	}
	return n, err
}

// Content returns all content read so far.
func (s *StreamingToolReader) Content() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.buf.Bytes()...)
}

// Snapshot returns a copy of the reader state, including accumulated content.
func (s *StreamingToolReader) Snapshot() StreamingReaderSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	return StreamingReaderSnapshot{
		Content:   append([]byte(nil), s.buf.Bytes()...),
		TotalRead: s.totalRead,
		Done:      s.done,
		Err:       s.err,
	}
}

// ContentSince returns the bytes read since the provided byte offset.
// If offset is at or beyond the current content length, it returns nil.
func (s *StreamingToolReader) ContentSince(offset int) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	if offset < 0 {
		offset = 0
	}
	content := s.buf.Bytes()
	if offset >= len(content) {
		return nil
	}
	return append([]byte(nil), content[offset:]...)
}

// TotalRead returns the total number of bytes read so far.
func (s *StreamingToolReader) TotalRead() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totalRead
}

// Done returns true if the underlying reader has been fully consumed.
func (s *StreamingToolReader) Done() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done
}

// ReadAll reads the entire underlying reader in chunks, invoking the
// chunk handler for each chunk. Returns the complete content and any error.
func (s *StreamingToolReader) ReadAll() ([]byte, error) {
	buf := make([]byte, s.chunkSize)
	for {
		n, err := s.Read(buf)
		if err != nil {
			if err == io.EOF {
				return s.Content(), nil
			}
			return s.Content(), err
		}
		if n == 0 {
			return s.Content(), nil
		}
	}
}

// ---------------------------------------------------------------------------
// PartialContentInjector feeds partial content from a StreamingToolReader
// to the LLM prompt incrementally. It tracks what has already been injected
// and only provides new content on each call to NextChunk().
// ---------------------------------------------------------------------------

// PartialContentInjector manages incremental content injection into the LLM
// prompt. It tracks the injection cursor so that each call to NextChunk()
// returns only the newly available content since the last injection.
type PartialContentInjector struct {
	reader *StreamingToolReader
	mu     sync.Mutex
	cursor int // byte offset of last injected content
}

// PartialContentProgress captures the current readable/injection state.
type PartialContentProgress struct {
	Cursor    int
	Available int
	TotalRead int
	Done      bool
}

// NewPartialContentInjector creates an injector backed by the given
// StreamingToolReader.
func NewPartialContentInjector(reader *StreamingToolReader) *PartialContentInjector {
	return &PartialContentInjector{
		reader: reader,
	}
}

// NextChunk returns any new content available since the last call.
// Returns nil if no new content is available. Returns io.EOF when the
// underlying reader is fully consumed and all content has been injected.
func (p *PartialContentInjector) NextChunk() ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	snapshot := p.reader.Snapshot()
	available := len(snapshot.Content)

	if available > p.cursor {
		chunk := make([]byte, available-p.cursor)
		copy(chunk, snapshot.Content[p.cursor:available])
		p.cursor = available
		return chunk, nil
	}

	if snapshot.Done {
		return nil, io.EOF
	}

	return nil, nil
}

// AllInjected returns true when all content has been read and injected.
func (p *PartialContentInjector) AllInjected() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	content := p.reader.Content()
	return p.reader.Done() && p.cursor >= len(content)
}

// Cursor returns the current injection cursor position.
func (p *PartialContentInjector) Cursor() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cursor
}

// Progress returns the current partial-content state without advancing the
// injection cursor.
func (p *PartialContentInjector) Progress() PartialContentProgress {
	p.mu.Lock()
	defer p.mu.Unlock()

	snapshot := p.reader.Snapshot()
	return PartialContentProgress{
		Cursor:    p.cursor,
		Available: len(snapshot.Content),
		TotalRead: snapshot.TotalRead,
		Done:      snapshot.Done,
	}
}

// ---------------------------------------------------------------------------
// StreamingToolOperation represents a streaming tool execution (web search
// or file read) that can provide partial results to the LLM.
// ---------------------------------------------------------------------------

// StreamingToolOperation wraps a streaming tool execution, providing both
// the StreamingToolReader for reading and the PartialContentInjector for
// incremental LLM prompt injection.
type StreamingToolOperation struct {
	// ToolName is the name of the tool being executed (e.g., "web_search", "file_read").
	ToolName string

	// SpanID is the span tracking ID for this operation (if span tracking is enabled).
	SpanID string

	// Reader provides streaming access to the tool's output.
	Reader *StreamingToolReader

	// Injector manages incremental content injection into the LLM prompt.
	Injector *PartialContentInjector
}

// NewStreamingToolOperation creates a new streaming tool operation from an
// io.Reader source. The chunkSize controls how frequently partial content
// is made available. The onChunk callback is invoked for each chunk read.
func NewStreamingToolOperation(toolName string, source io.Reader, chunkSize int, onChunk ChunkHandler) *StreamingToolOperation {
	reader := NewStreamingToolReader(source, chunkSize, onChunk)
	injector := NewPartialContentInjector(reader)
	return &StreamingToolOperation{
		ToolName: toolName,
		Reader:   reader,
		Injector: injector,
	}
}
