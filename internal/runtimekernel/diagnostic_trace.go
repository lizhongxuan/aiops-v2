package runtimekernel

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"aiops-v2/internal/diagnostics"
	"aiops-v2/internal/promptcompiler"
)

func buildRuntimeDiagnosticTrace(
	turnID string,
	session *SessionState,
	req TurnRequest,
	compileCtx promptcompiler.CompileContext,
) diagnostics.DiagnosticTrace {
	if compileCtx.DisableDiagnosticProtocol {
		return diagnostics.DiagnosticTrace{}
	}
	scopeSummary := runtimeDiagnosticScopeSummary(session, req)
	scopeHash := runtimeDiagnosticScopeHash(session, req, scopeSummary)
	input := diagnostics.TraceBuildInput{
		TurnID:           turnID,
		CurrentScope:     diagnostics.DiagnosticScope{Hash: scopeHash, Summary: scopeSummary, Confirmed: scopeSummary != ""},
		Facts:            runtimeDiagnosticFacts(scopeHash, compileCtx),
		ToolFailures:     runtimePendingToolFailures(scopeHash, session),
		RequiresApproval: len(sessionPendingApprovals(session)) > 0,
	}
	for _, pending := range sessionPendingEvidence(session) {
		reason := strings.TrimSpace(pending.Reason)
		if reason == "" {
			reason = fmt.Sprintf("%s requires additional evidence", firstNonEmpty(pending.ToolName, "tool"))
		}
		input.Facts = append(input.Facts, diagnostics.DiagnosticFact{
			ScopeHash: scopeHash,
			Summary:   reason,
			Status:    diagnostics.EvidenceStatusMissing,
			Critical:  true,
		})
	}
	return diagnostics.BuildTrace(input)
}

func runtimeDiagnosticScopeSummary(session *SessionState, req TurnRequest) string {
	hostID := strings.TrimSpace(req.HostID)
	if hostID == "" && session != nil {
		hostID = strings.TrimSpace(session.HostID)
	}
	sessionType := strings.TrimSpace(string(req.SessionType))
	if sessionType == "" && session != nil {
		sessionType = strings.TrimSpace(string(session.Type))
	}
	switch {
	case hostID != "":
		return "host:" + hostID
	case sessionType != "":
		return "session_type:" + sessionType
	default:
		return ""
	}
}

func runtimeDiagnosticScopeHash(session *SessionState, req TurnRequest, summary string) string {
	var parts []string
	if session != nil {
		parts = append(parts, session.ID, string(session.Type), session.HostID)
	}
	parts = append(parts, string(req.SessionType), req.HostID, summary)
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])[:16]
}

func runtimeDiagnosticFacts(scopeHash string, compileCtx promptcompiler.CompileContext) []diagnostics.DiagnosticFact {
	var facts []diagnostics.DiagnosticFact
	for _, section := range compileCtx.ExtraSections {
		title := strings.TrimSpace(section.Title)
		content := strings.TrimSpace(section.Content)
		if content == "" {
			continue
		}
		if !strings.EqualFold(title, "Runtime Environment Context") {
			continue
		}
		facts = append(facts, diagnostics.DiagnosticFact{
			ScopeHash: scopeHash,
			Summary:   compactDiagnosticFact(title, content),
			Status:    diagnostics.EvidenceStatusActive,
		})
	}
	return facts
}

func runtimePendingToolFailures(scopeHash string, session *SessionState) []diagnostics.ToolFailure {
	approvals := sessionPendingApprovals(session)
	if len(approvals) == 0 {
		return nil
	}
	failures := make([]diagnostics.ToolFailure, 0, len(approvals))
	for _, approval := range approvals {
		failures = append(failures, diagnostics.ToolFailure{
			ToolName:  approval.ToolName,
			Semantic:  diagnostics.ToolFailurePolicyBlocked,
			Detail:    firstNonEmpty(approval.Reason, "approval required before execution"),
			Critical:  true,
			ScopeHash: scopeHash,
		})
	}
	return failures
}

func sessionPendingApprovals(session *SessionState) []PendingApproval {
	if session == nil {
		return nil
	}
	if session.CurrentTurn != nil && len(session.CurrentTurn.PendingApprovals) > 0 {
		return session.CurrentTurn.PendingApprovals
	}
	return session.PendingApprovals
}

func sessionPendingEvidence(session *SessionState) []PendingEvidence {
	if session == nil {
		return nil
	}
	if session.CurrentTurn != nil && len(session.CurrentTurn.PendingEvidence) > 0 {
		return session.CurrentTurn.PendingEvidence
	}
	return session.PendingEvidence
}

func compactDiagnosticFact(title, content string) string {
	content = strings.Join(strings.Fields(content), " ")
	if len(content) > 360 {
		content = content[:360] + "..."
	}
	if strings.TrimSpace(title) == "" {
		return content
	}
	return strings.TrimSpace(title) + ": " + content
}
