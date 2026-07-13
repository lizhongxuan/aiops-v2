package modeltrace

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCanonicalRolloutFreezeNormalizesAndStabilizesIdentity(t *testing.T) {
	payload := map[string]any{
		"status": "admitted",
		"nested": map[string]any{"attempt": 1},
	}
	sourceRefs := []string{" trace:z ", "trace:a", "trace:z", ""}
	input := CanonicalRolloutEvent{
		Sequence:   1,
		SessionID:  " session-1 ",
		TurnID:     " turn-1 ",
		StepID:     " step-1 ",
		Kind:       CanonicalRolloutKindAdmission,
		SourceRefs: sourceRefs,
		Payload:    payload,
	}

	first, err := FreezeCanonicalRolloutEvent(input)
	if err != nil {
		t.Fatalf("FreezeCanonicalRolloutEvent() error = %v", err)
	}
	second, err := FreezeCanonicalRolloutEvent(input)
	if err != nil {
		t.Fatalf("FreezeCanonicalRolloutEvent() second error = %v", err)
	}
	if first.SchemaVersion != CanonicalRolloutSchemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", first.SchemaVersion, CanonicalRolloutSchemaVersion)
	}
	if first.EventID == "" || first.Hash == "" {
		t.Fatalf("frozen identity is incomplete: %#v", first)
	}
	if first.EventID != second.EventID || first.Hash != second.Hash {
		t.Fatalf("same input produced unstable identity: first=%#v second=%#v", first, second)
	}
	if got, want := first.SourceRefs, []string{"trace:a", "trace:z"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sourceRefs = %#v, want %#v", got, want)
	}
	if first.SessionID != "session-1" || first.TurnID != "turn-1" || first.StepID != "step-1" {
		t.Fatalf("identity fields were not normalized: %#v", first)
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	// The frozen event must not alias caller-owned payload or source slices.
	payload["status"] = "mutated"
	payload["nested"].(map[string]any)["attempt"] = 2
	sourceRefs[0] = "trace:mutated"
	if first.Payload["status"] != "admitted" {
		t.Fatalf("frozen payload aliased caller: %#v", first.Payload)
	}
	if got := first.Payload["nested"].(map[string]any)["attempt"]; got != json.Number("1") {
		t.Fatalf("frozen nested payload = %#v, want json.Number(1)", got)
	}
	if got := first.SourceRefs[0]; got != "trace:a" {
		t.Fatalf("frozen source refs aliased caller: %#v", first.SourceRefs)
	}

	nextSequence := input
	nextSequence.Sequence = 2
	next, err := FreezeCanonicalRolloutEvent(nextSequence)
	if err != nil {
		t.Fatalf("FreezeCanonicalRolloutEvent(next sequence) error = %v", err)
	}
	if next.EventID == first.EventID || next.Hash == first.Hash {
		t.Fatalf("sequence must participate in identity: first=%#v next=%#v", first, next)
	}

	differentPayload := input
	differentPayload.Payload = map[string]any{"status": "blocked"}
	different, err := FreezeCanonicalRolloutEvent(differentPayload)
	if err != nil {
		t.Fatalf("FreezeCanonicalRolloutEvent(different payload) error = %v", err)
	}
	if different.EventID != first.EventID {
		t.Fatalf("same fact coordinates should keep EventID: first=%q different=%q", first.EventID, different.EventID)
	}
	if different.Hash == first.Hash {
		t.Fatal("different fact payload should change event hash")
	}
}

