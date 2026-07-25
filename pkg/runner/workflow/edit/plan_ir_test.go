package edit

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestWorkflowPlanIRRoundTripsAllControlAndRequirementKinds(t *testing.T) {
	plan := workflowPlanIRFixture()
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var decoded WorkflowPlanIR
	if err := DecodeStrict(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, plan) {
		t.Fatalf("round-trip changed IR\n got: %#v\nwant: %#v", decoded, plan)
	}
}

func TestWorkflowPlanIRGEN001ThroughGEN014AreExpressibleAndStable(t *testing.T) {
	checks := []struct {
		id    string
		check func(WorkflowPlanIR) bool
	}{
		{"GEN-001", func(p WorkflowPlanIR) bool { return p.Goal.Summary != "" && len(p.Goal.SuccessCriteria) > 0 }},
		{"GEN-002", func(p WorkflowPlanIR) bool { return planHasKind(p, PlanItemAction) }},
		{"GEN-003", func(p WorkflowPlanIR) bool { return planHasKind(p, PlanItemBranch) }},
		{"GEN-004", func(p WorkflowPlanIR) bool {
			return planHasOutcome(p, PlanOutcomeSuccess) && planHasOutcome(p, PlanOutcomeFailure) && planHasOutcome(p, PlanOutcomeAlways)
		}},
		{"GEN-005", func(p WorkflowPlanIR) bool { return planHasKind(p, PlanItemParallel) }},
		{"GEN-006", func(p WorkflowPlanIR) bool { return planHasKind(p, PlanItemJoin) }},
		{"GEN-007", func(p WorkflowPlanIR) bool { return planHasKind(p, PlanItemLoop) }},
		{"GEN-008", func(p WorkflowPlanIR) bool { return planHasKind(p, PlanItemApproval) && p.Risk.RequiresApproval }},
		{"GEN-009", func(p WorkflowPlanIR) bool { return planHasKind(p, PlanItemSubflow) }},
		{"GEN-010", func(p WorkflowPlanIR) bool { return planHasKind(p, PlanItemAggregation) }},
		{"GEN-011", func(p WorkflowPlanIR) bool { return len(p.Variables) > 1 && len(p.Environment) > 0 }},
		{"GEN-012", func(p WorkflowPlanIR) bool { return len(p.Unresolved) > 0 && p.Unresolved[0].SecretRefScope != "" }},
		{"GEN-013", workflowPlanIRIsDeterministic},
		{"GEN-014", func(p WorkflowPlanIR) bool { return validateWorkflowPlanSensitiveValues(p) == nil }},
	}
	for _, testCase := range checks {
		t.Run(testCase.id, func(t *testing.T) {
			plan := workflowPlanIRFixture()
			if err := plan.Validate(); err != nil {
				t.Fatal(err)
			}
			if !testCase.check(plan) {
				t.Fatalf("%s semantic fixture is not expressible", testCase.id)
			}
		})
	}
}

func TestWorkflowPlanIRRejectsInlineSecretsPlaceholdersAndUnversionedActions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*WorkflowPlanIR)
	}{
		{"inline secret", func(plan *WorkflowPlanIR) { plan.Items[0].Action.Arguments["password"] = "plain-text" }},
		{"placeholder", func(plan *WorkflowPlanIR) { plan.Items[0].Action.Arguments["script"] = "echo <todo>" }},
		{"unversioned action", func(plan *WorkflowPlanIR) { plan.Items[0].Action.CatalogVersion = "" }},
		{"arbitrary action text", func(plan *WorkflowPlanIR) { plan.Items[0].Action.CatalogID = "run this shell command" }},
		{"secret default", func(plan *WorkflowPlanIR) { plan.Variables[1].Default = json.RawMessage(`"plain-text"`) }},
		{"duplicate client ref", func(plan *WorkflowPlanIR) { plan.Items[1].ClientRef = plan.Items[0].ClientRef }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			plan := workflowPlanIRFixture()
			testCase.mutate(&plan)
			if err := plan.Validate(); err == nil {
				t.Fatal("invalid WorkflowPlanIR was accepted")
			}
		})
	}
}

func TestPlanDiagnosticCoversGenerationRepairFailures(t *testing.T) {
	codes := []WorkflowPlanDiagnosticCode{
		PlanDiagnosticUnknownAction, PlanDiagnosticArgumentType, PlanDiagnosticPortMismatch,
		PlanDiagnosticMissingSecret, PlanDiagnosticMissingEnvironment, PlanDiagnosticIllegalCycle,
		PlanDiagnosticUnresolved, PlanDiagnosticReferenceLocked,
	}
	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			diagnostic := WorkflowPlanDiagnostic{Code: code, Severity: "error", Message: "repair required", ClientRef: "collect", Path: "/items/0"}
			if code == PlanDiagnosticMissingSecret {
				diagnostic.Requirement = &WorkflowPlanUnresolved{ClientRef: "collect", Path: "/arguments/secretRef", Requirement: "secret_ref", Reason: "credential is not configured", SecretRefScope: "workflow"}
			}
			if err := diagnostic.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := (WorkflowPlanDiagnostic{Code: PlanDiagnosticUnknownAction, Severity: "success", Message: "invalid", Path: "not-a-pointer"}).Validate(); err == nil {
		t.Fatal("invalid diagnostic was accepted")
	}
}

