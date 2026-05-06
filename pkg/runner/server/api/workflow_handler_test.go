package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"runner/server/service"
)

func TestWorkflowRoutesPersistSaveNote(t *testing.T) {
	workflowSvc := service.NewWorkflowService(t.TempDir())
	router := NewRouter(RouterOptions{Workflow: NewWorkflowHandler(workflowSvc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows", map[string]any{
		"name":      "api-note",
		"yaml":      testAPIWorkflowYAML("api-note", "echo initial"),
		"save_note": "created from visual editor",
	})
	if code != http.StatusCreated {
		t.Fatalf("create workflow status = %d payload=%s", code, payload)
	}

	code, payload = serveJSON(t, router, http.MethodGet, "/api/v1/workflows/api-note", nil)
	if code != http.StatusOK {
		t.Fatalf("get workflow status = %d payload=%s", code, payload)
	}
	var got struct {
		SaveNote string `json:"save_note"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode workflow response: %v", err)
	}
	if got.SaveNote != "created from visual editor" {
		t.Fatalf("save note after create = %q", got.SaveNote)
	}

	code, payload = serveJSON(t, router, http.MethodPut, "/api/v1/workflows/api-note", map[string]any{
		"yaml":      testAPIWorkflowYAML("api-note", "echo changed"),
		"save_note": "validated dry run before save",
	})
	if code != http.StatusOK {
		t.Fatalf("update workflow status = %d payload=%s", code, payload)
	}

	code, payload = serveJSON(t, router, http.MethodGet, "/api/v1/workflows/api-note", nil)
	if code != http.StatusOK {
		t.Fatalf("get workflow after update status = %d payload=%s", code, payload)
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode workflow response after update: %v", err)
	}
	if got.SaveNote != "validated dry run before save" {
		t.Fatalf("save note after update = %q", got.SaveNote)
	}
}

func TestWorkflowRoutesPublishDraftWorkflow(t *testing.T) {
	workflowSvc := service.NewWorkflowService(t.TempDir())
	router := NewRouter(RouterOptions{Workflow: NewWorkflowHandler(workflowSvc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows", map[string]any{
		"name": "api-publish",
		"yaml": testAPIWorkflowYAML("api-publish", "echo initial"),
	})
	if code != http.StatusCreated {
		t.Fatalf("create workflow status = %d payload=%s", code, payload)
	}

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/api-publish/publish", map[string]any{
		"save_note": "too early",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("publish before validate status = %d, want 400 payload=%s", code, payload)
	}
	validateWorkflowRoute(t, router, "api-publish")

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/api-publish/publish", map[string]any{
		"save_note": "still too early",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("publish before dry run status = %d, want 400 payload=%s", code, payload)
	}
	markWorkflowDryRunPassed(t, workflowSvc, "api-publish")

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/api-publish/publish", map[string]any{
		"save_note": "approved change window",
	})
	if code != http.StatusOK {
		t.Fatalf("publish workflow status = %d payload=%s", code, payload)
	}
	var published struct {
		Status             string `json:"status"`
		SaveNote           string `json:"save_note"`
		PublishedAt        string `json:"published_at"`
		PublishedGraphHash string `json:"published_graph_hash"`
	}
	if err := json.Unmarshal(payload, &published); err != nil {
		t.Fatalf("decode publish response: %v", err)
	}
	if published.Status != service.WorkflowStatusPublished {
		t.Fatalf("publish status = %q", published.Status)
	}
	if published.SaveNote != "approved change window" {
		t.Fatalf("publish save note = %q", published.SaveNote)
	}
	if published.PublishedAt == "" {
		t.Fatal("publish response missing published_at")
	}
	if published.PublishedGraphHash == "" {
		t.Fatal("publish response missing published_graph_hash")
	}

	code, payload = serveJSON(t, router, http.MethodPut, "/api/v1/workflows/api-publish", map[string]any{
		"yaml": testAPIWorkflowYAML("api-publish", "echo changed"),
	})
	if code != http.StatusOK {
		t.Fatalf("update workflow status = %d payload=%s", code, payload)
	}
	code, payload = serveJSON(t, router, http.MethodGet, "/api/v1/workflows/api-publish", nil)
	if code != http.StatusOK {
		t.Fatalf("get workflow status = %d payload=%s", code, payload)
	}
	var got struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode workflow response: %v", err)
	}
	if got.Status != service.WorkflowStatusDraft {
		t.Fatalf("status after update = %q", got.Status)
	}
}

func TestWorkflowRoutesPublishRequiresRiskAcknowledgement(t *testing.T) {
	workflowSvc := service.NewWorkflowService(t.TempDir())
	router := NewRouter(RouterOptions{Workflow: NewWorkflowHandler(workflowSvc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows", map[string]any{
		"name": "api-publish-risk",
		"yaml": testAPIWorkflowYAMLWithAction("api-publish-risk", "shell.run", "script", "echo risky"),
	})
	if code != http.StatusCreated {
		t.Fatalf("create workflow status = %d payload=%s", code, payload)
	}
	validateWorkflowRoute(t, router, "api-publish-risk")
	markWorkflowDryRunPassed(t, workflowSvc, "api-publish-risk")

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/api-publish-risk/publish", map[string]any{})
	if code != http.StatusBadRequest {
		t.Fatalf("high-risk publish status = %d, want 400 payload=%s", code, payload)
	}

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/api-publish-risk/publish", map[string]any{"risk_acknowledged": true})
	if code != http.StatusOK {
		t.Fatalf("acknowledged high-risk publish status = %d payload=%s", code, payload)
	}
}

func TestWorkflowRoutesPublishRequiresWarningAcknowledgement(t *testing.T) {
	logBuf, restore := captureAPIAuditLogs(t)
	defer restore()

	workflowSvc := service.NewWorkflowService(t.TempDir())
	router := NewRouter(RouterOptions{Workflow: NewWorkflowHandler(workflowSvc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows", map[string]any{
		"name": "api-publish-warning",
		"yaml": testAPIWorkflowYAML("api-publish-warning", "echo ${missing_token}"),
	})
	if code != http.StatusCreated {
		t.Fatalf("create workflow status = %d payload=%s", code, payload)
	}
	validateWorkflowRoute(t, router, "api-publish-warning")
	markWorkflowDryRunPassed(t, workflowSvc, "api-publish-warning")

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/api-publish-warning/publish", map[string]any{})
	if code != http.StatusBadRequest {
		t.Fatalf("warning publish status = %d, want 400 payload=%s", code, payload)
	}

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/api-publish-warning/publish", map[string]any{"warning_acknowledged": true})
	if code != http.StatusOK {
		t.Fatalf("acknowledged warning publish status = %d payload=%s", code, payload)
	}
	if !strings.Contains(logBuf.String(), `"warning_acknowledged":true`) {
		t.Fatalf("publish audit log should record warning acknowledgement, got:\n%s", logBuf.String())
	}
}

func TestWorkflowRoutesHistoryAndRollback(t *testing.T) {
	workflowSvc := service.NewWorkflowService(t.TempDir())
	router := NewRouter(RouterOptions{Workflow: NewWorkflowHandler(workflowSvc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows", map[string]any{
		"name": "api-history",
		"yaml": testAPIWorkflowYAML("api-history", "echo initial"),
	})
	if code != http.StatusCreated {
		t.Fatalf("create workflow status = %d payload=%s", code, payload)
	}
	code, payload = serveJSON(t, router, http.MethodPut, "/api/v1/workflows/api-history", map[string]any{
		"yaml": testAPIWorkflowYAML("api-history", "echo updated"),
	})
	if code != http.StatusOK {
		t.Fatalf("update workflow status = %d payload=%s", code, payload)
	}

	code, payload = serveJSON(t, router, http.MethodGet, "/api/v1/workflows/api-history/versions", nil)
	if code != http.StatusOK {
		t.Fatalf("list versions status = %d payload=%s", code, payload)
	}
	var history struct {
		Items []struct {
			ID   string `json:"id"`
			YAML string `json:"yaml"`
		} `json:"items"`
	}
	if err := json.Unmarshal(payload, &history); err != nil {
		t.Fatalf("decode versions: %v", err)
	}
	var initialVersionID string
	for _, item := range history.Items {
		if strings.Contains(item.YAML, "echo initial") {
			initialVersionID = item.ID
			break
		}
	}
	if initialVersionID == "" {
		t.Fatalf("initial version not found in payload=%s", payload)
	}

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/api-history/versions/"+initialVersionID+"/rollback", map[string]any{
		"save_note": "api rollback",
	})
	if code != http.StatusOK {
		t.Fatalf("rollback status = %d payload=%s", code, payload)
	}
	code, payload = serveJSON(t, router, http.MethodGet, "/api/v1/workflows/api-history", nil)
	if code != http.StatusOK {
		t.Fatalf("get workflow status = %d payload=%s", code, payload)
	}
	if !strings.Contains(string(payload), "echo initial") {
		t.Fatalf("workflow was not rolled back: %s", payload)
	}
}

func TestWorkflowRoutesExportAndImportBundle(t *testing.T) {
	workflowSvc := service.NewWorkflowService(t.TempDir())
	router := NewRouter(RouterOptions{Workflow: NewWorkflowHandler(workflowSvc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows", map[string]any{
		"name": "api-bundle",
		"yaml": testAPIWorkflowYAML("api-bundle", "echo bundle"),
		"labels": map[string]string{
			"env": "prod",
		},
	})
	if code != http.StatusCreated {
		t.Fatalf("create workflow status = %d payload=%s", code, payload)
	}

	code, payload = serveJSON(t, router, http.MethodGet, "/api/v1/workflows/api-bundle/bundle", nil)
	if code != http.StatusOK {
		t.Fatalf("export bundle status = %d payload=%s", code, payload)
	}
	var bundle service.WorkflowBundle
	if err := json.Unmarshal(payload, &bundle); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	if bundle.Name != "api-bundle" || !strings.Contains(bundle.YAML, "echo bundle") {
		t.Fatalf("bundle mismatch: %+v", bundle)
	}

	code, payload = serveJSON(t, router, http.MethodDelete, "/api/v1/workflows/api-bundle", nil)
	if code != http.StatusOK {
		t.Fatalf("delete workflow status = %d payload=%s", code, payload)
	}

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/bundles/import", map[string]any{
		"bundle":    bundle,
		"save_note": "api import",
	})
	if code != http.StatusCreated {
		t.Fatalf("import bundle status = %d payload=%s", code, payload)
	}
	code, payload = serveJSON(t, router, http.MethodGet, "/api/v1/workflows/api-bundle", nil)
	if code != http.StatusOK {
		t.Fatalf("get imported workflow status = %d payload=%s", code, payload)
	}
	if !strings.Contains(string(payload), "echo bundle") || !strings.Contains(string(payload), `"status":"draft"`) {
		t.Fatalf("imported workflow mismatch: %s", payload)
	}
}

func testAPIWorkflowYAML(name, cmd string) string {
	return testAPIWorkflowYAMLWithAction(name, "cmd.run", "cmd", cmd)
}

func validateWorkflowRoute(t *testing.T, router http.Handler, name string) {
	t.Helper()
	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows/"+name+"/validate", nil)
	if code != http.StatusOK {
		t.Fatalf("validate workflow %s status = %d payload=%s", name, code, payload)
	}
	if !strings.Contains(string(payload), `"validated_graph_hash"`) {
		t.Fatalf("validate response missing validated_graph_hash: %s", payload)
	}
}

func markWorkflowDryRunPassed(t *testing.T, svc *service.WorkflowService, name string) {
	t.Helper()
	record, err := svc.Get(context.Background(), name)
	if err != nil {
		t.Fatalf("get %s before dry-run mark: %v", name, err)
	}
	if _, err := svc.MarkDryRunPassed(context.Background(), name, service.WorkflowDryRunOptions{
		Actor:             "api-test",
		ExpectedGraphHash: record.ValidatedGraphHash,
	}); err != nil {
		t.Fatalf("mark %s dry-run passed: %v", name, err)
	}
}

func testAPIWorkflowYAMLWithAction(name, action, argKey, argValue string) string {
	return `version: v0.1
name: ` + name + `
steps:
  - name: run
    action: ` + action + `
    args:
      ` + argKey + `: ` + argValue + `
`
}
