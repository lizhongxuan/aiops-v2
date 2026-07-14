package runtimekernel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/agentassembly"
)

const ReplayArtifactSchemaVersion = "aiops.replay-artifact.v1"

type ReplayArtifactKind string

const (
	ReplayArtifactKindTurnAssembly        ReplayArtifactKind = "turn_assembly"
	ReplayArtifactKindStepContext         ReplayArtifactKind = "step_context"
	ReplayArtifactKindFinalFacts          ReplayArtifactKind = "final_facts"
	ReplayArtifactKindApprovalActionToken ReplayArtifactKind = "approval_action_token"
)

// ReplayArtifactSink is an optional, production-neutral capture boundary for
// deterministic replay inputs. Runtime control never reads from this sink.
type ReplayArtifactSink interface {
	CaptureReplayArtifact(context.Context, ReplayArtifact) error
}

type ReplayArtifactCaptureError struct {
	Kind  ReplayArtifactKind
	cause error
}

func (err *ReplayArtifactCaptureError) Error() string {
	if err == nil {
		return "replay artifact capture failed"
	}
	return fmt.Sprintf("replay artifact capture failed for kind %q", err.Kind)
}

func (err *ReplayArtifactCaptureError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

// ReplayArtifact is a closed union. Exactly one typed payload must be present
// for Kind; control facts are never routed through turn metadata or references.
type ReplayArtifact struct {
	SchemaVersion string             `json:"schemaVersion"`
	Kind          ReplayArtifactKind `json:"kind"`
	SessionID     string             `json:"sessionId"`
	TurnID        string             `json:"turnId"`
	StepID        string             `json:"stepId,omitempty"`

	TurnAssembly *agentassembly.TurnAssembly `json:"turnAssembly,omitempty"`
	StepContext  *RuntimeStepContext         `json:"stepContext,omitempty"`
	Final        *ReplayFinalArtifact        `json:"final,omitempty"`
	ActionToken  *ActionToken                `json:"actionToken,omitempty"`
}

type ReplayFinalArtifact struct {
	RuntimeFacts     FinalRuntimeFacts `json:"runtimeFacts"`
	RuntimeFactsHash string            `json:"runtimeFactsHash"`
	Contract         FinalContract     `json:"contract"`
	ContractHash     string            `json:"contractHash"`
}

// FreezeReplayArtifact validates and deep-copies all caller-owned typed facts.
// The returned value can be retained or mutated by a sink without aliasing the
// live runtime state.
func FreezeReplayArtifact(input ReplayArtifact) (ReplayArtifact, error) {
	input.SchemaVersion = strings.TrimSpace(input.SchemaVersion)
	if input.SchemaVersion == "" {
		input.SchemaVersion = ReplayArtifactSchemaVersion
	}
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.TurnID = strings.TrimSpace(input.TurnID)
	input.StepID = strings.TrimSpace(input.StepID)

	frozen := ReplayArtifact{
		SchemaVersion: input.SchemaVersion,
		Kind:          input.Kind,
		SessionID:     input.SessionID,
		TurnID:        input.TurnID,
		StepID:        input.StepID,
	}
	switch input.Kind {
	case ReplayArtifactKindTurnAssembly:
		if input.TurnAssembly == nil || input.StepContext != nil || input.Final != nil || input.ActionToken != nil {
			return ReplayArtifact{}, fmt.Errorf("turn assembly replay artifact has invalid payload")
		}
		assembly, err := cloneReplayJSON(*input.TurnAssembly)
		if err != nil {
			return ReplayArtifact{}, fmt.Errorf("clone replay TurnAssembly: %w", err)
		}
		frozen.TurnAssembly = &assembly
	case ReplayArtifactKindStepContext:
		if input.StepContext == nil || input.TurnAssembly != nil || input.Final != nil || input.ActionToken != nil {
			return ReplayArtifact{}, fmt.Errorf("step context replay artifact has invalid payload")
		}
		step, err := cloneRuntimeStepContext(*input.StepContext)
		if err != nil {
			return ReplayArtifact{}, fmt.Errorf("clone replay RuntimeStepContext: %w", err)
		}
		if frozen.StepID == "" {
			frozen.StepID = step.Hash
		}
		frozen.StepContext = &step
	case ReplayArtifactKindFinalFacts:
		if input.Final == nil || input.TurnAssembly != nil || input.StepContext != nil || input.ActionToken != nil {
			return ReplayArtifact{}, fmt.Errorf("final facts replay artifact has invalid payload")
		}
		final, err := freezeReplayFinalArtifact(*input.Final)
		if err != nil {
			return ReplayArtifact{}, err
		}
		frozen.Final = &final
	case ReplayArtifactKindApprovalActionToken:
		if input.ActionToken == nil || input.TurnAssembly != nil || input.StepContext != nil || input.Final != nil {
			return ReplayArtifact{}, fmt.Errorf("approval action token replay artifact has invalid payload")
		}
		if err := input.ActionToken.Validate(); err != nil {
			return ReplayArtifact{}, fmt.Errorf("invalid replay ActionToken: %w", err)
		}
		token, err := cloneReplayJSON(normalizeActionToken(*input.ActionToken))
		if err != nil {
			return ReplayArtifact{}, fmt.Errorf("clone replay ActionToken: %w", err)
		}
		frozen.ActionToken = &token
	default:
		return ReplayArtifact{}, fmt.Errorf("unsupported replay artifact kind %q", input.Kind)
	}
	if err := frozen.Validate(); err != nil {
		return ReplayArtifact{}, err
	}
	return frozen, nil
}

func (artifact ReplayArtifact) Validate() error {
	if artifact.SchemaVersion != ReplayArtifactSchemaVersion {
		return fmt.Errorf("unsupported replay artifact schema version %q", artifact.SchemaVersion)
	}
	if artifact.SessionID == "" || artifact.SessionID != strings.TrimSpace(artifact.SessionID) {
		return fmt.Errorf("replay artifact session id is required and normalized")
	}
	if artifact.TurnID == "" || artifact.TurnID != strings.TrimSpace(artifact.TurnID) {
		return fmt.Errorf("replay artifact turn id is required and normalized")
	}
	if artifact.StepID != strings.TrimSpace(artifact.StepID) {
		return fmt.Errorf("replay artifact step id is not normalized")
	}
	switch artifact.Kind {
	case ReplayArtifactKindTurnAssembly:
		if artifact.TurnAssembly == nil || artifact.StepContext != nil || artifact.Final != nil || artifact.ActionToken != nil || artifact.StepID != "" {
			return fmt.Errorf("turn assembly replay artifact has invalid payload")
		}
		return artifact.TurnAssembly.Validate()
	case ReplayArtifactKindStepContext:
		if artifact.StepContext == nil || artifact.TurnAssembly != nil || artifact.Final != nil || artifact.ActionToken != nil {
			return fmt.Errorf("step context replay artifact has invalid payload")
		}
		if err := artifact.StepContext.Validate(); err != nil {
			return err
		}
		if artifact.StepID == "" || artifact.StepID != artifact.StepContext.Hash {
			return fmt.Errorf("replay step id does not match RuntimeStepContext hash")
		}
		if artifact.SessionID != artifact.StepContext.Turn.SessionID || artifact.TurnID != artifact.StepContext.Turn.TurnID {
			return fmt.Errorf("replay step coordinates do not match RuntimeStepContext")
		}
		return nil
	case ReplayArtifactKindFinalFacts:
		if artifact.Final == nil || artifact.TurnAssembly != nil || artifact.StepContext != nil || artifact.ActionToken != nil {
			return fmt.Errorf("final facts replay artifact has invalid payload")
		}
		return artifact.Final.Validate()
	case ReplayArtifactKindApprovalActionToken:
		if artifact.ActionToken == nil || artifact.TurnAssembly != nil || artifact.StepContext != nil || artifact.Final != nil || artifact.StepID != "" {
			return fmt.Errorf("approval action token replay artifact has invalid payload")
		}
		if err := artifact.ActionToken.Validate(); err != nil {
			return err
		}
		if artifact.TurnID != artifact.ActionToken.TurnID {
			return fmt.Errorf("replay ActionToken turn does not match artifact")
		}
		return nil
	default:
		return fmt.Errorf("unsupported replay artifact kind %q", artifact.Kind)
	}
}

func (artifact ReplayFinalArtifact) Validate() error {
	if artifact.Contract.SchemaVersion != FinalContractSchemaVersion {
		return fmt.Errorf("invalid replay final contract schema version %q", artifact.Contract.SchemaVersion)
	}
	if err := artifact.Contract.Validate(); err != nil {
		return err
	}
	factsHash, err := canonicalRolloutFactHash(artifact.RuntimeFacts)
	if err != nil {
		return err
	}
	if artifact.RuntimeFactsHash == "" || artifact.RuntimeFactsHash != factsHash {
		return fmt.Errorf("replay final runtime facts hash mismatch")
	}
	contractHash, err := canonicalFinalContractFactHash(&artifact.Contract)
	if err != nil {
		return err
	}
	if artifact.ContractHash == "" || artifact.ContractHash != contractHash {
		return fmt.Errorf("replay final contract hash mismatch")
	}
	return nil
}

func freezeReplayFinalArtifact(input ReplayFinalArtifact) (ReplayFinalArtifact, error) {
	facts, err := cloneReplayFinalRuntimeFacts(input.RuntimeFacts)
	if err != nil {
		return ReplayFinalArtifact{}, fmt.Errorf("clone replay FinalRuntimeFacts: %w", err)
	}
	contract, err := cloneReplayJSON(input.Contract)
	if err != nil {
		return ReplayFinalArtifact{}, fmt.Errorf("clone replay FinalContract: %w", err)
	}
	factsHash, err := canonicalRolloutFactHash(facts)
	if err != nil {
		return ReplayFinalArtifact{}, err
	}
	contractHash, err := canonicalFinalContractFactHash(&contract)
	if err != nil {
		return ReplayFinalArtifact{}, err
	}
	return ReplayFinalArtifact{
		RuntimeFacts: facts, RuntimeFactsHash: factsHash,
		Contract: contract, ContractHash: contractHash,
	}, nil
}

func cloneReplayFinalRuntimeFacts(input FinalRuntimeFacts) (FinalRuntimeFacts, error) {
	output, err := cloneReplayJSON(input)
	if err != nil {
		return FinalRuntimeFacts{}, err
	}
	evidenceState, err := cloneReplayJSON(input.EvidenceState)
	if err != nil {
		return FinalRuntimeFacts{}, err
	}
	evidenceDecision, err := cloneReplayJSON(input.EvidenceDecision)
	if err != nil {
		return FinalRuntimeFacts{}, err
	}
	policyCompletion, err := cloneReplayJSON(input.PolicyCompletion)
	if err != nil {
		return FinalRuntimeFacts{}, err
	}
	output.EvidenceState = evidenceState
	output.EvidenceDecision = evidenceDecision
	output.PolicyCompletion = policyCompletion
	return output, nil
}

func cloneReplayJSON[T any](input T) (T, error) {
	var output T
	data, err := json.Marshal(input)
	if err != nil {
		return output, err
	}
	if err := json.Unmarshal(data, &output); err != nil {
		return output, err
	}
	return output, nil
}

func (k *RuntimeKernel) captureReplayArtifact(ctx context.Context, artifact ReplayArtifact) error {
	if k == nil || k.replayArtifactSink == nil {
		return nil
	}
	frozen, err := FreezeReplayArtifact(artifact)
	if err != nil {
		return fmt.Errorf("freeze replay artifact %q: %w", artifact.Kind, err)
	}
	if err := k.replayArtifactSink.CaptureReplayArtifact(ctx, frozen); err != nil {
		return &ReplayArtifactCaptureError{Kind: frozen.Kind, cause: err}
	}
	return nil
}

func (k *RuntimeKernel) captureReplayTurnAssembly(ctx context.Context, snapshot *TurnSnapshot, assembly *agentassembly.TurnAssembly) error {
	if k == nil || k.replayArtifactSink == nil {
		return nil
	}
	if snapshot == nil || assembly == nil {
		return fmt.Errorf("capture replay TurnAssembly: turn snapshot and assembly are required")
	}
	return k.captureReplayArtifact(ctx, ReplayArtifact{
		Kind: ReplayArtifactKindTurnAssembly, SessionID: snapshot.SessionID,
		TurnID: snapshot.ID, TurnAssembly: assembly,
	})
}

func (k *RuntimeKernel) captureReplayStepContext(ctx context.Context, step RuntimeStepContext) error {
	if k == nil || k.replayArtifactSink == nil {
		return nil
	}
	return k.captureReplayArtifact(ctx, ReplayArtifact{
		Kind: ReplayArtifactKindStepContext, SessionID: step.Turn.SessionID,
		TurnID: step.Turn.TurnID, StepID: step.Hash, StepContext: &step,
	})
}

func (k *RuntimeKernel) captureReplayFinalFacts(ctx context.Context, snapshot *TurnSnapshot, facts FinalRuntimeFacts, contract FinalContract) error {
	if k == nil || k.replayArtifactSink == nil {
		return nil
	}
	if snapshot == nil {
		return fmt.Errorf("capture replay final facts: turn snapshot is required")
	}
	stepID := ""
	if snapshot.LatestStepReference != nil {
		stepID = snapshot.LatestStepReference.StepHash
	}
	return k.captureReplayArtifact(ctx, ReplayArtifact{
		Kind: ReplayArtifactKindFinalFacts, SessionID: snapshot.SessionID,
		TurnID: snapshot.ID, StepID: stepID,
		Final: &ReplayFinalArtifact{RuntimeFacts: facts, Contract: contract},
	})
}

func (k *RuntimeKernel) captureReplayApprovalActionToken(ctx context.Context, snapshot *TurnSnapshot, token ActionToken) error {
	if k == nil || k.replayArtifactSink == nil {
		return nil
	}
	if snapshot == nil {
		return fmt.Errorf("capture replay ActionToken: turn snapshot is required")
	}
	return k.captureReplayArtifact(ctx, ReplayArtifact{
		Kind: ReplayArtifactKindApprovalActionToken, SessionID: snapshot.SessionID,
		TurnID: snapshot.ID, ActionToken: &token,
	})
}
