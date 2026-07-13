package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/runtimekernel"
)

// Hydrate resolves a reference overlay through the real full-story backend and
// returns an in-memory executable fixture. The source file remains the single
// persisted command/provider/tool truth and is protected by Source.Hash.
func (r RolloutReplayRunner) Hydrate(ctx context.Context, fixture RolloutReplayFixture) (RolloutReplayFixture, error) {
	if len(fixture.Rollout) > 0 {
		return fixture, nil
	}
	if r.Backend == nil {
		return RolloutReplayFixture{}, errors.New("rollout replay backend is required to hydrate a reference fixture")
	}
	execution, err := r.Backend.ReplayFullStory(ctx, fixture)
	if err != nil {
		return RolloutReplayFixture{}, fmt.Errorf("hydrate rollout replay fixture: %w", err)
	}
	if len(execution.Rollout) == 0 || execution.TransportState == nil || len(execution.Contract.Steps) == 0 {
		return RolloutReplayFixture{}, errors.New("hydrated rollout replay fixture is incomplete")
	}
	actualBaseline, err := replayExecutionBaseline(execution)
	if err != nil {
		return RolloutReplayFixture{}, err
	}
	if fixture.Baseline.RolloutHash != "" {
		if err := compareReplayBaselines(fixture.Baseline, actualBaseline); err != nil {
			return RolloutReplayFixture{}, err
		}
	}
	fixture.Rollout = execution.Rollout
	fixture.Contract = execution.Contract
	fixture.ExpectedTransport = execution.TransportState
	fixture.Baseline = actualBaseline
	return fixture, nil
}

func LoadRolloutReplayFixture(reader io.Reader) (RolloutReplayFixture, error) {
	if reader == nil {
		return RolloutReplayFixture{}, errors.New("rollout replay fixture reader is required")
	}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var fixture RolloutReplayFixture
	if err := decoder.Decode(&fixture); err != nil {
		return RolloutReplayFixture{}, fmt.Errorf("decode rollout replay fixture: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return RolloutReplayFixture{}, errors.New("rollout replay fixture contains trailing JSON")
		}
		return RolloutReplayFixture{}, fmt.Errorf("decode rollout replay fixture trailing data: %w", err)
	}
	if fixture.SchemaVersion != RolloutReplayFixtureSchemaVersion {
		return RolloutReplayFixture{}, fmt.Errorf("unsupported rollout replay fixture schema %q", fixture.SchemaVersion)
	}
	fixture.Name = strings.TrimSpace(fixture.Name)
	if fixture.Name == "" {
		return RolloutReplayFixture{}, errors.New("rollout replay fixture name is required")
	}
	return fixture, nil
}

// LoadRolloutReplayFixtureFile also verifies that a referenced source corpus
// is still the exact content the replay overlay was recorded against.
func LoadRolloutReplayFixtureFile(path string) (RolloutReplayFixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RolloutReplayFixture{}, fmt.Errorf("read rollout replay fixture: %w", err)
	}
	fixture, err := LoadRolloutReplayFixture(bytes.NewReader(data))
	if err != nil {
		return RolloutReplayFixture{}, err
	}
	if strings.TrimSpace(fixture.Source.Path) == "" {
		return fixture, nil
	}
	if fixture.Source.Kind != "assistant_transport_story" {
		return RolloutReplayFixture{}, fmt.Errorf("unsupported rollout replay source kind %q", fixture.Source.Kind)
	}
	fixtureDir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return RolloutReplayFixture{}, fmt.Errorf("resolve rollout replay fixture directory: %w", err)
	}
	sourcePath := filepath.Clean(filepath.Join(fixtureDir, fixture.Source.Path))
	allowedRoot := filepath.Clean(filepath.Join(fixtureDir, "../../../server/testdata/assistant_transport_story"))
	relative, err := filepath.Rel(allowedRoot, sourcePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return RolloutReplayFixture{}, errors.New("rollout replay source escapes the assistant story corpus")
	}
	if err := validateReplayBaseline(fixture.Baseline); err != nil {
		return RolloutReplayFixture{}, err
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return RolloutReplayFixture{}, fmt.Errorf("read rollout replay source: %w", err)
	}
	actual := replaySourceContentHash(source)
	if actual != strings.TrimSpace(fixture.Source.Hash) {
		return RolloutReplayFixture{}, fmt.Errorf("rollout replay source hash mismatch: expected %s, actual %s", fixture.Source.Hash, actual)
	}
	fixture.Source.Path = sourcePath
	fixture.sourceData = append([]byte(nil), source...)
	return fixture, nil
}

