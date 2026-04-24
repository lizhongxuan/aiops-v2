package scheduler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"runner/logging"
	"runner/workflow"
)

type stubDispatcher struct {
	result Result
	err    error
	calls  int
}

func (s *stubDispatcher) Dispatch(ctx context.Context, task Task) (Result, error) {
	_ = ctx
	_ = task
	s.calls++
	return s.result, s.err
}

func TestHybridDispatcherDispatchLocal(t *testing.T) {
	local := &stubDispatcher{result: Result{Status: "success"}}
	remote := &stubDispatcher{result: Result{Status: "success"}}
	hybrid := NewHybridDispatcherWithRouters(local, remote)

	_, err := hybrid.Dispatch(context.Background(), Task{
		Host: workflow.HostSpec{Address: "local"},
		Step: workflow.Step{Name: "test-step", Action: "shell.run"},
	})
	if err != nil {
		t.Fatalf("dispatch local failed: %v", err)
	}
	if local.calls != 1 {
		t.Fatalf("expected local dispatcher called once, got %d", local.calls)
	}
	if remote.calls != 0 {
		t.Fatalf("expected remote dispatcher not called, got %d", remote.calls)
	}
}

func TestHybridDispatcherDispatchLocalByTargetName(t *testing.T) {
	local := &stubDispatcher{result: Result{Status: "success"}}
	remote := &stubDispatcher{result: Result{Status: "success"}}
	hybrid := NewHybridDispatcherWithRouters(local, remote)

	_, err := hybrid.Dispatch(context.Background(), Task{
		Host: workflow.HostSpec{Name: "local"},
		Step: workflow.Step{Name: "test-step", Action: "shell.run"},
	})
	if err != nil {
		t.Fatalf("dispatch local by target name failed: %v", err)
	}
	if local.calls != 1 {
		t.Fatalf("expected local dispatcher called once, got %d", local.calls)
	}
	if remote.calls != 0 {
		t.Fatalf("expected remote dispatcher not called, got %d", remote.calls)
	}
}

func TestHybridDispatcherDispatchRemote(t *testing.T) {
	local := &stubDispatcher{result: Result{Status: "success"}}
	remote := &stubDispatcher{result: Result{Status: "success"}}
	hybrid := NewHybridDispatcherWithRouters(local, remote)

	_, err := hybrid.Dispatch(context.Background(), Task{
		Host: workflow.HostSpec{Address: "http://127.0.0.1:7072"},
		Step: workflow.Step{Name: "test-step", Action: "shell.run"},
	})
	if err != nil {
		t.Fatalf("dispatch remote failed: %v", err)
	}
	if local.calls != 0 {
		t.Fatalf("expected local dispatcher not called, got %d", local.calls)
	}
	if remote.calls != 1 {
		t.Fatalf("expected remote dispatcher called once, got %d", remote.calls)
	}
}

func TestHybridDispatcherRemoteNil(t *testing.T) {
	local := &stubDispatcher{result: Result{Status: "success"}}
	hybrid := NewHybridDispatcherWithRouters(local, nil)

	_, err := hybrid.Dispatch(context.Background(), Task{
		Host: workflow.HostSpec{Address: "https://127.0.0.1:7072"},
		Step: workflow.Step{Name: "test-step", Action: "shell.run"},
	})
	if err == nil {
		t.Fatalf("expected error when remote dispatcher is nil")
	}
}

func TestHybridDispatcherLocalNil(t *testing.T) {
	remote := &stubDispatcher{result: Result{Status: "success"}}
	hybrid := NewHybridDispatcherWithRouters(nil, remote)

	_, err := hybrid.Dispatch(context.Background(), Task{
		Host: workflow.HostSpec{Address: "127.0.0.1"},
		Step: workflow.Step{Name: "test-step", Action: "shell.run"},
	})
	if err == nil {
		t.Fatalf("expected error when local dispatcher is nil")
	}
}

