package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aiops-v2/internal/integrations/localtools"
	"aiops-v2/internal/server"
	"aiops-v2/internal/store"
	"runner/scheduler"
	"runner/workflow"
)

type hostAgentCommandRunner struct {
	grpc          *server.GRPCServer
	repo          localtools.HostRepository
	tokenResolver hostAgentTokenResolver
	httpClient    *http.Client
	now           func() time.Time
}

type hostAgentTokenResolver interface {
	ResolveHostAgentToken(ctx context.Context, ref string) (string, error)
}

func (r hostAgentCommandRunner) RunHostAgentCommand(ctx context.Context, req localtools.HostAgentCommandRequest) (localtools.HostAgentCommandResult, error) {
	if r.grpc != nil {
		result, err := r.runGRPC(ctx, req)
		if err == nil {
			return result, nil
		}
		if !strings.Contains(err.Error(), "not connected") {
			return localtools.HostAgentCommandResult{}, err
		}
	}
	return r.runHTTP(ctx, req)
}

func (r hostAgentCommandRunner) runGRPC(ctx context.Context, req localtools.HostAgentCommandRequest) (localtools.HostAgentCommandResult, error) {
	if r.grpc == nil {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("host-agent grpc server is not configured")
	}
	timeoutMs := 0
	if req.Timeout > 0 {
		timeoutMs = int(req.Timeout.Milliseconds())
	}
	resp, err := r.grpc.RunExec(ctx, req.HostID, server.HostExecRequest{
		Command:        req.Command,
		Args:           append([]string(nil), req.Args...),
		WorkingDir:     req.WorkingDir,
		TimeoutMs:      timeoutMs,
		MaxOutputBytes: req.MaxOutputBytes,
	})
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	if resp.Error != "" && resp.ExitCode == 0 {
		return localtools.HostAgentCommandResult{}, errors.New(resp.Error)
	}
	return localtools.HostAgentCommandResult{
		Stdout:   resp.Stdout,
		Stderr:   resp.Stderr,
		ExitCode: resp.ExitCode,
		Source:   "host.agent_grpc",
	}, nil
}

type hostAgentRunRequest struct {
	Task scheduler.Task `json:"task"`
	Wait *bool          `json:"wait,omitempty"`
}

type hostAgentRunResponse struct {
	Result scheduler.Result `json:"result"`
	RunID  string           `json:"runId,omitempty"`
	Error  string           `json:"error,omitempty"`
}

func (r hostAgentCommandRunner) runHTTP(ctx context.Context, req localtools.HostAgentCommandRequest) (localtools.HostAgentCommandResult, error) {
	result, err := r.runHTTPExec(ctx, req)
	if err == nil {
		return result, nil
	}
	if !isHTTPExecFallbackError(err) {
		return localtools.HostAgentCommandResult{}, err
	}
	return r.runHTTPRun(ctx, req)
}

func (r hostAgentCommandRunner) runHTTPExec(ctx context.Context, req localtools.HostAgentCommandRequest) (localtools.HostAgentCommandResult, error) {
	host, token, err := r.resolveHTTPHostAgent(ctx, req.HostID)
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(host.AgentURL), "/")
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	timeoutMs := 0
	if req.Timeout > 0 {
		timeoutMs = int(req.Timeout.Milliseconds())
	}
	body := server.HostExecRequest{
		Command:        req.Command,
		Args:           append([]string(nil), req.Args...),
		WorkingDir:     req.WorkingDir,
		TimeoutMs:      timeoutMs,
		MaxOutputBytes: req.MaxOutputBytes,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(runCtx, http.MethodPost, baseURL+"/exec", bytes.NewReader(data))
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	client := r.httpClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	defer resp.Body.Close()
	respData, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return localtools.HostAgentCommandResult{}, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return localtools.HostAgentCommandResult{}, hostAgentHTTPStatusError{Path: "/exec", StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(respData))}
	}
	var payload server.HostExecResponse
	if err := json.Unmarshal(respData, &payload); err != nil {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("decode host-agent /exec response: %w", err)
	}
	if strings.TrimSpace(payload.Error) != "" && payload.ExitCode == 0 {
		return localtools.HostAgentCommandResult{}, errors.New(strings.TrimSpace(payload.Error))
	}
	return localtools.HostAgentCommandResult{
		Stdout:   payload.Stdout,
		Stderr:   payload.Stderr,
		ExitCode: payload.ExitCode,
		Source:   "host.agent_http_exec",
	}, nil
}

