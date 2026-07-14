package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/runtimekernel"
)

func TestRolloutReplayContractRebuildsTypedFactsWithoutProviderOrTool(t *testing.T) {
	fixture := replayFixtureForTest(t, "contract")
	backend := &replayBackendForTest{}
	report, err := (RolloutReplayRunner{Backend: backend}).Replay(context.Background(), ReplayContract, fixture)
	if err != nil {
		t.Fatalf("Replay(contract) error = %v", err)
	}
	if backend.providerCalls != 0 || backend.fullStoryCalls != 0 {
		t.Fatalf("contract replay called backend: provider=%d full=%d", backend.providerCalls, backend.fullStoryCalls)
	}
	if report.Mode != ReplayContract || report.ComparedEvents == 0 || report.HeadHash != fixture.Rollout[len(fixture.Rollout)-1].Hash {
		t.Fatalf("contract report = %#v", report)
	}
}

func TestRolloutReplayContractReportsTypedPermissionTamperAtPrompt(t *testing.T) {
	fixture := replayFixtureForTest(t, "contract-permission-tamper")
	fixture.Contract.Steps[0].PermissionHash = "sha256:tampered-permission"
	_, err := (RolloutReplayRunner{}).Replay(context.Background(), ReplayContract, fixture)
	divergence := requireReplayDivergence(t, err)
	if divergence.Sequence != 3 || divergence.ExpectedKind != modeltrace.CanonicalRolloutKindPrompt || divergence.OwnerModule != "runtimekernel" {
		t.Fatalf("divergence = %#v, want first prompt/runtimekernel", divergence)
	}
}

func TestRolloutReplayProviderFixtureUsesSavedResponseAndFindsFirstToolDivergence(t *testing.T) {
	fixture := replayFixtureForTest(t, "provider")
	backend := &replayBackendForTest{providerExecution: ReplayExecution{Rollout: cloneReplayEvents(fixture.Rollout), Contract: fixture.Contract}}
	report, err := (RolloutReplayRunner{Backend: backend}).Replay(context.Background(), ReplayProviderFixture, fixture)
	if err != nil {
		t.Fatalf("Replay(provider_fixture) error = %v", err)
	}
	if backend.providerCalls != 1 || backend.lastProviderResponses != len(fixture.ProviderResponses) || report.Mode != ReplayProviderFixture {
		t.Fatalf("provider replay did not consume saved response: backend=%#v report=%#v", backend, report)
	}

	actual := cloneReplayEvents(fixture.Rollout)
	toolIndex := replayEventIndex(actual, modeltrace.CanonicalRolloutKindToolProposed)
	actual[toolIndex] = refreezeReplayEvent(t, actual[toolIndex], func(event *modeltrace.CanonicalRolloutEvent) {
		event.Payload["name"] = "tampered_tool"
	})
	backend.providerExecution.Rollout = actual
	_, err = (RolloutReplayRunner{Backend: backend}).Replay(context.Background(), ReplayProviderFixture, fixture)
	divergence := requireReplayDivergence(t, err)
	if divergence.Sequence != fixture.Rollout[toolIndex].Sequence || divergence.ExpectedKind != modeltrace.CanonicalRolloutKindToolProposed || divergence.OwnerModule != "runtimekernel.ToolDispatcher" {
		t.Fatalf("divergence = %#v, want first tool proposal", divergence)
	}
}

func TestRolloutReplayFullStoryRunsTransportCommandToTypedState(t *testing.T) {
	fixture := replayFixtureForTest(t, "full-story")
	state := fullStoryTransportStateForTest(t, fixture.TransportCommand)
	fixture.ExpectedTransport = &state
	backend := &replayBackendForTest{fullStoryExecution: ReplayExecution{
		Rollout:        cloneReplayEvents(fixture.Rollout),
		TransportState: ptrReplayTransportState(state),
		Contract:       fixture.Contract,
	}}
	report, err := (RolloutReplayRunner{Backend: backend}).Replay(context.Background(), ReplayFullStory, fixture)
	if err != nil {
		t.Fatalf("Replay(full_story) error = %v", err)
	}
	if backend.fullStoryCalls != 1 || report.TransportHash == "" || report.Mode != ReplayFullStory {
		t.Fatalf("full-story report/backend = %#v / %#v", report, backend)
	}

	backend.fullStoryExecution.TransportState.Status = appui.AiopsTransportStatusFailed
	_, err = (RolloutReplayRunner{Backend: backend}).Replay(context.Background(), ReplayFullStory, fixture)
	divergence := requireReplayDivergence(t, err)
	if divergence.OwnerModule != "appui.TransportProjector" || divergence.ExpectedKind != modeltrace.CanonicalRolloutKindTransportProjection {
		t.Fatalf("transport divergence = %#v", divergence)
	}
}

