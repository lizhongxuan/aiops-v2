package runtimekernel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/runtimekernel/toolfailure"
	"aiops-v2/internal/tooling"
)

type ToolInvocationStatus string

const (
	ToolInvocationQueued    ToolInvocationStatus = "queued"
	ToolInvocationRunning   ToolInvocationStatus = "running"
	ToolInvocationCompleted ToolInvocationStatus = "completed"
	ToolInvocationFailed    ToolInvocationStatus = "failed"
	ToolInvocationBlocked   ToolInvocationStatus = "blocked"
)

type ToolAttemptAction string
type ToolAttemptOutcome string

const (
	ToolAttemptActionValidate        ToolAttemptAction = "validate"
	ToolAttemptActionRetry           ToolAttemptAction = "retry"
	ToolAttemptActionManualReconcile ToolAttemptAction = "manual_reconcile"

	ToolAttemptOutcomePlanned   ToolAttemptOutcome = "planned"
	ToolAttemptOutcomeSkipped   ToolAttemptOutcome = "skipped"
	ToolAttemptOutcomeStarted   ToolAttemptOutcome = "started"
	ToolAttemptOutcomeCompleted ToolAttemptOutcome = "completed"
	ToolAttemptOutcomeFailed    ToolAttemptOutcome = "failed"
)

type ToolInvocationState struct {
	ID                     string               `json:"id"`
	ToolCallID             string               `json:"toolCallId"`
	ToolName               string               `json:"toolName"`
	ArgumentsHash          string               `json:"argumentsHash"`
	ToolSurfaceFingerprint string               `json:"toolSurfaceFingerprint,omitempty"`
	Status                 ToolInvocationStatus `json:"status"`
	FailureKind            string               `json:"failureKind,omitempty"`
	Mutating               bool                 `json:"mutating,omitempty"`
	StartedAt              time.Time            `json:"startedAt,omitempty"`
	UpdatedAt              time.Time            `json:"updatedAt,omitempty"`
	CompletedAt            *time.Time           `json:"completedAt,omitempty"`
	Attempts               []ToolAttemptState   `json:"attempts,omitempty"`
}

type ToolAttemptState struct {
	ID                     string             `json:"id"`
	ToolCallID             string             `json:"toolCallId"`
	ToolName               string             `json:"toolName"`
	AttemptNo              int                `json:"attemptNo"`
	Action                 ToolAttemptAction  `json:"action"`
	TriggerFailureKind     string             `json:"triggerFailureKind,omitempty"`
	OriginalArgumentsHash  string             `json:"originalArgumentsHash,omitempty"`
	EffectiveArgumentsHash string             `json:"effectiveArgumentsHash,omitempty"`
	ToolSurfaceFingerprint string             `json:"toolSurfaceFingerprint,omitempty"`
	DecisionReason         string             `json:"decisionReason,omitempty"`
	BackoffMillis          int                `json:"backoffMillis,omitempty"`
	Outcome                ToolAttemptOutcome `json:"outcome"`
	ErrorKind              string             `json:"errorKind,omitempty"`
	StartedAt              time.Time          `json:"startedAt,omitempty"`
	CompletedAt            time.Time          `json:"completedAt,omitempty"`
}

func queueToolInvocation(snapshot *TurnSnapshot, iteration int, tc ToolCall, meta tooling.ToolMetadata) {
	iter := latestIteration(snapshot)
	if iter == nil || iter.Iteration != iteration || strings.TrimSpace(tc.ID) == "" {
		return
	}
	if findToolInvocation(iter, tc.ID) >= 0 {
		return
	}
	now := time.Now()
	iter.ToolInvocations = append(iter.ToolInvocations, ToolInvocationState{
		ID:                     fmt.Sprintf("%s-invocation-%s", snapshot.ID, tc.ID),
		ToolCallID:             tc.ID,
		ToolName:               tc.Name,
		ArgumentsHash:          toolInvocationArgumentsHash(tc),
		ToolSurfaceFingerprint: firstNonEmpty(iter.ToolSurfaceFingerprint, snapshot.StableToolFingerprint),
		Status:                 ToolInvocationQueued,
		Mutating:               meta.EffectiveGovernance(0).Mutating,
		StartedAt:              now,
		UpdatedAt:              now,
	})
}