func (r RolloutReplayRunner) Replay(ctx context.Context, mode ReplayMode, fixture RolloutReplayFixture) (RolloutReplayReport, error) {
	if fixture.SchemaVersion != RolloutReplayFixtureSchemaVersion {
		return RolloutReplayReport{}, fmt.Errorf("unsupported rollout replay fixture schema %q", fixture.SchemaVersion)
	}
	if err := validateExpectedReplayRollout(fixture.Rollout); err != nil {
		return RolloutReplayReport{}, err
	}
	report := RolloutReplayReport{Mode: mode, HeadHash: fixture.Rollout[len(fixture.Rollout)-1].Hash}
	switch mode {
	case ReplayContract:
		if err := validateReplayContract(fixture); err != nil {
			return RolloutReplayReport{}, err
		}
		report.ComparedEvents = len(fixture.Rollout)
		return report, nil
	case ReplayProviderFixture, ReplayFullStory:
		if r.Backend == nil {
			return RolloutReplayReport{}, errors.New("rollout replay backend is required")
		}
	default:
		return RolloutReplayReport{}, fmt.Errorf("unsupported rollout replay mode %q", mode)
	}

	var execution ReplayExecution
	var err error
	if mode == ReplayProviderFixture {
		execution, err = r.Backend.ReplayProviderFixture(ctx, fixture)
	} else {
		execution, err = r.Backend.ReplayFullStory(ctx, fixture)
	}
	if err != nil {
		return RolloutReplayReport{}, fmt.Errorf("%s replay backend: %w", mode, err)
	}
	compared, err := compareReplayRollouts(fixture.Rollout, execution.Rollout, fixture.Contract, execution.Contract)
	if err != nil {
		return RolloutReplayReport{}, err
	}
	report.ComparedEvents = compared
	if mode == ReplayFullStory && fixture.ExpectedTransport != nil {
		report.TransportHash, err = compareReplayTransport(*fixture.ExpectedTransport, execution.TransportState, fixture.Rollout)
		if err != nil {
			return RolloutReplayReport{}, err
		}
	}
	return report, nil
}

func validateExpectedReplayRollout(events []modeltrace.CanonicalRolloutEvent) error {
	if len(events) == 0 {
		return errors.New("rollout replay fixture requires canonical events")
	}
	for index, event := range events {
		if err := event.Validate(); err != nil {
			actualHash := ""
			if normalized, freezeErr := modeltrace.FreezeCanonicalRolloutEvent(event); freezeErr == nil {
				actualHash = normalized.Hash
			}
			return &ReplayDivergenceError{
				Sequence: event.Sequence, ExpectedKind: event.Kind, ActualKind: event.Kind,
				ExpectedHash: event.Hash, ActualHash: actualHash, OwnerModule: replayOwner(event.Kind),
			}
		}
		if event.Sequence != int64(index+1) {
			return fmt.Errorf("canonical rollout event[%d] sequence = %d, want %d", index, event.Sequence, index+1)
		}
		if index > 0 && !containsReplayString(event.SourceRefs, events[index-1].EventID) {
			return fmt.Errorf("canonical rollout event[%d] does not reference previous event", index)
		}
	}
	return nil
}

