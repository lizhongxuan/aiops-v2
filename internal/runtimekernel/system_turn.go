package runtimekernel

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
	runtimestate "aiops-v2/internal/runtimekernel/state"
)

// SystemTurnKind identifies a deterministic, non-model runtime response.
type SystemTurnKind string

const (
	SystemTurnKindNotice                SystemTurnKind = "notice"
	SystemTurnKindDeterministicPlan     SystemTurnKind = "deterministic_plan"
	SystemTurnKindDeterministicArtifact SystemTurnKind = "deterministic_artifact"
)

func (k SystemTurnKind) IsValid() bool {
	switch k {
	case SystemTurnKindNotice, SystemTurnKindDeterministicPlan, SystemTurnKindDeterministicArtifact:
		return true
	default:
		return false
	}
}

// SystemTurnRecord is the typed durable fact that distinguishes a committed
// deterministic response from a model-driven turn.
type SystemTurnRecord struct {
	Kind           SystemTurnKind      `json:"kind"`
	ContractStatus FinalContractStatus `json:"contractStatus"`
	FailureCodes   []string            `json:"failureCodes,omitempty"`
}

func (r SystemTurnRecord) Validate() error {
	if !r.Kind.IsValid() {
		return fmt.Errorf("invalid system turn kind %q", r.Kind)
	}
	if err := validateSystemTurnContractStatus(r.ContractStatus); err != nil {
		return err
	}
	for i, code := range r.FailureCodes {
		if strings.TrimSpace(code) == "" {
			return fmt.Errorf("system turn failure code[%d] is empty", i)
		}
	}
	return nil
}

// SystemTurnOutput is the deterministic output to commit. AgentItems are
// limited to completed domain facts; runtime-owned lifecycle, assistant,
// model, and tool facts are constructed only by RuntimeKernel.
type SystemTurnOutput struct {
	Kind           SystemTurnKind        `json:"kind"`
	FinalText      string                `json:"finalText"`
	ContractStatus FinalContractStatus   `json:"contractStatus"`
	FailureCodes   []string              `json:"failureCodes,omitempty"`
	AgentItems     []agentstate.TurnItem `json:"agentItems,omitempty"`
}

