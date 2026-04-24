package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"runner/logging"
	"runner/scriptstore"
	"runner/server/api"
	"runner/server/events"
	"runner/server/metrics"
	"runner/server/queue"
	"runner/server/service"
	"runner/server/store/agentstore"
	"runner/server/store/envstore"
	"runner/server/store/eventstore"
	"runner/server/store/mcpstore"
	"runner/server/store/skillstore"
	"runner/state"
)

type appOptions struct {
	maxConcurrent int
	queueSize     int
	token         string
	maxOutput     int
}

type acceptanceApp struct {
	router http.Handler
	runSvc *service.RunService
	token  string
	stores struct {
		workflowsDir string
		scriptsDir   string
		skillsDir    string
		envDir       string
		mcpDir       string
		runStateFile string
		agentFile    string
	}
}

func newAcceptanceApp(t *testing.T, opts appOptions) *acceptanceApp {
	t.Helper()
	if opts.maxConcurrent <= 0 {
		opts.maxConcurrent = 2
	}
	if opts.queueSize <= 0 {
		opts.queueSize = 16
	}
	if strings.TrimSpace(opts.token) == "" {
		opts.token = "test-token"
	}
	if opts.maxOutput <= 0 {
		opts.maxOutput = 65536
	}

	base := t.TempDir()
	app := &acceptanceApp{token: opts.token}
	app.stores.workflowsDir = filepath.Join(base, "workflows")
	app.stores.scriptsDir = filepath.Join(base, "scripts")
	app.stores.skillsDir = filepath.Join(base, "skills")
	app.stores.envDir = filepath.Join(base, "environments")
	app.stores.mcpDir = filepath.Join(base, "mcp")
	app.stores.runStateFile = filepath.Join(base, "run-state.json")
	app.stores.agentFile = filepath.Join(base, "agents.json")

	workflowSvc := service.NewWorkflowService(app.stores.workflowsDir)
	scriptSvc := service.NewScriptService(scriptstore.NewFileStore(app.stores.scriptsDir))
	skillSvc := service.NewSkillService(skillstore.NewFileStore(app.stores.skillsDir))
	environmentSvc := service.NewEnvironmentService(envstore.NewFileStore(app.stores.envDir))
	mcpSvc := service.NewMcpService(mcpstore.NewFileStore(app.stores.mcpDir))
	agentSvc := service.NewAgentService(agentstore.NewFileStore(app.stores.agentFile), 1)
	preprocessor := service.NewPreprocessor(scriptSvc, agentSvc, []string{
		"cmd.run", "shell.run", "script.shell", "script.python", "wait.event",
	})
	runStore := state.NewFileStore(app.stores.runStateFile)
	runQueue := queue.NewMemoryQueue(opts.queueSize)
	hub := events.NewHub()
	collector := metrics.NewCollector()
	runSvc := service.NewRunService(service.RunServiceConfig{
		MaxConcurrentRuns: opts.maxConcurrent,
		MaxOutputBytes:    opts.maxOutput,
		MetaStore:         service.NewFileRunRecordStore(service.DeriveRunRecordFile(app.stores.runStateFile)),
		EventStore:        eventstore.NewFileStore(eventstore.DeriveRunEventDir(app.stores.runStateFile)),
	}, workflowSvc, preprocessor, runStore, runQueue, hub, collector)
	app.runSvc = runSvc
	dashboardSvc := service.NewDashboardService(runSvc, agentSvc)
	systemSvc := service.NewSystemService(runSvc, agentSvc)

	app.router = api.NewRouter(api.RouterOptions{
		AuthEnabled:    true,
		AuthToken:      app.token,
		Health:         &api.HealthHandler{},
		Workflow:       api.NewWorkflowHandler(workflowSvc),
		Script:         api.NewScriptHandler(scriptSvc),
		Run:            api.NewRunHandler(runSvc),
		Agent:          api.NewAgentHandler(agentSvc),
		Skill:          api.NewSkillHandler(skillSvc),
		Environment:    api.NewEnvironmentHandler(environmentSvc),
		MCP:            api.NewMcpHandler(mcpSvc),
		Dashboard:      api.NewDashboardHandler(dashboardSvc),
		System:         api.NewSystemHandler(api.SystemInfo{Version: "test", BuildTime: "-", DocsURL: "https://example.test/docs", RepoURL: "https://example.test/repo", AuthEnabled: true}, systemSvc),
		MetricsHandler: collector.Handler(),
	})
	return app
}

func (a *acceptanceApp) close() {
	if a != nil && a.runSvc != nil {
		a.runSvc.Close()
	}
}

func (a *acceptanceApp) request(t *testing.T, method, path string, body any, auth bool) (int, map[string]any, string, http.Header) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		switch typed := body.(type) {
		case string:
			reader = strings.NewReader(typed)
		case []byte:
			reader = bytes.NewReader(typed)
		default:
			payload, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			reader = bytes.NewReader(payload)
		}
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)
	raw := rec.Body.String()
	var data map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &data)
	return rec.Code, data, raw, rec.Header()
}