func TestHybridDispatcherPropagatesError(t *testing.T) {
	remoteErr := errors.New("remote failed")
	local := &stubDispatcher{result: Result{Status: "success"}}
	remote := &stubDispatcher{result: Result{Status: "failed"}, err: remoteErr}
	hybrid := NewHybridDispatcherWithRouters(local, remote)

	_, err := hybrid.Dispatch(context.Background(), Task{
		Host: workflow.HostSpec{Address: "http://127.0.0.1:7072"},
		Step: workflow.Step{Name: "test-step", Action: "shell.run"},
	})
	if err == nil {
		t.Fatalf("expected dispatch error")
	}
	if !errors.Is(err, remoteErr) {
		t.Fatalf("expected remote error, got %v", err)
	}
}

func TestHybridDispatcherDispatchRemoteByPlainIP(t *testing.T) {
	local := &stubDispatcher{result: Result{Status: "success"}}
	remote := &stubDispatcher{result: Result{Status: "success"}}
	hybrid := NewHybridDispatcherWithRouters(local, remote)

	_, err := hybrid.Dispatch(context.Background(), Task{
		Host: workflow.HostSpec{Address: "192.25.0.1"},
		Step: workflow.Step{Name: "test-step", Action: "shell.run"},
	})
	if err != nil {
		t.Fatalf("dispatch remote by plain ip failed: %v", err)
	}
	if local.calls != 0 {
		t.Fatalf("expected local dispatcher not called, got %d", local.calls)
	}
	if remote.calls != 1 {
		t.Fatalf("expected remote dispatcher called once, got %d", remote.calls)
	}
}

