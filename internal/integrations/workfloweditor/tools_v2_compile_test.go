package workfloweditor

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/tooling"
	"runner/server/service"
	contract "runner/workflow/edit"
)

func TestWorkflowCompilePlanToolMCP007IsDeterministicAndUsesRealCatalog(t *testing.T) {
	app, backend := newWorkflowCompileFixture(t)
	request := workflowCompileRequest(t, app, backend, workflowCompilePlan(""))
	tool := WorkflowCompilePlanToolV2(app)
	ctx := workflowTrustedContext("tenant-a", "user-a", "workflow-a")
	first, err := tool.Execute(ctx, mustWorkflowCompileJSON(t, request))
	if err != nil {
		t.Fatal(err)
	}
	if first.IsError || strings.Contains(string(first.StructuredContent), "script.python") || strings.HasPrefix(strings.TrimSpace(first.Content), "{") {
		t.Fatalf("compile result = %#v", first)
	}
	var envelope contract.ToolResult[WorkflowCompilePlanResultV2]
	if err := json.Unmarshal(first.StructuredContent, &envelope); err != nil {
		t.Fatal(err)
	}
	compiled := envelope.StructuredContent
	if compiled.SchemaVersion != contract.WorkflowPlanIRSchemaVersion || compiled.CatalogVersion == "" || compiled.PlanHash == "" || len(compiled.Operations) == 0 || len(compiled.Diagnostics) != 0 {
		t.Fatalf("compiled plan = %#v", compiled)
	}
	if err := tooling.ValidateToolResult(tool.OutputSchema(), first); err != nil {
		t.Fatalf("output contract: %v", err)
	}
	for run := 1; run < 10; run++ {
		next, nextErr := tool.Execute(ctx, mustWorkflowCompileJSON(t, request))
		if nextErr != nil || !bytes.Equal(first.StructuredContent, next.StructuredContent) {
			t.Fatalf("MCP-007 run %d drifted: %v\nfirst=%s\nnext=%s", run, nextErr, first.StructuredContent, next.StructuredContent)
		}
	}
	if !tool.IsReadOnly(nil) || tool.IsDestructive(nil) || !tool.IsConcurrencySafe(nil) {
		t.Fatal("compile_plan must remain deterministic read-only")
	}
}

func TestWorkflowCompilePlanToolReturnsTypedDiagnosticsWithoutPlaceholders(t *testing.T) {
	tests := []struct {
		name   string
		code   contract.WorkflowPlanDiagnosticCode
		mutate func(*contract.WorkflowPlanIR)
	}{
		{"unknown action", contract.PlanDiagnosticUnknownAction, func(plan *contract.WorkflowPlanIR) { plan.Items[0].Action.CatalogID = "missing.action" }},
		{"argument type", contract.PlanDiagnosticArgumentType, func(plan *contract.WorkflowPlanIR) { plan.Items[0].Action.Arguments["script"] = true }},
		{"missing argument", contract.PlanDiagnosticArgumentType, func(plan *contract.WorkflowPlanIR) { delete(plan.Items[0].Action.Arguments, "script") }},
		{"port mismatch", contract.PlanDiagnosticPortMismatch, func(plan *contract.WorkflowPlanIR) {
			plan.Items[0].Action.InputPorts = []contract.WorkflowPlanPortBind{{CatalogPort: "missing", PlanPort: "in"}}
		}},
		{"missing environment", contract.PlanDiagnosticMissingEnvironment, func(plan *contract.WorkflowPlanIR) {
			plan.Environment = []contract.WorkflowPlanEnvironment{{Name: "runner-linux", Version: "2026.07"}}
		}},
		{"missing secret", contract.PlanDiagnosticMissingSecret, func(plan *contract.WorkflowPlanIR) {
			plan.Unresolved = []contract.WorkflowPlanUnresolved{{ClientRef: "collect", Path: "/action/arguments/secretRef", Requirement: "secret_ref", Reason: "select credential", SecretRefScope: "workflow"}}
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			app, backend := newWorkflowCompileFixture(t)
			plan := workflowCompilePlan("")
			request := workflowCompileRequest(t, app, backend, plan)
			testCase.mutate(&request.Plan)
			result, err := WorkflowCompilePlanToolV2(app).Execute(workflowTrustedContext("tenant-a", "user-a", "workflow-a"), mustWorkflowCompileJSON(t, request))
			if err != nil {
				t.Fatal(err)
			}
			var envelope contract.ToolResult[WorkflowCompilePlanResultV2]
			if json.Unmarshal(result.StructuredContent, &envelope) != nil || !result.IsError || !envelope.IsError || len(envelope.StructuredContent.Operations) != 0 || !compileDiagnosticsContain(envelope.StructuredContent.Diagnostics, testCase.code) {
				t.Fatalf("typed diagnostic result = %#v / %s", result, result.StructuredContent)
			}
			if strings.Contains(string(result.StructuredContent), "script.python") || strings.Contains(string(result.StructuredContent), `"status":"ok"`) {
				t.Fatalf("diagnostic created placeholder/fake success: %s", result.StructuredContent)
			}
		})
	}
}

