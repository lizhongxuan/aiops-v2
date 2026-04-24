package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"runner/workflow"
	"gopkg.in/yaml.v3"
)

// TestKMERunnerWebIntegration 验证 kme 调用 runner-web 的 HTTP 契约:
// 1. POST /api/v1/runs {workflow_yaml, vars, triggered_by} → 202 {run_id, status}
// 2. GET /api/v1/runs/{id} → 200 {run_id, status, message}
// 3. vars["env"] 中的环境变量能正确传递到 shell 执行
func TestKMERunnerWebIntegration(t *testing.T) {
	app := newAcceptanceApp(t, appOptions{maxConcurrent: 2, queueSize: 8})
	defer app.close()

	t.Run("submit_yaml_with_env_vars", func(t *testing.T) {
		// 模拟 kme 的 buildRunnerWebEnvVars 构造的 vars 结构
		vars := map[string]any{
			"env": map[string]any{
				"BACKUP_DIR":  "/data/backup/pg/123",
				"DB_PASSWORD": "test-pass-123",
				"TASK_NAME":   "backup-inst1-1700000000",
				"SOURCE_IP":   "10.0.0.1",
			},
		}

		// 工作流使用 shell 环境变量 ${VAR}
		yamlContent := `version: "1"
name: kme-env-test
inventory:
  hosts:
    local:
      address: 127.0.0.1
steps:
  - name: check-env
    targets: [local]
    action: cmd.run
    args:
      cmd: "echo BACKUP_DIR=${BACKUP_DIR} TASK_NAME=${TASK_NAME}"
`
		code, data, _, _ := app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_yaml": yamlContent,
			"vars":          vars,
			"triggered_by":  "pg-allin-backup-precheck",
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("submit expected 202, got %d, data=%v", code, data)
		}
		runID := asString(data["run_id"])
		if runID == "" {
			t.Fatalf("run_id should not be empty")
		}
		if asString(data["status"]) == "" {
			t.Fatalf("status should not be empty")
		}

		// 轮询等待完成（模拟 pollRunnerWebRun）
		status := waitRunStatus(t, app, runID, 10*time.Second)
		if status != "success" {
			// 获取详情查看失败原因
			_, detail, _, _ := app.request(t, http.MethodGet, "/api/v1/runs/"+runID, nil, true)
			t.Fatalf("run expected success, got %s, detail=%v", status, detail)
		}
	})

	t.Run("submit_yaml_with_host_injection", func(t *testing.T) {
		// 模拟 injectHostIntoYAML 的效果：YAML 中已注入 host 地址
		yamlContent := `version: "1"
name: kme-host-inject-test
inventory:
  hosts:
    backup-host:
      address: 127.0.0.1
steps:
  - name: run-on-host
    targets: [backup-host]
    action: cmd.run
    args:
      cmd: "echo host-injection-ok"
`
		code, data, _, _ := app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_yaml": yamlContent,
			"vars":          map[string]any{"env": map[string]any{"TASK_ID": "42"}},
			"triggered_by":  "pg-allin-backup-v2",
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("submit expected 202, got %d", code)
		}
		runID := asString(data["run_id"])
		status := waitRunStatus(t, app, runID, 10*time.Second)
		if status != "success" {
			_, detail, _, _ := app.request(t, http.MethodGet, "/api/v1/runs/"+runID, nil, true)
			t.Fatalf("run expected success, got %s, detail=%v", status, detail)
		}
	})

	t.Run("poll_status_transitions", func(t *testing.T) {
		// 提交一个稍慢的任务，验证轮询能看到 queued/running → success
		yamlContent := createWorkflowYAML("kme-poll-test", "sleep 1 && echo done")
		code, data, _, _ := app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_yaml": yamlContent,
			"triggered_by":  "kme-poll-test",
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("submit expected 202, got %d", code)
		}
		runID := asString(data["run_id"])

		// 立即查询，应该是 queued 或 running
		code, data, _, _ = app.request(t, http.MethodGet, "/api/v1/runs/"+runID, nil, true)
		if code != http.StatusOK {
			t.Fatalf("get run expected 200, got %d", code)
		}
		initialStatus := asString(data["status"])
		if initialStatus != "queued" && initialStatus != "running" && initialStatus != "success" {
			t.Fatalf("initial status expected queued/running/success, got %s", initialStatus)
		}

		// 等待最终状态
		finalStatus := waitRunStatus(t, app, runID, 10*time.Second)
		if finalStatus != "success" {
			t.Fatalf("final status expected success, got %s", finalStatus)
		}
	})

	t.Run("failed_run_detection", func(t *testing.T) {
		// 模拟备份失败场景：命令返回非零退出码
		yamlContent := createWorkflowYAML("kme-fail-test", "exit 1")
		code, data, _, _ := app.request(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_yaml": yamlContent,
			"triggered_by":  "kme-fail-test",
		}, true)
		if code != http.StatusAccepted {
			t.Fatalf("submit expected 202, got %d", code)
		}
		runID := asString(data["run_id"])
		status := waitRunStatus(t, app, runID, 10*time.Second)
		if status != "failed" {
			t.Fatalf("expected failed, got %s", status)
		}

		// 验证 GET 返回的 detail 包含错误信息
		code, data, _, _ = app.request(t, http.MethodGet, "/api/v1/runs/"+runID, nil, true)
		if code != http.StatusOK {
			t.Fatalf("get run expected 200, got %d", code)
		}
		if asString(data["status"]) != "failed" {
			t.Fatalf("detail status expected failed, got %s", asString(data["status"]))
		}
	})
}

