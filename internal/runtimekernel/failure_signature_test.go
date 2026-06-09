package runtimekernel

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFailureSignatureDedupesEquivalentToolFailures(t *testing.T) {
	argsA := json.RawMessage(`{"action":"synthetic_read","resourceType":"synthetic_resource","resourceId":"synthetic-resource-alpha","attempt":1,"requestId":"synthetic-request-a","credential":"synthetic_credential_value"}`)
	argsB := json.RawMessage(`{"action":"synthetic_read","resourceType":"synthetic_resource","resourceId":"synthetic-resource-alpha","attempt":2,"requestId":"synthetic-request-b","credential":"synthetic_credential_value"}`)

	sigA := BuildFailureSignature("synthetic.tool", argsA, ToolResult{Error: "timeout after 30 seconds; request synthetic-request-a"})
	sigB := BuildFailureSignature("synthetic.tool", argsB, ToolResult{Error: "timeout after 60 seconds; request synthetic-request-b"})

	if sigA != sigB {
		t.Fatalf("signatures differ:\nA=%s\nB=%s", sigA, sigB)
	}
	if strings.Contains(sigA, "synthetic_credential_value") || strings.Contains(sigA, "synthetic-request-a") {
		t.Fatalf("signature leaked sensitive or per-request input: %s", sigA)
	}
}

func TestFailureSignatureSwitchPathAfterRepeatedFailure(t *testing.T) {
	signature := BuildFailureSignature("synthetic.tool", json.RawMessage(`{"resourceType":"synthetic_resource","resourceId":"synthetic-resource-alpha"}`), ToolResult{Error: "timeout after 30 seconds"})

	decision := EvaluateFailureSignatureDecision(signature, 3)

	if decision.Action != "switch_path" {
		t.Fatalf("Action = %q, want switch_path", decision.Action)
	}
	if decision.Signature != signature {
		t.Fatalf("Signature = %q, want %q", decision.Signature, signature)
	}
	if decision.SeenCount != 3 {
		t.Fatalf("SeenCount = %d, want 3", decision.SeenCount)
	}
	if strings.TrimSpace(decision.SwitchPathReason) == "" {
		t.Fatalf("SwitchPathReason is empty")
	}
}