func waitRunStatus(t *testing.T, app *acceptanceApp, runID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		code, data, _, _ := app.request(t, http.MethodGet, "/api/v1/runs/"+runID, nil, true)
		if code == http.StatusOK {
			status := asString(data["status"])
			switch status {
			case "success", "failed", "canceled", "interrupted":
				return status
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("wait run status timeout for run %s", runID)
	return ""
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func createWorkflowYAML(name, cmd string) string {
	return fmt.Sprintf(`version: "1"
name: %s
inventory:
  hosts:
    local:
      address: 127.0.0.1
steps:
  - name: run
    targets: [local]
    action: cmd.run
    args:
      cmd: %q
`, name, cmd)
}

func mustMetricValue(t *testing.T, raw, metric string) float64 {
	t.Helper()
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if fields[0] != metric {
			continue
		}
		v, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			t.Fatalf("parse metric %s: %v", metric, err)
		}
		return v
	}
	t.Fatalf("metric %s not found", metric)
	return 0
}

func withCapturedLogs(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	var buf bytes.Buffer
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encCfg),
		zapcore.AddSync(&buf),
		zap.InfoLevel,
	)
	prev := logging.L()
	logging.SetLogger(zap.New(core))
	restore := func() {
		logging.SetLogger(prev)
	}
	return &buf, restore
}