func TestRolloutReplayPromptAndPermissionTamperReportFirstDivergentEvent(t *testing.T) {
	fixture := replayFixtureForTest(t, "first-divergence")
	tests := []struct {
		name  string
		kind  string
		field string
		value any
		owner string
	}{
		{name: "prompt hash", kind: modeltrace.CanonicalRolloutKindPrompt, field: "modelInputHash", value: "sha256:tampered-prompt", owner: "promptcompiler"},
		{name: "permission hash", kind: modeltrace.CanonicalRolloutKindApprovalRequested, field: "permissionHash", value: "sha256:tampered-permission", owner: "runtimekernel.approval"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := cloneReplayEvents(fixture.Rollout)
			index := replayEventIndex(actual, tc.kind)
			actual[index] = refreezeReplayEvent(t, actual[index], func(event *modeltrace.CanonicalRolloutEvent) {
				event.Payload[tc.field] = tc.value
			})
			backend := &replayBackendForTest{providerExecution: ReplayExecution{Rollout: actual, Contract: fixture.Contract}}
			_, err := (RolloutReplayRunner{Backend: backend}).Replay(context.Background(), ReplayProviderFixture, fixture)
			divergence := requireReplayDivergence(t, err)
			if divergence.Sequence != fixture.Rollout[index].Sequence || divergence.OwnerModule != tc.owner || divergence.ExpectedHash == "" || divergence.ActualHash == "" {
				t.Fatalf("divergence = %#v", divergence)
			}
		})
	}
}

func TestRolloutReplayTamperedControlHashFailsAllModesAtFirstEvent(t *testing.T) {
	for _, tc := range []struct {
		name, kind, field string
	}{
		{name: "prompt", kind: modeltrace.CanonicalRolloutKindPrompt, field: "modelInputHash"},
		{name: "tool", kind: modeltrace.CanonicalRolloutKindToolProposed, field: "argsHash"},
		{name: "permission", kind: modeltrace.CanonicalRolloutKindApprovalRequested, field: "permissionHash"},
	} {
		for _, mode := range []ReplayMode{ReplayContract, ReplayProviderFixture, ReplayFullStory} {
			t.Run(tc.name+"/"+string(mode), func(t *testing.T) {
				fixture := replayFixtureForTest(t, tc.name)
				index := replayEventIndex(fixture.Rollout, tc.kind)
				fixture.Rollout[index].Payload[tc.field] = "sha256:tampered-without-valid-event-hash"
				backend := &replayBackendForTest{}
				_, err := (RolloutReplayRunner{Backend: backend}).Replay(context.Background(), mode, fixture)
				divergence := requireReplayDivergence(t, err)
				if divergence.Sequence != fixture.Rollout[index].Sequence || divergence.OwnerModule != replayOwner(tc.kind) {
					t.Fatalf("divergence = %#v", divergence)
				}
				if backend.providerCalls != 0 || backend.fullStoryCalls != 0 {
					t.Fatalf("tampered fixture reached backend: %#v", backend)
				}
			})
		}
	}
}