func markToolInvocationRunning(snapshot *TurnSnapshot, toolCallID string) {
	updateToolInvocation(snapshot, toolCallID, ToolInvocationRunning, "", nil)
}

func markToolInvocationBlocked(snapshot *TurnSnapshot, toolCallID string) {
	updateToolInvocation(snapshot, toolCallID, ToolInvocationBlocked, "", nil)
}

func markToolInvocationCompleted(snapshot *TurnSnapshot, toolCallID string) {
	updateToolInvocation(snapshot, toolCallID, ToolInvocationCompleted, "", nil)
}

func markToolInvocationFailed(snapshot *TurnSnapshot, toolCallID string, failureKind string) {
	updateToolInvocation(snapshot, toolCallID, ToolInvocationFailed, failureKind, nil)
	if failureKind == string(toolfailure.KindInvalidArguments) {
		appendToolAttempt(snapshot, toolCallID, ToolAttemptActionValidate, ToolAttemptOutcomeFailed, failureKind, "schema validation failed", 0, nil)
	}
}

func updateToolInvocation(snapshot *TurnSnapshot, toolCallID string, status ToolInvocationStatus, failureKind string, at *time.Time) {
	iter := latestIteration(snapshot)
	if iter == nil {
		return
	}
	idx := findToolInvocation(iter, toolCallID)
	if idx < 0 {
		return
	}
	now := time.Now()
	if at != nil {
		now = *at
	}
	inv := &iter.ToolInvocations[idx]
	inv.Status = status
	inv.UpdatedAt = now
	if failureKind != "" {
		inv.FailureKind = failureKind
	}
	switch status {
	case ToolInvocationCompleted, ToolInvocationFailed:
		inv.CompletedAt = &now
	}
}

func findToolInvocation(iter *IterationState, toolCallID string) int {
	if iter == nil {
		return -1
	}
	for i := range iter.ToolInvocations {
		if iter.ToolInvocations[i].ToolCallID == toolCallID {
			return i
		}
	}
	return -1
}

func failureKindForDispatchResult(result DispatchResult) string {
	if strings.TrimSpace(result.Error) == "" {
		return ""
	}
	decision := toolfailure.NewClassifier().Classify(toolfailure.ClassificationInput{
		Source:  result.Source,
		Outcome: result.Outcome,
		Error:   result.Error,
	})
	return string(decision.Kind)
}

func toolInvocationArgumentsHash(tc ToolCall) string {
	return toolArgumentsHash(tc.Arguments)
}

