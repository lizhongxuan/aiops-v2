package server

import (
	"bytes"
	"testing"
)

type assistantTransportStreamTestFlushBuffer struct {
	bytes.Buffer
	flushes int
}

func (b *assistantTransportStreamTestFlushBuffer) Flush() {
	b.flushes++
}

func TestAssistantTransportStreamWritesSetStateOp(t *testing.T) {
	var buf bytes.Buffer
	encoder := newAssistantTransportStreamEncoder(&buf)

	err := encoder.WriteStateOps([]assistantTransportStreamStateOp{
		{
			Type:  assistantTransportStreamOpSet,
			Path:  []any{"turns", 0, "status"},
			Value: "running",
		},
	})
	if err != nil {
		t.Fatalf("WriteStateOps() error = %v", err)
	}

	want := "aui-state:[{\"type\":\"set\",\"path\":[\"turns\",0,\"status\"],\"value\":\"running\"}]\n"
	if got := buf.String(); got != want {
		t.Fatalf("WriteStateOps() = %q, want %q", got, want)
	}
}

func TestAssistantTransportStreamWritesAppendTextStateOp(t *testing.T) {
	var buf bytes.Buffer
	encoder := newAssistantTransportStreamEncoder(&buf)

	err := encoder.WriteStateOps([]assistantTransportStreamStateOp{
		{
			Type:  assistantTransportStreamOpAppendText,
			Path:  []any{"turns", 0, "blocksById", "final-1", "text"},
			Value: "hello",
		},
	})
	if err != nil {
		t.Fatalf("WriteStateOps() error = %v", err)
	}

	want := "aui-state:[{\"type\":\"append-text\",\"path\":[\"turns\",0,\"blocksById\",\"final-1\",\"text\"],\"value\":\"hello\"}]\n"
	if got := buf.String(); got != want {
		t.Fatalf("WriteStateOps() = %q, want %q", got, want)
	}
}

func TestAssistantTransportStreamWritesErrorRecord(t *testing.T) {
	var buf bytes.Buffer
	encoder := newAssistantTransportStreamEncoder(&buf)

	if err := encoder.WriteError("transport failed"); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	want := "3:\"transport failed\"\n"
	if got := buf.String(); got != want {
		t.Fatalf("WriteError() = %q, want %q", got, want)
	}
}

func TestAssistantTransportStreamJSONEncodesUnicodeQuotesAndNewlines(t *testing.T) {
	var buf bytes.Buffer
	encoder := newAssistantTransportStreamEncoder(&buf)

	err := encoder.WriteStateOps([]assistantTransportStreamStateOp{
		{
			Type:  assistantTransportStreamOpAppendText,
			Path:  []any{"turns", "turn-1", "blocksById", "final-1", "text"},
			Value: "中文 \"quoted\"\nline-2",
		},
	})
	if err != nil {
		t.Fatalf("WriteStateOps() error = %v", err)
	}

	want := "aui-state:[{\"type\":\"append-text\",\"path\":[\"turns\",\"turn-1\",\"blocksById\",\"final-1\",\"text\"],\"value\":\"中文 \\\"quoted\\\"\\nline-2\"}]\n"
	if got := buf.String(); got != want {
		t.Fatalf("WriteStateOps() = %q, want %q", got, want)
	}
}

func TestAssistantTransportStreamFlushesWhenSupported(t *testing.T) {
	var buf assistantTransportStreamTestFlushBuffer
	encoder := newAssistantTransportStreamEncoder(&buf)

	if err := encoder.WriteStateOps([]assistantTransportStreamStateOp{
		{
			Type:  assistantTransportStreamOpSet,
			Path:  []any{"seq"},
			Value: 1,
		},
	}); err != nil {
		t.Fatalf("WriteStateOps() error = %v", err)
	}
	if err := encoder.WriteError("boom"); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}

	if buf.flushes != 2 {
		t.Fatalf("flush count = %d, want 2", buf.flushes)
	}
}
