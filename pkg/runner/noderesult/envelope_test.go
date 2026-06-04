package noderesult

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseEnvelopeFromMarkedStdoutPreservesLogs(t *testing.T) {
	raw := `line before
AIOPS_NODE_RESULT_BEGIN
{"schema_version":"aiops.node_result/v1","run_id":"run-1","node_id":"extract","node_type":"script.python","status":"success","outputs":{"items":[{"title":"A"}]},"metrics":{"count":1}}
AIOPS_NODE_RESULT_END
line after`

	parsed, ok, err := ParseStdout(raw)
	if err != nil {
		t.Fatalf("ParseStdout() error = %v", err)
	}
	if !ok {
		t.Fatal("ParseStdout() ok = false, want true")
	}
	if parsed.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", parsed.SchemaVersion, SchemaVersion)
	}
	if parsed.Outputs["items"] == nil {
		t.Fatalf("outputs = %#v, want items", parsed.Outputs)
	}
	if parsed.Logs.StdoutPreview == "" || parsed.Logs.Truncated {
		t.Fatalf("logs = %#v, want non-truncated stdout preview", parsed.Logs)
	}
}

func TestParseEnvelopeFallsBackToLastJSONLine(t *testing.T) {
	raw := "debug\n" + `{"schema_version":"aiops.node_result/v1","node_id":"n1","status":"failed","error":{"message":"bad json"}}`
	parsed, ok, err := ParseStdout(raw)
	if err != nil {
		t.Fatalf("ParseStdout() error = %v", err)
	}
	if !ok {
		t.Fatal("ParseStdout() ok = false, want true")
	}
	if parsed.Status != StatusFailed || parsed.Error == nil || parsed.Error.Message != "bad json" {
		t.Fatalf("parsed = %#v, want failed envelope", parsed)
	}
}

func TestParseEnvelopeReturnsLegacyWhenNoEnvelope(t *testing.T) {
	parsed, ok, err := ParseStdout("plain output\n")
	if err != nil {
		t.Fatalf("ParseStdout() error = %v", err)
	}
	if ok {
		t.Fatal("ParseStdout() ok = true, want false for legacy output")
	}
	if parsed.SchemaVersion != "" {
		t.Fatalf("legacy parsed = %#v, want empty envelope", parsed)
	}
}

func TestSuccessEnvelopeJSONRoundTrip(t *testing.T) {
	env := Success(Options{
		RunID:    "run-1",
		NodeID:   "node-1",
		NodeType: "script.shell",
		Outputs: map[string]any{"value": "ok"},
		Metrics: map[string]any{"count": 1},
		StartedAt: time.Date(2026, 5, 25, 8, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 5, 25, 8, 0, 1, 0, time.UTC),
		ExitCode: 0,
	})
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.DurationMs != 1000 {
		t.Fatalf("duration = %d, want 1000", decoded.DurationMs)
	}
	if decoded.Outputs["value"] != "ok" {
		t.Fatalf("outputs = %#v", decoded.Outputs)
	}
}

