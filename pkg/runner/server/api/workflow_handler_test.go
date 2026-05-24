package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"runner/server/events"
	"runner/server/metrics"
	"runner/server/queue"
	"runner/server/service"
	"runner/state"
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
		"yaml": testAPIWorkflowYAMLWithAction("api-publish-risk", "script.shell", "script", "echo risky"),
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
		"yaml": testAPIWorkflowYAMLWithAction("api-publish-warning", "builtin.dns_resolve", "name", "${missing_token}"),
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

func TestWorkflowRouteDeleteReturnsReferenceGuardErrorCode(t *testing.T) {
	workflowSvc := service.NewWorkflowService(t.TempDir())
	workflowSvc.SetWorkflowReferenceChecker(apiWorkflowReferenceChecker{refs: []service.WorkflowReference{
		{ManualID: "manual-postgres-backup-ubuntu", Status: "verified"},
	}})
	router := NewRouter(RouterOptions{Workflow: NewWorkflowHandler(workflowSvc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows", map[string]any{
		"name": "api-locked-delete",
		"yaml": testAPIWorkflowYAML("api-locked-delete", "echo initial"),
	})
	if code != http.StatusCreated {
		t.Fatalf("create workflow status = %d payload=%s", code, payload)
	}

	code, payload = serveJSON(t, router, http.MethodDelete, "/api/v1/workflows/api-locked-delete", nil)
	if code != http.StatusConflict {
		t.Fatalf("delete guarded workflow status = %d, want 409 payload=%s", code, payload)
	}
	var got struct {
		Error      string `json:"error"`
		Message    string `json:"message"`
		References []struct {
			ManualID string `json:"manual_id"`
			Status   string `json:"status"`
		} `json:"references"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode guarded delete response: %v", err)
	}
	if got.Error != "workflow_in_use" {
		t.Fatalf("error code = %q payload=%s", got.Error, payload)
	}
	if got.Message == "" {
		t.Fatalf("message should be clear: %s", payload)
	}
	if len(got.References) != 1 || got.References[0].ManualID != "manual-postgres-backup-ubuntu" {
		t.Fatalf("references mismatch: %+v payload=%s", got.References, payload)
	}
}

func TestWorkflowRouteUpdateReturnsVersionLockedCode(t *testing.T) {
	workflowSvc := service.NewWorkflowService(t.TempDir())
	workflowSvc.SetWorkflowReferenceChecker(apiWorkflowReferenceChecker{refs: []service.WorkflowReference{
		{ManualID: "manual-redis-restart", Status: "verified"},
	}})
	router := NewRouter(RouterOptions{Workflow: NewWorkflowHandler(workflowSvc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows", map[string]any{
		"name": "api-locked-update",
		"yaml": testAPIWorkflowYAML("api-locked-update", "echo initial"),
	})
	if code != http.StatusCreated {
		t.Fatalf("create workflow status = %d payload=%s", code, payload)
	}

	code, payload = serveJSON(t, router, http.MethodPut, "/api/v1/workflows/api-locked-update", map[string]any{
		"yaml": testAPIWorkflowYAML("api-locked-update", "echo changed"),
	})
	if code != http.StatusConflict {
		t.Fatalf("update guarded workflow status = %d, want 409 payload=%s", code, payload)
	}
	var got struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode guarded update response: %v", err)
	}
	if got.Error != "workflow_version_locked" {
		t.Fatalf("error code = %q payload=%s", got.Error, payload)
	}
}

func TestWorkflowRouteUpdateReturnsWarningWhenGuardDowngraded(t *testing.T) {
	workflowSvc := service.NewWorkflowService(t.TempDir())
	workflowSvc.SetWorkflowReferenceChecker(apiWorkflowReferenceChecker{refs: []service.WorkflowReference{
		{ManualID: "manual-redis-restart", Status: "verified"},
	}})
	workflowSvc.SetWorkflowReferenceGuardMode(service.WorkflowReferenceGuardModeWarn)
	router := NewRouter(RouterOptions{Workflow: NewWorkflowHandler(workflowSvc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows", map[string]any{
		"name": "api-warning-update",
		"yaml": testAPIWorkflowYAML("api-warning-update", "echo initial"),
	})
	if code != http.StatusCreated {
		t.Fatalf("create workflow status = %d payload=%s", code, payload)
	}

	code, payload = serveJSON(t, router, http.MethodPut, "/api/v1/workflows/api-warning-update", map[string]any{
		"yaml": testAPIWorkflowYAML("api-warning-update", "echo changed"),
	})
	if code != http.StatusOK {
		t.Fatalf("warning update status = %d, want 200 payload=%s", code, payload)
	}
	var got struct {
		Warnings []struct {
			Code string `json:"code"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode warning update response: %v", err)
	}
	if len(got.Warnings) != 1 || got.Warnings[0].Code != "workflow_version_locked" {
		t.Fatalf("warnings = %+v payload=%s", got.Warnings, payload)
	}
}

func TestWorkflowRouteRollbackReturnsVersionLockedCode(t *testing.T) {
	workflowSvc := service.NewWorkflowService(t.TempDir())
	router := NewRouter(RouterOptions{Workflow: NewWorkflowHandler(workflowSvc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows", map[string]any{
		"name": "api-locked-rollback",
		"yaml": testAPIWorkflowYAML("api-locked-rollback", "echo initial"),
	})
	if code != http.StatusCreated {
		t.Fatalf("create workflow status = %d payload=%s", code, payload)
	}
	code, payload = serveJSON(t, router, http.MethodPut, "/api/v1/workflows/api-locked-rollback", map[string]any{
		"yaml": testAPIWorkflowYAML("api-locked-rollback", "echo changed"),
	})
	if code != http.StatusOK {
		t.Fatalf("update workflow status = %d payload=%s", code, payload)
	}
	code, payload = serveJSON(t, router, http.MethodGet, "/api/v1/workflows/api-locked-rollback/versions", nil)
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

	workflowSvc.SetWorkflowReferenceChecker(apiWorkflowReferenceChecker{refs: []service.WorkflowReference{
		{ManualID: "manual-rollback", Status: "verified"},
	}})
	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/api-locked-rollback/versions/"+initialVersionID+"/rollback", map[string]any{})
	if code != http.StatusConflict {
		t.Fatalf("rollback guarded workflow status = %d, want 409 payload=%s", code, payload)
	}
	var got struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode rollback guard response: %v", err)
	}
	if got.Error != "workflow_version_locked" {
		t.Fatalf("error code = %q payload=%s", got.Error, payload)
	}
}

func TestWorkflowRouteImportOverwriteReturnsVersionLockedCode(t *testing.T) {
	workflowSvc := service.NewWorkflowService(t.TempDir())
	router := NewRouter(RouterOptions{Workflow: NewWorkflowHandler(workflowSvc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows", map[string]any{
		"name": "api-locked-import",
		"yaml": testAPIWorkflowYAML("api-locked-import", "echo current"),
	})
	if code != http.StatusCreated {
		t.Fatalf("create workflow status = %d payload=%s", code, payload)
	}
	sourceSvc := service.NewWorkflowService(t.TempDir())
	if err := sourceSvc.Create(context.Background(), &service.WorkflowRecord{
		Name:    "api-locked-import",
		RawYAML: []byte(testAPIWorkflowYAML("api-locked-import", "echo imported")),
	}); err != nil {
		t.Fatalf("create source workflow: %v", err)
	}
	bundle, err := sourceSvc.ExportBundle(context.Background(), "api-locked-import")
	if err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	workflowSvc.SetWorkflowReferenceChecker(apiWorkflowReferenceChecker{refs: []service.WorkflowReference{
		{ManualID: "manual-import", Status: "verified"},
	}})
	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/bundles/import", map[string]any{
		"bundle":    bundle,
		"overwrite": true,
	})
	if code != http.StatusConflict {
		t.Fatalf("import guarded workflow status = %d, want 409 payload=%s", code, payload)
	}
	var got struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode import guard response: %v", err)
	}
	if got.Error != "workflow_version_locked" {
		t.Fatalf("error code = %q payload=%s", got.Error, payload)
	}
}