func TestWorkflowCompilePlanToolRejectsUntrustedScopeAndUnknownInput(t *testing.T) {
	app, backend := newWorkflowCompileFixture(t)
	request := workflowCompileRequest(t, app, backend, workflowCompilePlan(""))
	tool := WorkflowCompilePlanToolV2(app)
	if _, err := tool.Execute(workflowTrustedContext("tenant-b", "user-a", "workflow-a"), mustWorkflowCompileJSON(t, request)); err == nil || !strings.Contains(err.Error(), "trusted workflow scope") {
		t.Fatalf("cross-tenant error = %v", err)
	}
	raw := append(bytes.TrimSuffix(mustWorkflowCompileJSON(t, request), []byte("}")), []byte(`,"unknown":true}`)...)
	if err := tool.ValidateInput(context.Background(), raw); err == nil {
		t.Fatal("strict compile input accepted unknown field")
	}
}

func newWorkflowCompileFixture(t *testing.T) (*service.WorkflowEditApplication, *workflowReadBackend) {
	t.Helper()
	app, backend := newWorkflowReadFixture(t)
	backend.mu.Lock()
	backend.actions = []service.ActionSpec{{Action: "script.safe", Title: "Safe", Category: "script", Risk: "low", NodeType: "action", ArgsSchema: json.RawMessage(`{"type":"object","required":["script"],"additionalProperties":false,"properties":{"script":{"type":"string"}}}`), DefaultPorts: service.ActionDefaultPorts{Inputs: []service.ActionPortSpec{{ID: "in"}}, Outputs: []service.ActionPortSpec{{ID: "success"}}}}}
	backend.mu.Unlock()
	return app, backend
}

func workflowCompileRequest(t *testing.T, app *service.WorkflowEditApplication, _ *workflowReadBackend, plan contract.WorkflowPlanIR) WorkflowCompilePlanRequestV2 {
	t.Helper()
	key := contract.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}
	include := true
	page, err := app.GetActionCatalog(context.Background(), key, service.WorkflowActionCatalogQuery{Names: []string{"script.safe"}, IncludeSchema: &include, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	plan.Items[0].Action.CatalogVersion = page.CatalogDigest
	return WorkflowCompilePlanRequestV2{SchemaVersion: contract.WorkflowPlanIRSchemaVersion, WorkflowKey: key, Plan: plan}
}

func workflowCompilePlan(version string) contract.WorkflowPlanIR {
	return contract.WorkflowPlanIR{SchemaVersion: contract.WorkflowPlanIRSchemaVersion, Goal: contract.WorkflowPlanGoal{Summary: "collect safely", SuccessCriteria: []string{"step compiled"}}, Items: []contract.WorkflowPlanItem{{ClientRef: "collect", Kind: contract.PlanItemAction, Title: "Collect", Action: &contract.WorkflowPlanAction{CatalogID: "script.safe", CatalogVersion: version, Arguments: map[string]any{"script": "echo safe"}}}}, Constraints: contract.WorkflowPlanConstraints{RequiredPaths: []string{"/items"}, ForbiddenPlaceholders: []string{"<todo>"}}, Risk: contract.WorkflowPlanRiskExpectation{Level: "low"}}
}

func mustWorkflowCompileJSON(t *testing.T, request WorkflowCompilePlanRequestV2) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func compileDiagnosticsContain(diagnostics []contract.WorkflowPlanDiagnostic, code contract.WorkflowPlanDiagnosticCode) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
