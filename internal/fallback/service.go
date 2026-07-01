package fallback

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/terminalpolicy"
)

const terminalToolName = "exec_" + "command"

type Service struct {
	signer *actionproposal.Signer
	store  *InMemoryStore
	now    func() time.Time
}

func NewService(signer *actionproposal.Signer, store *InMemoryStore, now func() time.Time) *Service {
	if store == nil {
		store = NewInMemoryStore()
	}
	if now == nil {
		now = time.Now
	}
	return &Service{signer: signer, store: store, now: now}
}

func (s *Service) PlanExec(req PlanExecRequest) (PlanExecResult, error) {
	if strings.TrimSpace(req.IncidentID) == "" {
		return PlanExecResult{}, fmt.Errorf("incidentId is required")
	}
	if strings.TrimSpace(req.WhyNoRunbook) == "" && len(req.RunbookMatches) == 0 {
		return PlanExecResult{}, fmt.Errorf("whyNoRunbook or runbook match evidence is required")
	}
	if hasHighCoverageRunbook(req.RunbookMatches) {
		return PlanExecResult{}, fmt.Errorf("fallback rejected: high coverage runbook candidate exists")
	}
	if len(req.Actions) == 0 {
		return PlanExecResult{}, fmt.Errorf("at least one action is required")
	}
	now := s.now()
	plan := FallbackPlan{
		ID:           fmt.Sprintf("fb_%d", now.UnixNano()),
		IncidentID:   req.IncidentID,
		Goal:         req.Goal,
		WhyNoRunbook: req.WhyNoRunbook,
		EvidenceRefs: append([]string(nil), req.EvidenceRefs...),
		Risk:         actionproposal.RiskLow,
		CreatedAt:    now,
	}
	for _, proposed := range req.Actions {
		classification, err := classifyAction(proposed)
		if err != nil {
			return PlanExecResult{}, err
		}
		if classification.Risk != actionproposal.RiskLow && len(req.EvidenceRefs) == 0 {
			return PlanExecResult{}, fmt.Errorf("evidenceRefs are required for non-read-only action")
		}
		inputHash, err := actionproposal.NormalizedInputHash(proposed.ToolInput)
		if err != nil {
			return PlanExecResult{}, err
		}
		expiresAt := now.Add(15 * time.Minute)
		targetSummary := fallbackTargetSummary(req, proposed)
		actionSummary := fallbackActionSummary(proposed)
		riskSummary := fallbackRiskSummary(classification.Risk, classification.ApprovalRequired, proposed)
		claims := actionproposal.ActionTokenClaims{
			SessionID:      req.SessionID,
			TurnID:         req.TurnID,
			TenantID:       req.TenantID,
			UserID:         req.UserID,
			IncidentID:     req.IncidentID,
			ToolName:       proposed.ToolName,
			InputHash:      inputHash,
			Source:         actionproposal.SourceFallback,
			TargetSummary:  targetSummary,
			ActionSummary:  actionSummary,
			Risk:           classification.Risk,
			RiskSummary:    riskSummary,
			Reason:         proposed.Reason,
			ExpectedEffect: proposed.ExpectedEffect,
			Rollback:       proposed.Rollback,
			ExpiresAt:      expiresAt,
		}
		token, err := s.signer.Sign(claims)
		if err != nil {
			return PlanExecResult{}, err
		}
		action := actionproposal.ActionProposal{
			SessionID:        req.SessionID,
			TurnID:           req.TurnID,
			TenantID:         req.TenantID,
			UserID:           req.UserID,
			IncidentID:       req.IncidentID,
			Source:           actionproposal.SourceFallback,
			ToolName:         proposed.ToolName,
			ToolInput:        append([]byte(nil), proposed.ToolInput...),
			TargetSummary:    targetSummary,
			ActionSummary:    actionSummary,
			Risk:             classification.Risk,
			RiskSummary:      riskSummary,
			ApprovalRequired: classification.ApprovalRequired,
			Reason:           proposed.Reason,
			EvidenceRefs:     append([]string(nil), req.EvidenceRefs...),
			ExpectedEffect:   proposed.ExpectedEffect,
			Rollback:         proposed.Rollback,
			Verification:     append([]actionproposal.VerificationStep(nil), proposed.Verification...),
			ActionToken:      token,
			ExpiresAt:        expiresAt,
		}
		plan.Actions = append(plan.Actions, action)
		if riskRank(action.Risk) > riskRank(plan.Risk) {
			plan.Risk = action.Risk
		}
	}
	s.store.Put(plan)
	return PlanExecResult{Plan: plan}, nil
}