func workflowPlanIRFixture() WorkflowPlanIR {
	return WorkflowPlanIR{
		SchemaVersion: WorkflowPlanIRSchemaVersion,
		Goal:          WorkflowPlanGoal{Summary: "Collect health evidence and repair safely", SuccessCriteria: []string{"evidence is collected", "operator reviews destructive repair"}},
		Items: []WorkflowPlanItem{
			{ClientRef: "collect", Kind: PlanItemAction, Title: "Collect health", Action: &WorkflowPlanAction{CatalogID: "script.shell", CatalogVersion: "catalog-v3", Arguments: map[string]any{"script": "echo ok", "secretRef": "secret://workflow/ssh"}, InputPorts: []WorkflowPlanPortBind{{CatalogPort: "in", PlanPort: "in"}}, OutputPorts: []WorkflowPlanPortBind{{CatalogPort: "success", PlanPort: "success"}}}},
			{ClientRef: "notify", Kind: PlanItemAction, Title: "Notify", DependsOn: []WorkflowPlanDependency{{ClientRef: "collect", Outcome: PlanOutcomeSuccess}, {ClientRef: "approve", Outcome: PlanOutcomeFailure}, {ClientRef: "join", Outcome: PlanOutcomeAlways}}, Action: &WorkflowPlanAction{CatalogID: "notify.send", CatalogVersion: "catalog-v3", Arguments: map[string]any{"message": "health workflow finished"}}},
			{ClientRef: "route", Kind: PlanItemBranch, Title: "Route result", Branch: &WorkflowPlanBranch{Cases: []WorkflowPlanBranchCase{{Expression: "result.healthy", TargetRef: "notify"}}, DefaultRef: "approve"}},
			{ClientRef: "fanout", Kind: PlanItemParallel, Title: "Run checks", Parallel: &WorkflowPlanParallel{BranchRefs: []string{"collect", "notify"}}},
			{ClientRef: "join", Kind: PlanItemJoin, Title: "Wait for checks", Join: &WorkflowPlanJoin{WaitForRefs: []string{"collect", "notify"}, Strategy: "all"}},
			{ClientRef: "retry", Kind: PlanItemLoop, Title: "Retry unhealthy targets", Loop: &WorkflowPlanLoop{CollectionPath: "/vars/unhealthyHosts", BodyRefs: []string{"collect"}, MaxIterations: 3}},
			{ClientRef: "approve", Kind: PlanItemApproval, Title: "Approve repair", Approval: &WorkflowPlanApproval{Prompt: "Review repair risk", ApproverRefs: []string{"on-call"}, Timeout: "30m"}},
			{ClientRef: "subflow", Kind: PlanItemSubflow, Title: "Run remediation", Subflow: &WorkflowPlanSubflow{WorkflowRef: "workflow://remediation/v2", Inputs: map[string]any{"target": "${vars.host}"}}},
			{ClientRef: "aggregate", Kind: PlanItemAggregation, Title: "Aggregate evidence", Aggregation: &WorkflowPlanAggregation{InputRefs: []string{"collect", "notify"}, Strategy: "object", OutputVar: "evidence"}},
		},
		Variables:   []WorkflowPlanVariable{{Name: "host", Type: "string", Default: json.RawMessage(`"db-primary"`)}, {Name: "sshCredential", Type: "secret_ref", SecretRef: "secret://workflow/ssh"}},
		Environment: []WorkflowPlanEnvironment{{Name: "runner-linux", Version: "2026.07", Capabilities: []string{"shell", "notifications"}}},
		Unresolved:  []WorkflowPlanUnresolved{{ClientRef: "collect", Path: "/arguments/secretRef", Requirement: "secret_ref", Reason: "select an approved host credential", SecretRefScope: "workflow"}},
		Constraints: WorkflowPlanConstraints{RequiredPaths: []string{"/goal", "/items"}, ForbiddenPlaceholders: []string{"<todo>", "replace_me"}, Invariants: []WorkflowSemanticInvariant{{Kind: "approval_before_repair", Description: "Repair cannot run before approval", ClientRefs: []string{"approve", "subflow"}}}},
		Risk:        WorkflowPlanRiskExpectation{Level: "high", Reasons: []string{"remediation changes remote state"}, RequiresApproval: true, ApprovalRefs: []string{"approve"}},
	}
}

func planHasKind(plan WorkflowPlanIR, kind WorkflowPlanItemKind) bool {
	for _, item := range plan.Items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func planHasOutcome(plan WorkflowPlanIR, outcome WorkflowPlanOutcome) bool {
	for _, item := range plan.Items {
		for _, dependency := range item.DependsOn {
			if dependency.Outcome == outcome {
				return true
			}
		}
	}
	return false
}

func workflowPlanIRIsDeterministic(plan WorkflowPlanIR) bool {
	first, err := json.Marshal(plan)
	if err != nil {
		return false
	}
	for iteration := 0; iteration < 10; iteration++ {
		next, marshalErr := json.Marshal(plan)
		if marshalErr != nil || !bytes.Equal(first, next) {
			return false
		}
	}
	return true
}