func TestRunRouteSubmitReturnsDigestMismatchCode(t *testing.T) {
	runSvc := service.NewRunService(service.RunServiceConfig{
		MaxConcurrentRuns: 1,
		MaxOutputBytes:    65536,
	}, nil, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(8), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()
	router := NewRouter(RouterOptions{Run: NewRunHandler(runSvc)})
	workflowYAML := testAPIWorkflowYAML("api-digest-run", "echo current")

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/runs", map[string]any{
		"workflow_yaml":    workflowYAML,
		"manual_id":        "manual-digest-run",
		"workflow_digest":  service.DigestWorkflowContent([]byte(testAPIWorkflowYAML("api-digest-run", "echo stale"))),
		"workflow_version": "v1",
	})
	if code != http.StatusConflict {
		t.Fatalf("digest mismatch submit status = %d, want 409 payload=%s", code, payload)
	}
	var got struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode run submit error: %v", err)
	}
	if got.Error != "workflow_digest_mismatch" {
		t.Fatalf("error code = %q payload=%s", got.Error, payload)
	}
}

func TestRunRouteSubmitPersistsOpsManualMetadata(t *testing.T) {
	runSvc := service.NewRunService(service.RunServiceConfig{
		MaxConcurrentRuns: 1,
		MaxOutputBytes:    65536,
	}, nil, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(8), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()
	router := NewRouter(RouterOptions{Run: NewRunHandler(runSvc)})
	workflowYAML := testAPIWorkflowYAML("api-run-meta", "echo metadata")

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/runs", map[string]any{
		"workflow_yaml":          workflowYAML,
		"manual_id":              "manual-api-run-meta",
		"workflow_id":            "workflow-api-run-meta",
		"workflow_version":       "v5",
		"workflow_digest":        service.DigestWorkflowContent([]byte(workflowYAML)),
		"preflight_status":       "passed",
		"preflight_evidence_ref": "preflight:api-run-meta:ok",
		"triggered_by":           "sre-api",
		"metadata":               map[string]any{"decision_state": "approved"},
	})
	if code != http.StatusAccepted {
		t.Fatalf("submit run status = %d payload=%s", code, payload)
	}
	var submitted service.RunResponse
	if err := json.Unmarshal(payload, &submitted); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}

	code, payload = serveJSON(t, router, http.MethodGet, "/api/v1/runs/"+submitted.RunID, nil)
	if code != http.StatusOK {
		t.Fatalf("get run status = %d payload=%s", code, payload)
	}
	var detail service.RunDetail
	if err := json.Unmarshal(payload, &detail); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if detail.ManualID != "manual-api-run-meta" || detail.WorkflowID != "workflow-api-run-meta" || detail.WorkflowVersion != "v5" || detail.WorkflowDigest == "" {
		t.Fatalf("api run metadata not persisted: %+v", detail.RunMeta)
	}
	if detail.TriggeredBy != "sre-api" || detail.Metadata["decision_state"] != "approved" {
		t.Fatalf("api request metadata not persisted: %+v", detail.RunMeta)
	}
}

type apiWorkflowReferenceChecker struct {
	refs []service.WorkflowReference
}

func (s apiWorkflowReferenceChecker) ReferencesForWorkflow(_ context.Context, _ string) ([]service.WorkflowReference, error) {
	return append([]service.WorkflowReference{}, s.refs...), nil
}

func testAPIWorkflowYAML(name, cmd string) string {
	return testAPIWorkflowYAMLWithAction(name, "builtin.dns_resolve", "name", cmd)
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