func toolArgumentsHash(args json.RawMessage) string {
	hash, err := actionproposal.NormalizedInputHash(args)
	if err == nil && strings.TrimSpace(hash) != "" {
		return hash
	}
	trimmed := strings.TrimSpace(string(args))
	if trimmed == "" {
		trimmed = "{}"
	}
	sum := sha256.Sum256([]byte(trimmed))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func MarkRunningMutatingInvocationsSideEffectUnknown(snapshot *TurnSnapshot, at time.Time) int {
	if snapshot == nil {
		return 0
	}
	changed := 0
	for iterIdx := range snapshot.Iterations {
		iter := &snapshot.Iterations[iterIdx]
		for invocationIdx := range iter.ToolInvocations {
			invocation := &iter.ToolInvocations[invocationIdx]
			if invocation.Status != ToolInvocationRunning || !invocation.Mutating {
				continue
			}
			invocation.Status = ToolInvocationFailed
			invocation.FailureKind = string(toolfailure.KindSideEffectUnknown)
			invocation.UpdatedAt = at
			invocation.CompletedAt = &at
			appendToolAttemptToInvocation(invocation, ToolAttemptActionManualReconcile, ToolAttemptOutcomePlanned, string(toolfailure.KindSideEffectUnknown), "running mutating tool state is side-effect-unknown after recovery", 0, at)
			changed++
		}
	}
	return changed
}

func appendToolAttempt(snapshot *TurnSnapshot, toolCallID string, action ToolAttemptAction, outcome ToolAttemptOutcome, failureKind, reason string, backoffMillis int, at *time.Time) {
	iter := latestIteration(snapshot)
	if iter == nil {
		return
	}
	idx := findToolInvocation(iter, toolCallID)
	if idx < 0 {
		return
	}
	now := time.Now()
	if at != nil {
		now = *at
	}
	appendToolAttemptToInvocation(&iter.ToolInvocations[idx], action, outcome, failureKind, reason, backoffMillis, now)
}

func appendToolAttemptStates(snapshot *TurnSnapshot, toolCallID string, attempts []ToolAttemptState) {
	if len(attempts) == 0 {
		return
	}
	iter := latestIteration(snapshot)
	if iter == nil {
		return
	}
	idx := findToolInvocation(iter, toolCallID)
	if idx < 0 {
		return
	}
	invocation := &iter.ToolInvocations[idx]
	for _, attempt := range attempts {
		attemptNo := len(invocation.Attempts) + 1
		if strings.TrimSpace(attempt.ID) == "" {
			attempt.ID = fmt.Sprintf("%s-attempt-%d", invocation.ID, attemptNo)
		}
		attempt.AttemptNo = attemptNo
		if strings.TrimSpace(attempt.ToolCallID) == "" {
			attempt.ToolCallID = invocation.ToolCallID
		}
		if strings.TrimSpace(attempt.ToolName) == "" {
			attempt.ToolName = invocation.ToolName
		}
		if strings.TrimSpace(attempt.OriginalArgumentsHash) == "" {
			attempt.OriginalArgumentsHash = invocation.ArgumentsHash
		}
		if strings.TrimSpace(attempt.EffectiveArgumentsHash) == "" {
			attempt.EffectiveArgumentsHash = invocation.ArgumentsHash
		}
		if strings.TrimSpace(attempt.ToolSurfaceFingerprint) == "" {
			attempt.ToolSurfaceFingerprint = invocation.ToolSurfaceFingerprint
		}
		now := time.Now()
		if attempt.StartedAt.IsZero() {
			attempt.StartedAt = now
		}
		if attempt.CompletedAt.IsZero() {
			attempt.CompletedAt = attempt.StartedAt
		}
		invocation.Attempts = append(invocation.Attempts, attempt)
	}
}

func appendToolAttemptToInvocation(invocation *ToolInvocationState, action ToolAttemptAction, outcome ToolAttemptOutcome, failureKind, reason string, backoffMillis int, at time.Time) {
	if invocation == nil {
		return
	}
	attemptNo := len(invocation.Attempts) + 1
	attempt := ToolAttemptState{
		ID:                     fmt.Sprintf("%s-attempt-%d", invocation.ID, attemptNo),
		ToolCallID:             invocation.ToolCallID,
		ToolName:               invocation.ToolName,
		AttemptNo:              attemptNo,
		Action:                 action,
		TriggerFailureKind:     strings.TrimSpace(failureKind),
		OriginalArgumentsHash:  invocation.ArgumentsHash,
		EffectiveArgumentsHash: invocation.ArgumentsHash,
		ToolSurfaceFingerprint: invocation.ToolSurfaceFingerprint,
		DecisionReason:         strings.TrimSpace(reason),
		BackoffMillis:          backoffMillis,
		Outcome:                outcome,
		StartedAt:              at,
		CompletedAt:            at,
	}
	if outcome == ToolAttemptOutcomeFailed {
		attempt.ErrorKind = strings.TrimSpace(failureKind)
	}
	invocation.Attempts = append(invocation.Attempts, attempt)
}
