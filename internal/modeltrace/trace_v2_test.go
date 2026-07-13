package modeltrace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aiops-v2/internal/specialinputmemory"
)

func TestWriteTraceDocumentV2WritesSummaryRawRefsAndHarnessTurn(t *testing.T) {
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
		HarnessTurn: map[string]any{
			"schemaVersion": "aiops.harness.turn.v1",
			"sessionId":     "session-1",
			"turnId":        "turn-1",
		},
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
	harnessTurn, ok := got.HarnessTurn.(map[string]any)
	if !ok || harnessTurn["schemaVersion"] != "aiops.harness.turn.v1" {
		t.Fatalf("harnessTurn = %#v", got.HarnessTurn)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.json")); err != nil {
		t.Fatalf("index.json missing: %v", err)
	}
}

func TestWriteTraceDocumentV2CarriesTurnAssemblyShadow(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteTraceDocumentV2(dir, TraceDocumentV2{
		SessionID: "session-assembly", TurnID: "turn-assembly",
		TurnAssembly:                map[string]any{"hash": "assembly-hash"},
		LegacyAgentAssemblySnapshot: map[string]any{"specHash": "legacy-hash"},
		TurnAssemblyShadow: map[string]any{
			"assemblyHash": "assembly-hash",
			"fieldDiffs":   []any{map[string]any{"field": "loopPolicy", "legacyHash": "a", "projectedHash": "b"}},
		},
	})
	if err != nil {
		t.Fatalf("WriteTraceDocumentV2() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["turnAssembly"] == nil || payload["legacyAgentAssemblySnapshot"] == nil || payload["turnAssemblyShadow"] == nil {
		t.Fatalf("trace assembly fields = %#v", payload)
	}
}

func TestWriteTraceDocumentV2RedactsSensitiveKeysWithoutCorruptingTokenCounters(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteTraceDocumentV2(dir, TraceDocumentV2{
		SessionID: "session-redaction", TurnID: "turn-redaction",
		TurnContext: map[string]any{"metadata": map[string]any{
			"apiKey": "secret-canary-v2", "note": "token=secret-canary-v2",
		}},
		PromptInputTrace: map[string]any{"contextUsage": map[string]any{
			"categories": []any{map[string]any{"name": "prompt", "tokensEstimate": 42}},
		}},
	})
	if err != nil {
		t.Fatalf("WriteTraceDocumentV2() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), "secret-canary-v2") {
		t.Fatalf("trace leaked secret canary: %s", data)
	}
	var payload struct {
		PromptInputTrace struct {
			ContextUsage struct {
				Categories []struct {
					TokensEstimate int `json:"tokensEstimate"`
				} `json:"categories"`
			} `json:"contextUsage"`
		} `json:"promptInputTrace"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := payload.PromptInputTrace.ContextUsage.Categories[0].TokensEstimate; got != 42 {
		t.Fatalf("tokensEstimate = %d, want 42", got)
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
		HarnessTurn: map[string]any{
			"schemaVersion": "aiops.harness.turn.v1",
			"sessionId":     "workflow-session",
			"turnId":        "workflow-turn",
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
	harnessTurn, ok := got.HarnessTurn.(map[string]any)
	if !ok || harnessTurn["schemaVersion"] != "aiops.harness.turn.v1" {
		t.Fatalf("harnessTurn = %#v", got.HarnessTurn)
	}
}

func TestWriteTraceDocumentV2FromRequestCarriesSpecialInputWorldState(t *testing.T) {
	dir := t.TempDir()
	worldState := &specialinputmemory.SpecialInputWorldStateSection{
		SchemaVersion: specialinputmemory.SchemaVersion,
		ActiveExecutionScope: &specialinputmemory.ExecutionScopeGrantTrace{
			ID:           "grant-host-a",
			ResourceKind: specialinputmemory.ResourceKindHost,
			ResourceID:   "host-a",
			CanonicalKey: "host:host-a",
			Display:      "host-a",
		},
		ReadPlan: &specialinputmemory.MemoryReadPlanTrace{
			ActiveGrantID:      "grant-host-a",
			ActiveResourceKind: specialinputmemory.ResourceKindHost,
			ActiveResourceID:   "host-a",
		},
	}

	path, err := WriteTraceDocumentV2FromRequestWithConfig(Config{Enabled: true, RootDir: dir}, Request{
		Kind:                   "runtime_model_input",
		SessionID:              "sess-special",
		TurnID:                 "turn-special",
		SpecialInputWorldState: worldState,
	})
	if err != nil {
		t.Fatalf("WriteTraceDocumentV2FromRequestWithConfig() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got struct {
		SpecialInputWorldState *specialinputmemory.SpecialInputWorldStateSection `json:"specialInputWorldState"`
		PromptInputTrace       struct {
			SpecialInputWorldState *specialinputmemory.SpecialInputWorldStateSection `json:"specialInputWorldState"`
		} `json:"promptInputTrace"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.SpecialInputWorldState == nil || got.SpecialInputWorldState.ActiveExecutionScope.ResourceID != "host-a" {
		t.Fatalf("top-level specialInputWorldState = %#v, want host-a", got.SpecialInputWorldState)
	}
	if got.PromptInputTrace.SpecialInputWorldState == nil || got.PromptInputTrace.SpecialInputWorldState.ActiveExecutionScope.ResourceID != "host-a" {
		t.Fatalf("promptInputTrace.specialInputWorldState = %#v, want host-a", got.PromptInputTrace.SpecialInputWorldState)
	}
}