func TestAcceptanceChecklist(t *testing.T) {
	app := newAcceptanceApp(t, appOptions{})
	defer app.close()

	t.Run("F01 基础服务与网关", func(t *testing.T) {
		code, _, _, headers := app.request(t, http.MethodGet, "/healthz", nil, false)
		if code != http.StatusOK {
			t.Fatalf("healthz expected 200, got %d", code)
		}
		if trace := headers.Get(api.HeaderTraceID); strings.TrimSpace(trace) == "" {
			t.Fatalf("trace header is missing")
		}

		code, _, _, _ = app.request(t, http.MethodGet, "/readyz", nil, false)
		if code != http.StatusOK {
			t.Fatalf("readyz expected 200, got %d", code)
		}

		code, _, _, _ = app.request(t, http.MethodGet, "/api/v1/workflows", nil, false)
		if code != http.StatusUnauthorized {
			t.Fatalf("unauthorized expected 401, got %d", code)
		}

		code, _, _, _ = app.request(t, http.MethodGet, "/api/v1/workflows", nil, true)
		if code != http.StatusOK {
			t.Fatalf("authorized expected 200, got %d", code)
		}
	})

	t.Run("F02 Workflow 管理", func(t *testing.T) {
		yaml := createWorkflowYAML("wf-basic", "echo wf-basic")
		code, _, _, _ := app.request(t, http.MethodPost, "/api/v1/workflows", map[string]any{
			"name":        "wf-basic",
			"description": "workflow basic",
			"yaml":        yaml,
			"labels": map[string]string{
				"env": "prod",
			},
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create workflow expected 201, got %d", code)
		}

		code, data, _, _ := app.request(t, http.MethodGet, "/api/v1/workflows/wf-basic", nil, true)
		if code != http.StatusOK {
			t.Fatalf("get workflow expected 200, got %d", code)
		}
		if got := asString(data["name"]); got != "wf-basic" {
			t.Fatalf("unexpected workflow name: %s", got)
		}
		updatedAtBefore := asString(data["updated_at"])
		if updatedAtBefore == "" {
			t.Fatalf("workflow updated_at should not be empty before update")
		}

		code, data, _, _ = app.request(t, http.MethodGet, "/api/v1/workflows?labels=env:prod", nil, true)
		if code != http.StatusOK {
			t.Fatalf("list workflow expected 200, got %d", code)
		}
		items, ok := data["items"].([]any)
		if !ok || len(items) == 0 {
			t.Fatalf("expected filtered workflow list")
		}

		updatedYAML := createWorkflowYAML("wf-basic", "echo wf-basic-updated")
		code, _, _, _ = app.request(t, http.MethodPut, "/api/v1/workflows/wf-basic", map[string]any{
			"yaml": updatedYAML,
		}, true)
		if code != http.StatusOK {
			t.Fatalf("update workflow expected 200, got %d", code)
		}
		code, data, _, _ = app.request(t, http.MethodGet, "/api/v1/workflows/wf-basic", nil, true)
		if code != http.StatusOK {
			t.Fatalf("get workflow after update expected 200, got %d", code)
		}
		updatedAtAfter := asString(data["updated_at"])
		if updatedAtAfter == "" {
			t.Fatalf("workflow updated_at should not be empty after update")
		}
		if updatedAtAfter == updatedAtBefore {
			t.Fatalf("workflow updated_at should change after update")
		}
		labels, ok := data["labels"].(map[string]any)
		if !ok {
			t.Fatalf("workflow labels should exist after update")
		}
		if got := asString(labels["env"]); got != "prod" {
			t.Fatalf("workflow labels should be preserved after update, got env=%s", got)
		}

		code, data, _, _ = app.request(t, http.MethodPost, "/api/v1/workflows/wf-basic/validate", nil, true)
		if code != http.StatusOK {
			t.Fatalf("validate workflow expected 200, got %d", code)
		}
		if valid := data["valid"]; valid != true {
			t.Fatalf("validate response expected true, got %v", valid)
		}

		badYAML := createWorkflowYAML("another-name", "echo bad")
		code, _, _, _ = app.request(t, http.MethodPost, "/api/v1/workflows", map[string]any{
			"name": "bad-name",
			"yaml": badYAML,
		}, true)
		if code != http.StatusBadRequest {
			t.Fatalf("invalid workflow expected 400, got %d", code)
		}

		code, _, _, _ = app.request(t, http.MethodDelete, "/api/v1/workflows/wf-basic", nil, true)
		if code != http.StatusOK {
			t.Fatalf("delete workflow expected 200, got %d", code)
		}
		code, _, _, _ = app.request(t, http.MethodGet, "/api/v1/workflows/wf-basic", nil, true)
		if code != http.StatusNotFound {
			t.Fatalf("deleted workflow expected 404, got %d", code)
		}
	})

	t.Run("F03 Script 管理", func(t *testing.T) {
		code, _, _, _ := app.request(t, http.MethodPost, "/api/v1/scripts", map[string]any{
			"name":        "script1",
			"language":    "shell",
			"content":     "echo ${name}",
			"description": "script demo",
			"tags":        []string{"demo"},
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create script expected 201, got %d", code)
		}

		code, data, _, _ := app.request(t, http.MethodGet, "/api/v1/scripts/script1", nil, true)
		if code != http.StatusOK {
			t.Fatalf("get script expected 200, got %d", code)
		}
		if got := int(data["version"].(float64)); got != 1 {
			t.Fatalf("expected version 1, got %d", got)
		}
		if asString(data["checksum"]) == "" {
			t.Fatalf("checksum should not be empty")
		}

		code, _, _, _ = app.request(t, http.MethodPut, "/api/v1/scripts/script1", map[string]any{
			"content": "echo ${name}-v2",
		}, true)
		if code != http.StatusOK {
			t.Fatalf("update script expected 200, got %d", code)
		}
		code, data, _, _ = app.request(t, http.MethodGet, "/api/v1/scripts/script1", nil, true)
		if got := int(data["version"].(float64)); got != 2 {
			t.Fatalf("expected version 2, got %d", got)
		}

		code, data, _, _ = app.request(t, http.MethodPost, "/api/v1/scripts/script1/render", map[string]any{
			"vars": map[string]any{"name": "runner"},
		}, true)
		if code != http.StatusOK {
			t.Fatalf("render script expected 200, got %d", code)
		}
		if !strings.Contains(asString(data["rendered"]), "runner") {
			t.Fatalf("render output missing rendered var")
		}

		code, _, _, _ = app.request(t, http.MethodPost, "/api/v1/scripts", map[string]any{
			"name":     "script-bad",
			"language": "ruby",
			"content":  "puts 1",
		}, true)
		if code != http.StatusBadRequest {
			t.Fatalf("invalid script language expected 400, got %d", code)
		}

		code, _, _, _ = app.request(t, http.MethodDelete, "/api/v1/scripts/script1", nil, true)
		if code != http.StatusOK {
			t.Fatalf("delete script expected 200, got %d", code)
		}
		code, _, _, _ = app.request(t, http.MethodGet, "/api/v1/scripts/script1", nil, true)
		if code != http.StatusNotFound {
			t.Fatalf("deleted script expected 404, got %d", code)
		}
	})

	t.Run("F04/F05/F07/F10 运行链路+预处理+事件+指标", func(t *testing.T) {
		quickYAML := createWorkflowYAML("wf-run-quick", "echo quick")
		code, _, _, _ := app.request(t, http.MethodPost, "/api/v1/workflows", map[string]any{
			"name": "wf-run-quick",
			"yaml": quickYAML,
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create quick workflow expected 201, got %d", code)
		}

		code, data, _, _ := app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_name": "wf-run-quick",
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("submit by name expected 202, got %d", code)
		}
		runID1 := asString(data["run_id"])
		if runID1 == "" {
			t.Fatalf("run_id should not be empty")
		}
		if status := waitRunStatus(t, app, runID1, 5*time.Second); status != "success" {
			t.Fatalf("quick run expected success, got %s", status)
		}

		code, data, _, _ = app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_yaml": quickYAML,
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("submit by yaml expected 202, got %d", code)
		}
		runID2 := asString(data["run_id"])
		if status := waitRunStatus(t, app, runID2, 5*time.Second); status != "success" {
			t.Fatalf("yaml run expected success, got %s", status)
		}

		code, data, _, _ = app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_name":   "wf-run-quick",
			"idempotency_key": "same-key",
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("idempotency submit expected 202, got %d", code)
		}
		keyRunA := asString(data["run_id"])
		code, data, _, _ = app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_name":   "wf-run-quick",
			"idempotency_key": "same-key",
		}, true)
		keyRunB := asString(data["run_id"])
		if keyRunA == "" || keyRunA != keyRunB {
			t.Fatalf("idempotency expected same run id, got %s vs %s", keyRunA, keyRunB)
		}

		// GET /runs workflow+limit filtering
		code, data, _, _ = app.request(t, http.MethodGet, "/api/v1/runs?workflow=wf-run-quick&limit=1", nil, true)
		if code != http.StatusOK {
			t.Fatalf("list runs with filter expected 200, got %d", code)
		}
		runItems, ok := data["items"].([]any)
		if !ok || len(runItems) != 1 {
			t.Fatalf("expected exactly 1 run by limit filter, got %v", data["items"])
		}
		firstRun, ok := runItems[0].(map[string]any)
		if !ok {
			t.Fatalf("invalid run item shape")
		}
		if got := asString(firstRun["workflow_name"]); got != "wf-run-quick" {
			t.Fatalf("workflow filter mismatch, got %s", got)
		}

		// event completeness: ensure run_start/step_start/host_result/run_finish all appear
		eventApp := newAcceptanceApp(t, appOptions{
			maxConcurrent: 1,
			queueSize:     4,
			token:         "event-token",
		})
		defer eventApp.close()
		blockerYAML := createWorkflowYAML("wf-blocker", "sleep 1")
		code, _, _, _ = eventApp.request(t, http.MethodPost, "/api/v1/workflows", map[string]any{
			"name": "wf-blocker",
			"yaml": blockerYAML,
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create blocker workflow expected 201, got %d", code)
		}
		targetYAML := createWorkflowYAML("wf-event-target", "echo evt")
		code, _, _, _ = eventApp.request(t, http.MethodPost, "/api/v1/workflows", map[string]any{
			"name": "wf-event-target",
			"yaml": targetYAML,
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create target workflow expected 201, got %d", code)
		}
		code, _, _, _ = eventApp.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_name": "wf-blocker",
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("submit blocker run expected 202, got %d", code)
		}
		code, data, _, _ = eventApp.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_name": "wf-event-target",
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("submit target run expected 202, got %d", code)
		}
		eventRunID := asString(data["run_id"])
		evtCh, cancelEvt, err := eventApp.runSvc.Subscribe(context.Background(), eventRunID)
		if err != nil {
			t.Fatalf("subscribe event target run: %v", err)
		}
		defer cancelEvt()
		if status := waitRunStatus(t, eventApp, eventRunID, 6*time.Second); status != "success" {
			t.Fatalf("target run expected success, got %s", status)
		}
		seenTypes := map[string]bool{}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case evt := <-evtCh:
				seenTypes[strings.TrimSpace(evt.Type)] = true
				if seenTypes["run_start"] && seenTypes["step_start"] && seenTypes["host_result"] && seenTypes["run_finish"] {
					deadline = time.Now()
				}
			default:
				time.Sleep(50 * time.Millisecond)
			}
		}
		for _, expected := range []string{"run_start", "step_start", "host_result", "run_finish"} {
			if !seenTypes[expected] {
				t.Fatalf("event stream missing %s", expected)
			}
		}

		slowYAML := createWorkflowYAML("wf-run-slow", "sleep 2")
		code, _, _, _ = app.request(t, http.MethodPost, "/api/v1/workflows", map[string]any{
			"name": "wf-run-slow",
			"yaml": slowYAML,
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create slow workflow expected 201, got %d", code)
		}

		code, data, _, _ = app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_name": "wf-run-slow",
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("submit slow run expected 202, got %d", code)
		}
		slowRunID := asString(data["run_id"])
		evtCh, cancelEvents, err := app.runSvc.Subscribe(context.Background(), slowRunID)
		if err != nil {
			t.Fatalf("subscribe events: %v", err)
		}
		defer cancelEvents()

		time.Sleep(300 * time.Millisecond)
		code, _, _, _ = app.request(t, http.MethodPost, "/api/v1/runs/"+slowRunID+"/cancel", nil, true)
		if code != http.StatusOK {
			t.Fatalf("cancel run expected 200, got %d", code)
		}
		status := waitRunStatus(t, app, slowRunID, 8*time.Second)
		if status != "canceled" && status != "failed" {
			t.Fatalf("canceled run expected canceled/failed, got %s", status)
		}

		seenFinish := false
		waitDeadline := time.After(2 * time.Second)
		for !seenFinish {
			select {
			case evt := <-evtCh:
				if strings.TrimSpace(evt.Type) == "run_finish" {
					seenFinish = true
				}
			case <-waitDeadline:
				t.Fatalf("event stream did not receive run_finish")
			}
		}

		code, data, rawMetrics, _ := app.request(t, http.MethodGet, "/metrics", nil, false)
		if code != http.StatusOK || len(data) != 0 {
			// metrics is text, ignore data map
		}
		if !strings.Contains(rawMetrics, "runner_server_runs_submitted_total") {
			t.Fatalf("metrics missing submitted_total")
		}
		if !strings.Contains(rawMetrics, "runner_server_queue_depth") {
			t.Fatalf("metrics missing queue_depth")
		}

		// script_ref success
		code, _, _, _ = app.request(t, http.MethodPost, "/api/v1/scripts", map[string]any{
			"name":     "script-ref-ok",
			"language": "shell",
			"content":  "echo from-ref",
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create script_ref script expected 201, got %d", code)
		}
		refYAML := `version: "1"
name: wf-script-ref
inventory:
  hosts:
    local:
      address: 127.0.0.1
steps:
  - name: run
    targets: [local]
    action: script.shell
    args:
      script_ref: script-ref-ok
`
		code, data, _, _ = app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_yaml": refYAML,
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("submit script_ref run expected 202, got %d", code)
		}
		if status := waitRunStatus(t, app, asString(data["run_id"]), 5*time.Second); status != "success" {
			t.Fatalf("script_ref run expected success, got %s", status)
		}

		// script & script_ref conflict
		conflictYAML := `version: "1"
name: wf-script-conflict
inventory:
  hosts:
    local:
      address: 127.0.0.1
steps:
  - name: run
    targets: [local]
    action: script.shell
    args:
      script: "echo direct"
      script_ref: script-ref-ok
`
		code, _, _, _ = app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_yaml": conflictYAML,
		}, true)
		if code != http.StatusBadRequest {
			t.Fatalf("script conflict expected 400, got %d", code)
		}

		// action whitelist
		badActionYAML := `version: "1"
name: wf-bad-action
inventory:
  hosts:
    local:
      address: 127.0.0.1
steps:
  - name: run
    targets: [local]
    action: bad.action
    args:
      x: 1
`
		code, _, _, _ = app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_yaml": badActionYAML,
		}, true)
		if code != http.StatusBadRequest {
			t.Fatalf("bad action expected 400, got %d", code)
		}
	})

	t.Run("F05/F06 Agent 预处理与生命周期", func(t *testing.T) {
		code, _, _, _ := app.request(t, http.MethodPost, "/api/v1/agents", map[string]any{
			"id":           "agent-a",
			"name":         "agent-a",
			"address":      "http://127.0.0.1:65530",
			"token":        "agent-secret-token",
			"tags":         []string{"prod"},
			"capabilities": []string{"shell.run"},
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create agent expected 201, got %d", code)
		}

		code, data, _, _ := app.request(t, http.MethodGet, "/api/v1/agents/agent-a", nil, true)
		if code != http.StatusOK {
			t.Fatalf("get agent expected 200, got %d", code)
		}
		if token := asString(data["token"]); token != "***" {
			t.Fatalf("agent token expected masked, got %s", token)
		}

		raw, err := os.ReadFile(app.stores.agentFile)
		if err != nil {
			t.Fatalf("read agent file: %v", err)
		}
		if strings.Contains(string(raw), "agent-secret-token") {
			t.Fatalf("agent token should not persist in plain text")
		}

		// capability mismatch
		yamlCapability := `version: "1"
name: wf-agent-capability
inventory:
  hosts:
    remote1:
      address: agent://agent-a
steps:
  - name: run
    targets: [remote1]
    action: cmd.run
    args:
      cmd: "echo hi"
`
		code, _, _, _ = app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_yaml": yamlCapability,
		}, true)
		if code != http.StatusBadRequest {
			t.Fatalf("capability mismatch expected 400, got %d", code)
		}

		// update capability to cmd.run and heartbeat offline
		code, _, _, _ = app.request(t, http.MethodPut, "/api/v1/agents/agent-a", map[string]any{
			"capabilities": []string{"cmd.run"},
		}, true)
		if code != http.StatusOK {
			t.Fatalf("update agent expected 200, got %d", code)
		}
		code, _, _, _ = app.request(t, http.MethodPost, "/api/v1/agents/agent-a/heartbeat", map[string]any{
			"status": "offline",
		}, true)
		if code != http.StatusOK {
			t.Fatalf("heartbeat offline expected 200, got %d", code)
		}
		code, _, _, _ = app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_yaml": yamlCapability,
		}, true)
		if code != http.StatusServiceUnavailable {
			t.Fatalf("offline agent expected 503, got %d", code)
		}

		// heartbeat online, submission accepted (runtime may fail due unreachable remote)
		code, _, _, _ = app.request(t, http.MethodPost, "/api/v1/agents/agent-a/heartbeat", map[string]any{
			"status": "online",
		}, true)
		if code != http.StatusOK {
			t.Fatalf("heartbeat online expected 200, got %d", code)
		}
		code, _, _, _ = app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_yaml": yamlCapability,
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("online agent submit expected 202, got %d", code)
		}
		// probe failure path (unreachable agent)
		code, _, _, _ = app.request(t, http.MethodPost, "/api/v1/agents/agent-a/probe", nil, true)
		if code != http.StatusInternalServerError {
			t.Fatalf("probe failure expected 500, got %d", code)
		}

		code, _, _, _ = app.request(t, http.MethodDelete, "/api/v1/agents/agent-a", nil, true)
		if code != http.StatusOK {
			t.Fatalf("delete agent expected 200, got %d", code)
		}
		code, _, _, _ = app.request(t, http.MethodGet, "/api/v1/agents/agent-a", nil, true)
		if code != http.StatusNotFound {
			t.Fatalf("deleted agent expected 404, got %d", code)
		}
	})

	t.Run("F08 队列限流与重启恢复", func(t *testing.T) {
		queueApp := newAcceptanceApp(t, appOptions{
			maxConcurrent: 1,
			queueSize:     1,
			token:         "queue-token",
		})
		defer queueApp.close()

		slowYAML := createWorkflowYAML("wf-queue-slow", "sleep 2")
		code, _, _, _ := queueApp.request(t, http.MethodPost, "/api/v1/workflows", map[string]any{
			"name": "wf-queue-slow",
			"yaml": slowYAML,
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create queue workflow expected 201, got %d", code)
		}
		var data map[string]any
		for i := 0; i < 2; i++ {
			code, data, _, _ = queueApp.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
				"workflow_name": "wf-queue-slow",
			}, true)
			if code != http.StatusAccepted {
				t.Fatalf("submit queue run expected 202, got %d", code)
			}
			if asString(data["run_id"]) == "" {
				t.Fatalf("queue run id should not be empty")
			}
		}
		code, _, _, _ = queueApp.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_name": "wf-queue-slow",
		}, true)
		if code != http.StatusServiceUnavailable {
			t.Fatalf("queue full expected 503, got %d", code)
		}

		// concurrent limit: with maxConcurrent=1, observed running count should never exceed 1.
		maxRunning := 0
		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			code, data, _, _ = queueApp.request(t, http.MethodGet, "/api/v1/runs?status=running&limit=100", nil, true)
			if code != http.StatusOK {
				t.Fatalf("list running runs expected 200, got %d", code)
			}
			items, _ := data["items"].([]any)
			if len(items) > maxRunning {
				maxRunning = len(items)
			}
			code, data, _, _ = queueApp.request(t, http.MethodGet, "/api/v1/runs?status=success&limit=100", nil, true)
			if code == http.StatusOK {
				successItems, _ := data["items"].([]any)
				code2, data2, _, _ := queueApp.request(t, http.MethodGet, "/api/v1/runs?status=failed&limit=100", nil, true)
				failedItems := []any{}
				if code2 == http.StatusOK {
					failedItems, _ = data2["items"].([]any)
				}
				if len(successItems)+len(failedItems) >= 2 {
					break
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		if maxRunning > 1 {
			t.Fatalf("max running expected <=1, got %d", maxRunning)
		}

		// output truncation
		truncApp := newAcceptanceApp(t, appOptions{
			maxConcurrent: 1,
			queueSize:     4,
			token:         "trunc-token",
			maxOutput:     32,
		})
		defer truncApp.close()
		truncYAML := createWorkflowYAML("wf-output-trunc", "printf 'abcdefghijklmnopqrstuvwxyz0123456789'")
		code, _, _, _ = truncApp.request(t, http.MethodPost, "/api/v1/workflows", map[string]any{
			"name": "wf-output-trunc",
			"yaml": truncYAML,
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create trunc workflow expected 201, got %d", code)
		}
		code, data, _, _ = truncApp.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_name": "wf-output-trunc",
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("submit trunc run expected 202, got %d", code)
		}
		truncRunID := asString(data["run_id"])
		if status := waitRunStatus(t, truncApp, truncRunID, 6*time.Second); status != "success" {
			t.Fatalf("trunc run expected success, got %s", status)
		}
		code, data, _, _ = truncApp.request(t, http.MethodGet, "/api/v1/runs/"+truncRunID, nil, true)
		if code != http.StatusOK {
			t.Fatalf("get trunc run expected 200, got %d", code)
		}
		steps, ok := data["steps"].([]any)
		if !ok || len(steps) == 0 {
			t.Fatalf("trunc run missing steps")
		}
		step0, ok := steps[0].(map[string]any)
		if !ok {
			t.Fatalf("invalid step shape")
		}
		hosts, ok := step0["hosts"].(map[string]any)
		if !ok || len(hosts) == 0 {
			t.Fatalf("trunc run missing hosts")
		}
		var stdout string
		for _, rawHost := range hosts {
			host, ok := rawHost.(map[string]any)
			if !ok {
				continue
			}
			output, ok := host["output"].(map[string]any)
			if !ok {
				continue
			}
			stdout = asString(output["stdout"])
			break
		}
		if stdout == "" {
			t.Fatalf("stdout should not be empty")
		}
		if !strings.Contains(stdout, "...(truncated)") {
			t.Fatalf("stdout should contain truncated marker, got: %s", stdout)
		}
		if len(stdout) > 64 {
			t.Fatalf("stdout truncation should keep bounded size, got %d", len(stdout))
		}

		// restart recovery
		store := state.NewFileStore(filepath.Join(t.TempDir(), "restart-run-state.json"))
		runID := state.NewRunID()
		if err := store.CreateRun(context.Background(), state.RunState{
			RunID:        runID,
			WorkflowName: "wf-restart",
			Status:       state.RunStatusRunning,
			StartedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
			Version:      1,
		}); err != nil {
			t.Fatalf("create running run: %v", err)
		}
		restartSvc := service.NewRunService(service.RunServiceConfig{
			MaxConcurrentRuns: 1,
			MaxOutputBytes:    1024,
		}, nil, nil, store, queue.NewMemoryQueue(1), events.NewHub(), metrics.NewCollector())
		defer restartSvc.Close()
		restored, err := store.GetRun(context.Background(), runID)
		if err != nil {
			t.Fatalf("get restored run: %v", err)
		}
		if restored.Status != state.RunStatusInterrupted {
			t.Fatalf("expected interrupted after restart, got %s", restored.Status)
		}
	})

	t.Run("F09 审计日志字段与脱敏", func(t *testing.T) {
		logBuf, restore := withCapturedLogs(t)
		defer restore()

		logApp := newAcceptanceApp(t, appOptions{
			maxConcurrent: 1,
			queueSize:     4,
			token:         "audit-token",
		})
		defer logApp.close()

		code, _, _, _ := logApp.request(t, http.MethodPost, "/api/v1/agents", map[string]any{
			"id":      "agent-audit",
			"name":    "agent-audit",
			"address": "http://127.0.0.1:7072",
			"token":   "very-secret-token",
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create agent expected 201, got %d", code)
		}
		code, _, _, _ = logApp.request(t, http.MethodPost, "/api/v1/scripts", map[string]any{
			"name":     "audit-script",
			"language": "shell",
			"content":  "echo ok",
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create script expected 201, got %d", code)
		}
		wfYAML := createWorkflowYAML("wf-audit", "echo audit")
		code, _, _, _ = logApp.request(t, http.MethodPost, "/api/v1/workflows", map[string]any{
			"name": "wf-audit",
			"yaml": wfYAML,
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create workflow expected 201, got %d", code)
		}
		code, _, _, _ = logApp.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_name": "wf-audit",
			"vars": map[string]any{
				"api_key": "secret-key-123",
			},
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("submit run expected 202, got %d", code)
		}

		rawLogs := strings.TrimSpace(logBuf.String())
		if rawLogs == "" {
			t.Fatalf("expected audit logs, got none")
		}
		seenAudit := false
		seenRawToken := false
		seenRawKey := false
		for _, line := range strings.Split(rawLogs, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			entry := map[string]any{}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			if asString(entry["msg"]) != "audit" {
				continue
			}
			seenAudit = true
			if asString(entry["action"]) == "" || asString(entry["resource"]) == "" || asString(entry["actor"]) == "" {
				t.Fatalf("audit log missing required fields: %+v", entry)
			}
			payload, _ := entry["payload"].(map[string]any)
			if payload != nil {
				if asString(payload["token"]) == "very-secret-token" {
					seenRawToken = true
				}
				if vars, ok := payload["vars"].(map[string]any); ok {
					if asString(vars["api_key"]) == "secret-key-123" {
						seenRawKey = true
					}
				}
			}
		}
		if !seenAudit {
			t.Fatalf("expected audit entries")
		}
		if !seenRawToken {
			t.Fatalf("expected raw token in audit payload")
		}
		if !seenRawKey {
			t.Fatalf("expected raw api_key in audit payload")
		}
	})

	t.Run("F10 指标值严格一致性", func(t *testing.T) {
		metricApp := newAcceptanceApp(t, appOptions{
			maxConcurrent: 1,
			queueSize:     4,
			token:         "metric-token",
		})
		defer metricApp.close()

		wfYAML := createWorkflowYAML("wf-metric", "echo metric")
		code, _, _, _ := metricApp.request(t, http.MethodPost, "/api/v1/workflows", map[string]any{
			"name": "wf-metric",
			"yaml": wfYAML,
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create metric workflow expected 201, got %d", code)
		}

		code, _, beforeRaw, _ := metricApp.request(t, http.MethodGet, "/metrics", nil, false)
		if code != http.StatusOK {
			t.Fatalf("metrics before expected 200, got %d", code)
		}
		subBefore := mustMetricValue(t, beforeRaw, "runner_server_runs_submitted_total")
		startBefore := mustMetricValue(t, beforeRaw, "runner_server_runs_started_total")
		finishBefore := mustMetricValue(t, beforeRaw, "runner_server_runs_finished_total")
		successBefore := mustMetricValue(t, beforeRaw, "runner_server_runs_success_total")

		code, data, _, _ := metricApp.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_name": "wf-metric",
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("submit metric run expected 202, got %d", code)
		}
		runID := asString(data["run_id"])
		if status := waitRunStatus(t, metricApp, runID, 6*time.Second); status != "success" {
			t.Fatalf("metric run expected success, got %s", status)
		}

		code, _, afterRaw, _ := metricApp.request(t, http.MethodGet, "/metrics", nil, false)
		if code != http.StatusOK {
			t.Fatalf("metrics after expected 200, got %d", code)
		}
		subAfter := mustMetricValue(t, afterRaw, "runner_server_runs_submitted_total")
		startAfter := mustMetricValue(t, afterRaw, "runner_server_runs_started_total")
		finishAfter := mustMetricValue(t, afterRaw, "runner_server_runs_finished_total")
		successAfter := mustMetricValue(t, afterRaw, "runner_server_runs_success_total")

		if math.Abs((subAfter-subBefore)-1) > 0.001 {
			t.Fatalf("submitted_total diff expected 1, got %.3f", subAfter-subBefore)
		}
		if math.Abs((startAfter-startBefore)-1) > 0.001 {
			t.Fatalf("started_total diff expected 1, got %.3f", startAfter-startBefore)
		}
		if math.Abs((finishAfter-finishBefore)-1) > 0.001 {
			t.Fatalf("finished_total diff expected 1, got %.3f", finishAfter-finishBefore)
		}
		if math.Abs((successAfter-successBefore)-1) > 0.001 {
			t.Fatalf("success_total diff expected 1, got %.3f", successAfter-successBefore)
		}
	})

	t.Run("F11 聚合页与资产管理 API", func(t *testing.T) {
		assetApp := newAcceptanceApp(t, appOptions{
			maxConcurrent: 1,
			queueSize:     4,
			token:         "asset-token",
		})
		defer assetApp.close()

		code, data, _, _ := assetApp.request(t, http.MethodGet, "/api/v1/dashboard", nil, true)
		if code != http.StatusOK {
			t.Fatalf("dashboard expected 200, got %d", code)
		}
		if _, ok := data["total_runs"]; !ok {
			t.Fatalf("dashboard missing total_runs: %+v", data)
		}

		code, data, _, _ = assetApp.request(t, http.MethodGet, "/api/v1/system/info", nil, true)
		if code != http.StatusOK {
			t.Fatalf("system info expected 200, got %d", code)
		}
		if asString(data["version"]) != "test" {
			t.Fatalf("unexpected system version: %+v", data)
		}

		code, data, _, _ = assetApp.request(t, http.MethodGet, "/api/v1/system/metrics", nil, true)
		if code != http.StatusOK {
			t.Fatalf("system metrics expected 200, got %d", code)
		}
		if _, ok := data["summary"].(map[string]any); !ok {
			t.Fatalf("system metrics missing summary: %+v", data)
		}

		code, _, _, _ = assetApp.request(t, http.MethodPost, "/api/v1/skills", map[string]any{
			"name":        "doc-skill",
			"description": "doc helper",
			"triggers":    []string{"docs"},
			"content":     "# Skill\nDo things.",
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create skill expected 201, got %d", code)
		}
		code, data, _, _ = assetApp.request(t, http.MethodGet, "/api/v1/skills/doc-skill", nil, true)
		if code != http.StatusOK || asString(data["name"]) != "doc-skill" {
			t.Fatalf("get skill expected doc-skill, got %d %+v", code, data)
		}

		code, _, _, _ = assetApp.request(t, http.MethodPost, "/api/v1/environments", map[string]any{
			"name":        "prod",
			"description": "production",
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create environment expected 201, got %d", code)
		}
		code, _, _, _ = assetApp.request(t, http.MethodPost, "/api/v1/environments/prod/vars", map[string]any{
			"key":         "API_TOKEN",
			"value":       "secret",
			"description": "main token",
			"sensitive":   true,
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("add environment var expected 201, got %d", code)
		}
		code, data, _, _ = assetApp.request(t, http.MethodGet, "/api/v1/environments/prod", nil, true)
		if code != http.StatusOK {
			t.Fatalf("get environment expected 200, got %d", code)
		}
		vars, ok := data["vars"].([]any)
		if !ok || len(vars) != 1 {
			t.Fatalf("environment vars mismatch: %+v", data)
		}

		mcpHTTP := newHTTPTestServerOrSkip(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/mcp":
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
			case r.Method == http.MethodGet && r.URL.Path == "/mcp/tools":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"tools": []map[string]any{
						{
							"name":        "search_docs",
							"description": "Search docs",
							"inputSchema": map[string]any{"type": "object"},
						},
					},
				})
			default:
				http.NotFound(w, r)
			}
		}))
		defer mcpHTTP.Close()

		code, _, _, _ = assetApp.request(t, http.MethodPost, "/api/v1/mcp/servers", map[string]any{
			"id":       "docs",
			"name":     "Docs MCP",
			"type":     "http",
			"url":      mcpHTTP.URL + "/mcp",
			"env_vars": map[string]string{"TOKEN": "secret"},
		}, true)
		if code != http.StatusCreated {
			t.Fatalf("create mcp server expected 201, got %d", code)
		}
		code, data, _, _ = assetApp.request(t, http.MethodPost, "/api/v1/mcp/servers/docs/toggle", nil, true)
		if code != http.StatusOK {
			t.Fatalf("toggle mcp server expected 200, got %d", code)
		}
		if asString(data["status"]) != "running" {
			t.Fatalf("expected running mcp server, got %+v", data)
		}
		code, data, _, _ = assetApp.request(t, http.MethodGet, "/api/v1/mcp/servers/docs/tools", nil, true)
		if code != http.StatusOK {
			t.Fatalf("list mcp tools expected 200, got %d", code)
		}
		items, ok := data["items"].([]any)
		if !ok || len(items) != 1 {
			t.Fatalf("unexpected mcp tools payload: %+v", data)
		}
	})
}
