package modeltrace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteTraceDocumentV2WritesSummaryAndRawRefs(t *testing.T) {
	dir := t.TempDir()
	doc := TraceDocumentV2{
		SchemaVersion: "aiops.trace/v2",
		SessionID:     "session-1",
		TurnID:        "turn-1",
		Iteration:     0,
		ProviderRequest: ProviderRequestTrace{
			ModelInputHash: "mih",
			PromptCacheKey: "cache",
		},
		RawPayloadRefs: []RawPayloadRef{{
			ID:     "raw-request",
			Kind:   "provider_request",
			Path:   "raw/raw-request.json",
			Sha256: "abc",
		}},
	}
	path, err := WriteTraceDocumentV2(dir, doc)
	if err != nil {
		t.Fatalf("WriteTraceDocumentV2() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got TraceDocumentV2
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.SchemaVersion != "aiops.trace/v2" {
		t.Fatalf("schema version = %q", got.SchemaVersion)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.json")); err != nil {
		t.Fatalf("index.json missing: %v", err)
	}
}

func TestWriteTraceDocumentV2FromRequestWritesV2Schema(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteTraceDocumentV2FromRequestWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:    "opsmanual-workflow-manual-llm-summary",
		TraceID: "workflow-1",
		Metadata: map[string]string{
			"provider": "zai",
		},
		Prompt: Prompt{
			System:  "system prompt",
			Dynamic: "user prompt",
		},
	})
	if err != nil {
		t.Fatalf("WriteTraceDocumentV2FromRequest() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got TraceDocumentV2
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.SchemaVersion != TraceDocumentV2SchemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", got.SchemaVersion, TraceDocumentV2SchemaVersion)
	}
	if got.SessionID == "" || got.TurnID == "" {
		t.Fatalf("session/turn ids should be populated for v2 index: %#v", got)
	}
}