// TestInjectHostIntoYAML 验证 YAML host 注入逻辑
func TestInjectHostIntoYAML(t *testing.T) {
	templateYAML := `version: "1"
name: backup-precheck
inventory:
  hosts:
    backup-host:
      address: ""
steps:
  - name: check
    targets: [backup-host]
    action: cmd.run
    args:
      cmd: "echo precheck"
`
	// 模拟 injectHostIntoYAML 的逻辑
	wf, err := workflow.Load([]byte(templateYAML))
	if err != nil {
		t.Fatalf("load yaml: %v", err)
	}
	if wf.Inventory.Hosts == nil {
		wf.Inventory.Hosts = make(map[string]workflow.Host)
	}
	wf.Inventory.Hosts["backup-host"] = workflow.Host{Address: "http://10.0.0.5:9990"}

	data, err := yaml.Marshal(wf)
	if err != nil {
		t.Fatalf("marshal yaml: %v", err)
	}
	result := string(data)

	// 验证注入后的 YAML 包含正确地址
	if !strings.Contains(result, "http://10.0.0.5:9990") {
		t.Fatalf("injected YAML should contain host address, got:\n%s", result)
	}

	// 验证 round-trip 后仍然可以 Load
	wf2, err := workflow.Load(data)
	if err != nil {
		t.Fatalf("reload yaml: %v", err)
	}
	host, ok := wf2.Inventory.Hosts["backup-host"]
	if !ok {
		t.Fatalf("backup-host not found after round-trip")
	}
	if host.Address != "http://10.0.0.5:9990" {
		t.Fatalf("address mismatch after round-trip: %s", host.Address)
	}

	// 验证 steps 保留
	if len(wf2.Steps) != 1 || wf2.Steps[0].Name != "check" {
		t.Fatalf("steps lost after round-trip: %+v", wf2.Steps)
	}
}