func TestHybridDispatcherResolveBaseURLDefaultsHTTP(t *testing.T) {
	hybrid := NewHybridDispatcherWithRouters(nil, nil)

	got := hybrid.resolveBaseURL(Task{
		Host: workflow.HostSpec{Address: "192.25.0.1:17072"},
	})
	if got != "http://192.25.0.1:17072" {
		t.Fatalf("unexpected base url %q", got)
	}

	hybrid.BaseURL = "10.0.0.8:17072"
	got = hybrid.resolveBaseURL(Task{})
	if got != "http://10.0.0.8:17072" {
		t.Fatalf("unexpected default base url %q", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func withCapturedSchedulerLogs(t *testing.T) (*bytes.Buffer, func()) {
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

func TestHybridDispatcherInjectsConfiguredRunnerAgentTokenHeaders(t *testing.T) {
	var (
		seenAuthHeader      string
		seenTokenHeader     string
		seenAgentAuthHeader string
	)
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seenAuthHeader = strings.TrimSpace(req.Header.Get("Authorization"))
			seenTokenHeader = strings.TrimSpace(req.Header.Get("X-Runner-Token"))
			seenAgentAuthHeader = strings.TrimSpace(req.Header.Get("X-Agent-Auth"))
			respBody := `{"result":{"task_id":"remote-task","status":"success","output":{"stdout":"ok","stderr":""}}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(respBody)),
				Request:    req,
			}, nil
		}),
	}

	dispatcher := NewHybridDispatcher(nil)
	dispatcher.Client = client
	dispatcher.Token = "agent-secret-token"

	result, err := dispatcher.Dispatch(context.Background(), Task{
		ID: "task-1",
		Host: workflow.HostSpec{
			Name:    "remote1",
			Address: "http://agent.mock:7072",
		},
		Step: workflow.Step{
			Name:   "step-1",
			Action: "cmd.run",
			Args: map[string]any{
				"cmd": "echo ok",
			},
		},
	})
	if err != nil {
		t.Fatalf("dispatch remote failed: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if seenAuthHeader != "Bearer agent-secret-token" {
		t.Fatalf("unexpected authorization header: %q", seenAuthHeader)
	}
	if seenTokenHeader != "agent-secret-token" {
		t.Fatalf("unexpected x-runner-token header: %q", seenTokenHeader)
	}
	if seenAgentAuthHeader != "agent-secret-token" {
		t.Fatalf("unexpected x-agent-auth header: %q", seenAgentAuthHeader)
	}
}

func TestHybridDispatcherInjectsHostRunnerAgentTokenHeaders(t *testing.T) {
	var (
		seenAuthHeader      string
		seenTokenHeader     string
		seenAgentAuthHeader string
	)
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seenAuthHeader = strings.TrimSpace(req.Header.Get("Authorization"))
			seenTokenHeader = strings.TrimSpace(req.Header.Get("X-Runner-Token"))
			seenAgentAuthHeader = strings.TrimSpace(req.Header.Get("X-Agent-Auth"))
			respBody := `{"result":{"task_id":"remote-task","status":"success","output":{"stdout":"ok","stderr":""}}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(respBody)),
				Request:    req,
			}, nil
		}),
	}

	dispatcher := NewHybridDispatcher(nil)
	dispatcher.Client = client
	dispatcher.Token = "dispatcher-default-token"

	result, err := dispatcher.Dispatch(context.Background(), Task{
		ID: "task-env-1",
		Host: workflow.HostSpec{
			Name:    "remote1",
			Address: "http://agent.mock:7072",
			Vars: map[string]any{
				"RUNNER_AGENT_TOKEN": "agent-secret-token-from-host",
			},
		},
		Step: workflow.Step{
			Name:   "step-env-1",
			Action: "cmd.run",
			Args: map[string]any{
				"cmd": "echo ok",
			},
		},
	})
	if err != nil {
		t.Fatalf("dispatch remote failed: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if seenAuthHeader != "Bearer agent-secret-token-from-host" {
		t.Fatalf("unexpected authorization header: %q", seenAuthHeader)
	}
	if seenTokenHeader != "agent-secret-token-from-host" {
		t.Fatalf("unexpected x-runner-token header: %q", seenTokenHeader)
	}
	if seenAgentAuthHeader != "agent-secret-token-from-host" {
		t.Fatalf("unexpected x-agent-auth header: %q", seenAgentAuthHeader)
	}
}

func TestHybridDispatcherIgnoresRunnerAgentTokenFromTaskVars(t *testing.T) {
	var (
		seenAuthHeader      string
		seenTokenHeader     string
		seenAgentAuthHeader string
	)
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seenAuthHeader = strings.TrimSpace(req.Header.Get("Authorization"))
			seenTokenHeader = strings.TrimSpace(req.Header.Get("X-Runner-Token"))
			seenAgentAuthHeader = strings.TrimSpace(req.Header.Get("X-Agent-Auth"))
			respBody := `{"result":{"task_id":"remote-task","status":"success","output":{"stdout":"ok","stderr":""}}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(respBody)),
				Request:    req,
			}, nil
		}),
	}

	dispatcher := NewHybridDispatcher(nil)
	dispatcher.Client = client
	dispatcher.Token = "configured-agent-token"

	result, err := dispatcher.Dispatch(context.Background(), Task{
		ID: "task-vars-1",
		Host: workflow.HostSpec{
			Name:    "remote1",
			Address: "http://agent.mock:7072",
		},
		Step: workflow.Step{
			Name:   "step-vars-1",
			Action: "cmd.run",
			Args: map[string]any{
				"cmd": "echo ok",
			},
		},
		Vars: map[string]any{
			"RUNNER_AGENT_TOKEN": "token-from-run-vars-should-be-ignored",
		},
	})
	if err != nil {
		t.Fatalf("dispatch remote failed: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if seenAuthHeader != "Bearer configured-agent-token" {
		t.Fatalf("unexpected authorization header: %q", seenAuthHeader)
	}
	if seenTokenHeader != "configured-agent-token" {
		t.Fatalf("unexpected x-runner-token header: %q", seenTokenHeader)
	}
	if seenAgentAuthHeader != "configured-agent-token" {
		t.Fatalf("unexpected x-agent-auth header: %q", seenAgentAuthHeader)
	}
}

func TestHybridDispatcherDispatchRemoteStatusFailureReturnsFailedResult(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/run":
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"result":{"task_id":"remote-task","status":"running","output":{"stdout":"partial"}}}`)),
					Request:    req,
				}, nil
			case "/status":
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Status:     "401 Unauthorized",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"error":"runner token mismatch"}`)),
					Request:    req,
				}, nil
			default:
				t.Fatalf("unexpected path %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	dispatcher := NewHybridDispatcher(nil)
	dispatcher.Client = client

	result, err := dispatcher.Dispatch(context.Background(), Task{
		ID:    "task-2",
		RunID: "run-2",
		Host: workflow.HostSpec{
			Name:    "remote1",
			Address: "http://agent.mock:7072",
		},
		Step: workflow.Step{
			Name:   "step-2",
			Action: "shell.run",
			Args: map[string]any{
				"script": "echo ok",
			},
		},
		Vars: map[string]any{
			"RUNNER_AGENT_TOKEN": "agent-secret-token",
		},
	})
	if err == nil {
		t.Fatalf("expected dispatch error")
	}
	if result.Status != "failed" {
		t.Fatalf("expected failed result, got %s", result.Status)
	}
	if !strings.Contains(result.Error, "401 Unauthorized") || !strings.Contains(result.Error, "runner token mismatch") {
		t.Fatalf("unexpected result error: %q", result.Error)
	}
	debug, _ := result.Output[runnerDebugKey].(map[string]any)
	if got := strings.TrimSpace(debug["phase"].(string)); got != "status" {
		t.Fatalf("expected status phase, got %q", got)
	}
}

func TestHybridDispatcherDispatchRemoteDecodedErrorReturnsFailedResult(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"result":{"task_id":"remote-task","status":"failed"},"error":"runner token mismatch"}`)),
				Request:    req,
			}, nil
		}),
	}

	dispatcher := NewHybridDispatcher(nil)
	dispatcher.Client = client

	result, err := dispatcher.Dispatch(context.Background(), Task{
		ID:    "task-3",
		RunID: "run-3",
		Host: workflow.HostSpec{
			Name:    "remote1",
			Address: "http://agent.mock:7072",
		},
		Step: workflow.Step{
			Name:   "step-3",
			Action: "shell.run",
			Args: map[string]any{
				"script": "echo ok",
			},
		},
	})
	if err == nil {
		t.Fatalf("expected dispatch error")
	}
	if result.Status != "failed" {
		t.Fatalf("expected failed result, got %s", result.Status)
	}
	if result.Error != "runner token mismatch" {
		t.Fatalf("unexpected result error: %q", result.Error)
	}
}

