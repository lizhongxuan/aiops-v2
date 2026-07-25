package edit

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCompilePlanGEN013IsDeterministicAcrossTenRuns(t *testing.T) {
	plan := workflowPlanIRFixture()
	plan.Unresolved = nil
	catalog := workflowPlanCompilerCatalog()
	options := WorkflowPlanCompileOptions{EnvironmentVersions: map[string]string{"runner-linux": "2026.07"}, PositionSpacing: 180}

	first, err := CompileWorkflowPlan(plan, catalog, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Diagnostics) != 0 || len(first.Operations) <= len(plan.Items) || first.PlanHash == "" {
		t.Fatalf("compile result = %#v", first)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	for iteration := 1; iteration < 10; iteration++ {
		next, compileErr := CompileWorkflowPlan(plan, catalog, options)
		if compileErr != nil {
			t.Fatal(compileErr)
		}
		nextJSON, marshalErr := json.Marshal(next)
		if marshalErr != nil || !bytes.Equal(firstJSON, nextJSON) {
			t.Fatalf("compile iteration %d drifted\nfirst=%s\n next=%s", iteration, firstJSON, nextJSON)
		}
	}

	firstNode, ok := first.Operations[0].Value.(*AddNodeOperation)
	if !ok || firstNode.ClientRef != "collect" || firstNode.Node.Step == nil || firstNode.Node.Step.Action != "script.shell" || len(*firstNode.Node.Ports) != 2 {
		t.Fatalf("compiled action lost Catalog contract: %#v", first.Operations[0].Value)
	}
}

func TestCompilePlanReturnsTypedDiagnosticsWithoutPlaceholderOperations(t *testing.T) {
	tests := []struct {
		name   string
		code   WorkflowPlanDiagnosticCode
		mutate func(*WorkflowPlanIR, *WorkflowPlanCatalog, *WorkflowPlanCompileOptions)
	}{
		{"unknown action", PlanDiagnosticUnknownAction, func(plan *WorkflowPlanIR, _ *WorkflowPlanCatalog, _ *WorkflowPlanCompileOptions) {
			plan.Items[0].Action.CatalogID = "missing.action"
		}},
		{"argument type", PlanDiagnosticArgumentType, func(plan *WorkflowPlanIR, _ *WorkflowPlanCatalog, _ *WorkflowPlanCompileOptions) {
			plan.Items[0].Action.Arguments["script"] = true
		}},
		{"missing argument", PlanDiagnosticArgumentType, func(plan *WorkflowPlanIR, _ *WorkflowPlanCatalog, _ *WorkflowPlanCompileOptions) {
			delete(plan.Items[0].Action.Arguments, "script")
		}},
		{"port mismatch", PlanDiagnosticPortMismatch, func(plan *WorkflowPlanIR, _ *WorkflowPlanCatalog, _ *WorkflowPlanCompileOptions) {
			plan.Items[0].Action.InputPorts[0].CatalogPort = "unknown"
		}},
		{"environment", PlanDiagnosticMissingEnvironment, func(_ *WorkflowPlanIR, _ *WorkflowPlanCatalog, options *WorkflowPlanCompileOptions) {
			options.EnvironmentVersions = nil
		}},
		{"secret requirement", PlanDiagnosticMissingSecret, func(plan *WorkflowPlanIR, _ *WorkflowPlanCatalog, _ *WorkflowPlanCompileOptions) {
			plan.Unresolved = []WorkflowPlanUnresolved{{ClientRef: "collect", Path: "/action/arguments/secretRef", Requirement: "secret_ref", Reason: "select a credential", SecretRefScope: "workflow"}}
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			plan := workflowPlanIRFixture()
			plan.Unresolved = nil
			catalog := workflowPlanCompilerCatalog()
			options := WorkflowPlanCompileOptions{EnvironmentVersions: map[string]string{"runner-linux": "2026.07"}}
			testCase.mutate(&plan, &catalog, &options)
			result, err := CompileWorkflowPlan(plan, catalog, options)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Operations) != 0 || !workflowPlanDiagnosticsContain(result.Diagnostics, testCase.code) {
				t.Fatalf("result = %#v, want %s and no placeholder operations", result, testCase.code)
			}
		})
	}
}

func TestCompilePlanRejectsInvalidIRAndCatalogContract(t *testing.T) {
	plan := workflowPlanIRFixture()
	plan.Unresolved = nil
	plan.SchemaVersion = "future"
	if _, err := CompileWorkflowPlan(plan, workflowPlanCompilerCatalog(), WorkflowPlanCompileOptions{}); err == nil {
		t.Fatal("invalid IR was compiled")
	}
	plan = workflowPlanIRFixture()
	plan.Unresolved = nil
	catalog := workflowPlanCompilerCatalog()
	catalog.Version = ""
	if _, err := CompileWorkflowPlan(plan, catalog, WorkflowPlanCompileOptions{}); err == nil {
		t.Fatal("unversioned Catalog was compiled")
	}
}

func workflowPlanCompilerCatalog() WorkflowPlanCatalog {
	strict := false
	schema := func(required string) json.RawMessage {
		raw, _ := json.Marshal(map[string]any{
			"type": "object", "required": []string{required}, "additionalProperties": strict,
			"properties": map[string]any{required: map[string]any{"type": "string"}, "secretRef": map[string]any{"type": "string"}},
		})
		return raw
	}
	return WorkflowPlanCatalog{Version: "catalog-v3", Actions: []WorkflowPlanCatalogAction{
		{ID: "script.shell", Version: "catalog-v3", NodeType: "action", ArgumentsJSON: schema("script"), InputPorts: []WorkflowPlanCatalogPort{{ID: "in", Required: true}}, OutputPorts: []WorkflowPlanCatalogPort{{ID: "success", Required: true}}},
		{ID: "notify.send", Version: "catalog-v3", NodeType: "action", ArgumentsJSON: schema("message")},
	}}
}

func workflowPlanDiagnosticsContain(diagnostics []WorkflowPlanDiagnostic, code WorkflowPlanDiagnosticCode) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
