package runtimekernel

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type ActiveTurnMigrationDecision string

const (
	ActiveTurnMigrationDecisionResumable           ActiveTurnMigrationDecision = "resumable"
	ActiveTurnMigrationDecisionRequiresManualClose ActiveTurnMigrationDecision = "requires_manual_close"
	ActiveTurnMigrationDecisionUnrecoverable       ActiveTurnMigrationDecision = "unrecoverable"
	ActiveTurnMigrationDecisionTerminalNoop        ActiveTurnMigrationDecision = "terminal_noop"
)

type ActiveTurnMigrationOptions struct {
	Now         time.Time
	DryRun      bool
	StoreDriver string
}

type ActiveTurnMigrationApplyOptions struct {
	Now    time.Time
	DryRun bool
}

type ActiveTurnMigrationItem struct {
	SessionID     string                      `json:"sessionId"`
	TurnID        string                      `json:"turnId"`
	Lifecycle     string                      `json:"lifecycle"`
	ResumeState   string                      `json:"resumeState"`
	Decision      ActiveTurnMigrationDecision `json:"decision"`
	Reasons       []string                    `json:"reasons,omitempty"`
	MutatingState string                      `json:"mutatingState,omitempty"`
	ArgsHashes    []string                    `json:"argsHashes,omitempty"`
}

type ActiveTurnMigrationReport struct {
	RunID       string                     `json:"runId"`
	StartedAt   time.Time                  `json:"startedAt"`
	CompletedAt time.Time                  `json:"completedAt"`
	DryRun      bool                       `json:"dryRun"`
	StoreDriver string                     `json:"storeDriver,omitempty"`
	Items       []ActiveTurnMigrationItem  `json:"items"`
	Summary     ActiveTurnMigrationSummary `json:"summary"`
}

type ActiveTurnMigrationSummary struct {
	Total               int `json:"total"`
	Resumable           int `json:"resumable"`
	RequiresManualClose int `json:"requiresManualClose"`
	Unrecoverable       int `json:"unrecoverable"`
	TerminalNoop        int `json:"terminalNoop"`
}

func ClassifyActiveTurnForMigration(snapshot *TurnSnapshot, now time.Time) ActiveTurnMigrationItem {
	if now.IsZero() {
		now = time.Now()
	}
	item := ActiveTurnMigrationItem{Decision: ActiveTurnMigrationDecisionUnrecoverable}
	if snapshot == nil {
		item.Reasons = []string{"snapshot is nil"}
		return item
	}
	item.SessionID = snapshot.SessionID
	item.TurnID = snapshot.ID
	item.Lifecycle = string(snapshot.Lifecycle)
	item.ResumeState = string(snapshot.ResumeState)
	item.ArgsHashes = collectTurnArgumentHashes(snapshot)
	item.MutatingState = describeTurnMutatingState(snapshot)

	recovery := InspectTurnRecovery(snapshot)
	item.Reasons = append(item.Reasons, recovery.Reasons...)

	if snapshot.Lifecycle.IsTerminal() {
		item.Decision = ActiveTurnMigrationDecisionTerminalNoop
		item.Reasons = nil
		return item
	}

	if snapshot.Lifecycle.CanResume() {
		switch snapshot.ResumeState {
		case TurnResumeStatePendingApproval:
			if len(snapshot.PendingApprovals) == 0 {
				item.Decision = ActiveTurnMigrationDecisionRequiresManualClose
				item.Reasons = appendReason(item.Reasons, "pending approval resume state has no pending approval records")
				return item
			}
		case TurnResumeStatePendingEvidence:
			if len(snapshot.PendingEvidence) == 0 {
				item.Decision = ActiveTurnMigrationDecisionRequiresManualClose
				item.Reasons = appendReason(item.Reasons, "pending evidence resume state has no pending evidence records")
				return item
			}
		}
		if recovery.Resumable && len(recovery.Reasons) == 0 {
			item.Decision = ActiveTurnMigrationDecisionResumable
			return item
		}
		item.Decision = ActiveTurnMigrationDecisionRequiresManualClose
		return item
	}

	if snapshot.Lifecycle == TurnLifecycleRunning && !recovery.HasCheckpoint && !recovery.HasIterations {
		item.Decision = ActiveTurnMigrationDecisionUnrecoverable
		item.Reasons = appendReason(item.Reasons, "running turn has no checkpoint or iteration history")
		return item
	}

	item.Decision = ActiveTurnMigrationDecisionRequiresManualClose
	if snapshot.Lifecycle == TurnLifecycleRunning {
		item.Reasons = appendReason(item.Reasons, "running turn requires manual close before production migration")
	}
	return item
}

