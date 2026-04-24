package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func requireListenOrSkip(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skip live integration test: cannot listen on local port: %v", err)
		return
	}
	_ = ln.Close()
}

type liveResp struct {
	status int
	data   map[string]any
	raw    string
}

func liveRequest(t *testing.T, client *http.Client, baseURL, token, method, path string, body any, auth bool) liveResp {
	t.Helper()
	var reader io.Reader
	if body != nil {
		switch v := body.(type) {
		case string:
			reader = strings.NewReader(v)
		case []byte:
			reader = bytes.NewReader(v)
		default:
			payload, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("marshal live request: %v", err)
			}
			reader = bytes.NewReader(payload)
		}
	}
	req, err := http.NewRequest(method, strings.TrimRight(baseURL, "/")+path, reader)
	if err != nil {
		t.Fatalf("new live request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do live request %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	rawBytes, _ := io.ReadAll(resp.Body)
	raw := string(rawBytes)
	data := map[string]any{}
	_ = json.Unmarshal(rawBytes, &data)
	return liveResp{
		status: resp.StatusCode,
		data:   data,
		raw:    raw,
	}
}

func waitLiveRunStatus(t *testing.T, client *http.Client, baseURL, token, runID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		res := liveRequest(t, client, baseURL, token, http.MethodGet, "/api/v1/runs/"+runID, nil, true)
		if res.status == http.StatusOK {
			status := asString(res.data["status"])
			switch status {
			case "success", "failed", "canceled", "interrupted":
				return status
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("wait live run status timeout for run %s", runID)
	return ""
}

func TestAcceptanceChecklistLive(t *testing.T) {
	requireListenOrSkip(t)

	app := newAcceptanceApp(t, appOptions{
		maxConcurrent: 2,
		queueSize:     32,
		token:         "live-token",
	})
	defer app.close()

	runnerSrv := newHTTPTestServerOrSkip(t, app.router)
	defer runnerSrv.Close()
	client := &http.Client{Timeout: 10 * time.Second}

	t.Run("F01 live 服务监听与优雅关闭", func(t *testing.T) {
		res := liveRequest(t, client, runnerSrv.URL, app.token, http.MethodGet, "/healthz", nil, false)
		if res.status != http.StatusOK {
			t.Fatalf("healthz expected 200, got %d", res.status)
		}

		tempSrv := newHTTPTestServerOrSkip(t, app.router)
		tempURL := tempSrv.URL
		tempSrv.Close()
		req, _ := http.NewRequest(http.MethodGet, tempURL+"/healthz", nil)
		req.Header.Set("Authorization", "Bearer "+app.token)
		_, err := client.Do(req)
		if err == nil {
			t.Fatalf("expected request failure after server close")
		}
	})

	t.Run("F05/F06 live Agent token透传 + Probe成功", func(t *testing.T) {
		var (
			mu       sync.Mutex
			runAuth  string
			runToken string
			runCalls int
		)
		agentSrv := newHTTPTestServerOrSkip(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/health":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"status":"ok"}`))
				return
			case "/heartbeat":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"status":"ok"}`))
				return
			case "/run":
				mu.Lock()
				runCalls++
				runAuth = strings.TrimSpace(r.Header.Get("Authorization"))
				runToken = strings.TrimSpace(r.Header.Get("X-Runner-Token"))
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"result":{"task_id":"agent-task-live","status":"success","output":{"stdout":"live-ok","stderr":""}}}`))
				return
			default:
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not found"}`))
			}
		}))
		defer agentSrv.Close()

		res := liveRequest(t, client, runnerSrv.URL, app.token, http.MethodPost, "/api/v1/agents", map[string]any{
			"id":           "agent-live",
			"name":         "agent-live",
			"address":      agentSrv.URL,
			"token":        "agent-live-token",
			"tags":         []string{"live"},
			"capabilities": []string{"cmd.run"},
		}, true)
		if res.status != http.StatusCreated {
			t.Fatalf("create live agent expected 201, got %d", res.status)
		}

		res = liveRequest(t, client, runnerSrv.URL, app.token, http.MethodPost, "/api/v1/agents/agent-live/probe", nil, true)
		if res.status != http.StatusOK {
			t.Fatalf("probe success expected 200, got %d", res.status)
		}

		wfYAML := `version: "1"
name: wf-live-agent
inventory:
  hosts:
    remote1:
      address: agent://agent-live
steps:
  - name: run
    targets: [remote1]
    action: cmd.run
    args:
      cmd: "echo hi"
`
		res = liveRequest(t, client, runnerSrv.URL, app.token, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_yaml": wfYAML,
		}, true)
		if res.status != http.StatusAccepted {
			t.Fatalf("submit live agent run expected 202, got %d", res.status)
		}
		runID := asString(res.data["run_id"])
		if status := waitLiveRunStatus(t, client, runnerSrv.URL, app.token, runID, 8*time.Second); status != "success" {
			t.Fatalf("live agent run expected success, got %s", status)
		}

		mu.Lock()
		calls := runCalls
		auth := runAuth
		token := runToken
		mu.Unlock()
		if calls < 1 {
			t.Fatalf("expected remote /run to be called")
		}
		if auth != "Bearer agent-live-token" {
			t.Fatalf("authorization header mismatch: %s", auth)
		}
		if token != "agent-live-token" {
			t.Fatalf("x-runner-token header mismatch: %s", token)
		}
	})

	t.Run("F07 live SSE 长连接稳定性", func(t *testing.T) {
		blockerYAML := `version: "1"
name: wf-live-sse-blocker
inventory:
  hosts:
    local:
      address: 127.0.0.1
steps:
  - name: run
    targets: [local]
    action: cmd.run
    args:
      cmd: "sleep 2"
`
		res := liveRequest(t, client, runnerSrv.URL, app.token, http.MethodPost, "/api/v1/workflows", map[string]any{
			"name": "wf-live-sse-blocker",
			"yaml": blockerYAML,
		}, true)
		if res.status != http.StatusCreated {
			t.Fatalf("create sse blocker workflow expected 201, got %d", res.status)
		}
		for i := 0; i < 2; i++ {
			res = liveRequest(t, client, runnerSrv.URL, app.token, http.MethodPost, "/api/v1/runs", map[string]any{
				"workflow_name": "wf-live-sse-blocker",
			}, true)
			if res.status != http.StatusAccepted {
				t.Fatalf("submit sse blocker run expected 202, got %d", res.status)
			}
		}

		targetYAML := `version: "1"
name: wf-live-sse-target
inventory:
  hosts:
    local:
      address: 127.0.0.1
steps:
  - name: run
    targets: [local]
    action: cmd.run
    args:
      cmd: "echo sse-target"
`
		res = liveRequest(t, client, runnerSrv.URL, app.token, http.MethodPost, "/api/v1/runs", map[string]any{
			"workflow_yaml": targetYAML,
		}, true)
		if res.status != http.StatusAccepted {
			t.Fatalf("submit live sse run expected 202, got %d", res.status)
		}
		runID := asString(res.data["run_id"])

		sseClient := &http.Client{Timeout: 20 * time.Second}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, runnerSrv.URL+"/api/v1/runs/"+runID+"/events", nil)
		if err != nil {
			t.Fatalf("new sse request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+app.token)
		resp, err := sseClient.Do(req)
		if err != nil {
			t.Fatalf("open sse stream: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("sse stream expected 200, got %d", resp.StatusCode)
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		seen := map[string]bool{}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if !scanner.Scan() {
				break
			}
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			event := map[string]any{}
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				continue
			}
			typ := asString(event["type"])
			if typ != "" {
				seen[typ] = true
			}
			if seen["run_start"] && seen["step_start"] && seen["host_result"] && seen["run_finish"] {
				break
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatalf("sse scanner error: %v", err)
		}
		for _, evt := range []string{"run_start", "step_start", "host_result", "run_finish"} {
			if !seen[evt] {
				t.Fatalf("sse stream missing %s", evt)
			}
		}
		if status := waitLiveRunStatus(t, client, runnerSrv.URL, app.token, runID, 10*time.Second); status != "success" {
			t.Fatalf("sse target run expected success, got %s", status)
		}
	})

	t.Run("F08 live 并发上限压力验证", func(t *testing.T) {
		wfYAML := `version: "1"
name: wf-live-concurrency
inventory:
  hosts:
    local:
      address: 127.0.0.1
steps:
  - name: run
    targets: [local]
    action: cmd.run
    args:
      cmd: "sleep 1"
`
		res := liveRequest(t, client, runnerSrv.URL, app.token, http.MethodPost, "/api/v1/workflows", map[string]any{
			"name": "wf-live-concurrency",
			"yaml": wfYAML,
		}, true)
		if res.status != http.StatusCreated {
			t.Fatalf("create concurrency workflow expected 201, got %d", res.status)
		}

		totalRuns := 8
		runIDs := make([]string, 0, totalRuns)
		for i := 0; i < totalRuns; i++ {
			res = liveRequest(t, client, runnerSrv.URL, app.token, http.MethodPost, "/api/v1/runs", map[string]any{
				"workflow_name": "wf-live-concurrency",
			}, true)
			if res.status != http.StatusAccepted {
				t.Fatalf("submit concurrency run expected 202, got %d", res.status)
			}
			runIDs = append(runIDs, asString(res.data["run_id"]))
		}

		maxRunning := 0
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			res = liveRequest(t, client, runnerSrv.URL, app.token, http.MethodGet, "/api/v1/runs?status=running&limit=100", nil, true)
			if res.status != http.StatusOK {
				t.Fatalf("list running runs expected 200, got %d", res.status)
			}
			items, _ := res.data["items"].([]any)
			if len(items) > maxRunning {
				maxRunning = len(items)
			}

			doneCount := 0
			for _, runID := range runIDs {
				statusRes := liveRequest(t, client, runnerSrv.URL, app.token, http.MethodGet, "/api/v1/runs/"+runID, nil, true)
				if statusRes.status != http.StatusOK {
					continue
				}
				status := asString(statusRes.data["status"])
				if status == "success" || status == "failed" || status == "canceled" || status == "interrupted" {
					doneCount++
				}
			}
			if doneCount == len(runIDs) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if maxRunning > 2 {
			t.Fatalf("max running expected <=2, got %d", maxRunning)
		}
		for _, runID := range runIDs {
			status := waitLiveRunStatus(t, client, runnerSrv.URL, app.token, runID, 10*time.Second)
			if status != "success" {
				t.Fatalf("concurrency run %s expected success, got %s", runID, status)
			}
		}
	})
}
