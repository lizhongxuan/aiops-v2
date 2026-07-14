package runtimekernel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/promptinput"
)

func TestRuntimeStepContextHashRequiresControlFacts(t *testing.T) {
	step := mustFreezeRuntimeStepContextForTest(t, validRuntimeStepContextForHashTest())
	tests := []struct {
		name   string
		mutate func(*RuntimeStepContext)
	}{
		{name: "turn assembly", mutate: func(step *RuntimeStepContext) { step.TurnAssemblyHash = "" }},
		{name: "permission", mutate: func(step *RuntimeStepContext) { step.PermissionHash = "" }},
		{name: "router", mutate: func(step *RuntimeStepContext) { step.ToolSurface.Fingerprint = "" }},
		{name: "step hash", mutate: func(step *RuntimeStepContext) { step.Hash = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := step
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestRuntimeStepContextHashChangesWithInputCheckpointAndRouter(t *testing.T) {
	baseInput := validRuntimeStepContextForHashTest()
	base := mustFreezeRuntimeStepContextForTest(t, baseInput)

	modelInputChanged := validRuntimeStepContextForHashTest()
	modelInputChanged.ModelInput[0].Content = "different input"
	modelInputChanged.ProviderRequest.Input[0].Content = "different input"
	checkpointChanged := validRuntimeStepContextForHashTest()
	checkpointChanged.CheckpointRef = "checkpoint-2"
	routerChanged := validRuntimeStepContextForHashTest()
	routerChanged.ToolSurface.Fingerprint = "router-fingerprint-2"

	for name, input := range map[string]RuntimeStepContext{
		"model input": modelInputChanged,
		"checkpoint":  checkpointChanged,
		"router":      routerChanged,
	} {
		t.Run(name, func(t *testing.T) {
			got := mustFreezeRuntimeStepContextForTest(t, input)
			if got.Hash == base.Hash {
				t.Fatalf("Hash = %q, want change from base", got.Hash)
			}
		})
	}
}

func TestRuntimeStepContextHashNormalizesWallClockButPreservesContextFacts(t *testing.T) {
	baseInput := validRuntimeStepContextForHashTest()
	baseInput.ContextState.Messages = []Message{{ID: "context-1", Role: "user", Content: "stable context", Timestamp: time.Date(2026, 7, 10, 1, 2, 3, 0, time.UTC)}}
	base := mustFreezeRuntimeStepContextForTest(t, baseInput)

	clockChanged := validRuntimeStepContextForHashTest()
	clockChanged.ContextState.Messages = []Message{{ID: "context-random-2", Role: "user", Content: "stable context", Timestamp: time.Date(2026, 7, 13, 9, 8, 7, 0, time.UTC)}}
	clockFrozen := mustFreezeRuntimeStepContextForTest(t, clockChanged)
	if clockFrozen.Hash != base.Hash {
		t.Fatalf("wall-clock changed step control hash: %q != %q", clockFrozen.Hash, base.Hash)
	}

	factChanged := validRuntimeStepContextForHashTest()
	factChanged.ContextState.Messages = []Message{{ID: "context-1", Role: "user", Content: "changed context", Timestamp: baseInput.ContextState.Messages[0].Timestamp}}
	factFrozen := mustFreezeRuntimeStepContextForTest(t, factChanged)
	if factFrozen.Hash == base.Hash {
		t.Fatal("context content change did not change step control hash")
	}
}

func TestRuntimeStepContextHashDeepFreezesAndRejectsProviderTamper(t *testing.T) {
	conflictingInput, err := cloneRuntimeStepContext(validRuntimeStepContextForHashTest())
	if err != nil {
		t.Fatalf("cloneRuntimeStepContext() error = %v", err)
	}
	conflictingInput.ModelInput[0].Content = "non-authoritative-shadow"
	if _, err := FreezeRuntimeStepContext(conflictingInput); err == nil {
		t.Fatal("FreezeRuntimeStepContext() accepted model/provider input conflict")
	}

	input := validRuntimeStepContextForHashTest()
	frozen := mustFreezeRuntimeStepContextForTest(t, input)
	input.ModelInput[0].Metadata["scope"] = "mutated"
	input.ModelInput[0].ToolCalls[0].Arguments[0] = 'X'
	input.ToolSurface.HiddenReasons["danger"][0] = "mutated"
	input.ProviderRequest.ClientMetadata["sessionId"] = "mutated"
	input.ProviderRequest.MessageAudit.Items[0].ItemHash = "mutated"
	input.ProviderRequest.Input[0].Content = "mutated"
	if err := frozen.Validate(); err != nil {
		t.Fatalf("frozen.Validate() error after input mutation = %v", err)
	}
	data, _ := json.Marshal(frozen)
	if strings.Contains(string(data), "mutated") {
		t.Fatalf("frozen step changed with source mutation: %s", data)
	}

	tamperedInput, err := cloneRuntimeStepContext(frozen)
	if err != nil {
		t.Fatalf("cloneRuntimeStepContext() error = %v", err)
	}
	tamperedInput.ProviderRequest.Input[0].Content = "tampered"
	if err := tamperedInput.Validate(); err == nil {
		t.Fatal("Validate() accepted tampered provider input")
	}
	tamperedHash := frozen
	tamperedHash.ProviderRequest.ModelInputHash = "tampered"
	if err := tamperedHash.Validate(); err == nil {
		t.Fatal("Validate() accepted tampered provider model input hash")
	}
	tamperedFingerprint := frozen
	tamperedFingerprint.ProviderRequest.PromptFingerprint.ModelInputHash = "tampered"
	if err := tamperedFingerprint.Validate(); err == nil {
		t.Fatal("Validate() accepted tampered provider prompt fingerprint")
	}
	tamperedMetadata, err := cloneRuntimeStepContext(frozen)
	if err != nil {
		t.Fatalf("cloneRuntimeStepContext() error = %v", err)
	}
	tamperedMetadata.ProviderRequest.ClientMetadata["turnId"] = "tampered"
	if err := tamperedMetadata.Validate(); err == nil {
		t.Fatal("Validate() accepted tampered provider client metadata")
	}

	providerRequest, err := frozen.ValidatedProviderRequest()
	if err != nil {
		t.Fatalf("ValidatedProviderRequest() error = %v", err)
	}
	providerRequest.Input[0].Content = "caller mutation"
	if frozen.ProviderRequest.Input[0].Content == "caller mutation" {
		t.Fatal("ValidatedProviderRequest returned shared input")
	}
}

func TestRuntimeStepContextTraceStoresHashWithoutSecret(t *testing.T) {
	input := validRuntimeStepContextForHashTest()
	input.Turn.Metadata = map[string]string{"apiKey": "secret-canary-step"}
	input.ModelInput[0].Content = "token=secret-canary-step"
	input.ProviderRequest.Input[0].Content = "token=secret-canary-step"
	step := mustFreezeRuntimeStepContextForTest(t, input)
	dir := t.TempDir()
	path, err := writeRuntimeStepTrace(modeltrace.Config{Enabled: true, RootDir: dir}, step, RuntimeTraceDebugRequest{ModelInput: []promptinput.ModelInputItem{{
		ID: "trace-shadow", ProviderRole: promptinput.ProviderRoleUser, Content: "trace-shadow-sentinel",
	}}})
	if err != nil {
		t.Fatalf("writeRuntimeStepTrace() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(trace) error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal(trace) error = %v", err)
	}
	if payload["stepContextHash"] != step.Hash {
		t.Fatalf("stepContextHash = %#v, want %q", payload["stepContextHash"], step.Hash)
	}
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(content), "secret-canary-step") {
			t.Fatalf("trace file %s leaked secret canary", path)
		}
		if strings.Contains(string(content), "trace-shadow-sentinel") {
			t.Fatalf("trace file %s used non-authoritative caller model input", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(trace) error = %v", err)
	}
}

func TestRuntimeStepContextTraceCarriesPreviousPromptFingerprint(t *testing.T) {
	step := mustFreezeRuntimeStepContextForTest(t, validRuntimeStepContextForHashTest())
	dir := t.TempDir()
	path, err := writeRuntimeStepTrace(modeltrace.Config{Enabled: true, RootDir: dir}, step, RuntimeTraceDebugRequest{
		PreviousPromptFingerprint: map[string]string{"stableHash": "previous-stable"},
	})
	if err != nil {
		t.Fatalf("writeRuntimeStepTrace() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(trace) error = %v", err)
	}
	var payload modeltrace.TraceDocumentV2
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal(trace) error = %v", err)
	}
	if payload.PreviousPromptFingerprint["stableHash"] != "previous-stable" {
		t.Fatalf("previousPromptFingerprint = %#v", payload.PreviousPromptFingerprint)
	}
}

func TestLatestRuntimePromptFingerprintUsesLastCompletedIterationAndClones(t *testing.T) {
	snapshot := &TurnSnapshot{Iterations: []IterationState{
		{PromptFingerprint: map[string]string{"stableHash": "first"}},
		{PromptFingerprint: map[string]string{"stableHash": "latest"}},
	}}
	got := latestRuntimePromptFingerprint(snapshot)
	if got["stableHash"] != "latest" {
		t.Fatalf("latestRuntimePromptFingerprint() = %#v", got)
	}
	got["stableHash"] = "mutated"
	if snapshot.Iterations[1].PromptFingerprint["stableHash"] != "latest" {
		t.Fatal("latestRuntimePromptFingerprint() returned shared state")
	}
}

func validRuntimeStepContextForHashTest() RuntimeStepContext {
	items := []promptinput.ModelInputItem{{
		ID: "user-1", ProviderRole: promptinput.ProviderRoleUser, SemanticRole: "user_request", Content: "inspect host",
		ToolCalls: []promptinput.ModelInputToolCall{{ID: "call-1", Name: "host_read", Arguments: json.RawMessage(`{"host":"a"}`)}},
		Metadata:  map[string]string{"scope": "host-a"},
	}}
	audit, _ := modelrouter.ProviderMessageAuditFromModelInputItems(items)
	providerRequest := modelrouter.ProviderRequestSnapshot{
		Provider: "mock", Model: "mock", Input: items,
		Tools:                []modelrouter.ProviderToolSpec{{Name: "host_read", Hash: "router-fingerprint-1"}},
		ClientMetadata:       map[string]string{"sessionId": "session-1", "turnId": "turn-1"},
		MessageAudit:         &audit,
		ProviderMessagesHash: audit.ProviderMessagesHash,
	}
	providerRequest.ComputeHashes()
	return RuntimeStepContext{
		Turn:             RuntimeTurnContext{SessionID: "session-1", TurnID: "turn-1", SessionType: SessionTypeHost, Mode: ModeInspect},
		TurnAssemblyHash: "assembly-hash-1",
		PermissionHash:   "permission-hash-1",
		CheckpointRef:    "checkpoint-1",
		Iteration:        1,
		ModelInput:       items,
		ToolSurface: RuntimeToolRouterSnapshot{
			RegisteredTools: []string{"host_read"}, ModelVisibleTools: []string{"host_read"}, DispatchableTools: []string{"host_read"},
			HiddenReasons: map[string][]string{"danger": {"policy_denied"}}, PolicyHash: "policy-hash-1", Fingerprint: "router-fingerprint-1",
		},
		ProviderRequest: providerRequest,
	}
}

func mustFreezeRuntimeStepContextForTest(t *testing.T, input RuntimeStepContext) RuntimeStepContext {
	t.Helper()
	step, err := FreezeRuntimeStepContext(input)
	if err != nil {
		t.Fatalf("FreezeRuntimeStepContext() error = %v", err)
	}
	return step
}