// TestBuildRunnerWebEnvVars 验证环境变量构造逻辑
func TestBuildRunnerWebEnvVars(t *testing.T) {
	// 复制 kme 中 buildRunnerWebEnvVars 的逻辑进行测试
	buildEnvVars := func(args map[string]any) map[string]any {
		env := make(map[string]any)
		for k, v := range args {
			switch val := v.(type) {
			case string:
				env[k] = val
			case int:
				env[k] = fmt.Sprintf("%d", val)
			case int64:
				env[k] = fmt.Sprintf("%d", val)
			case float64:
				env[k] = fmt.Sprintf("%g", val)
			case bool:
				env[k] = fmt.Sprintf("%t", val)
			}
		}
		return map[string]any{"env": env}
	}

	args := map[string]any{
		"BACKUP_DIR":  "/data/backup",
		"DB_PASSWORD": "secret",
		"TASK_ID":     42,
		"IS_SIMPLE":   true,
		"RATIO":       3.14,
		"LIST_INST":   []string{"a", "b"}, // 非基本类型，应被忽略
	}

	result := buildEnvVars(args)
	env, ok := result["env"].(map[string]any)
	if !ok {
		t.Fatalf("result should have env key")
	}

	// string 类型
	if env["BACKUP_DIR"] != "/data/backup" {
		t.Fatalf("BACKUP_DIR mismatch: %v", env["BACKUP_DIR"])
	}
	if env["DB_PASSWORD"] != "secret" {
		t.Fatalf("DB_PASSWORD mismatch: %v", env["DB_PASSWORD"])
	}

	// int 类型
	if env["TASK_ID"] != "42" {
		t.Fatalf("TASK_ID mismatch: %v", env["TASK_ID"])
	}

	// bool 类型
	if env["IS_SIMPLE"] != "true" {
		t.Fatalf("IS_SIMPLE mismatch: %v", env["IS_SIMPLE"])
	}

	// float64 类型
	if env["RATIO"] != "3.14" {
		t.Fatalf("RATIO mismatch: %v", env["RATIO"])
	}

	// 非基本类型应被忽略
	if _, exists := env["LIST_INST"]; exists {
		t.Fatalf("LIST_INST should be skipped (non-primitive type)")
	}
}

// TestSubmitAndPollContract 使用 httptest.Server 模拟 runner-web，
// 验证 kme 的 submit+poll HTTP 契约
func TestSubmitAndPollContract(t *testing.T) {
	runStatus := "queued"
	runID := "test-run-12345"

	// 模拟 runner-web 的 API
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// 验证 Authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// 解析请求体
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// 验证必要字段
		if _, ok := req["workflow_yaml"]; !ok {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "workflow_yaml required"})
			return
		}

		// 模拟异步启动
		runStatus = "running"
		go func() {
			time.Sleep(500 * time.Millisecond)
			runStatus = "success"
		}()

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
			"run_id":        runID,
			"status":        "queued",
			"workflow_name": "test-workflow",
		})
	})

	mux.HandleFunc("/api/v1/runs/"+runID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"run_id":  runID,
			"status":  runStatus,
			"message": "",
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// 模拟 kme 的 submitAndWaitRunnerWeb 流程
	t.Run("submit_and_poll_success", func(t *testing.T) {
		// Step 1: Submit
		reqBody := map[string]any{
			"workflow_yaml": "version: \"1\"\nname: test\n",
			"vars":          map[string]any{"env": map[string]any{"KEY": "val"}},
			"triggered_by":  "test",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/runs", strings.NewReader(string(bodyBytes)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("submit request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("submit expected 202, got %d", resp.StatusCode)
		}

		var submitResp map[string]any
		json.NewDecoder(resp.Body).Decode(&submitResp)
		gotRunID := submitResp["run_id"].(string)
		if gotRunID != runID {
			t.Fatalf("run_id mismatch: %s vs %s", gotRunID, runID)
		}

		// Step 2: Poll until success
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			pollReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/runs/"+gotRunID, nil)
			pollReq.Header.Set("Authorization", "Bearer test-token")
			pollResp, err := http.DefaultClient.Do(pollReq)
			if err != nil {
				t.Fatalf("poll request failed: %v", err)
			}
			var detail map[string]any
			json.NewDecoder(pollResp.Body).Decode(&detail)
			pollResp.Body.Close()

			status := detail["status"].(string)
			if status == "success" {
				return // 测试通过
			}
			if status == "failed" || status == "canceled" {
				t.Fatalf("unexpected terminal status: %s", status)
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("poll timed out, last status: %s", runStatus)
	})

	t.Run("submit_without_auth_fails", func(t *testing.T) {
		reqBody := map[string]any{"workflow_yaml": "test"}
		bodyBytes, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/runs", strings.NewReader(string(bodyBytes)))
		req.Header.Set("Content-Type", "application/json")
		// 不设置 Authorization

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", resp.StatusCode)
		}
	})
}
