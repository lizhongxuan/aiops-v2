package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"runner/logging"
	"runner/server/store/agentstore"
)

type testRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn testRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func withCapturedAgentServiceLogs(t *testing.T) (*bytes.Buffer, func()) {
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

func TestAgentServiceProbeSuccessWithoutNetworkBind(t *testing.T) {
	logBuf, restoreLogs := withCapturedAgentServiceLogs(t)
	defer restoreLogs()

	var (
		seenAuthHeader      string
		seenTokenHeader     string
		seenAgentAuthHeader string
	)
	store := agentstore.NewFileStore(filepath.Join(t.TempDir(), "agents.json"))
	svc := NewAgentService(store, 90)
	svc.httpClient = &http.Client{
		Transport: testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/health" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			seenAuthHeader = strings.TrimSpace(req.Header.Get("Authorization"))
			seenTokenHeader = strings.TrimSpace(req.Header.Get("X-Runner-Token"))
			seenAgentAuthHeader = strings.TrimSpace(req.Header.Get("X-Agent-Auth"))
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"status":"ok"}`)),
				Request:    req,
			}, nil
		}),
	}

	if err := svc.Register(context.Background(), &agentstore.AgentRecord{
		ID:      "agent-probe",
		Name:    "agent-probe",
		Address: "http://agent.mock:7072",
		Token:   "probe-token",
	}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	if err := svc.Probe(context.Background(), "agent-probe"); err != nil {
		t.Fatalf("probe should succeed: %v", err)
	}

	item, err := svc.Get(context.Background(), "agent-probe")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if item.Status != agentstore.StatusOnline {
		t.Fatalf("expected online after probe, got %s", item.Status)
	}
	if seenAuthHeader != "Bearer probe-token" {
		t.Fatalf("unexpected authorization header: %q", seenAuthHeader)
	}
	if seenTokenHeader != "probe-token" {
		t.Fatalf("unexpected x-runner-token header: %q", seenTokenHeader)
	}
	if seenAgentAuthHeader != "probe-token" {
		t.Fatalf("unexpected x-agent-auth header: %q", seenAgentAuthHeader)
	}
	logs := logBuf.String()
	if !strings.Contains(logs, `"resolved_token":"probe-token"`) {
		t.Fatalf("expected resolved token in logs, got %s", logs)
	}
	if !strings.Contains(logs, `"x_agent_auth":"probe-token"`) {
		t.Fatalf("expected x-agent-auth in logs, got %s", logs)
	}
}
