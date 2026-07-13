package modeltrace

import (
	"encoding/hex"
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
			"https://example.invalid/doc?id=1&secret_ref=" + canary,
		},
		Payload: map[string]any{
			"apiKey":                canary,
			"authorization":         "Bearer " + canary,
			"providerCredential":    canary,
			"providerCredentialRef": "vault://provider/credential?secret_ref=" + canary,
			"note":                  "token=" + canary,
			"error":                 canary,
			"err":                   canary,
			"errorMessage":          canary,
			"nested": map[string]any{
				"password": canary,
				"safe":     "visible",
			},
			"actionArgs": map[string]any{
				"host":    "host-a",
				"command": "echo " + canary,
			},
			"ragRawContent":   "retrieved " + canary,
			"secretRef":       "vault://provider/key",
			"secretReference": "secret://provider/key",
			"eventRef":        "event:admission-1",
			"contentHash":     "sha256:trusted",
			"payloadDigest":   "sha256:payload",
			"tokensEstimate":  42,
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
	if len(event.SourceRefs) != 1 || !strings.HasPrefix(event.SourceRefs[0], "redacted:") {
		t.Fatalf("sensitive source ref = %#v, want hash-backed redaction", event.SourceRefs)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(event.SourceRefs[0], "redacted:")); err != nil {
		t.Fatalf("sensitive source ref is not hash-backed: %#v: %v", event.SourceRefs, err)
	}
	if got := event.Payload["nested"].(map[string]any)["safe"]; got != "visible" {
		t.Fatalf("non-sensitive nested fact = %#v, want visible", got)
	}
	if got := event.Payload["eventRef"]; got != "event:admission-1" {
		t.Fatalf("non-sensitive event ref should remain referential: %#v", got)
	}
	if got := event.Payload["contentHash"]; got != "sha256:trusted" {
		t.Fatalf("content hash should remain referential: %#v", got)
	}
	if got := event.Payload["payloadDigest"]; got != "sha256:payload" {
		t.Fatalf("payload digest should remain referential: %#v", got)
	}
	if got := event.Payload["tokensEstimate"]; got != json.Number("42") {
		t.Fatalf("token counter must not be redacted: %#v", got)
	}
	for _, key := range []string{
		"apiKey", "authorization", "providerCredential", "providerCredentialRef",
		"secretRef", "secretReference", "error", "err", "errorMessage", "actionArgs", "ragRawContent",
	} {
		marker, ok := event.Payload[key].(map[string]any)
		if !ok || marker["redacted"] != true || marker["sha256"] == "" {
			t.Fatalf("payload[%q] = %#v, want hash-backed redaction marker", key, event.Payload[key])
		}
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestCanonicalRolloutFreezeRedactsSecretStoreSourceRefs(t *testing.T) {
	sourceRefs := []string{
		"vault://provider/key",
		"secret://provider/key",
		"keychain://provider/key",
		"aws-secretsmanager://provider/key",
		"gcp-secretmanager://provider/key",
	}
	event, err := FreezeCanonicalRolloutEvent(CanonicalRolloutEvent{
		Sequence:   8,
		SessionID:  "session-secret-stores",
		TurnID:     "turn-secret-stores",
		Kind:       CanonicalRolloutKindProviderRequest,
		SourceRefs: sourceRefs,
	})
	if err != nil {
		t.Fatalf("FreezeCanonicalRolloutEvent() error = %v", err)
	}
	if len(event.SourceRefs) != len(sourceRefs) {
		t.Fatalf("source refs = %#v, want %d hash-backed refs", event.SourceRefs, len(sourceRefs))
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, sourceRef := range sourceRefs {
		if strings.Contains(string(encoded), sourceRef) {
			t.Fatalf("canonical event leaked secret-store ref %q: %s", sourceRef, encoded)
		}
	}
	for _, sourceRef := range event.SourceRefs {
		if !strings.HasPrefix(sourceRef, "redacted:") {
			t.Fatalf("secret-store source ref = %q, want redacted hash", sourceRef)
		}
		if _, err := hex.DecodeString(strings.TrimPrefix(sourceRef, "redacted:")); err != nil {
			t.Fatalf("secret-store source ref = %q, want valid sha256 hex: %v", sourceRef, err)
		}
	}
}

func TestCanonicalRolloutFreezeRedactsErrorFieldsButPreservesClosedErrorFacts(t *testing.T) {
	const canary = "raw-provider-error-canary"
	event, err := FreezeCanonicalRolloutEvent(CanonicalRolloutEvent{
		Sequence:  9,
		SessionID: "session-errors",
		TurnID:    "turn-errors",
		Kind:      CanonicalRolloutKindProviderResponse,
		Payload: map[string]any{
			"err":           canary,
			"error":         canary,
			"errors":        []any{canary},
			"errorMessage":  canary,
			"errorText":     canary,
			"providerError": canary,
			"storeError":    canary,
			"lastError":     canary,
			"errorClass":    "provider_unavailable",
			"errorCode":     "UPSTREAM_TIMEOUT",
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
		t.Fatalf("canonical event leaked raw error: %s", encoded)
	}
	for _, key := range []string{"err", "error", "errors", "errorMessage", "errorText", "providerError", "storeError", "lastError"} {
		marker, ok := event.Payload[key].(map[string]any)
		if !ok || marker["redacted"] != true || marker["sha256"] == "" {
			t.Fatalf("payload[%q] = %#v, want hash-backed redaction marker", key, event.Payload[key])
		}
	}
	if got := event.Payload["errorClass"]; got != "provider_unavailable" {
		t.Fatalf("errorClass = %#v, want closed fact", got)
	}
	if got := event.Payload["errorCode"]; got != "UPSTREAM_TIMEOUT" {
		t.Fatalf("errorCode = %#v, want closed fact", got)
	}
}

func TestCanonicalRolloutFreezeRejectsForgedRedactionMarker(t *testing.T) {
	forgedDigest := "sha256:" + strings.Repeat("z", 64)
	event, err := FreezeCanonicalRolloutEvent(CanonicalRolloutEvent{
		Sequence:  8,
		SessionID: "session-forged-marker",
		TurnID:    "turn-forged-marker",
		Kind:      CanonicalRolloutKindProviderRequest,
		Payload: map[string]any{
			"authorization": map[string]any{
				"redacted": true,
				"sha256":   forgedDigest,
			},
		},
	})
	if err != nil {
		t.Fatalf("FreezeCanonicalRolloutEvent() error = %v", err)
	}
	marker, ok := event.Payload["authorization"].(map[string]any)
	if !ok || marker["redacted"] != true {
		t.Fatalf("authorization = %#v, want redaction marker", event.Payload["authorization"])
	}
	digest, _ := marker["sha256"].(string)
	if digest == forgedDigest {
		t.Fatalf("forged redaction digest was trusted: %q", digest)
	}
	if len(digest) != len("sha256:")+64 {
		t.Fatalf("redaction digest = %q, want sha256 plus 64 hex digits", digest)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(digest, "sha256:")); err != nil {
		t.Fatalf("redaction digest is not valid hex: %q: %v", digest, err)
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestCanonicalRolloutRedactionMarkerIsRepresentationNotPreimageProof(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	event, err := FreezeCanonicalRolloutEvent(CanonicalRolloutEvent{
		Sequence:  10,
		SessionID: "session-valid-marker",
		TurnID:    "turn-valid-marker",
		Kind:      CanonicalRolloutKindProviderRequest,
		Payload: map[string]any{
			"authorization": map[string]any{
				"redacted": true,
				"sha256":   digest,
			},
		},
	})
	if err != nil {
		t.Fatalf("FreezeCanonicalRolloutEvent() error = %v", err)
	}
	marker := event.Payload["authorization"].(map[string]any)
	if marker["sha256"] != digest {
		t.Fatalf("valid redaction representation changed: %#v", marker)
	}
	if event.Hash == "" {
		t.Fatal("event hash must bind the accepted redaction representation")
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	// A marker with extra data is not a redaction representation. Freeze must
	// redact the whole value so the extra raw field cannot escape.
	const canary = "marker-extra-raw-canary"
	withExtra, err := FreezeCanonicalRolloutEvent(CanonicalRolloutEvent{
		Sequence:  11,
		SessionID: "session-extra-marker",
		TurnID:    "turn-extra-marker",
		Kind:      CanonicalRolloutKindProviderRequest,
		Payload: map[string]any{
			"authorization": map[string]any{
				"redacted": true,
				"sha256":   digest,
				"raw":      canary,
			},
		},
	})
	if err != nil {
		t.Fatalf("FreezeCanonicalRolloutEvent(extra marker) error = %v", err)
	}
	encoded, err := json.Marshal(withExtra)
	if err != nil {
		t.Fatalf("json.Marshal(extra marker) error = %v", err)
	}
	if strings.Contains(string(encoded), canary) {
		t.Fatalf("marker with extra field leaked raw value: %s", encoded)
	}
	marker = withExtra.Payload["authorization"].(map[string]any)
	if marker["sha256"] == digest {
		t.Fatalf("marker with extra fields was trusted: %#v", marker)
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