func TestHybridDispatcherDispatchRejectedLogsResolvedAndFinalTokenHeaders(t *testing.T) {
	logBuf, restoreLogs := withCapturedSchedulerLogs(t)
	defer restoreLogs()

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
				Request:    req,
			}, nil
		}),
	}

	dispatcher := NewHybridDispatcher(nil)
	dispatcher.Client = client
	dispatcher.Token = "configured-agent-token"
	dispatcher.Headers = map[string]string{
		"Authorization":  "Bearer overridden-agent-token",
		"X-Runner-Token": "overridden-agent-token",
		"X-Agent-Auth":   "overridden-agent-token",
	}

	_, err := dispatcher.Dispatch(context.Background(), Task{
		ID:    "task-log-1",
		RunID: "run-log-1",
		Host: workflow.HostSpec{
			Name:    "remote1",
			Address: "http://agent.mock:7072",
		},
		Step: workflow.Step{
			Name:   "step-log-1",
			Action: "shell.run",
			Args: map[string]any{
				"script": "echo ok",
			},
		},
	})
	if err == nil {
		t.Fatalf("expected dispatch error")
	}

	logs := logBuf.String()
	if !strings.Contains(logs, `"resolved_token":"configured-agent-token"`) {
		t.Fatalf("expected resolved token in logs, got %s", logs)
	}
	if !strings.Contains(logs, `"authorization_header":"Bearer overridden-agent-token"`) {
		t.Fatalf("expected authorization header in logs, got %s", logs)
	}
	if !strings.Contains(logs, `"authorization_token":"overridden-agent-token"`) {
		t.Fatalf("expected authorization token in logs, got %s", logs)
	}
	if !strings.Contains(logs, `"x_runner_token":"overridden-agent-token"`) {
		t.Fatalf("expected x-runner-token in logs, got %s", logs)
	}
	if !strings.Contains(logs, `"x_agent_auth":"overridden-agent-token"`) {
		t.Fatalf("expected x-agent-auth in logs, got %s", logs)
	}
}