func validateReplayContract(fixture RolloutReplayFixture) error {
	assemblyEvent, ok := replayEventByOrdinal(fixture.Rollout, modeltrace.CanonicalRolloutKindAssembly, 0)
	if !ok {
		return errors.New("contract replay requires assembly event")
	}
	assembly := fixture.Contract.TurnAssembly
	rebuiltAssembly, rebuildErr := agentassembly.BuildTurnAssembly(agentassembly.TurnAssemblyInput{
		AdmissionFacts: assembly.AdmissionFacts, PermissionProfile: assembly.PermissionProfile,
		CapabilityPolicy: assembly.CapabilityPolicy, ContextPolicy: assembly.ContextPolicy,
		LoopPolicy: assembly.LoopPolicy, FinalContractPolicy: assembly.FinalContractPolicy,
		RollbackPolicy: assembly.RollbackPolicy, SourceRefs: assembly.SourceRefs,
	})
	actualAssemblyHash := rebuiltAssembly.Hash
	if rebuildErr != nil || assembly.Validate() != nil || assemblyEvent.TurnAssemblyHash != actualAssemblyHash {
		return replayHashDivergence(assemblyEvent, "agentassembly", assemblyEvent.TurnAssemblyHash, actualAssemblyHash)
	}
	if len(fixture.Contract.Steps) == 0 {
		return errors.New("contract replay requires at least one typed step context")
	}
	if promptCount := replayEventCount(fixture.Rollout, modeltrace.CanonicalRolloutKindPrompt); promptCount != len(fixture.Contract.Steps) {
		return fmt.Errorf("contract replay typed step count = %d, want %d", len(fixture.Contract.Steps), promptCount)
	}
	for index, step := range fixture.Contract.Steps {
		promptEvent, found := replayEventByOrdinal(fixture.Rollout, modeltrace.CanonicalRolloutKindPrompt, index)
		if !found {
			return errors.New("contract replay step has no matching prompt event")
		}
		actualStepHash := runtimekernel.ComputeRuntimeStepContextHash(step)
		frozen, err := runtimekernel.FreezeRuntimeStepContext(step)
		if step.Validate() != nil || err != nil || frozen.Hash != actualStepHash || step.TurnAssemblyHash != actualAssemblyHash ||
			promptEvent.StepContextHash != actualStepHash || promptEvent.TurnAssemblyHash != actualAssemblyHash ||
			replayPayloadString(promptEvent, "modelInputHash") != step.ProviderRequest.ModelInputHash {
			return replayHashDivergence(promptEvent, "runtimekernel", promptEvent.StepContextHash, actualStepHash)
		}
		if requestEvent, requestFound := replayEventByOrdinal(fixture.Rollout, modeltrace.CanonicalRolloutKindProviderRequest, index); requestFound {
			if requestEvent.StepContextHash != actualStepHash || replayPayloadString(requestEvent, "modelInputHash") != step.ProviderRequest.ModelInputHash {
				return replayHashDivergence(requestEvent, "modelrouter", requestEvent.StepContextHash, actualStepHash)
			}
		}
	}
	for index, token := range fixture.Contract.ActionTokens {
		approvalEvent, found := replayEventByOrdinal(fixture.Rollout, modeltrace.CanonicalRolloutKindApprovalRequested, index)
		if !found {
			return errors.New("contract replay action token has no matching approval_requested event")
		}
		if token.Validate() != nil || replayPayloadString(approvalEvent, "actionTokenHash") != token.Hash ||
			replayPayloadString(approvalEvent, "approvalId") != token.ApprovalID ||
			replayPayloadString(approvalEvent, "toolCallId") != token.ToolCallID ||
			replayPayloadString(approvalEvent, "toolName") != token.ToolName ||
			replayPayloadString(approvalEvent, "argsHash") != token.ArgumentsHash ||
			strings.Join(replayPayloadStrings(approvalEvent, "targetRefs"), "\x00") != strings.Join(token.TargetRefs, "\x00") ||
			replayPayloadString(approvalEvent, "toolSurfaceFingerprint") != token.ToolSurfaceFingerprint ||
			replayPayloadString(approvalEvent, "permissionHash") != token.PermissionHash ||
			replayPayloadString(approvalEvent, "rollbackHash") != token.RollbackHash ||
			replayPayloadString(approvalEvent, "checkpointId") != token.CheckpointID {
			return replayHashDivergence(approvalEvent, "runtimekernel.approval", replayPayloadString(approvalEvent, "actionTokenHash"), token.Hash)
		}
	}
	if approvalCount := replayEventCount(fixture.Rollout, modeltrace.CanonicalRolloutKindApprovalRequested); approvalCount != len(fixture.Contract.ActionTokens) {
		return fmt.Errorf("contract replay approval token count = %d, want %d", len(fixture.Contract.ActionTokens), approvalCount)
	}
	finalEvent, ok := replayEventByOrdinal(fixture.Rollout, modeltrace.CanonicalRolloutKindFinalFacts, 0)
	if !ok {
		return errors.New("contract replay requires final_facts event")
	}
	factsHash, err := rawReplayFactHash(fixture.Contract.FinalRuntimeFacts)
	if err != nil {
		return err
	}
	if expected := replayPayloadString(finalEvent, "finalRuntimeFactsHash"); expected != factsHash {
		return replayHashDivergence(finalEvent, "runtimekernel", expected, factsHash)
	}
	contract := fixture.Contract.FinalContract
	contract.AnswerText = ""
	contractHash, err := rawReplayFactHash(contract)
	if err != nil {
		return err
	}
	if expected := replayPayloadString(finalEvent, "finalContractHash"); contract.SchemaVersion != runtimekernel.FinalContractSchemaVersion || contract.Validate() != nil || expected != contractHash {
		return replayHashDivergence(finalEvent, "runtimekernel", expected, contractHash)
	}
	return nil
}

