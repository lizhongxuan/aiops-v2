package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aiops-v2/internal/hostagent"
	"runner/scheduler"
	"runner/workflow"
)

func TestHostAgentHandlerRejectsLegacyActions(t *testing.T) {
	cfg := hostagent.Config{
		ServerURL:         "http://aiops.example.test",
		HostID:            "prod-web-01",
		ListenAddr:        ":7072",
		Token:             "secret-token",
		HeartbeatInterval: time.Second,
		Capabilities:      hostagent.DefaultCapabilities(),
	}
	handler := newAgentHandler(cfg, agentOptions{AsyncThreshold: time.Second, MaxOutputBytes: 4096})

	for _, action := range []string{"cmd.run", "shell.run"} {
		body := runRequest{
			Task: scheduler.Task{
				ID:    "task-" + action,
				RunID: "run-" + action,
				Step:  workflow.Step{Name: "legacy", Action: action, Args: map[string]any{"script": "echo no"}},
			},
		}
		resp := postJSON(t, handler, "/run", "secret-token", body)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("action %s status = %d, want 400; body=%s", action, resp.Code, resp.Body.String())
		}
		if !bytes.Contains(resp.Body.Bytes(), []byte("unsupported action")) {
			t.Fatalf("action %s body = %s, want unsupported action", action, resp.Body.String())
		}
	}
}

func TestHostAgentHandlerRunsScriptShellAndReportsHealth(t *testing.T) {
	cfg := hostagent.Config{
		ServerURL:         "http://aiops.example.test",
		HostID:            "prod-web-01",
		ListenAddr:        ":7072",
		Token:             "secret-token",
		HeartbeatInterval: time.Second,
		Capabilities:      hostagent.DefaultCapabilities(),
	}
	handler := newAgentHandler(cfg, agentOptions{AsyncThreshold: time.Second, MaxOutputBytes: 4096})

	healthReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthResp := httptest.NewRecorder()
	handler.ServeHTTP(healthResp, healthReq)
	if healthResp.Code != http.StatusOK {
		t.Fatalf("GET /health status = %d, want 200; body=%s", healthResp.Code, healthResp.Body.String())
	}

	body := runRequest{
		Task: scheduler.Task{
			ID:    "task-script",
			RunID: "run-script",
			Step: workflow.Step{
				Name:   "script",
				Action: "script.shell",
				Args:   map[string]any{"script": "printf host-agent-ok"},
			},
		},
	}
	resp := postJSON(t, handler, "/run", "secret-token", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /run status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	var payload runResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Result.Status != "success" {
		t.Fatalf("status = %q, want success; payload=%+v", payload.Result.Status, payload)
	}
	if payload.Result.Output["stdout"] != "host-agent-ok" {
		t.Fatalf("stdout = %#v, want host-agent-ok", payload.Result.Output["stdout"])
	}
}

func TestHostAgentGRPCExecRunsLocalCommand(t *testing.T) {
	result := runLocalExecCommand(context.Background(), agentExecRequest{
		Command: "printf",
		Args:    []string{"grpc-agent-ok"},
	}, 4096)

	if result.Status != "success" || result.ExitCode != 0 {
		t.Fatalf("result = %#v, want success", result)
	}
	if result.Stdout != "grpc-agent-ok" {
		t.Fatalf("stdout = %q, want grpc-agent-ok", result.Stdout)
	}
}

func TestHostAgentHandlerExecRunsDirectCommand(t *testing.T) {
	cfg := hostagent.Config{
		ServerURL:         "http://aiops.example.test",
		HostID:            "prod-web-01",
		ListenAddr:        ":7072",
		Token:             "secret-token",
		HeartbeatInterval: time.Second,
		Capabilities:      hostagent.DefaultCapabilities(),
	}
	handler := newAgentHandler(cfg, agentOptions{AsyncThreshold: time.Second, MaxOutputBytes: 4096})

	resp := postJSON(t, handler, "/exec", "secret-token", agentExecRequest{
		Command: "printf",
		Args:    []string{"http-agent-ok"},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /exec status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	var payload agentExecResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != "success" || payload.ExitCode != 0 || payload.Stdout != "http-agent-ok" {
		t.Fatalf("payload = %#v, want direct exec success", payload)
	}
}

func TestBuildHostAgentEventPayloadIncludesSystemBasics(t *testing.T) {
	payload := buildHostAgentEventPayload(hostAgentEventPayloadInput{
		HostID:        "prod-web-01",
		Hostname:      "node-a",
		ListenAddress: ":7072",
		Capabilities:  []string{"script.shell", "terminal"},
		Labels:        map[string]string{"role": "web"},
		System: hostSystemInfo{
			OS:            "linux",
			Arch:          "amd64",
			OSRelease:     "Ubuntu 24.04 LTS",
			KernelVersion: "6.8.0-31-generic",
			CPUCores:      8,
			MemoryBytes:   34359738368,
		},
		Registration: true,
	})

	if payload["hostId"] != "prod-web-01" || payload["hostname"] != "node-a" || payload["listenAddress"] != ":7072" {
		t.Fatalf("registration identity payload = %+v", payload)
	}
	if payload["os"] != "linux" || payload["arch"] != "amd64" || payload["osRelease"] != "Ubuntu 24.04 LTS" || payload["kernelVersion"] != "6.8.0-31-generic" {
		t.Fatalf("system identity payload = %+v", payload)
	}
	if payload["cpuCores"] != 8 || payload["memoryBytes"] != uint64(34359738368) {
		t.Fatalf("resource identity payload = %+v", payload)
	}
}

func TestParseLinuxMeminfoBytes(t *testing.T) {
	got := parseLinuxMeminfoBytes("MemTotal:       32768000 kB\nMemFree: 1 kB\n")
	if got != 33554432000 {
		t.Fatalf("parseLinuxMeminfoBytes() = %d, want 33554432000", got)
	}
}

func postJSON(t *testing.T, handler http.Handler, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}