func (o SystemTurnOutput) Validate() error {
	if strings.TrimSpace(o.FinalText) == "" {
		return fmt.Errorf("system turn final text is required")
	}
	record := SystemTurnRecord{Kind: o.Kind, ContractStatus: o.ContractStatus, FailureCodes: o.FailureCodes}
	if err := record.Validate(); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(o.AgentItems))
	for i, item := range o.AgentItems {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("system turn agent item[%d]: %w", i, err)
		}
		if item.Status != agentstate.ItemStatusCompleted {
			return fmt.Errorf("system turn agent item[%d] must be completed", i)
		}
		switch item.Type {
		case agentstate.TurnItemTypeRouteSelected,
			agentstate.TurnItemTypePlan,
			agentstate.TurnItemTypeEvidence,
			agentstate.TurnItemTypeEvidenceCollected:
		default:
			return fmt.Errorf("system turn agent item[%d] type %q is not a deterministic domain fact", i, item.Type)
		}
		if _, ok := seen[item.ID]; ok {
			return fmt.Errorf("system turn agent item id %q is duplicated", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
	return nil
}

// SystemTurnRequest commits one deterministic response through the canonical
// runtime lifecycle without invoking a model or dispatching a tool.
type SystemTurnRequest struct {
	Turn   TurnRequest      `json:"turn"`
	Output SystemTurnOutput `json:"output"`
}

func (r SystemTurnRequest) Validate() error {
	if err := r.Turn.Validate(); err != nil {
		return fmt.Errorf("system turn request: %w", err)
	}
	if strings.TrimSpace(r.Turn.Input) == "" {
		return fmt.Errorf("system turn user input is required")
	}
	if err := r.Output.Validate(); err != nil {
		return err
	}
	return nil
}

func validateSystemTurnContractStatus(status FinalContractStatus) error {
	switch status {
	case FinalContractStatusPartial,
		FinalContractStatusBlocked,
		FinalContractStatusNeedsEvidence,
		FinalContractStatusApprovalDenied,
		FinalContractStatusToolUnavailable:
		return nil
	case FinalContractStatusVerified:
		return fmt.Errorf("verified system turn requires typed checked evidence and is not supported")
	default:
		return fmt.Errorf("invalid system turn contract status %q", status)
	}
}

// CommitSystemTurn records a deterministic terminal response using the same
// checkpoint, final-facts, transport, session, and ownership boundaries as a
// model-driven turn. It deliberately contains no model or tool execution path.
func (k *RuntimeKernel) CommitSystemTurn(ctx context.Context, req SystemTurnRequest) (TurnResult, error) {
	if k == nil || k.sessions == nil {
		return TurnResult{}, fmt.Errorf("runtime kernel sessions are required")
	}
	if err := contextError(ctx); err != nil {
		return TurnResult{}, err
	}
	if err := req.Validate(); err != nil {
		return TurnResult{}, fmt.Errorf("invalid system turn request: %w", err)
	}

	turnReq := req.Turn
	turnID := strings.TrimSpace(turnReq.TurnID)
	if turnID == "" {
		turnID = fmt.Sprintf("turn-%d", time.Now().UnixNano())
	}
	if err := validateSystemTurnItemIDs(turnID, req.Output.AgentItems); err != nil {
		return TurnResult{}, err
	}
	session := k.sessions.GetOrCreate(turnReq.SessionID, turnReq.SessionType, turnReq.Mode)
	if current := session.CurrentTurn; current != nil && current.ID != turnID && !current.Lifecycle.IsTerminal() {
		pending := appendPendingInputToActiveTurn(session, current, turnReq)
		k.persistTurnSnapshot(session, current)
		return TurnResult{
			SessionType: turnReq.SessionType, Mode: turnReq.Mode, SessionID: session.ID,
			TurnID: current.ID, ClientTurnID: pending.ClientTurnID, ClientMessageID: pending.ClientMessageID,
			Status: "pending_input",
		}, nil
	}
	if current := session.CurrentTurn; current != nil && current.ID == turnID {
		if current.Lifecycle.IsTerminal() {
			if systemTurnMatches(current, req.Output) {
				return systemTurnResult(session, current), nil
			}
			return TurnResult{}, fmt.Errorf("turn %q is already terminal with different system facts", turnID)
		}
		if current.Lifecycle != TurnLifecycleRunning || current.ResumeState != TurnResumeStateNone {
			return TurnResult{}, fmt.Errorf("turn %q cannot commit system output from lifecycle %q/%q", turnID, current.Lifecycle, current.ResumeState)
		}
	}
	if current := session.CurrentTurn; current != nil && current.ID != turnID {
		upsertTurnHistory(&session.TurnHistory, *current)
	}

	persistSessionTargetRequestState(session, turnReq)
	if session.HostID == "" {
		session.HostID = strings.TrimSpace(turnReq.HostID)
	}
	snapshot := k.ensureCurrentTurnSnapshot(session, turnReq, turnID)
	userMessageID := firstNonEmptyString(strings.TrimSpace(turnReq.ClientMessageID), turnID+":user")
	if !systemTurnMessageExists(session.Messages, userMessageID, "user") {
		now := time.Now().UTC()
		session.Messages = append(session.Messages, Message{
			ID: userMessageID, ClientMessageID: turnReq.ClientMessageID, ClientTurnID: turnReq.ClientTurnID,
			Role: "user", Content: strings.TrimSpace(turnReq.Input), Timestamp: now,
		})
	}
	userItemID := turnID + "-user-message"
	if !hasAgentItemID(snapshot.AgentItems, userItemID) {
		appendAgentItem(snapshot, newAgentItem(
			userItemID, agentstate.TurnItemTypeUserMessage, agentstate.ItemStatusCompleted,
			turnReq.Input, map[string]string{"messageId": userMessageID, "prompt": turnReq.Input},
		))
	}
	recomputeContextWindow(&session.Context, session.Messages)
	k.persistTurnSnapshot(session, snapshot)
	k.emitRuntimeEvent(EventTurnStarted, session.ID, turnID, map[string]any{
		"clientTurnId": turnReq.ClientTurnID, "clientMessageId": turnReq.ClientMessageID,
		"systemTurnKind": req.Output.Kind,
	})

	now := time.Now().UTC()
	if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionTurnCompleted, TurnLifecycleCompleted); err != nil {
		return TurnResult{}, err
	}
	record := SystemTurnRecord{
		Kind: req.Output.Kind, ContractStatus: req.Output.ContractStatus,
		FailureCodes: compactStringList(req.Output.FailureCodes),
	}
	facts := systemTurnFinalRuntimeFacts(record)
	contract := BuildTerminalFinalContract(req.Output.FinalText, record.ContractStatus, record.FailureCodes)
	checkpoint := newCheckpointMetadata(session.ID, turnID, snapshot.Iteration, nextCheckpointSequence(snapshot), "system_turn", TurnLifecycleCompleted, TurnResumeStateNone)
	assistantMessageID := turnID + ":assistant"
	boundary, action := systemTurnFinalBoundary(record.ContractStatus)
	finalCommit := assistantOutputCommitInput{
		TurnID: turnID, Iteration: snapshot.Iteration, MessageID: assistantMessageID,
		AssistantText: req.Output.FinalText, EvidenceBoundary: boundary,
		BoundaryAction: action, FinalContract: &contract,
	}
	recordSnapshot := *snapshot
	recordSnapshot.AgentItems = append([]agentstate.TurnItem(nil), snapshot.AgentItems...)
	recordSnapshot.SystemTurn = &record
	recordSnapshot.Lifecycle = TurnLifecycleCompleted
	recordSnapshot.ResumeState = TurnResumeStateNone
	recordSnapshot.PendingApprovals = nil
	recordSnapshot.PendingEvidence = nil
	recordSnapshot.LatestCheckpoint = checkpoint
	recordSnapshot.UpdatedAt = now
	recordSnapshot.CompletedAt = &now
	for _, item := range cloneSystemTurnAgentItems(req.Output.AgentItems) {
		appendAgentItem(&recordSnapshot, item)
	}
	commitFinalAssistantOutput(&recordSnapshot, finalCommit)
	recordSnapshot.FinalOutput = FinalTextFromAssistantMessage(&recordSnapshot)
	if err := recordSnapshot.Validate(); err != nil {
		return TurnResult{}, fmt.Errorf("invalid committed system turn: %w", err)
	}
	if err := k.recordCanonicalCheckpoint(ctx, &recordSnapshot, checkpoint); err != nil {
		k.retainSystemTurnRolloutHead(session, snapshot, recordSnapshot.CanonicalRolloutHead)
		return TurnResult{}, err
	}
	if err := k.recordCanonicalFinalFacts(ctx, &recordSnapshot, facts, contract); err != nil {
		k.retainSystemTurnRolloutHead(session, snapshot, recordSnapshot.CanonicalRolloutHead)
		return TurnResult{}, err
	}
	if err := k.recordCanonicalTransportProjection(ctx, &recordSnapshot, TurnLifecycleCompleted, TurnResumeStateNone, checkpoint.ID, &contract); err != nil {
		k.retainSystemTurnRolloutHead(session, snapshot, recordSnapshot.CanonicalRolloutHead)
		return TurnResult{}, err
	}

	*snapshot = recordSnapshot
	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteTurnLifecycle, OwnerRuntimeKernel)
	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteAssistantMessage, OwnerRuntimeKernel)
	if !systemTurnMessageExists(session.Messages, assistantMessageID, "assistant") {
		session.Messages = append(session.Messages, Message{
			ID: assistantMessageID, ClientTurnID: turnReq.ClientTurnID,
			Role: "assistant", Content: snapshot.FinalOutput, Timestamp: now,
		})
	}
	session.PendingApprovals = nil
	session.PendingEvidence = nil
	session.LatestCheckpoint = checkpoint
	recomputeContextWindow(&session.Context, session.Messages)
	k.persistTurnSnapshot(session, snapshot)
	k.emitRuntimeEvent(EventTurnComplete, session.ID, turnID, map[string]any{"systemTurnKind": record.Kind})
	return systemTurnResult(session, snapshot), nil
}

