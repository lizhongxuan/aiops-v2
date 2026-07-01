package tooling

import (
	"strings"

	"aiops-v2/internal/runtimecontract"
)

type ApprovalSnapshot struct {
	HostExecApproved bool
	ApprovedRisks    []runtimecontract.ActionRisk
}

type ToolDescriptor struct {
	Name       string
	DataScopes []runtimecontract.DataScope
	Risks      []runtimecontract.ActionRisk
}

func DecideToolSurface(frame runtimecontract.IntentFrame, approvals ApprovalSnapshot, configured []ToolDescriptor) SurfaceDecision {
	frame = runtimecontract.NormalizeIntentFrame(frame)
	decision := SurfaceDecision{
		AllowToolSearch: false,
		Reasons:         []string{"tool_search_requires_explicit_deferred_discovery"},
	}

	if hasOpsKnowledgeScope(frame) || hasCapabilityDataScope(frame, runtimecontract.DataScopeOpsKnowledge) {
		decision.AllowOpsManual = true
		decision.Reasons = append(decision.Reasons, "ops_knowledge_scope")
	}
	if frame.Kind == runtimecontract.IntentKindResearch || hasDataScope(frame.DataScopes, runtimecontract.DataScopePublicWeb) || hasDataScope(frame.Evidence.DataScopes, runtimecontract.DataScopePublicWeb) || hasCapabilityDataScope(frame, runtimecontract.DataScopePublicWeb) {
		decision.AllowPublicWeb = true
		decision.Reasons = append(decision.Reasons, "public_web_scope_or_research_intent")
	}
	if hasActionRisk(frame.RiskBudget, runtimecontract.ActionRiskHostExec) || hasCapabilityRisk(frame, runtimecontract.ActionRiskHostExec) || hasApprovedRisk(approvals, runtimecontract.ActionRiskHostExec) {
		if approvals.HostExecApproved || hasApprovedRisk(approvals, runtimecontract.ActionRiskHostExec) {
			decision.AllowHostExec = true
			decision.Reasons = append(decision.Reasons, "host_exec_approved")
		} else {
			decision.Reasons = append(decision.Reasons, "host_exec_requires_approval")
		}
	}
	return decision
}

func hasOpsKnowledgeScope(frame runtimecontract.IntentFrame) bool {
	return hasDataScope(frame.DataScopes, runtimecontract.DataScopeOpsKnowledge) ||
		hasDataScope(frame.Evidence.DataScopes, runtimecontract.DataScopeOpsKnowledge)
}

func hasCapabilityDataScope(frame runtimecontract.IntentFrame, want runtimecontract.DataScope) bool {
	for _, capability := range frame.Capabilities {
		if hasDataScope(capability.DataScopes, want) {
			return true
		}
	}
	return false
}

func hasCapabilityRisk(frame runtimecontract.IntentFrame, want runtimecontract.ActionRisk) bool {
	for _, capability := range frame.Capabilities {
		if hasActionRisk(capability.Risks, want) {
			return true
		}
	}
	return false
}

func hasDataScope(values []runtimecontract.DataScope, want runtimecontract.DataScope) bool {
	for _, value := range values {
		if runtimecontract.DataScope(strings.TrimSpace(string(value))) == want {
			return true
		}
	}
	return false
}

func hasActionRisk(values []runtimecontract.ActionRisk, want runtimecontract.ActionRisk) bool {
	for _, value := range values {
		if runtimecontract.ActionRisk(strings.TrimSpace(string(value))) == want {
			return true
		}
	}
	return false
}

func hasApprovedRisk(approvals ApprovalSnapshot, want runtimecontract.ActionRisk) bool {
	return hasActionRisk(approvals.ApprovedRisks, want)
}
