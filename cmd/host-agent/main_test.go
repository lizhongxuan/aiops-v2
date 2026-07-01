package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRunHostAgentStartsHTTPServerWhenInitialRegisterFails(t *testing.T) {
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not registered", http.StatusNotFound)
	}))
	defer control.Close()

	cfg := hostagent.Config{
		ServerURL:         control.URL,
		HostID:            "prod-web-01",
		ListenAddr:        "127.0.0.1:0",
		Token:             "secret-token",
		HeartbeatInterval: time.Hour,
		Capabilities:      hostagent.DefaultCapabilities(),
	}
	var stderr bytes.Buffer
	serveStopped := errors.New("server stopped")
	served := false
	err := runHostAgent(
		context.Background(),
		cfg,
		agentOptions{AsyncThreshold: time.Second, MaxOutputBytes: 4096},
		&http.Client{Timeout: time.Second},
		&stderr,
		func(handler http.Handler) error {
			served = true
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				t.Fatalf("GET /health status = %d, want 200; body=%s", resp.Code, resp.Body.String())
			}
			return serveStopped
		},
	)
	if !errors.Is(err, serveStopped) {
		t.Fatalf("runHostAgent() error = %v, want serve sentinel", err)
	}
	if !served {
		t.Fatal("serve function was not called after register failure")
	}
	if !strings.Contains(stderr.String(), "register host-agent") {
		t.Fatalf("stderr = %q, want register failure log", stderr.String())
	}
}

func TestRunHostAgentAIOPSPullSkipsPushRegistration(t *testing.T) {
	cfg := hostagent.Config{
		ConnectionMode:    hostagent.ConnectionModeAIOPSPull,
		HostID:            "prod-web-01",
		ListenAddr:        "127.0.0.1:0",
		Token:             "secret-token",
		HeartbeatInterval: time.Hour,
		Capabilities:      hostagent.DefaultCapabilities(),
	}
	var stderr bytes.Buffer
	serveStopped := errors.New("server stopped")
	served := false
	err := runHostAgent(
		context.Background(),
		cfg,
		agentOptions{AsyncThreshold: time.Second, MaxOutputBytes: 4096},
		&http.Client{Timeout: time.Second},
		&stderr,
		func(handler http.Handler) error {
			served = true
			req := httptest.NewRequest(http.MethodGet, "/diagnostics", nil)
			req.Header.Set("Authorization", "Bearer secret-token")
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				t.Fatalf("GET /diagnostics status = %d, want 200; body=%s", resp.Code, resp.Body.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode diagnostics: %v", err)
			}
			if payload["connection_mode"] != hostagent.ConnectionModeAIOPSPull {
				t.Fatalf("diagnostics = %+v, want connection_mode aiops_pull", payload)
			}
			if _, ok := payload["server_url"]; ok {
				t.Fatalf("diagnostics should not expose empty server_url in pull mode: %+v", payload)
			}
			return serveStopped
		},
	)
	if !errors.Is(err, serveStopped) {
		t.Fatalf("runHostAgent() error = %v, want serve sentinel", err)
	}
	if !served {
		t.Fatal("serve function was not called")
	}
	if strings.Contains(stderr.String(), "register host-agent") {
		t.Fatalf("stderr = %q, want no push register attempt in aiops_pull mode", stderr.String())
	}
}

func TestHostAgentDiagnosticsReportsInitialRegisterFailure(t *testing.T) {
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not registered", http.StatusNotFound)
	}))
	defer control.Close()

	cfg := hostagent.Config{
		ServerURL:         control.URL,
		GRPCURL:           "127.0.0.1:1",
		HostID:            "prod-web-01",
		ListenAddr:        "127.0.0.1:0",
		Token:             "secret-token",
		HeartbeatInterval: time.Hour,
		Capabilities:      hostagent.DefaultCapabilities(),
	}
	serveStopped := errors.New("server stopped")
	err := runHostAgent(
		context.Background(),
		cfg,
		agentOptions{AsyncThreshold: time.Second, MaxOutputBytes: 4096},
		&http.Client{Timeout: time.Second},
		io.Discard,
		func(handler http.Handler) error {
			req := httptest.NewRequest(http.MethodGet, "/diagnostics", nil)
			req.Header.Set("Authorization", "Bearer secret-token")
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				t.Fatalf("GET /diagnostics status = %d, want 200; body=%s", resp.Code, resp.Body.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode diagnostics: %v", err)
			}
			if payload["host_id"] != "prod-web-01" || payload["server_url"] != control.URL || payload["token_configured"] != true {
				t.Fatalf("diagnostics identity = %+v", payload)
			}
			if _, ok := payload["token"]; ok {
				t.Fatalf("diagnostics leaked token: %+v", payload)
			}
			register, ok := payload["register"].(map[string]any)
			if !ok {
				t.Fatalf("register diagnostics = %#v, want object", payload["register"])
			}
			if register["last_status_code"] != float64(http.StatusNotFound) || register["last_category"] != "not_found" {
				t.Fatalf("register diagnostics = %+v, want 404 not_found", register)
			}
			if got := strings.TrimSpace(fmt.Sprint(register["last_error"])); !strings.Contains(got, "status 404") {
				t.Fatalf("register last_error = %q, want status 404", got)
			}
			return serveStopped
		},
	)
	if !errors.Is(err, serveStopped) {
		t.Fatalf("runHostAgent() error = %v, want serve sentinel", err)
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