func validateSystemTurnItemIDs(turnID string, items []agentstate.TurnItem) error {
	reserved := map[string]struct{}{
		turnID + "-user-message":          {},
		assistantMessageItemID(turnID, 0): {},
		finalResponseItemID(turnID, 0):    {},
	}
	for _, item := range items {
		if _, ok := reserved[strings.TrimSpace(item.ID)]; ok {
			return fmt.Errorf("system turn agent item id %q is runtime-owned", item.ID)
		}
	}
	return nil
}

func systemTurnFinalRuntimeFacts(record SystemTurnRecord) FinalRuntimeFacts {
	completion := FinalCompletionStatusBlocked
	if record.ContractStatus == FinalContractStatusPartial {
		completion = FinalCompletionStatusPartial
	}
	return FinalRuntimeFacts{
		CompletionStatus: completion,
		PostcheckStatus:  FinalPostcheckStatusNotRequired,
		RollbackStatus:   FinalRollbackStatusNotRequired,
		FailureCodes:     append([]string(nil), record.FailureCodes...),
	}
}

func systemTurnFinalBoundary(status FinalContractStatus) (string, FinalMessageBoundaryAction) {
	if status == FinalContractStatusPartial || status == FinalContractStatusNeedsEvidence {
		return "limited", FinalMessageBoundaryConstrain
	}
	return "blocked", FinalMessageBoundaryBlock
}