func (r hostAgentCommandRunner) runHTTPRun(ctx context.Context, req localtools.HostAgentCommandRequest) (localtools.HostAgentCommandResult, error) {
	if r.repo == nil {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("host repository is not configured")
	}
	if r.tokenResolver == nil {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("host-agent token resolver is not configured")
	}
	host, err := r.repo.GetHost(strings.TrimSpace(req.HostID))
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	if host == nil {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("host %q not found", req.HostID)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(host.AgentURL), "/")
	if baseURL == "" {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("host %q does not have an agent URL", req.HostID)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("host %q agent URL is invalid", req.HostID)
	}
	tokenRef := strings.TrimSpace(host.AgentTokenSecretRef)
	if tokenRef == "" {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("host %q does not have a local host-agent token secret", req.HostID)
	}
	token, err := r.tokenResolver.ResolveHostAgentToken(ctx, tokenRef)
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	script := shellScriptForCommand(req.Command, req.Args, req.WorkingDir)
	now := r.now
	if now == nil {
		now = time.Now
	}
	runID := "host-tool-" + sanitizeHostRunnerID(req.HostID) + "-" + now().UTC().Format("20060102150405.000000000")
	wait := true
	body := hostAgentRunRequest{
		Wait: &wait,
		Task: scheduler.Task{
			ID:    runID,
			RunID: runID,
			Step: workflow.Step{
				Name:   "host-tool",
				Action: "script.shell",
				Args: map[string]any{
					"script":           script,
					"max_output_bytes": req.MaxOutputBytes,
				},
			},
			Host: workflow.HostSpec{
				Name:    host.ID,
				Address: host.Address,
			},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(runCtx, http.MethodPost, baseURL+"/run", bytes.NewReader(data))
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	client := r.httpClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	defer resp.Body.Close()
	respData, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return localtools.HostAgentCommandResult{}, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return localtools.HostAgentCommandResult{}, hostAgentHTTPStatusError{Path: "/run", StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(respData))}
	}
	var payload hostAgentRunResponse
	if err := json.Unmarshal(respData, &payload); err != nil {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("decode host-agent /run response: %w", err)
	}
	stdout, _ := payload.Result.Output["stdout"].(string)
	stderr, _ := payload.Result.Output["stderr"].(string)
	exitCode := 0
	if strings.TrimSpace(payload.Result.Status) != "" && payload.Result.Status != "success" {
		exitCode = 1
	}
	if strings.TrimSpace(payload.Error) != "" && stderr == "" {
		stderr = strings.TrimSpace(payload.Error)
	}
	return localtools.HostAgentCommandResult{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
		Source:   "host.agent_http_run",
	}, nil
}

func (r hostAgentCommandRunner) resolveHTTPHostAgent(ctx context.Context, hostID string) (*store.HostRecord, string, error) {
	if r.repo == nil {
		return nil, "", fmt.Errorf("host repository is not configured")
	}
	if r.tokenResolver == nil {
		return nil, "", fmt.Errorf("host-agent token resolver is not configured")
	}
	host, err := r.repo.GetHost(strings.TrimSpace(hostID))
	if err != nil {
		return nil, "", err
	}
	if host == nil {
		return nil, "", fmt.Errorf("host %q not found", hostID)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(host.AgentURL), "/")
	if baseURL == "" {
		return nil, "", fmt.Errorf("host %q does not have an agent URL", hostID)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, "", fmt.Errorf("host %q agent URL is invalid", hostID)
	}
	tokenRef := strings.TrimSpace(host.AgentTokenSecretRef)
	if tokenRef == "" {
		return nil, "", fmt.Errorf("host %q does not have a local host-agent token secret", hostID)
	}
	token, err := r.tokenResolver.ResolveHostAgentToken(ctx, tokenRef)
	if err != nil {
		return nil, "", err
	}
	return host, token, nil
}

type hostAgentHTTPStatusError struct {
	Path       string
	StatusCode int
	Body       string
}

func (e hostAgentHTTPStatusError) Error() string {
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Sprintf("host-agent %s status %d", e.Path, e.StatusCode)
	}
	return fmt.Sprintf("host-agent %s status %d: %s", e.Path, e.StatusCode, truncateHostAgentErrorBody(e.Body, 300))
}

func isHTTPExecFallbackError(err error) bool {
	var statusErr hostAgentHTTPStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	return statusErr.StatusCode == http.StatusNotFound || statusErr.StatusCode == http.StatusMethodNotAllowed
}

func truncateHostAgentErrorBody(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func shellScriptForCommand(command string, args []string, workingDir string) string {
	parts := []string{"set -e"}
	if dir := strings.TrimSpace(workingDir); dir != "" {
		parts = append(parts, "cd -- "+shellQuote(dir))
	}
	cmdParts := []string{shellQuote(strings.TrimSpace(command))}
	for _, arg := range args {
		cmdParts = append(cmdParts, shellQuote(arg))
	}
	parts = append(parts, "exec "+strings.Join(cmdParts, " "))
	return strings.Join(parts, "\n")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func sanitizeHostRunnerID(value string) string {
	var builder strings.Builder
	for _, r := range strings.TrimSpace(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	out := strings.Trim(builder.String(), "-._")
	if out == "" {
		return "host"
	}
	return out
}
