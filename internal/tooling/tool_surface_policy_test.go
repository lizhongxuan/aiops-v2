package tooling

import (
	"encoding/json"
	"testing"

	"aiops-v2/internal/runtimecontract"
)

func TestDecideToolSurfaceKeepsUnknownIntentConservative(t *testing.T) {
	decision := DecideToolSurface(runtimecontract.IntentFrame{
		Kind: runtimecontract.IntentKindUnknown,
	}, ApprovalSnapshot{}, nil)

	if !decision.AllowToolSearch {
		t.Fatalf("AllowToolSearch = false, want true for capability discovery")
	}
	if decision.AllowPublicWeb {
		t.Fatalf("AllowPublicWeb = true, want false for unknown intent")
	}
	if decision.AllowHostExec {
		t.Fatalf("AllowHostExec = true, want false for unknown intent")
	}
}

func TestDecideToolSurfaceAllowsPublicWebForResearchScope(t *testing.T) {
	decision := DecideToolSurface(runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindResearch,
		DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopePublicWeb},
	}, ApprovalSnapshot{}, nil)

	if !decision.AllowPublicWeb {
		t.Fatalf("AllowPublicWeb = false, want true for research public_web scope; decision=%#v", decision)
	}
}

func TestDecideToolSurfaceDoesNotAllowPublicWebForLocalDiagnose(t *testing.T) {
	decision := DecideToolSurface(runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindDiagnose,
		DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopeLocalRuntime},
	}, ApprovalSnapshot{}, nil)

	if decision.AllowPublicWeb {
		t.Fatalf("AllowPublicWeb = true, want false for diagnose/local_runtime")
	}
}

func TestDecideToolSurfaceRequiresApprovalForHostExec(t *testing.T) {
	decision := DecideToolSurface(runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindDiagnose,
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskHostExec},
	}, ApprovalSnapshot{HostExecApproved: false}, nil)

	if decision.AllowHostExec {
		t.Fatalf("AllowHostExec = true, want false until host exec is approved")
	}

	approved := DecideToolSurface(runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindDiagnose,
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskHostExec},
	}, ApprovalSnapshot{HostExecApproved: true}, nil)
	if !approved.AllowHostExec {
		t.Fatalf("AllowHostExec = false, want true after approval; decision=%#v", approved)
	}
}

func TestDecideToolSurfaceAllowsOpsManualForOpsKnowledge(t *testing.T) {
	decision := DecideToolSurface(runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindDiagnose,
		DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopeOpsKnowledge},
	}, ApprovalSnapshot{}, nil)

	if !decision.AllowOpsManual {
		t.Fatalf("AllowOpsManual = false, want true for ops_knowledge scope; decision=%#v", decision)
	}
}

func TestDecideToolSurfaceDoesNotAllowOpsManualJustBecauseToolIsConfigured(t *testing.T) {
	decision := DecideToolSurface(runtimecontract.IntentFrame{
		Kind: runtimecontract.IntentKindUnknown,
	}, ApprovalSnapshot{}, []ToolDescriptor{{
		Name:       "search_ops_manuals",
		DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopeOpsKnowledge},
	}})

	if decision.AllowOpsManual {
		t.Fatalf("AllowOpsManual = true, want false without ops_knowledge intent/capability")
	}
}

func TestTurnMetadataFilterUsesIntentFrameForToolVisibility(t *testing.T) {
	frameJSON := mustIntentFrameJSONForToolSurfaceTest(t, runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindResearch,
		DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopePublicWeb},
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskHostExec},
	})
	metadata := map[string]string{
		"aiops.intent.frame": frameJSON,
	}

	if !IsToolVisibleForTurnMetadata(ToolMetadata{Name: "web_search", Pack: "public_web"}, metadata) {
		t.Fatalf("web_search should be visible for research public_web intent")
	}
	if IsToolVisibleForTurnMetadata(ToolMetadata{Name: "exec_command"}, metadata) {
		t.Fatalf("exec_command should be hidden when host_exec is requested but not approved")
	}
}

func TestTurnMetadataFilterUsesPublicIntentFields(t *testing.T) {
	metadata := map[string]string{
		"aiops.intent.kind":       "diagnose",
		"aiops.intent.dataScopes": "local_runtime",
		"aiops.intent.riskBudget": "host_exec",
	}

	if IsToolVisibleForTurnMetadata(ToolMetadata{Name: "web_search", Pack: "public_web"}, metadata) {
		t.Fatalf("web_search should be hidden for diagnose/local_runtime intent")
	}
	if IsToolVisibleForTurnMetadata(ToolMetadata{Name: "exec_command"}, metadata) {
		t.Fatalf("exec_command should be hidden for unapproved host_exec intent")
	}

	manualMetadata := map[string]string{
		"aiops.intent.kind":       "diagnose",
		"aiops.intent.dataScopes": "ops_knowledge",
	}
	if IsToolVisibleForTurnMetadata(ToolMetadata{Name: "search_ops_manuals", Pack: "ops_manual_flow"}, manualMetadata) {
		t.Fatalf("search_ops_manuals should stay hidden for ops_knowledge intent without explicit @ops_manuals")
	}
	manualMetadata["enableTool"] = "search_ops_manuals"
	if !IsToolVisibleForTurnMetadata(ToolMetadata{Name: "search_ops_manuals", Pack: "ops_manual_flow"}, manualMetadata) {
		t.Fatalf("search_ops_manuals should be visible when explicitly requested")
	}
}

func mustIntentFrameJSONForToolSurfaceTest(t *testing.T, frame runtimecontract.IntentFrame) string {
	t.Helper()
	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("json.Marshal(IntentFrame) error = %v", err)
	}
	return string(data)
}