func (s *Service) ObserveResult(req ObserveResultRequest) error {
	if _, ok := s.store.Get(req.PlanID); !ok {
		return fmt.Errorf("fallback plan %q not found", req.PlanID)
	}
	s.store.AddObservation(req.PlanID, ActionObservation{
		ActionToken:   req.ActionToken,
		ToolResultRef: req.ToolResultRef,
		EvidenceRef:   req.EvidenceRef,
		Failed:        req.Failed,
		Reason:        req.Reason,
		ObservedAt:    s.now(),
	})
	return nil
}

func fallbackTargetSummary(req PlanExecRequest, action ProposedAction) string {
	if value := strings.TrimSpace(action.TargetSummary); value != "" {
		return value
	}
	if value := strings.TrimSpace(req.Goal); value != "" {
		return value
	}
	return strings.TrimSpace(req.IncidentID)
}

func fallbackActionSummary(action ProposedAction) string {
	if value := strings.TrimSpace(action.ActionSummary); value != "" {
		return value
	}
	if value := strings.TrimSpace(action.Reason); value != "" {
		return value
	}
	if action.ToolName != "" {
		return "执行 " + strings.TrimSpace(action.ToolName)
	}
	return "执行 fallback 动作"
}

func fallbackRiskSummary(risk actionproposal.Risk, approvalRequired bool, action ProposedAction) string {
	if value := strings.TrimSpace(action.RiskSummary); value != "" {
		return value
	}
	parts := []string{"风险等级：" + string(risk)}
	if approvalRequired {
		parts = append(parts, "需要用户审批后才可执行")
	} else {
		parts = append(parts, "只读动作可直接采集证据")
	}
	if reason := strings.TrimSpace(action.Reason); reason != "" {
		parts = append(parts, reason)
	}
	return strings.Join(parts, "；")
}

type actionClassification struct {
	Risk             actionproposal.Risk
	ApprovalRequired bool
}

func classifyAction(action ProposedAction) (actionClassification, error) {
	if action.ToolName != terminalToolName {
		return actionClassification{Risk: actionproposal.RiskMedium, ApprovalRequired: true}, nil
	}
	command, args, err := terminalCommand(action.ToolInput)
	if err != nil {
		return actionClassification{}, err
	}
	if isForbiddenCommand(command, args) {
		return actionClassification{}, fmt.Errorf("forbidden terminal action rejected")
	}
	if terminalpolicy.IsReadOnlyCommand(command, args) {
		return actionClassification{Risk: actionproposal.RiskLow, ApprovalRequired: false}, nil
	}
	return actionClassification{Risk: actionproposal.RiskHigh, ApprovalRequired: true}, nil
}

func terminalCommand(input json.RawMessage) (string, []string, error) {
	var payload struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Cmd     string   `json:"cmd"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(payload.Command) != "" {
		return strings.TrimSpace(payload.Command), append([]string(nil), payload.Args...), nil
	}
	if strings.TrimSpace(payload.Cmd) != "" {
		command, args, ok := terminalpolicy.SplitCommandLine(payload.Cmd)
		if !ok {
			return "", nil, fmt.Errorf("invalid terminal command line")
		}
		return command, args, nil
	}
	return "", nil, fmt.Errorf("terminal command is required")
}

func isForbiddenCommand(command string, args []string) bool {
	base := filepath.Base(strings.TrimSpace(command))
	forbidden := map[string]bool{
		"rm": true, "reboot": true, "shutdown": true, "halt": true, "poweroff": true,
		"mkfs": true, "dd": true, "chmod": true, "chown": true,
	}
	if forbidden[base] {
		return true
	}
	for _, arg := range args {
		if strings.ContainsAny(arg, "\x00\n\r`$<>;|") {
			return true
		}
	}
	return false
}

func hasHighCoverageRunbook(matches []RunbookMatchSummary) bool {
	for _, match := range matches {
		if match.Score >= 80 || strings.EqualFold(match.Coverage, "high") {
			return true
		}
	}
	return false
}

func riskRank(risk actionproposal.Risk) int {
	switch risk {
	case actionproposal.RiskLow:
		return 1
	case actionproposal.RiskMedium:
		return 2
	case actionproposal.RiskHigh:
		return 3
	case actionproposal.RiskCritical:
		return 4
	default:
		return 0
	}
}
