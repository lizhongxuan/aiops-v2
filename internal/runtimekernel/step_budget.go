package runtimekernel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/tooling"
)

const (
	workflowRepairBudgetMetadataKey   = "aiops.workflow.repairBudget.v1"
	workflowRepairBudgetSchemaVersion = "aiops.workflow.repair-budget.v1"
	workflowRepairMaxAttempts         = 2
	workflowRepairHistoryLimit        = 12
)

type workflowRepairDiagnostic struct {
	Code       string `json:"code,omitempty"`
	Path       string `json:"path,omitempty"`
	Message    string `json:"message,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

type workflowRepairAttemptRecord struct {
	Iteration   int                        `json:"iteration"`
	RepairRound int                        `json:"repairRound"`
	ToolCallID  string                     `json:"toolCallId"`
	Stage       string                     `json:"stage"`
	Failed      bool                       `json:"failed"`
	Diagnostics []workflowRepairDiagnostic `json:"diagnostics,omitempty"`
	ObservedAt  time.Time                  `json:"observedAt"`
}

// workflowRepairBudgetState is serialized under one centrally-owned metadata
// key so refresh, approval resume, and replay reuse the same turn-scoped budget.
// It is an execution trace/state record; profile and tool discovery metadata
// remain the typed policy inputs that decide whether the budget applies.
type workflowRepairBudgetState struct {
	SchemaVersion           string                        `json:"schemaVersion"`
	MaxRepairAttempts       int                           `json:"maxRepairAttempts"`
	InitialFailureObserved  bool                          `json:"initialFailureObserved"`
	InitialFailureIteration int                           `json:"initialFailureIteration,omitempty"`
	FailureIterations       []int                         `json:"failureIterations,omitempty"`
	FailureCount            int                           `json:"failureCount"`
	RepairAttempts          int                           `json:"repairAttempts"`
	RelevantCalls           int                           `json:"relevantCalls"`
	PromptTokens            int                           `json:"promptTokens,omitempty"`
	CompletionTokens        int                           `json:"completionTokens,omitempty"`
	TotalTokens             int                           `json:"totalTokens,omitempty"`
	ModelDurationMillis     int64                         `json:"modelDurationMillis,omitempty"`
	ElapsedMillis           int64                         `json:"elapsedMillis,omitempty"`
	SeenToolCallIDs         []string                      `json:"seenToolCallIds,omitempty"`
	CountedModelIterations  []int                         `json:"countedModelIterations,omitempty"`
	LastStage               string                        `json:"lastStage,omitempty"`
	LastToolCallID          string                        `json:"lastToolCallId,omitempty"`
	LastDiagnosticDigest    string                        `json:"lastDiagnosticDigest,omitempty"`
	LastDiagnostics         []workflowRepairDiagnostic    `json:"lastDiagnostics,omitempty"`
	AllowedNextActions      []string                      `json:"allowedNextActions,omitempty"`
	History                 []workflowRepairAttemptRecord `json:"history,omitempty"`
	StartedAt               time.Time                     `json:"startedAt,omitempty"`
	UpdatedAt               time.Time                     `json:"updatedAt,omitempty"`
	Exhausted               bool                          `json:"exhausted,omitempty"`
	Canceled                bool                          `json:"canceled,omitempty"`
	CancelReason            string                        `json:"cancelReason,omitempty"`
	CanceledAt              *time.Time                    `json:"canceledAt,omitempty"`
	IntegrityError          string                        `json:"integrityError,omitempty"`
}

type workflowRepairBudgetObservation struct {
	Iteration        int
	ToolCallID       string
	Metadata         tooling.ToolMetadata
	Result           tooling.ToolResult
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ModelDuration    time.Duration
	ObservedAt       time.Time
}

type workflowRepairBudgetDecision struct {
	Applies     bool
	Exhausted   bool
	Canceled    bool
	BlockReason string
	State       workflowRepairBudgetState
}

func observeWorkflowRepairBudget(snapshot *TurnSnapshot, observation workflowRepairBudgetObservation) workflowRepairBudgetDecision {
	stage, applies := workflowRepairStage(snapshot, observation.Metadata)
	if !applies {
		return workflowRepairBudgetDecision{}
	}
	state, err := workflowRepairBudgetStateFromSnapshot(snapshot)
	if err != nil {
		state = newWorkflowRepairBudgetState()
		state.Exhausted = true
		state.IntegrityError = "persisted repair budget state is invalid"
		state.LastStage = stage
		state.LastToolCallID = strings.TrimSpace(observation.ToolCallID)
		state.LastDiagnostics = workflowRepairDiagnostics(observation.Result.StructuredContent)
		state.AllowedNextActions = workflowRepairAllowedNextActions()
		storeWorkflowRepairBudgetState(snapshot, state)
		return workflowRepairBudgetDecision{Applies: true, Exhausted: true, BlockReason: workflowRepairBlockReason(state), State: state}
	}
	if state.Canceled {
		return workflowRepairBudgetDecision{Applies: true, Canceled: true, State: state}
	}
	if workflowRepairContainsString(state.SeenToolCallIDs, strings.TrimSpace(observation.ToolCallID)) {
		return workflowRepairBudgetDecision{Applies: true, Exhausted: state.Exhausted, BlockReason: workflowRepairBlockReason(state), State: state}
	}

	now := observation.ObservedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if state.StartedAt.IsZero() {
		state.StartedAt = now
	}
	state.UpdatedAt = now
	state.ElapsedMillis = workflowRepairMaxInt64(0, now.Sub(state.StartedAt).Milliseconds())
	state.RelevantCalls++
	state.LastStage = stage
	state.LastToolCallID = strings.TrimSpace(observation.ToolCallID)
	state.SeenToolCallIDs = append(state.SeenToolCallIDs, state.LastToolCallID)
	if !workflowRepairContainsInt(state.CountedModelIterations, observation.Iteration) {
		state.CountedModelIterations = append(state.CountedModelIterations, observation.Iteration)
		state.PromptTokens += workflowRepairMaxInt(0, observation.PromptTokens)
		state.CompletionTokens += workflowRepairMaxInt(0, observation.CompletionTokens)
		state.TotalTokens += workflowRepairMaxInt(0, observation.TotalTokens)
		state.ModelDurationMillis += workflowRepairMaxInt64(0, observation.ModelDuration.Milliseconds())
	}

	failed := observation.Result.ExecutionFailed()
	diagnostics := workflowRepairDiagnostics(observation.Result.StructuredContent)
	if failed {
		state.FailureCount++
		if !workflowRepairContainsInt(state.FailureIterations, observation.Iteration) {
			state.FailureIterations = append(state.FailureIterations, observation.Iteration)
		}
		if !state.InitialFailureObserved {
			state.InitialFailureObserved = true
			state.InitialFailureIteration = observation.Iteration
		}
		state.RepairAttempts = workflowRepairMaxInt(0, len(state.FailureIterations)-1)
		state.LastDiagnostics = diagnostics
		state.LastDiagnosticDigest = workflowRepairDiagnosticDigest(observation.Result.StructuredContent, observation.Result.Content)
		state.Exhausted = state.RepairAttempts >= state.MaxRepairAttempts
		if state.Exhausted {
			state.AllowedNextActions = workflowRepairAllowedNextActions()
		}
	}
	repairRound := workflowRepairRoundForIteration(state, observation.Iteration)
	state.History = append(state.History, workflowRepairAttemptRecord{
		Iteration: observation.Iteration, RepairRound: repairRound, ToolCallID: state.LastToolCallID,
		Stage: stage, Failed: failed, Diagnostics: diagnostics, ObservedAt: now,
	})
	if len(state.History) > workflowRepairHistoryLimit {
		state.History = append([]workflowRepairAttemptRecord(nil), state.History[len(state.History)-workflowRepairHistoryLimit:]...)
	}
	storeWorkflowRepairBudgetState(snapshot, state)
	return workflowRepairBudgetDecision{
		Applies: true, Exhausted: state.Exhausted, BlockReason: workflowRepairBlockReason(state), State: state,
	}
}

func workflowRepairStage(snapshot *TurnSnapshot, metadata tooling.ToolMetadata) (string, bool) {
	if snapshot == nil || snapshot.TurnAssembly == nil || strings.TrimSpace(snapshot.TurnAssembly.AdmissionFacts.Profile) != RuntimePromptProfileWorkflowAgent {
		return "", false
	}
	if !workflowRepairContainsString(metadata.Profiles, RuntimePromptProfileWorkflowAgent) && !workflowRepairContainsString(metadata.Discovery.AgentProfiles, RuntimePromptProfileWorkflowAgent) {
		return "", false
	}
	kinds := make(map[string]struct{}, len(metadata.Discovery.OperationKinds))
	for _, kind := range metadata.Discovery.OperationKinds {
		kinds[strings.ToLower(strings.TrimSpace(kind))] = struct{}{}
	}
	for _, stage := range []string{"compile", "preview", "validate"} {
		if _, ok := kinds[stage]; ok {
			return stage, true
		}
	}
	return "", false
}

func recordWorkflowRepairCancellation(snapshot *TurnSnapshot, reason string, canceledAt time.Time) {
	if snapshot == nil || snapshot.TurnAssembly == nil || strings.TrimSpace(snapshot.TurnAssembly.AdmissionFacts.Profile) != RuntimePromptProfileWorkflowAgent {
		return
	}
	state, err := workflowRepairBudgetStateFromSnapshot(snapshot)
	if err != nil {
		state = newWorkflowRepairBudgetState()
		state.IntegrityError = "persisted repair budget state is invalid"
	}
	if state.Canceled {
		return
	}
	canceledAt = canceledAt.UTC()
	if canceledAt.IsZero() {
		canceledAt = time.Now().UTC()
	}
	if state.StartedAt.IsZero() {
		state.StartedAt = canceledAt
	}
	state.Canceled = true
	state.CancelReason = strings.TrimSpace(reason)
	state.CanceledAt = &canceledAt
	state.UpdatedAt = canceledAt
	state.ElapsedMillis = workflowRepairMaxInt64(0, canceledAt.Sub(state.StartedAt).Milliseconds())
	storeWorkflowRepairBudgetState(snapshot, state)
}

func workflowRepairBudgetStateFromSnapshot(snapshot *TurnSnapshot) (workflowRepairBudgetState, error) {
	state := newWorkflowRepairBudgetState()
	if snapshot == nil || len(snapshot.Metadata) == 0 || strings.TrimSpace(snapshot.Metadata[workflowRepairBudgetMetadataKey]) == "" {
		return state, nil
	}
	if err := json.Unmarshal([]byte(snapshot.Metadata[workflowRepairBudgetMetadataKey]), &state); err != nil {
		return workflowRepairBudgetState{}, fmt.Errorf("decode workflow repair budget: %w", err)
	}
	if state.SchemaVersion != workflowRepairBudgetSchemaVersion || state.MaxRepairAttempts != workflowRepairMaxAttempts || state.RepairAttempts < 0 || state.FailureCount < 0 {
		return workflowRepairBudgetState{}, fmt.Errorf("invalid workflow repair budget state")
	}
	return state, nil
}

func newWorkflowRepairBudgetState() workflowRepairBudgetState {
	return workflowRepairBudgetState{SchemaVersion: workflowRepairBudgetSchemaVersion, MaxRepairAttempts: workflowRepairMaxAttempts}
}

func storeWorkflowRepairBudgetState(snapshot *TurnSnapshot, state workflowRepairBudgetState) {
	if snapshot == nil {
		return
	}
	if snapshot.Metadata == nil {
		snapshot.Metadata = map[string]string{}
	}
	raw, err := json.Marshal(state)
	if err == nil {
		snapshot.Metadata[workflowRepairBudgetMetadataKey] = string(raw)
	}
}

func preserveWorkflowRepairBudgetMetadata(frozen, merged map[string]string) map[string]string {
	value := strings.TrimSpace(frozen[workflowRepairBudgetMetadataKey])
	if value == "" {
		return merged
	}
	if merged == nil {
		merged = map[string]string{}
	}
	merged[workflowRepairBudgetMetadataKey] = value
	return merged
}

func workflowRepairDiagnostics(raw json.RawMessage) []workflowRepairDiagnostic {
	if len(raw) == 0 {
		return nil
	}
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]workflowRepairDiagnostic, 0, 4)
	var walk func(any)
	walk = func(value any) {
		if len(out) >= 4 {
			return
		}
		switch typed := value.(type) {
		case map[string]any:
			if list, ok := typed["diagnostics"].([]any); ok {
				for _, item := range list {
					entry, ok := item.(map[string]any)
					if !ok {
						continue
					}
					diagnostic := workflowRepairDiagnostic{
						Code: limitedWorkflowRepairText(entry["code"], 80), Path: firstWorkflowRepairPath(entry),
						Message: limitedWorkflowRepairText(entry["message"], 240), Suggestion: limitedWorkflowRepairText(entry["suggestion"], 240),
					}
					key := diagnostic.Code + "\x00" + diagnostic.Path + "\x00" + diagnostic.Message
					if _, duplicate := seen[key]; duplicate || key == "\x00\x00" {
						continue
					}
					seen[key] = struct{}{}
					out = append(out, diagnostic)
					if len(out) >= 4 {
						return
					}
				}
			}
			keys := make([]string, 0, len(typed))
			for key := range typed {
				if key != "diagnostics" {
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(typed[key])
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(root)
	return out
}

func firstWorkflowRepairPath(entry map[string]any) string {
	for _, key := range []string{"path", "field", "nodeId", "edgeId", "clientRef"} {
		if value := limitedWorkflowRepairText(entry[key], 160); value != "" {
			return value
		}
	}
	return ""
}

func limitedWorkflowRepairText(value any, limit int) string {
	text, _ := value.(string)
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}

func workflowRepairDiagnosticDigest(raw json.RawMessage, content string) string {
	payload := append(append([]byte(nil), raw...), []byte("\x00"+content)...)
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func workflowRepairRoundForIteration(state workflowRepairBudgetState, iteration int) int {
	for index, failedIteration := range state.FailureIterations {
		if failedIteration == iteration {
			return index
		}
	}
	if state.InitialFailureObserved && iteration != state.InitialFailureIteration {
		return workflowRepairMinInt(state.MaxRepairAttempts, len(state.FailureIterations))
	}
	return 0
}

func workflowRepairAllowedNextActions() []string {
	return []string{"review_typed_diagnostics", "revise_affected_plan_ir_paths_or_constraints", "start_new_turn_with_retained_candidate"}
}

func workflowRepairBlockReason(state workflowRepairBudgetState) string {
	if !state.Exhausted {
		return ""
	}
	blockers := make([]string, 0, len(state.LastDiagnostics))
	for _, diagnostic := range state.LastDiagnostics {
		label := strings.TrimSpace(strings.Join([]string{diagnostic.Code, diagnostic.Path}, " "))
		if label == "" {
			label = diagnostic.Message
		}
		if label != "" {
			blockers = append(blockers, label)
		}
	}
	if len(blockers) == 0 {
		blockers = append(blockers, firstNonEmptyString(state.IntegrityError, "typed diagnostics remain unresolved"))
	}
	return fmt.Sprintf("Workflow repair budget exhausted after %d repair attempts at %s; blockers: %s. The Candidate is retained and no further mutation will be dispatched in this turn. Review the typed diagnostics, revise only affected PlanIR paths or constraints, then start a new turn.", state.MaxRepairAttempts, firstNonEmptyString(state.LastStage, "validation"), strings.Join(blockers, "; "))
}

func (k *RuntimeKernel) blockWorkflowRepairBudget(session *SessionState, snapshot *TurnSnapshot, tc ToolCall, decision workflowRepairBudgetDecision) (*TurnResult, error) {
	reason := firstNonEmptyString(decision.BlockReason, workflowRepairBlockReason(decision.State))
	appendAgentItem(snapshot, newAgentItem(
		fmt.Sprintf("%s-workflow-repair-budget", snapshot.ID), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, reason,
		map[string]any{"failureKind": "workflow_repair_budget_exhausted", "repairAttempts": decision.State.RepairAttempts, "maxRepairAttempts": decision.State.MaxRepairAttempts, "allowedNextActions": decision.State.AllowedNextActions},
	))
	if err := k.markTurnFailed(session, snapshot, tc, DispatchResult{ToolCallID: tc.ID, Error: reason, Outcome: "workflow_repair_budget_exhausted", Source: "runtimekernel"}); err != nil {
		return nil, err
	}
	return &TurnResult{
		SessionType: snapshot.SessionType, Mode: snapshot.Mode, SessionID: snapshot.SessionID, TurnID: snapshot.ID,
		ClientTurnID: snapshot.ClientTurnID, ClientMessageID: snapshot.ClientMessageID, Status: "blocked", Error: reason,
	}, nil
}

func workflowRepairContainsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func workflowRepairContainsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func workflowRepairMaxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func workflowRepairMinInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func workflowRepairMaxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
