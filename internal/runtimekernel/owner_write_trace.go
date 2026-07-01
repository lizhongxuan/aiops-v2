package runtimekernel

import (
	"strings"
	"time"

	"aiops-v2/internal/promptinput"
)

type OwnerWriteResponsibility string

const (
	OwnerWriteTurnLifecycle     OwnerWriteResponsibility = "turn_lifecycle"
	OwnerWriteAssistantMessage  OwnerWriteResponsibility = "assistant_message"
	OwnerWriteApprovalLedger    OwnerWriteResponsibility = "approval_ledger"
	OwnerWriteToolResult        OwnerWriteResponsibility = "tool_result"
	OwnerWriteContextCompaction OwnerWriteResponsibility = "context_compaction"
)

const (
	OwnerRuntimeKernel   = "runtimekernel.RuntimeKernel"
	OwnerPendingApproval = "runtimekernel.PendingApproval"
	OwnerToolDispatcher  = "runtimekernel.ToolDispatcher"
	OwnerContextPipeline = "runtimekernel.ContextPipeline"
)

type OwnerWriteOutcome string

const (
	OwnerWriteOutcomeAccepted         OwnerWriteOutcome = "accepted"
	OwnerWriteOutcomeRejectedNonOwner OwnerWriteOutcome = "rejected_non_owner"
	OwnerWriteOutcomeLegacyAdapter    OwnerWriteOutcome = "legacy_adapter"
)

type OwnerWriteTraceInput struct {
	Responsibility OwnerWriteResponsibility
	Writer         string
	SessionID      string
	TurnID         string
	Outcome        OwnerWriteOutcome
	CreatedAt      time.Time
}

type OwnerWriteTrace struct {
	Responsibility OwnerWriteResponsibility `json:"responsibility"`
	Owner          string                   `json:"owner"`
	Writer         string                   `json:"writer"`
	SessionID      string                   `json:"sessionId,omitempty"`
	TurnID         string                   `json:"turnId,omitempty"`
	Outcome        OwnerWriteOutcome        `json:"outcome"`
	CreatedAt      time.Time                `json:"createdAt,omitempty"`
}

func NewOwnerWriteTrace(input OwnerWriteTraceInput) OwnerWriteTrace {
	writer := strings.TrimSpace(input.Writer)
	owner := ownerForWriteResponsibility(input.Responsibility)
	outcome := input.Outcome
	if outcome == "" {
		if owner != "" && writer == owner {
			outcome = OwnerWriteOutcomeAccepted
		} else {
			outcome = OwnerWriteOutcomeRejectedNonOwner
		}
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	} else {
		createdAt = createdAt.UTC()
	}
	return OwnerWriteTrace{
		Responsibility: input.Responsibility,
		Owner:          owner,
		Writer:         writer,
		SessionID:      strings.TrimSpace(input.SessionID),
		TurnID:         strings.TrimSpace(input.TurnID),
		Outcome:        outcome,
		CreatedAt:      createdAt,
	}
}

func AppendOwnerWriteTrace(session *SessionState, turn *TurnSnapshot, trace OwnerWriteTrace) {
	if session != nil {
		session.OwnerWriteTraces = append(session.OwnerWriteTraces, trace)
	}
	if turn != nil {
		turn.OwnerWriteTraces = append(turn.OwnerWriteTraces, trace)
	}
}

func appendAcceptedOwnerWriteTrace(session *SessionState, turn *TurnSnapshot, responsibility OwnerWriteResponsibility, writer string) {
	if session == nil && turn == nil {
		return
	}
	sessionID := ""
	turnID := ""
	if session != nil {
		sessionID = session.ID
	}
	if turn != nil {
		turnID = turn.ID
		if sessionID == "" {
			sessionID = turn.SessionID
		}
	}
	if strings.TrimSpace(writer) == "" {
		writer = ownerForWriteResponsibility(responsibility)
	}
	AppendOwnerWriteTrace(session, turn, NewOwnerWriteTrace(OwnerWriteTraceInput{
		Responsibility: responsibility,
		Writer:         writer,
		SessionID:      sessionID,
		TurnID:         turnID,
	}))
}

func promptInputOwnerWriteTraces(traces []OwnerWriteTrace) []promptinput.OwnerWriteTrace {
	if len(traces) == 0 {
		return nil
	}
	out := make([]promptinput.OwnerWriteTrace, 0, len(traces))
	for _, trace := range traces {
		createdAt := ""
		if !trace.CreatedAt.IsZero() {
			createdAt = trace.CreatedAt.UTC().Format(time.RFC3339Nano)
		}
		out = append(out, promptinput.OwnerWriteTrace{
			Responsibility: strings.TrimSpace(string(trace.Responsibility)),
			Owner:          strings.TrimSpace(trace.Owner),
			Writer:         strings.TrimSpace(trace.Writer),
			SessionID:      strings.TrimSpace(trace.SessionID),
			TurnID:         strings.TrimSpace(trace.TurnID),
			Outcome:        strings.TrimSpace(string(trace.Outcome)),
			CreatedAt:      createdAt,
		})
	}
	return out
}

func ownerForWriteResponsibility(responsibility OwnerWriteResponsibility) string {
	switch responsibility {
	case OwnerWriteTurnLifecycle, OwnerWriteAssistantMessage:
		return OwnerRuntimeKernel
	case OwnerWriteApprovalLedger:
		return OwnerPendingApproval
	case OwnerWriteToolResult:
		return OwnerToolDispatcher
	case OwnerWriteContextCompaction:
		return OwnerContextPipeline
	default:
		return ""
	}
}