func TestCanonicalRolloutFreezeRedactsSecretsActionArgsAndRAGOriginals(t *testing.T) {
	const canary = "canonical-secret-canary"
	event, err := FreezeCanonicalRolloutEvent(CanonicalRolloutEvent{
		Sequence:  7,
		SessionID: "session-redaction",
		TurnID:    "turn-redaction",
		Kind:      CanonicalRolloutKindToolProposed,
		SourceRefs: []string{
			"https://example.invalid/doc?id=1&api_key=" + canary,
		},
		Payload: map[string]any{
			"apiKey":             canary,
			"authorization":      "Bearer " + canary,
			"providerCredential": canary,
			"note":               "token=" + canary,
			"nested": map[string]any{
				"password": canary,
				"safe":     "visible",
			},
			"actionArgs": map[string]any{
				"host":    "host-a",
				"command": "echo " + canary,
			},
			"ragRawContent":  "retrieved " + canary,
			"secretRef":      "vault://provider/key",
			"contentHash":    "sha256:trusted",
			"tokensEstimate": 42,
		},
	})
	if err != nil {
		t.Fatalf("FreezeCanonicalRolloutEvent() error = %v", err)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), canary) {
		t.Fatalf("canonical event leaked secret canary: %s", encoded)
	}
	if got := event.Payload["nested"].(map[string]any)["safe"]; got != "visible" {
		t.Fatalf("non-sensitive nested fact = %#v, want visible", got)
	}
	if got := event.Payload["secretRef"]; got != "vault://provider/key" {
		t.Fatalf("secret ref should remain referential: %#v", got)
	}
	if got := event.Payload["contentHash"]; got != "sha256:trusted" {
		t.Fatalf("content hash should remain referential: %#v", got)
	}
	if got := event.Payload["tokensEstimate"]; got != json.Number("42") {
		t.Fatalf("token counter must not be redacted: %#v", got)
	}
	for _, key := range []string{"apiKey", "authorization", "providerCredential", "actionArgs", "ragRawContent"} {
		marker, ok := event.Payload[key].(map[string]any)
		if !ok || marker["redacted"] != true || marker["sha256"] == "" {
			t.Fatalf("payload[%q] = %#v, want hash-backed redaction marker", key, event.Payload[key])
		}
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestCanonicalRolloutValidateFailsClosed(t *testing.T) {
	base := CanonicalRolloutEvent{
		Sequence:  1,
		SessionID: "session-validate",
		TurnID:    "turn-validate",
		Kind:      CanonicalRolloutKindCheckpoint,
	}
	for name, mutate := range map[string]func(*CanonicalRolloutEvent){
		"zero sequence":   func(event *CanonicalRolloutEvent) { event.Sequence = 0 },
		"missing session": func(event *CanonicalRolloutEvent) { event.SessionID = "" },
		"missing turn":    func(event *CanonicalRolloutEvent) { event.TurnID = "" },
		"missing kind":    func(event *CanonicalRolloutEvent) { event.Kind = "" },
		"unknown kind":    func(event *CanonicalRolloutEvent) { event.Kind = "future_kind" },
		"unknown version": func(event *CanonicalRolloutEvent) { event.SchemaVersion = "aiops.canonical-rollout.v999" },
	} {
		t.Run(name, func(t *testing.T) {
			input := base
			mutate(&input)
			if _, err := FreezeCanonicalRolloutEvent(input); err == nil {
				t.Fatalf("FreezeCanonicalRolloutEvent(%s) unexpectedly succeeded", name)
			}
		})
	}

	frozen, err := FreezeCanonicalRolloutEvent(base)
	if err != nil {
		t.Fatalf("FreezeCanonicalRolloutEvent() error = %v", err)
	}
	for name, mutate := range map[string]func(*CanonicalRolloutEvent){
		"tampered event id": func(event *CanonicalRolloutEvent) { event.EventID = "event:tampered" },
		"tampered hash":     func(event *CanonicalRolloutEvent) { event.Hash = "sha256:tampered" },
		"tampered payload":  func(event *CanonicalRolloutEvent) { event.Payload = map[string]any{"status": "changed"} },
		"unnormalized refs": func(event *CanonicalRolloutEvent) { event.SourceRefs = []string{"z", "a"} },
	} {
		t.Run(name, func(t *testing.T) {
			input := frozen
			mutate(&input)
			if err := input.Validate(); err == nil {
				t.Fatalf("Validate(%s) unexpectedly succeeded", name)
			}
		})
	}
}

func TestCanonicalRolloutKindsAreClosedAndComplete(t *testing.T) {
	kinds := []string{
		CanonicalRolloutKindAdmission,
		CanonicalRolloutKindAssembly,
		CanonicalRolloutKindPrompt,
		CanonicalRolloutKindProviderRequest,
		CanonicalRolloutKindProviderResponse,
		CanonicalRolloutKindToolProposed,
		CanonicalRolloutKindToolDispatched,
		CanonicalRolloutKindToolResult,
		CanonicalRolloutKindApprovalRequested,
		CanonicalRolloutKindApprovalDecided,
		CanonicalRolloutKindCheckpoint,
		CanonicalRolloutKindFinalFacts,
		CanonicalRolloutKindTransportProjection,
		CanonicalRolloutKindRecorderDegraded,
	}
	seen := make(map[string]struct{}, len(kinds))
	for index, kind := range kinds {
		if kind == "" {
			t.Fatalf("kind %d is empty", index)
		}
		if _, duplicate := seen[kind]; duplicate {
			t.Fatalf("duplicate canonical kind %q", kind)
		}
		seen[kind] = struct{}{}
		if _, err := FreezeCanonicalRolloutEvent(CanonicalRolloutEvent{
			Sequence: int64(index + 1), SessionID: "session-kinds", TurnID: "turn-kinds", Kind: kind,
		}); err != nil {
			t.Fatalf("kind %q rejected: %v", kind, err)
		}
	}
}
