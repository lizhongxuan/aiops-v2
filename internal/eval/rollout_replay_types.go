package eval

import (
	"context"
	"encoding/json"
	"fmt"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/runtimekernel"
)

const RolloutReplayFixtureSchemaVersion = "aiops.rollout-replay.fixture.v1"

type ReplayMode string

const (
	ReplayContract        ReplayMode = "contract"
	ReplayProviderFixture ReplayMode = "provider_fixture"
	ReplayFullStory       ReplayMode = "full_story"
)

type RolloutReplaySource struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	Hash string `json:"hash"`
}

type ReplayContractArtifacts struct {
	TurnAssembly      agentassembly.TurnAssembly         `json:"turnAssembly"`
	Steps             []runtimekernel.RuntimeStepContext `json:"steps"`
	ActionTokens      []runtimekernel.ActionToken        `json:"actionTokens,omitempty"`
	FinalRuntimeFacts runtimekernel.FinalRuntimeFacts    `json:"finalRuntimeFacts"`
	FinalContract     runtimekernel.FinalContract        `json:"finalContract"`
}

type RolloutReplayFixture struct {
	SchemaVersion     string                             `json:"schemaVersion"`
	Name              string                             `json:"name"`
	Source            RolloutReplaySource                `json:"source,omitempty"`
	Rollout           []modeltrace.CanonicalRolloutEvent `json:"rollout,omitempty"`
	Contract          ReplayContractArtifacts            `json:"contract,omitempty"`
	ProviderResponses []json.RawMessage                  `json:"providerResponses,omitempty"`
	TransportCommand  appui.TransportCommand             `json:"transportCommand,omitempty"`
	ExpectedTransport *appui.AiopsTransportState         `json:"expectedTransport,omitempty"`
	Baseline          ReplayBaseline                     `json:"baseline,omitempty"`
	sourceData        []byte
}

type ReplayExecution struct {
	Rollout        []modeltrace.CanonicalRolloutEvent
	TransportState *appui.AiopsTransportState
	Contract       ReplayContractArtifacts
}

type RolloutReplayBackend interface {
	ReplayProviderFixture(context.Context, RolloutReplayFixture) (ReplayExecution, error)
	ReplayFullStory(context.Context, RolloutReplayFixture) (ReplayExecution, error)
}

type RolloutReplayRunner struct {
	Backend RolloutReplayBackend
}

type RolloutReplayReport struct {
	Mode           ReplayMode `json:"mode"`
	ComparedEvents int        `json:"comparedEvents"`
	HeadHash       string     `json:"headHash,omitempty"`
	TransportHash  string     `json:"transportHash,omitempty"`
}

type ReplayDivergenceError struct {
	Sequence     int64  `json:"sequence"`
	ExpectedKind string `json:"expectedKind"`
	ActualKind   string `json:"actualKind,omitempty"`
	ExpectedHash string `json:"expectedHash,omitempty"`
	ActualHash   string `json:"actualHash,omitempty"`
	OwnerModule  string `json:"ownerModule"`
}

func (e *ReplayDivergenceError) Error() string {
	if e == nil {
		return "rollout replay diverged"
	}
	return fmt.Sprintf("rollout replay diverged at sequence %d kind %q (owner %s): expected %s, actual %s", e.Sequence, e.ExpectedKind, e.OwnerModule, e.ExpectedHash, e.ActualHash)
}

func replayOwner(kind string) string {
	switch kind {
	case modeltrace.CanonicalRolloutKindAdmission:
		return "runtimecontract"
	case modeltrace.CanonicalRolloutKindAssembly:
		return "agentassembly"
	case modeltrace.CanonicalRolloutKindPrompt:
		return "promptcompiler"
	case modeltrace.CanonicalRolloutKindProviderRequest, modeltrace.CanonicalRolloutKindProviderResponse:
		return "modelrouter"
	case modeltrace.CanonicalRolloutKindToolProposed, modeltrace.CanonicalRolloutKindToolDispatched, modeltrace.CanonicalRolloutKindToolResult:
		return "runtimekernel.ToolDispatcher"
	case modeltrace.CanonicalRolloutKindApprovalRequested, modeltrace.CanonicalRolloutKindApprovalDecided:
		return "runtimekernel.approval"
	case modeltrace.CanonicalRolloutKindTransportProjection:
		return "appui.TransportProjector"
	default:
		return "runtimekernel"
	}
}
