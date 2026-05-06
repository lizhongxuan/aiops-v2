package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"runner/logging"
	"runner/server/events"
	"runner/server/metrics"
	"runner/server/queue"
	"runner/server/service"
	"runner/state"
	"runner/workflow/visual"
)

func TestVisualWorkflowAuditCoversGraphLifecycle(t *testing.T) {
	logBuf, restore := captureAPIAuditLogs(t)
	defer restore()

	workflowSvc := service.NewWorkflowService(t.TempDir())
	runStore := state.NewInMemoryRunStore()
	runSvc := service.NewRunService(service.RunServiceConfig{MaxConcurrentRuns: 1}, workflowSvc, nil, runStore, queue.NewMemoryQueue(4), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()
	visualSvc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{WorkflowService: workflowSvc, RunService: runSvc})
	router := NewRouter(RouterOptions{
		Workflow:       NewWorkflowHandler(workflowSvc),
		VisualWorkflow: NewVisualWorkflowHandler(visualSvc),
		Run:            NewRunHandler(runSvc),
	})

	graph := sampleAPIGraph()
	code, payload := serveJSONWithHeaders(t, router, http.MethodPost, "/api/v1/workflows/graph", map[string]any{
		"graph":     graph,
		"save_note": "audit create",
	}, auditHeaders())
	if code != http.StatusCreated {
		t.Fatalf("create graph status = %d payload=%s", code, payload)
	}

	code, payload = serveJSONWithHeaders(t, router, http.MethodGet, "/api/v1/workflows/api-graph/graph", nil, auditHeaders())
	if code != http.StatusOK {
		t.Fatalf("get graph status = %d payload=%s", code, payload)
	}
	var loaded visual.Graph
	if err := json.Unmarshal(payload, &loaded); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	loaded.Workflow.Description = "audit graph update"
	code, payload = serveJSONWithHeaders(t, router, http.MethodPut, "/api/v1/workflows/api-graph/graph", map[string]any{
		"graph":     loaded,
		"save_note": "audit graph update",
	}, auditHeaders())
	if code != http.StatusOK {
		t.Fatalf("update graph status = %d payload=%s", code, payload)
	}

	code, payload = serveJSONWithHeaders(t, router, http.MethodPost, "/api/v1/workflows/api-graph/validate", nil, auditHeaders())
	if code != http.StatusOK {
		t.Fatalf("validate status = %d payload=%s", code, payload)
	}

	code, payload = serveJSONWithHeaders(t, router, http.MethodPost, "/api/v1/workflows/graph/dry-run", map[string]any{
		"graph":         loaded,
		"workflow_name": "api-graph",
		"triggered_by":  "ops-auditor",
	}, auditHeaders())
	if code != http.StatusOK {
		t.Fatalf("dry-run status = %d payload=%s", code, payload)
	}

	code, payload = serveJSONWithHeaders(t, router, http.MethodPost, "/api/v1/workflows/api-graph/publish", map[string]any{
		"save_note": "audit publish",
	}, auditHeaders())
	if code != http.StatusOK {
		t.Fatalf("publish status = %d payload=%s", code, payload)
	}

	code, payload = serveJSONWithHeaders(t, router, http.MethodPost, "/api/v1/workflows/graph/runs", map[string]any{"graph": graph}, auditHeaders())
	if code != http.StatusAccepted {
		t.Fatalf("submit graph run status = %d payload=%s", code, payload)
	}

	cancelRunID := "run-audit-cancel"
	if err := runStore.CreateRun(context.Background(), state.RunState{
		RunID:        cancelRunID,
		WorkflowName: "api-graph",
		Status:       state.RunStatusRunning,
		StartedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		Version:      1,
	}); err != nil {
		t.Fatalf("create cancel run: %v", err)
	}
	code, payload = serveJSONWithHeaders(t, router, http.MethodPost, "/api/v1/runs/"+cancelRunID+"/cancel", nil, auditHeaders())
	if code != http.StatusOK {
		t.Fatalf("cancel run status = %d payload=%s", code, payload)
	}

	code, _ = serveJSONWithHeaders(t, router, http.MethodPost, "/api/v1/runs/"+cancelRunID+"/nodes/approval/approve", map[string]any{"comment": "approved"}, auditHeaders())
	if code != http.StatusNotFound {
		t.Fatalf("approval attempt status = %d, want 404", code)
	}

	actions := auditActionsFromLogs(t, logBuf.String())
	for _, action := range []string{
		"workflow.graph.create",
		"workflow.graph.update",
		"workflow.publish",
		"workflow.graph.run.submit",
		"run.cancel",
		"run.node.approve",
	} {
		if !actions[action] {
			t.Fatalf("missing audit action %q in logs:\n%s", action, logBuf.String())
		}
	}
}