func systemTurnMatches(snapshot *TurnSnapshot, output SystemTurnOutput) bool {
	if snapshot == nil || snapshot.SystemTurn == nil || snapshot.Lifecycle != TurnLifecycleCompleted {
		return false
	}
	record := snapshot.SystemTurn
	return record.Kind == output.Kind && record.ContractStatus == output.ContractStatus &&
		strings.TrimSpace(snapshot.FinalOutput) == strings.TrimSpace(output.FinalText) &&
		systemTurnStringsEqual(record.FailureCodes, compactStringList(output.FailureCodes))
}

func systemTurnStringsEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func systemTurnResult(session *SessionState, snapshot *TurnSnapshot) TurnResult {
	return TurnResult{
		SessionType: snapshot.SessionType, Mode: snapshot.Mode, SessionID: session.ID,
		TurnID: snapshot.ID, ClientTurnID: snapshot.ClientTurnID, ClientMessageID: snapshot.ClientMessageID,
		Status: "completed", Output: snapshot.FinalOutput,
	}
}

func systemTurnMessageExists(messages []Message, id, role string) bool {
	for _, message := range messages {
		if message.ID == id && message.Role == role {
			return true
		}
	}
	return false
}

func cloneSystemTurnAgentItems(items []agentstate.TurnItem) []agentstate.TurnItem {
	out := append([]agentstate.TurnItem(nil), items...)
	for i := range out {
		out[i].Payload.Data = append([]byte(nil), out[i].Payload.Data...)
	}
	return out
}

func (k *RuntimeKernel) retainSystemTurnRolloutHead(session *SessionState, snapshot *TurnSnapshot, head *CanonicalRolloutHeadRef) {
	if snapshot == nil {
		return
	}
	snapshot.CanonicalRolloutHead = head
	k.persistTurnSnapshot(session, snapshot)
}