func BuildActiveTurnMigrationReport(runID string, sessions []*SessionState, opts ActiveTurnMigrationOptions) ActiveTurnMigrationReport {
	startedAt := opts.Now
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	report := ActiveTurnMigrationReport{
		RunID:       strings.TrimSpace(runID),
		StartedAt:   startedAt,
		CompletedAt: startedAt,
		DryRun:      opts.DryRun,
		StoreDriver: strings.TrimSpace(opts.StoreDriver),
	}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if session.CurrentTurn != nil {
			report.Items = append(report.Items, ClassifyActiveTurnForMigration(session.CurrentTurn, startedAt))
		}
		for idx := range session.TurnHistory {
			report.Items = append(report.Items, ClassifyActiveTurnForMigration(&session.TurnHistory[idx], startedAt))
		}
	}
	report.Summary = summarizeActiveTurnMigration(report.Items)
	report.CompletedAt = time.Now()
	if opts.Now.IsZero() {
		return report
	}
	report.CompletedAt = opts.Now
	return report
}

func ApplyActiveTurnMigration(session *SessionState, report ActiveTurnMigrationReport, opts ActiveTurnMigrationApplyOptions) int {
	if session == nil || opts.DryRun {
		return 0
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	decisions := make(map[string]ActiveTurnMigrationDecision)
	for _, item := range report.Items {
		if item.SessionID != session.ID {
			continue
		}
		if item.Decision == ActiveTurnMigrationDecisionUnrecoverable || item.Decision == ActiveTurnMigrationDecisionRequiresManualClose {
			decisions[item.TurnID] = item.Decision
		}
	}
	changed := 0
	if session.CurrentTurn != nil {
		if applyActiveTurnMigrationToTurn(session.CurrentTurn, decisions[session.CurrentTurn.ID], now) {
			changed++
		}
	}
	for idx := range session.TurnHistory {
		if applyActiveTurnMigrationToTurn(&session.TurnHistory[idx], decisions[session.TurnHistory[idx].ID], now) {
			changed++
		}
	}
	if changed > 0 {
		session.UpdatedAt = now
	}
	return changed
}

func applyActiveTurnMigrationToTurn(turn *TurnSnapshot, decision ActiveTurnMigrationDecision, now time.Time) bool {
	if turn == nil {
		return false
	}
	switch decision {
	case ActiveTurnMigrationDecisionUnrecoverable, ActiveTurnMigrationDecisionRequiresManualClose:
	default:
		return false
	}
	if turn.Metadata == nil {
		turn.Metadata = make(map[string]string)
	}
	turn.Metadata["recovery.decision"] = string(decision)
	turn.Metadata["recovery.migratedAt"] = now.UTC().Format(time.RFC3339)
	turn.UpdatedAt = now
	if decision == ActiveTurnMigrationDecisionRequiresManualClose {
		turn.Metadata["recovery.requiresManualClose"] = "true"
		return true
	}
	turn.Lifecycle = TurnLifecycleFailed
	turn.ResumeState = TurnResumeStateNone
	turn.Error = strings.TrimSpace(firstNonEmpty(turn.Error, "unrecoverable active turn migration"))
	if !strings.Contains(turn.Error, "unrecoverable active turn migration") {
		turn.Error = strings.TrimSpace(turn.Error + "; unrecoverable active turn migration")
	}
	turn.CompletedAt = &now
	MarkRunningMutatingInvocationsSideEffectUnknown(turn, now)
	return true
}

func summarizeActiveTurnMigration(items []ActiveTurnMigrationItem) ActiveTurnMigrationSummary {
	summary := ActiveTurnMigrationSummary{Total: len(items)}
	for _, item := range items {
		switch item.Decision {
		case ActiveTurnMigrationDecisionResumable:
			summary.Resumable++
		case ActiveTurnMigrationDecisionRequiresManualClose:
			summary.RequiresManualClose++
		case ActiveTurnMigrationDecisionUnrecoverable:
			summary.Unrecoverable++
		case ActiveTurnMigrationDecisionTerminalNoop:
			summary.TerminalNoop++
		}
	}
	return summary
}

func collectTurnArgumentHashes(turn *TurnSnapshot) []string {
	if turn == nil {
		return nil
	}
	seen := make(map[string]struct{})
	for _, iter := range turn.Iterations {
		for _, invocation := range iter.ToolInvocations {
			addHash(seen, invocation.ArgumentsHash)
		}
		for _, call := range iter.ToolCalls {
			addHash(seen, toolInvocationArgumentsHash(call))
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func addHash(seen map[string]struct{}, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	seen[value] = struct{}{}
}

func describeTurnMutatingState(turn *TurnSnapshot) string {
	if turn == nil {
		return ""
	}
	runningMutating := 0
	completedMutating := 0
	for _, iter := range turn.Iterations {
		for _, invocation := range iter.ToolInvocations {
			if !invocation.Mutating {
				continue
			}
			switch invocation.Status {
			case ToolInvocationRunning:
				runningMutating++
			case ToolInvocationCompleted:
				completedMutating++
			}
		}
	}
	switch {
	case runningMutating > 0:
		return fmt.Sprintf("running_mutating:%d", runningMutating)
	case completedMutating > 0:
		return fmt.Sprintf("completed_mutating:%d", completedMutating)
	default:
		return ""
	}
}

func appendReason(reasons []string, reason string) []string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return reasons
	}
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}
