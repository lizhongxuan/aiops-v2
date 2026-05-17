package opsmanual

import "strings"

type ScorePenalty struct {
	Reason string  `json:"reason,omitempty"`
	Value  float64 `json:"value,omitempty"`
}

type CandidateFilterResult struct {
	Allowed     bool           `json:"allowed"`
	MaxDecision DecisionState  `json:"max_decision,omitempty"`
	Reasons     []string       `json:"reasons,omitempty"`
	Penalties   []ScorePenalty `json:"penalties,omitempty"`
}

func hardFilterCandidate(manual OpsManual, frame OperationFrame, summary RunRecordSummary) CandidateFilterResult {
	result := CandidateFilterResult{Allowed: true, MaxDecision: DecisionDirectExecute}
	if manual.Status != ManualStatusVerified {
		result.Allowed = false
		result.Reasons = appendUnique(result.Reasons, "manual is not verified")
		return result
	}
	manualTarget := manualTargetType(manual)
	targetMatches := frame.Target.Type != "" && manualTarget != "" && equalFold(manualTarget, frame.Target.Type)
	actionMatches := operationsCompatibleForSearch(manual.Operation.Action, frame.Operation.Action)
	if !targetMatches && !actionMatches {
		result.Allowed = false
		result.Reasons = appendUnique(result.Reasons, "object_type and operation_type differ")
		result.Penalties = append(result.Penalties, ScorePenalty{Reason: "cross domain", Value: 0.80})
		return result
	}
	if !targetMatches {
		result.Allowed = false
		result.Reasons = appendUnique(result.Reasons, "object_type differs")
		result.Penalties = append(result.Penalties, ScorePenalty{Reason: "object_type differs", Value: 0.45})
		return result
	}
	if !actionMatches {
		result.MaxDecision = DecisionReference
		result.Reasons = appendUnique(result.Reasons, "operation_type differs")
		result.Penalties = append(result.Penalties, ScorePenalty{Reason: "operation_type differs", Value: 0.35})
	}
	if noRestartConflict(manual, frame) {
		result.MaxDecision = DecisionReference
		result.Reasons = appendUnique(result.Reasons, "request explicitly forbids restart")
		result.Penalties = append(result.Penalties, ScorePenalty{Reason: "negative restart intent", Value: 0.50})
	}
	if workflowEnabled, reason := workflowAvailableForSearch(manual); !workflowEnabled {
		result.MaxDecision = DecisionReference
		result.Reasons = appendUnique(result.Reasons, reason)
	}
	if latestRunFailed(summary) {
		result.MaxDecision = DecisionReference
		reason := strings.TrimSpace(summary.SuppressedReason)
		if reason == "" {
			reason = "latest run record did not pass validation"
		}
		result.Reasons = appendUnique(result.Reasons, reason)
	}
	if riskExceedsManual(frame.Risk.Level, firstNonEmpty(manual.RunnableConditions.MaxRiskLevel, manual.Operation.RiskLevel)) {
		result.MaxDecision = DecisionReference
		result.Reasons = appendUnique(result.Reasons, "requested risk level exceeds manual risk boundary")
		result.Penalties = append(result.Penalties, ScorePenalty{Reason: "risk boundary exceeded", Value: 0.50})
	}
	return result
}

func capDecision(decision, max DecisionState) DecisionState {
	if max != DecisionReference {
		return decision
	}
	if decision == DecisionDirectExecute || decision == DecisionAdapt {
		return DecisionReference
	}
	return decision
}