func TestRolloutReplayNormalizerOnlyIgnoresClockAndRandomIDs(t *testing.T) {
	left := map[string]any{
		"updatedAt": "2026-07-10T00:00:00Z", "messageId": "message-random-a", "spanId": "span-random-a",
		"kind": "tool_dispatched", "hash": "sha256:control-a", "toolCallId": "call-a", "approvalId": "approval-a", "toolId": "exec_command",
	}
	right := map[string]any{
		"updatedAt": "2026-07-13T00:00:00Z", "messageId": "message-random-b", "spanId": "span-random-b",
		"kind": "tool_dispatched", "hash": "sha256:control-a", "toolCallId": "call-a", "approvalId": "approval-a", "toolId": "exec_command",
	}
	leftHash, err := normalizedReplayFactHash(left)
	if err != nil {
		t.Fatal(err)
	}
	rightHash, err := normalizedReplayFactHash(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftHash != rightHash {
		t.Fatalf("clock/random ID normalization drift: %q != %q", leftHash, rightHash)
	}
	for key, value := range map[string]string{"kind": "tool_result", "hash": "sha256:control-b", "toolCallId": "call-b", "approvalId": "approval-b", "toolId": "other"} {
		changed := cloneReplayMap(right)
		changed[key] = value
		changedHash, hashErr := normalizedReplayFactHash(changed)
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		if changedHash == rightHash {
			t.Fatalf("normalizer ignored control field %q", key)
		}
	}
}

func TestRolloutReplayNormalizerPreservesRandomIDReferencesAcrossEvents(t *testing.T) {
	fixture := replayFixtureForTest(t, "random-id-reference")
	expected := cloneReplayEvents(fixture.Rollout[:2])
	expected[0] = refreezeReplayEvent(t, expected[0], func(event *modeltrace.CanonicalRolloutEvent) { event.Payload["messageId"] = "random-a" })
	expected[1] = refreezeReplayEvent(t, expected[1], func(event *modeltrace.CanonicalRolloutEvent) { event.Payload["messageId"] = "random-a" })
	actual := cloneReplayEvents(expected)
	actual[0] = refreezeReplayEvent(t, actual[0], func(event *modeltrace.CanonicalRolloutEvent) { event.Payload["messageId"] = "random-b" })
	actual[1] = refreezeReplayEvent(t, actual[1], func(event *modeltrace.CanonicalRolloutEvent) { event.Payload["messageId"] = "random-c" })
	_, err := compareReplayRollouts(expected, actual)
	divergence := requireReplayDivergence(t, err)
	if divergence.Sequence != 2 {
		t.Fatalf("divergence sequence = %d, want second broken reference", divergence.Sequence)
	}
}

func TestRolloutReplayFixtureLoaderFailsClosedOnUnknownVersion(t *testing.T) {
	_, err := LoadRolloutReplayFixture(strings.NewReader(`{"schemaVersion":"aiops.rollout-replay.v999","name":"future"}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported rollout replay fixture schema") {
		t.Fatalf("LoadRolloutReplayFixture() error = %v", err)
	}
}

func TestRolloutReplayFixtureFileRejectsSourceTraversal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.json")
	data := []byte(`{"schemaVersion":"aiops.rollout-replay.fixture.v1","name":"escape","source":{"kind":"assistant_transport_story","path":"../../../../etc/passwd","hash":"sha256:forged"},"baseline":{"eventCount":1,"rolloutHash":"sha256:forged","transportHash":"sha256:forged"}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadRolloutReplayFixtureFile(path)
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("LoadRolloutReplayFixtureFile() error = %v, want traversal rejection", err)
	}
}

func TestRuntimeReplayUsesFrozenSourceBytesAfterFileReplacement(t *testing.T) {
	root := t.TempDir()
	fixtureDir := filepath.Join(root, "internal", "eval", "testdata", "rollout_replay")
	sourceDir := filepath.Join(root, "internal", "server", "testdata", "assistant_transport_story")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := []byte(`{"name":"bound","command":{"type":"add-message","message":{"id":"message-1","parts":[{"text":"hello"}]}},"providerResponses":[{"role":"assistant","content":"ok"}],"toolOutcomes":[]}`)
	sourcePath := filepath.Join(sourceDir, "bound.json")
	if err := os.WriteFile(sourcePath, source, 0o600); err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(fixtureDir, "bound.json")
	eventHash := "sha256:baseline"
	rolloutHash, err := normalizedReplayFactHash([]string{eventHash})
	if err != nil {
		t.Fatal(err)
	}
	fixtureJSON, err := json.Marshal(RolloutReplayFixture{
		SchemaVersion: RolloutReplayFixtureSchemaVersion,
		Name:          "bound",
		Source:        RolloutReplaySource{Kind: "assistant_transport_story", Path: "../../../server/testdata/assistant_transport_story/bound.json", Hash: replaySourceContentHash(source)},
		Baseline:      ReplayBaseline{EventCount: 1, Events: []ReplayBaselineEvent{{Sequence: 1, Kind: modeltrace.CanonicalRolloutKindAdmission, Hash: eventHash}}, RolloutHash: rolloutHash, TransportHash: "sha256:transport"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixturePath, fixtureJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	fixture, err := LoadRolloutReplayFixtureFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte(`{"name":"replaced"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	story, err := loadRuntimeReplayStory(fixture)
	if err != nil || story.Name != "bound" {
		t.Fatalf("loadRuntimeReplayStory() = %#v, %v; want frozen bound source", story, err)
	}
}

func TestRuntimeReplayRejectsReaderFixtureWithoutBoundSource(t *testing.T) {
	fixture, err := LoadRolloutReplayFixture(strings.NewReader(`{"schemaVersion":"aiops.rollout-replay.fixture.v1","name":"unbound","source":{"kind":"assistant_transport_story","path":"story.json","hash":"sha256:forged"}}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewRuntimeRolloutReplayBackend().ReplayFullStory(context.Background(), fixture)
	if err == nil || !strings.Contains(err.Error(), "source bound") {
		t.Fatalf("ReplayFullStory() error = %v, want unbound source rejection", err)
	}
}

func TestRolloutReplayReferenceFixturesCoverApprovalResumeAndToolNotFound(t *testing.T) {
	for _, name := range []string{"approval_resume", "tool_not_found"} {
		path := filepath.Join("testdata", "rollout_replay", name+".json")
		fixture, loadErr := LoadRolloutReplayFixtureFile(path)
		if loadErr != nil {
			t.Fatalf("load %s: %v", path, loadErr)
		}
		if fixture.Name != name || fixture.Source.Path == "" || fixture.Source.Hash == "" {
			t.Fatalf("fixture %s = %#v", path, fixture)
		}
	}
}

func TestRolloutReplayPersistedBaselineReportsFirstDivergentEvent(t *testing.T) {
	execution := ReplayExecution{Rollout: replayFixtureForTest(t, "baseline").Rollout}
	expected, err := replayExecutionBaseline(execution)
	if err != nil {
		t.Fatal(err)
	}
	actual := expected
	actual.Events = append([]ReplayBaselineEvent(nil), expected.Events...)
	actual.Events[2].Hash = "sha256:tampered-prompt"
	err = compareReplayBaselines(expected, actual)
	divergence := requireReplayDivergence(t, err)
	if divergence.Sequence != expected.Events[2].Sequence || divergence.ExpectedKind != expected.Events[2].Kind || divergence.OwnerModule != replayOwner(expected.Events[2].Kind) {
		t.Fatalf("baseline divergence = %#v", divergence)
	}
}

func TestRolloutReplayPersistedBaselineAttributesTransportDriftToProjector(t *testing.T) {
	execution := ReplayExecution{Rollout: replayFixtureForTest(t, "baseline-transport").Rollout}
	expected, err := replayExecutionBaseline(execution)
	if err != nil {
		t.Fatal(err)
	}
	actual := expected
	actual.TransportHash = "sha256:tampered-transport"
	err = compareReplayBaselines(expected, actual)
	divergence := requireReplayDivergence(t, err)
	if divergence.ExpectedKind != modeltrace.CanonicalRolloutKindTransportProjection || divergence.OwnerModule != "appui.TransportProjector" {
		t.Fatalf("transport baseline divergence = %#v", divergence)
	}
}

func TestRolloutReplayReferenceFixturesExecuteRealRuntimeAndTransport(t *testing.T) {
	for _, name := range []string{"approval_resume", "tool_not_found"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", "rollout_replay", name+".json")
			fixture, err := LoadRolloutReplayFixtureFile(path)
			if err != nil {
				t.Fatal(err)
			}
			backend := NewRuntimeRolloutReplayBackend()
			runner := RolloutReplayRunner{Backend: backend}
			fixture, err = runner.Hydrate(context.Background(), fixture)
			if err != nil {
				t.Fatalf("hydrate real full story: %v", err)
			}
			if len(fixture.Rollout) == 0 || fixture.ExpectedTransport == nil || fixture.Contract.TurnAssembly.Hash == "" || len(fixture.Contract.Steps) == 0 || fixture.Contract.FinalContract.SchemaVersion == "" {
				t.Fatalf("real capture is incomplete: events=%d state=%v contract=%#v", len(fixture.Rollout), fixture.ExpectedTransport != nil, fixture.Contract)
			}

			if _, err := (RolloutReplayRunner{}).Replay(context.Background(), ReplayContract, fixture); err != nil {
				t.Fatalf("contract replay over real capture: %v", err)
			}
			if _, err := runner.Replay(context.Background(), ReplayProviderFixture, fixture); err != nil {
				providerActual, providerErr := backend.ReplayProviderFixture(context.Background(), fixture)
				if providerErr != nil {
					t.Fatalf("provider replay failed: %v; diagnostic replay: %v", err, providerErr)
				}
				index := 0
				if divergence := requireReplayDivergence(t, err); divergence.Sequence > 0 {
					index = int(divergence.Sequence - 1)
				}
				wantJSON, _ := json.Marshal(fixture.Rollout[index])
				gotJSON, _ := json.Marshal(providerActual.Rollout[index])
				stepDiff := ""
				if len(fixture.Contract.Steps) > 1 && len(providerActual.Contract.Steps) > 1 {
					stepDiff = firstReplayJSONDifference(fixture.Contract.Steps[1], providerActual.Contract.Steps[1])
				}
				t.Fatalf("provider replay over real runtime: %v\nwant=%s\n got=%s\nstepDiff=%s", err, wantJSON, gotJSON, stepDiff)
			}
			if _, err := runner.Replay(context.Background(), ReplayFullStory, fixture); err != nil {
				fullActual, fullErr := backend.ReplayFullStory(context.Background(), fixture)
				if fullErr != nil {
					t.Fatalf("full-story replay failed: %v; diagnostic replay: %v", err, fullErr)
				}
				t.Fatalf("full-story replay over real transport: %v\nstateDiff=%s", err, firstReplayJSONDifference(normalizedReplayDebugValue(fixture.ExpectedTransport), normalizedReplayDebugValue(fullActual.TransportState)))
			}
		})
	}
}

type replayBackendForTest struct {
	providerCalls         int
	fullStoryCalls        int
	lastProviderResponses int
	providerExecution     ReplayExecution
	fullStoryExecution    ReplayExecution
}

func (b *replayBackendForTest) ReplayProviderFixture(_ context.Context, fixture RolloutReplayFixture) (ReplayExecution, error) {
	b.providerCalls++
	b.lastProviderResponses = len(fixture.ProviderResponses)
	return b.providerExecution, nil
}

func (b *replayBackendForTest) ReplayFullStory(_ context.Context, _ RolloutReplayFixture) (ReplayExecution, error) {
	b.fullStoryCalls++
	return b.fullStoryExecution, nil
}

func replayFixtureForTest(t *testing.T, name string) RolloutReplayFixture {
	t.Helper()
	facts, err := runtimecontract.BuildAdmissionFacts(runtimecontract.AdmissionInput{
		Intent:  &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindDiagnose, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly}},
		Profile: "advisor", SessionTarget: resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assembly, err := agentassembly.BuildTurnAssembly(agentassembly.TurnAssemblyInput{
		AdmissionFacts: facts, PermissionProfile: "workspace-default",
		CapabilityPolicy:    agentassembly.CapabilityPolicySnapshot{PolicyHash: "sha256:capability-policy"},
		ContextPolicy:       agentassembly.ContextSelectorSnapshot{Policy: "bounded", Budget: "default"},
		LoopPolicy:          agentassembly.LoopPolicySnapshot{MaxIterations: 6, ToolCallPolicy: "governed"},
		FinalContractPolicy: agentassembly.FinalContractSnapshot{Shape: "typed-final"},
		RollbackPolicy:      "not-required-for-read-only", SourceRefs: []string{"policy:v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	items := []promptinput.ModelInputItem{{ID: "user-1", ProviderRole: promptinput.ProviderRoleUser, SemanticRole: "user_request", Content: "inspect host"}}
	audit, err := modelrouter.ProviderMessageAuditFromModelInputItems(items)
	if err != nil {
		t.Fatal(err)
	}
	providerRequest := modelrouter.ProviderRequestSnapshot{
		Provider: "fixture", Model: "fixture", Input: items,
		Tools:          []modelrouter.ProviderToolSpec{{Name: "host_read", Hash: "router-fingerprint-1"}},
		ClientMetadata: map[string]string{"sessionId": "session-1", "turnId": "turn-1"}, MessageAudit: &audit, ProviderMessagesHash: audit.ProviderMessagesHash,
	}
	providerRequest.ComputeHashes()
	step, err := runtimekernel.FreezeRuntimeStepContext(runtimekernel.RuntimeStepContext{
		Turn:             runtimekernel.RuntimeTurnContext{SessionID: "session-1", TurnID: "turn-1", SessionType: runtimekernel.SessionTypeHost, Mode: runtimekernel.ModeInspect},
		TurnAssemblyHash: assembly.Hash, PermissionHash: "sha256:permission", Iteration: 1, ModelInput: items,
		ToolSurface: runtimekernel.RuntimeToolRouterSnapshot{
			RegisteredTools: []string{"host_read"}, ModelVisibleTools: []string{"host_read"}, DispatchableTools: []string{"host_read"},
			PolicyHash: "sha256:tool-policy", Fingerprint: "router-fingerprint-1",
		},
		ProviderRequest: providerRequest,
	})
	if err != nil {
		t.Fatal(err)
	}
	actionToken, err := runtimekernel.FreezeActionToken(runtimekernel.ActionToken{
		ApprovalID: "approval-1", TurnID: "turn-1", ToolCallID: "call-1", ToolName: "host_read",
		ArgumentsHash: "sha256:args", TargetRefs: []string{"host:host-a"}, ToolSurfaceFingerprint: step.ToolSurface.Fingerprint,
		PermissionHash: step.PermissionHash, RollbackHash: "sha256:rollback", CheckpointID: "checkpoint-1",
		ExpiresAt: time.Date(2026, 7, 10, 2, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	finalFacts := runtimekernel.FinalRuntimeFacts{
		CompletionStatus: runtimekernel.FinalCompletionStatusPartial, ToolOutcomes: []string{"host_read:call-1:failed"},
		PostcheckStatus: runtimekernel.FinalPostcheckStatusNotRequired, RollbackStatus: runtimekernel.FinalRollbackStatusNotRequired,
		FailureCodes: []string{"tool_not_found"},
	}
	finalContract := runtimekernel.BuildTerminalFinalContract("fixture answer", runtimekernel.FinalContractStatusToolUnavailable, finalFacts.FailureCodes)
	finalFactsHash := replayRawFactHash(t, finalFacts)
	contractForHash := finalContract
	contractForHash.AnswerText = ""
	finalContractHash := replayRawFactHash(t, contractForHash)

	var events []modeltrace.CanonicalRolloutEvent
	appendEvent := func(kind string, event modeltrace.CanonicalRolloutEvent) {
		event.Kind = kind
		event.Sequence = int64(len(events) + 1)
		event.SessionID, event.TurnID = "session-1", "turn-1"
		if len(events) > 0 {
			event.SourceRefs = append(event.SourceRefs, events[len(events)-1].EventID)
		}
		frozen, freezeErr := modeltrace.FreezeCanonicalRolloutEvent(event)
		if freezeErr != nil {
			t.Fatalf("freeze %s: %v", kind, freezeErr)
		}
		events = append(events, frozen)
	}
	appendEvent(modeltrace.CanonicalRolloutKindAdmission, modeltrace.CanonicalRolloutEvent{Payload: map[string]any{"status": "accepted"}})
	appendEvent(modeltrace.CanonicalRolloutKindAssembly, modeltrace.CanonicalRolloutEvent{TurnAssemblyHash: assembly.Hash, Payload: map[string]any{"schemaVersion": assembly.SchemaVersion}})
	appendEvent(modeltrace.CanonicalRolloutKindPrompt, modeltrace.CanonicalRolloutEvent{StepID: step.Hash, TurnAssemblyHash: assembly.Hash, StepContextHash: step.Hash, Payload: map[string]any{"modelInputHash": step.ProviderRequest.ModelInputHash}})
	appendEvent(modeltrace.CanonicalRolloutKindProviderRequest, modeltrace.CanonicalRolloutEvent{StepID: step.Hash, TurnAssemblyHash: assembly.Hash, StepContextHash: step.Hash, Payload: map[string]any{"modelInputHash": step.ProviderRequest.ModelInputHash}})
	appendEvent(modeltrace.CanonicalRolloutKindProviderResponse, modeltrace.CanonicalRolloutEvent{StepID: step.Hash, TurnAssemblyHash: assembly.Hash, StepContextHash: step.Hash, Payload: map[string]any{"outputHash": "sha256:fixture-response", "toolCallCount": 1}})
	appendEvent(modeltrace.CanonicalRolloutKindToolProposed, modeltrace.CanonicalRolloutEvent{StepID: step.Hash, TurnAssemblyHash: assembly.Hash, StepContextHash: step.Hash, Payload: map[string]any{"callId": "call-1", "name": "host_read", "argsHash": "sha256:args"}})
	appendEvent(modeltrace.CanonicalRolloutKindToolDispatched, modeltrace.CanonicalRolloutEvent{StepID: step.Hash, TurnAssemblyHash: assembly.Hash, StepContextHash: step.Hash, Payload: map[string]any{"callId": "call-1", "name": "host_read", "argsHash": "sha256:args"}})
	appendEvent(modeltrace.CanonicalRolloutKindApprovalRequested, modeltrace.CanonicalRolloutEvent{StepID: step.Hash, TurnAssemblyHash: assembly.Hash, StepContextHash: step.Hash, Payload: map[string]any{
		"approvalId": actionToken.ApprovalID, "toolCallId": actionToken.ToolCallID, "permissionHash": actionToken.PermissionHash,
		"rollbackHash": actionToken.RollbackHash, "checkpointId": actionToken.CheckpointID, "actionTokenHash": actionToken.Hash,
		"toolName": actionToken.ToolName, "argsHash": actionToken.ArgumentsHash, "targetRefs": actionToken.TargetRefs,
		"toolSurfaceFingerprint": actionToken.ToolSurfaceFingerprint,
	}})
	appendEvent(modeltrace.CanonicalRolloutKindFinalFacts, modeltrace.CanonicalRolloutEvent{StepID: step.Hash, TurnAssemblyHash: assembly.Hash, StepContextHash: step.Hash, Payload: map[string]any{"finalRuntimeFactsHash": finalFactsHash, "finalContractHash": finalContractHash}})
	appendEvent(modeltrace.CanonicalRolloutKindTransportProjection, modeltrace.CanonicalRolloutEvent{StepID: step.Hash, TurnAssemblyHash: assembly.Hash, StepContextHash: step.Hash, Payload: map[string]any{"projectionInputHash": "sha256:projection"}})

	return RolloutReplayFixture{
		SchemaVersion: RolloutReplayFixtureSchemaVersion, Name: name, Rollout: events,
		Contract:          ReplayContractArtifacts{TurnAssembly: assembly, Steps: []runtimekernel.RuntimeStepContext{step}, ActionTokens: []runtimekernel.ActionToken{actionToken}, FinalRuntimeFacts: finalFacts, FinalContract: finalContract},
		ProviderResponses: []json.RawMessage{json.RawMessage(`{"content":"","toolCalls":[{"id":"call-1","name":"host_read","arguments":{}}]}`)},
		TransportCommand: appui.TransportCommand{Type: appui.TransportCommandTypeAddMessage, AddMessage: &appui.TransportAddMessageCommand{
			SessionID: "session-1", SessionType: "host", Mode: "inspect", ThreadID: "thread-1", ClientMessageID: "message-1", ClientTurnID: "turn-1", Message: appui.TransportUserMessage{Text: "inspect host"},
		}},
	}
}

func fullStoryTransportStateForTest(t *testing.T, command appui.TransportCommand) appui.AiopsTransportState {
	t.Helper()
	chat := replayChatServiceForTest{}
	handler := appui.NewTransportCommandHandler(chat, nil, nil, nil)
	state, _, err := handler.Apply(context.Background(), appui.NewAiopsTransportState("session-1", "thread-1"), command)
	if err != nil {
		t.Fatal(err)
	}
	turn := &runtimekernel.TurnSnapshot{
		ID: "turn-1", SessionID: "session-1", SessionType: runtimekernel.SessionTypeHost, Mode: runtimekernel.ModeInspect,
		Lifecycle: runtimekernel.TurnLifecycleCompleted, ResumeState: runtimekernel.TurnResumeStateNone, FinalOutput: "fixture answer",
		StartedAt: time.Date(2026, 7, 10, 1, 2, 3, 0, time.UTC), UpdatedAt: time.Date(2026, 7, 10, 1, 2, 4, 0, time.UTC),
	}
	state, err = appui.NewTransportProjector().ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

type replayChatServiceForTest struct{}

func (replayChatServiceForTest) SendMessage(context.Context, appui.ChatCommand) (appui.TurnResponse, error) {
	return appui.TurnResponse{SessionID: "session-1", TurnID: "turn-1", ClientTurnID: "turn-1", ClientMessageID: "message-1", Status: "running"}, nil
}
func (replayChatServiceForTest) ResumeTurn(context.Context, appui.ResumeCommand) (appui.TurnResponse, error) {
	return appui.TurnResponse{}, errors.New("unexpected resume")
}
func (replayChatServiceForTest) CancelTurn(context.Context, appui.CancelCommand) (appui.TurnResponse, error) {
	return appui.TurnResponse{}, errors.New("unexpected cancel")
}
func (replayChatServiceForTest) StopTurn(context.Context, appui.StopCommand) (appui.TurnResponse, error) {
	return appui.TurnResponse{}, errors.New("unexpected stop")
}

func requireReplayDivergence(t *testing.T, err error) *ReplayDivergenceError {
	t.Helper()
	if err == nil {
		t.Fatal("Replay() error = nil, want divergence")
	}
	var divergence *ReplayDivergenceError
	if !errors.As(err, &divergence) {
		t.Fatalf("Replay() error = %T %v, want ReplayDivergenceError", err, err)
	}
	return divergence
}

func replayEventIndex(events []modeltrace.CanonicalRolloutEvent, kind string) int {
	for index, event := range events {
		if event.Kind == kind {
			return index
		}
	}
	return -1
}

func cloneReplayEvents(events []modeltrace.CanonicalRolloutEvent) []modeltrace.CanonicalRolloutEvent {
	data, _ := json.Marshal(events)
	var out []modeltrace.CanonicalRolloutEvent
	_ = json.Unmarshal(data, &out)
	return out
}

func refreezeReplayEvent(t *testing.T, event modeltrace.CanonicalRolloutEvent, mutate func(*modeltrace.CanonicalRolloutEvent)) modeltrace.CanonicalRolloutEvent {
	t.Helper()
	mutate(&event)
	frozen, err := modeltrace.FreezeCanonicalRolloutEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	return frozen
}

func replayRawFactHash(t *testing.T, value any) string {
	t.Helper()
	hash, err := rawReplayFactHash(value)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func cloneReplayMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstReplayJSONDifference(left, right any) string {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	var leftValue, rightValue any
	_ = json.Unmarshal(leftJSON, &leftValue)
	_ = json.Unmarshal(rightJSON, &rightValue)
	var walk func(string, any, any) string
	walk = func(path string, a, b any) string {
		key := path
		if slash := strings.LastIndex(path, "/"); slash >= 0 {
			key = path[slash+1:]
		}
		if replayTimeKey(key) || key == "hash" {
			return ""
		}
		if reflect.DeepEqual(a, b) {
			return ""
		}
		am, aok := a.(map[string]any)
		bm, bok := b.(map[string]any)
		if aok && bok {
			keys := make([]string, 0, len(am)+len(bm))
			seen := map[string]bool{}
			for key := range am {
				seen[key] = true
				keys = append(keys, key)
			}
			for key := range bm {
				if !seen[key] {
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			for _, key := range keys {
				if diff := walk(path+"/"+key, am[key], bm[key]); diff != "" {
					return diff
				}
			}
			return ""
		}
		as, aok := a.([]any)
		bs, bok := b.([]any)
		if aok && bok {
			if len(as) != len(bs) {
				return fmt.Sprintf("%s length %d != %d", path, len(as), len(bs))
			}
			for index := range as {
				if diff := walk(fmt.Sprintf("%s/%d", path, index), as[index], bs[index]); diff != "" {
					return diff
				}
			}
			return ""
		}
		return fmt.Sprintf("%s %#v != %#v", path, a, b)
	}
	return walk("$", leftValue, rightValue)
}

func normalizedReplayDebugValue(value any) any {
	data, _ := json.Marshal(value)
	var decoded any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	_ = decoder.Decode(&decoded)
	return (&replayFactNormalizer{ids: make(map[string]map[string]string)}).normalize("", decoded)
}

func ptrReplayTransportState(value appui.AiopsTransportState) *appui.AiopsTransportState {
	return &value
}