func compareReplayRollouts(expected, actual []modeltrace.CanonicalRolloutEvent, contracts ...ReplayContractArtifacts) (int, error) {
	var expectedContract, actualContract ReplayContractArtifacts
	if len(contracts) > 0 {
		expectedContract = contracts[0]
	}
	if len(contracts) > 1 {
		actualContract = contracts[1]
	}
	expectedHashes, err := normalizedReplayEventHashes(expected, expectedContract)
	if err != nil {
		return 0, err
	}
	actualHashes, err := normalizedReplayEventHashes(actual, actualContract)
	if err != nil {
		return 0, err
	}
	limit := len(expected)
	if len(actual) < limit {
		limit = len(actual)
	}
	for index := 0; index < limit; index++ {
		want, got := expected[index], actual[index]
		wantHash, gotHash := expectedHashes[index], actualHashes[index]
		if got.Validate() != nil || want.Sequence != got.Sequence || want.Kind != got.Kind || wantHash != gotHash {
			return index, &ReplayDivergenceError{
				Sequence: want.Sequence, ExpectedKind: want.Kind, ActualKind: got.Kind,
				ExpectedHash: want.Hash, ActualHash: firstReplayValue(got.Hash, gotHash), OwnerModule: replayOwner(want.Kind),
			}
		}
	}
	if len(expected) != len(actual) {
		if len(expected) > limit {
			want := expected[limit]
			return limit, &ReplayDivergenceError{Sequence: want.Sequence, ExpectedKind: want.Kind, ExpectedHash: want.Hash, OwnerModule: replayOwner(want.Kind)}
		}
		got := actual[limit]
		return limit, &ReplayDivergenceError{Sequence: got.Sequence, ActualKind: got.Kind, ActualHash: got.Hash, OwnerModule: replayOwner(got.Kind)}
	}
	return limit, nil
}

func compareReplayTransport(expected appui.AiopsTransportState, actual *appui.AiopsTransportState, events []modeltrace.CanonicalRolloutEvent) (string, error) {
	expectedHash, err := normalizedReplayFactHash(expected)
	if err != nil {
		return "", err
	}
	actualHash := ""
	if actual != nil {
		actualHash, err = normalizedReplayFactHash(*actual)
		if err != nil {
			return "", err
		}
	}
	if expectedHash == actualHash {
		return actualHash, nil
	}
	event, ok := replayEventByOrdinal(events, modeltrace.CanonicalRolloutKindTransportProjection, 0)
	if !ok {
		return "", errors.New("full-story replay requires transport_projection event")
	}
	return "", &ReplayDivergenceError{Sequence: event.Sequence, ExpectedKind: event.Kind, ActualKind: event.Kind, ExpectedHash: expectedHash, ActualHash: actualHash, OwnerModule: "appui.TransportProjector"}
}

func rawReplayFactHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal replay contract fact: %w", err)
	}
	hash, err := actionproposal.NormalizedInputHash(data)
	if err != nil {
		return "", fmt.Errorf("normalize replay contract fact: %w", err)
	}
	return hash, nil
}

func replayHashDivergence(event modeltrace.CanonicalRolloutEvent, owner, expected, actual string) error {
	return &ReplayDivergenceError{Sequence: event.Sequence, ExpectedKind: event.Kind, ActualKind: event.Kind, ExpectedHash: expected, ActualHash: actual, OwnerModule: owner}
}

func replayEventByOrdinal(events []modeltrace.CanonicalRolloutEvent, kind string, ordinal int) (modeltrace.CanonicalRolloutEvent, bool) {
	for _, event := range events {
		if event.Kind != kind {
			continue
		}
		if ordinal == 0 {
			return event, true
		}
		ordinal--
	}
	return modeltrace.CanonicalRolloutEvent{}, false
}

func replayEventCount(events []modeltrace.CanonicalRolloutEvent, kind string) int {
	count := 0
	for _, event := range events {
		if event.Kind == kind {
			count++
		}
	}
	return count
}

func replayPayloadString(event modeltrace.CanonicalRolloutEvent, key string) string {
	value, _ := event.Payload[key].(string)
	return strings.TrimSpace(value)
}

func replayPayloadStrings(event modeltrace.CanonicalRolloutEvent, key string) []string {
	var values []string
	switch input := event.Payload[key].(type) {
	case []string:
		values = append(values, input...)
	case []any:
		for _, value := range input {
			if text, ok := value.(string); ok {
				values = append(values, text)
			}
		}
	}
	sort.Strings(values)
	return values
}

func containsReplayString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstReplayValue(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