func TestVisualWorkflowAuditPublishRequiresValidatedGraphHash(t *testing.T) {
	logBuf, restore := captureAPIAuditLogs(t)
	defer restore()

	workflowSvc := service.NewWorkflowService(t.TempDir())
	visualSvc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{WorkflowService: workflowSvc})
	router := NewRouter(RouterOptions{
		Workflow:       NewWorkflowHandler(workflowSvc),
		VisualWorkflow: NewVisualWorkflowHandler(visualSvc),
	})

	graph := sampleAPIGraph()
	graph.Workflow.Name = "api-publish-validated"
	code, payload := serveJSONWithHeaders(t, router, http.MethodPost, "/api/v1/workflows/graph", map[string]any{"graph": graph}, auditHeaders())
	if code != http.StatusCreated {
		t.Fatalf("create graph status = %d payload=%s", code, payload)
	}

	code, payload = serveJSONWithHeaders(t, router, http.MethodPost, "/api/v1/workflows/api-publish-validated/publish", nil, auditHeaders())
	if code != http.StatusBadRequest {
		t.Fatalf("publish before validate status = %d, want 400 payload=%s", code, payload)
	}

	code, payload = serveJSONWithHeaders(t, router, http.MethodPost, "/api/v1/workflows/api-publish-validated/validate", nil, auditHeaders())
	if code != http.StatusOK {
		t.Fatalf("validate status = %d payload=%s", code, payload)
	}
	var validated struct {
		Status             string `json:"status"`
		ValidatedGraphHash string `json:"validated_graph_hash"`
		ValidatedBy        string `json:"validated_by"`
	}
	if err := json.Unmarshal(payload, &validated); err != nil {
		t.Fatalf("decode validate response: %v", err)
	}
	if validated.Status != service.WorkflowStatusValidated || validated.ValidatedGraphHash == "" || validated.ValidatedBy != "ops-auditor" {
		t.Fatalf("validated response mismatch: %+v", validated)
	}

	code, payload = serveJSONWithHeaders(t, router, http.MethodPost, "/api/v1/workflows/api-publish-validated/publish", map[string]any{
		"save_note": "missing dry-run",
	}, auditHeaders())
	if code != http.StatusBadRequest {
		t.Fatalf("publish after validate before dry-run status = %d, want 400 payload=%s", code, payload)
	}

	code, payload = serveJSONWithHeaders(t, router, http.MethodPost, "/api/v1/workflows/graph/dry-run", map[string]any{
		"graph":         graph,
		"workflow_name": "api-publish-validated",
		"triggered_by":  "ops-auditor",
	}, auditHeaders())
	if code != http.StatusOK {
		t.Fatalf("dry-run status = %d payload=%s", code, payload)
	}
	var dryRun struct {
		Status          string `json:"status"`
		DryRunGraphHash string `json:"dry_run_graph_hash"`
	}
	if err := json.Unmarshal(payload, &dryRun); err != nil {
		t.Fatalf("decode dry-run response: %v", err)
	}
	if dryRun.Status != service.WorkflowStatusDryRunPassed || dryRun.DryRunGraphHash == "" {
		t.Fatalf("dry-run response mismatch: %+v", dryRun)
	}

	code, payload = serveJSONWithHeaders(t, router, http.MethodPost, "/api/v1/workflows/api-publish-validated/publish", map[string]any{
		"save_note":            "validated publish",
		"validated_graph_hash": validated.ValidatedGraphHash,
		"dry_run_graph_hash":   dryRun.DryRunGraphHash,
		"diff": map[string]any{
			"semantic_changes": []map[string]any{{"title": "restore action", "detail": "shell.run"}},
		},
	}, auditHeaders())
	if code != http.StatusOK {
		t.Fatalf("publish after validate status = %d payload=%s", code, payload)
	}
	var published struct {
		Status             string `json:"status"`
		PublishedGraphHash string `json:"published_graph_hash"`
	}
	if err := json.Unmarshal(payload, &published); err != nil {
		t.Fatalf("decode publish response: %v", err)
	}
	if published.Status != service.WorkflowStatusPublished || published.PublishedGraphHash != validated.ValidatedGraphHash {
		t.Fatalf("publish response mismatch: %+v validated=%+v", published, validated)
	}

	actions := auditActionsFromLogs(t, logBuf.String())
	for _, action := range []string{"workflow.graph.create", "workflow.validate", "workflow.publish"} {
		if !actions[action] {
			t.Fatalf("missing audit action %q in logs:\n%s", action, logBuf.String())
		}
	}
	for _, token := range []string{"validated_graph_hash", "published_graph_hash", "semantic_changes", "restore action"} {
		if !strings.Contains(logBuf.String(), token) {
			t.Fatalf("audit logs should include %q:\n%s", token, logBuf.String())
		}
	}
}

func captureAPIAuditLogs(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	var buf bytes.Buffer
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encCfg), zapcore.AddSync(&buf), zap.InfoLevel)
	prev := logging.L()
	logging.SetLogger(zap.New(core))
	return &buf, func() { logging.SetLogger(prev) }
}

func auditHeaders() map[string]string {
	return map[string]string{"X-Actor": "ops-auditor"}
}

func auditActionsFromLogs(t *testing.T, rawLogs string) map[string]bool {
	t.Helper()
	actions := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(rawLogs), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry := map[string]any{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["msg"] != "audit" {
			continue
		}
		if entry["ts"] == nil || entry["actor"] == "" || entry["resource"] == "" || entry["path"] == "" || entry["payload"] == nil {
			t.Fatalf("audit entry missing required fields: %+v", entry)
		}
		if action, ok := entry["action"].(string); ok {
			actions[action] = true
		}
	}
	return actions
}
