package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const updateRolloutReplayEnv = "AIOPS_UPDATE_ROLLOUT_REPLAY"

// TestRefreshRolloutReplayReferenceFixtures intentionally does not run during
// the normal suite. It regenerates reference overlays from the real runtime and
// transport path while keeping the source corpus as the persisted story truth.
//
// Run with:
//
//	AIOPS_UPDATE_ROLLOUT_REPLAY=1 go test ./internal/eval -run '^TestRefreshRolloutReplayReferenceFixtures$' -count=1
func TestRefreshRolloutReplayReferenceFixtures(t *testing.T) {
	if os.Getenv(updateRolloutReplayEnv) != "1" {
		t.Skipf("set %s=1 to refresh rollout replay reference fixtures", updateRolloutReplayEnv)
	}

	for _, name := range []string{"approval_resume", "tool_not_found"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", "rollout_replay", name+".json")
			if err := refreshRolloutReplayReferenceFixture(context.Background(), path); err != nil {
				t.Fatalf("refresh %s: %v", path, err)
			}
		})
	}
}

func refreshRolloutReplayReferenceFixture(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read reference fixture: %w", err)
	}
	fixture, err := LoadRolloutReplayFixture(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if fixture.Source.Kind != "assistant_transport_story" {
		return fmt.Errorf("unsupported rollout replay source kind %q", fixture.Source.Kind)
	}

	relativeSourcePath := fixture.Source.Path
	sourcePath, err := refreshRolloutReplaySourcePath(path, relativeSourcePath)
	if err != nil {
		return err
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read rollout replay source: %w", err)
	}

	fixture.Source.Path = sourcePath
	fixture.Source.Hash = replaySourceContentHash(source)
	fixture.sourceData = append([]byte(nil), source...)
	fixture.Rollout = nil
	fixture.Contract = ReplayContractArtifacts{}
	fixture.ExpectedTransport = nil
	fixture.Baseline = ReplayBaseline{}

	execution, err := NewRuntimeRolloutReplayBackend().ReplayFullStory(ctx, fixture)
	if err != nil {
		return fmt.Errorf("capture real full story: %w", err)
	}
	baseline, err := replayExecutionBaseline(execution)
	if err != nil {
		return fmt.Errorf("build replay baseline: %w", err)
	}
	if err := validateReplayBaseline(baseline); err != nil {
		return fmt.Errorf("validate regenerated replay baseline: %w", err)
	}

	encoded, err := marshalRolloutReplayReferenceFixture(
		fixture.SchemaVersion,
		fixture.Name,
		RolloutReplaySource{
			Kind: fixture.Source.Kind,
			Path: relativeSourcePath,
			Hash: fixture.Source.Hash,
		},
		baseline,
	)
	if err != nil {
		return fmt.Errorf("marshal refreshed replay fixture: %w", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write refreshed replay fixture: %w", err)
	}
	return nil
}

func marshalRolloutReplayReferenceFixture(schemaVersion, name string, source RolloutReplaySource, baseline ReplayBaseline) ([]byte, error) {
	text := func(value string) (string, error) {
		encoded, err := json.Marshal(value)
		return string(encoded), err
	}
	schemaJSON, err := text(schemaVersion)
	if err != nil {
		return nil, err
	}
	nameJSON, err := text(name)
	if err != nil {
		return nil, err
	}
	sourceKindJSON, err := text(source.Kind)
	if err != nil {
		return nil, err
	}
	sourcePathJSON, err := text(source.Path)
	if err != nil {
		return nil, err
	}
	sourceHashJSON, err := text(source.Hash)
	if err != nil {
		return nil, err
	}
	rolloutHashJSON, err := text(baseline.RolloutHash)
	if err != nil {
		return nil, err
	}
	transportHashJSON, err := text(baseline.TransportHash)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	fmt.Fprintf(&out, "{\n  \"schemaVersion\": %s,\n  \"name\": %s,\n", schemaJSON, nameJSON)
	fmt.Fprintf(&out, "  \"source\": {\n    \"kind\": %s,\n    \"path\": %s,\n    \"hash\": %s\n  },\n", sourceKindJSON, sourcePathJSON, sourceHashJSON)
	fmt.Fprintf(&out, "  \"baseline\": {\n    \"eventCount\": %d,\n    \"events\": [\n", baseline.EventCount)
	for index, event := range baseline.Events {
		kindJSON, marshalErr := text(event.Kind)
		if marshalErr != nil {
			return nil, marshalErr
		}
		eventHashJSON, marshalErr := text(event.Hash)
		if marshalErr != nil {
			return nil, marshalErr
		}
		comma := ","
		if index == len(baseline.Events)-1 {
			comma = ""
		}
		fmt.Fprintf(&out, "      {\"sequence\": %d, \"kind\": %s, \"hash\": %s}%s\n", event.Sequence, kindJSON, eventHashJSON, comma)
	}
	fmt.Fprintf(&out, "    ],\n    \"rolloutHash\": %s,\n    \"transportHash\": %s\n  }\n}\n", rolloutHashJSON, transportHashJSON)
	return out.Bytes(), nil
}

func refreshRolloutReplaySourcePath(fixturePath, relativeSourcePath string) (string, error) {
	fixtureDir, err := filepath.Abs(filepath.Dir(fixturePath))
	if err != nil {
		return "", fmt.Errorf("resolve rollout replay fixture directory: %w", err)
	}
	sourcePath := filepath.Clean(filepath.Join(fixtureDir, relativeSourcePath))
	allowedRoot := filepath.Clean(filepath.Join(fixtureDir, "../../../server/testdata/assistant_transport_story"))
	relative, err := filepath.Rel(allowedRoot, sourcePath)
	if err != nil || relative == ".." || filepath.IsAbs(relative) {
		return "", fmt.Errorf("rollout replay source escapes the assistant story corpus")
	}
	if len(relative) > 3 && relative[:3] == ".."+string(filepath.Separator) {
		return "", fmt.Errorf("rollout replay source escapes the assistant story corpus")
	}
	return sourcePath, nil
}
