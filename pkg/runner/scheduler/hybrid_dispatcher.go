package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"runner/logging"
	"runner/modules"
	"runner/workflow"
)

type Task struct {
	ID    string
	RunID string
	Step  workflow.Step
	Host  workflow.HostSpec
	Vars  map[string]any

	// FSM metadata for traceability and idempotency.
	FSMJobID        string `json:"fsm_job_id,omitempty"`
	FSMStepName     string `json:"fsm_step_name,omitempty"`
	FSMStepIndex    int    `json:"fsm_step_index,omitempty"`
	FSMWaitingToken string `json:"fsm_waiting_token,omitempty"`
}

type Result struct {
	TaskID string         `json:"task_id"`
	Status string         `json:"status"`
	Output map[string]any `json:"output,omitempty"`
	Error  string         `json:"error,omitempty"`
}

var ErrTaskNotFound = errors.New("runner task not found")

type Dispatcher interface {
	Dispatch(ctx context.Context, task Task) (Result, error)
}

type SubmitOptions struct {
	Wait bool
}

type SubmitResult struct {
	TaskID string `json:"task_id"`
	RunID  string `json:"run_id,omitempty"`
	Status string `json:"status"`
}

type AsyncDispatcher interface {
	Submit(ctx context.Context, task Task, opts SubmitOptions) (SubmitResult, error)
	Get(ctx context.Context, taskID string) (Result, error)
	Cancel(ctx context.Context, taskID string) error
}

// HybridDispatcher routes each task with strict local rules:
// - local dispatch only when host target is "local" or address is "127.0.0.1"
// - everything else dispatches to runner agent
// Remote URLs default to http:// when scheme is missing.
// It is now the only built-in dispatcher implementation.
type HybridDispatcher struct {
	Registry *modules.Registry

	BaseURL string
	Client  *http.Client
	Headers map[string]string
	Token   string
	// Heartbeat enables a pre-flight heartbeat call before each dispatch.
	Heartbeat bool
	// HeartbeatPath overrides the default heartbeat endpoint path.
	HeartbeatPath string
	// StatusPath overrides the default status endpoint path.
	StatusPath string
	// RetryMax defines how many retries after the initial attempt.
	RetryMax int
	// RetryDelay defines the delay between retries.
	RetryDelay time.Duration
	// DispatchTimeout overrides the http client timeout when Client is nil.
	DispatchTimeout time.Duration
	// HeartbeatTimeout overrides the heartbeat http client timeout when Client is nil.
	HeartbeatTimeout time.Duration
	// AsyncTimeout controls how long to wait for async task completion.
	AsyncTimeout time.Duration
	// PollInterval controls how often to poll /status.
	PollInterval time.Duration
	// OnOutput receives streaming output chunks while polling async tasks.
	OnOutput func(taskID, step, host, stream, chunk string)

	mu      sync.Mutex
	results map[string]Result

	outputMu      sync.Mutex
	outputOffsets map[string]outputOffset

	taskMetaMu sync.Mutex
	taskMeta   map[string]taskMeta

	// Optional override routers for tests.
	localRouter  Dispatcher
	remoteRouter Dispatcher
}

type outputOffset struct {
	stdout int
	stderr int
}

type taskMeta struct {
	runID       string
	step        string
	action      string
	host        string
	hostAddress string
	baseURL     string
	token       string
	heartbeat   bool
	remote      bool
}

type remoteConfig struct {
	token            string
	heartbeat        bool
	retryMax         int
	retryDelay       time.Duration
	dispatchTimeout  time.Duration
	heartbeatTimeout time.Duration
	asyncTimeout     time.Duration
	pollInterval     time.Duration
}

const runnerDebugKey = "runner_debug"

type runnerDebug struct {
	Mode             string
	TaskID           string
	RunID            string
	Step             string
	Action           string
	Host             string
	HostAddress      string
	ResolvedAddress  string
	HeartbeatEnabled bool
	Attempt          int
	Phase            string
}

func remoteFailedResult(task Task, baseURL string, attempt int, heartbeat bool, phase, message string) Result {
	return attachRunnerDebug(Result{
		TaskID: task.ID,
		Status: "failed",
		Error:  strings.TrimSpace(message),
	}, runnerDebug{
		Mode:             "remote",
		TaskID:           task.ID,
		RunID:            task.RunID,
		Step:             task.Step.Name,
		Action:           task.Step.Action,
		Host:             task.Host.Name,
		HostAddress:      strings.TrimSpace(task.Host.Address),
		ResolvedAddress:  baseURL,
		HeartbeatEnabled: heartbeat,
		Attempt:          attempt,
		Phase:            phase,
	})
}

func remoteFailedResultFromMeta(meta taskMeta, taskID, baseURL string, attempt int, phase, message string) Result {
	return attachRunnerDebug(Result{
		TaskID: taskID,
		Status: "failed",
		Error:  strings.TrimSpace(message),
	}, runnerDebug{
		Mode:             "remote",
		TaskID:           taskID,
		RunID:            meta.runID,
		Step:             meta.step,
		Action:           meta.action,
		Host:             meta.host,
		HostAddress:      meta.hostAddress,
		ResolvedAddress:  baseURL,
		HeartbeatEnabled: meta.heartbeat,
		Attempt:          attempt,
		Phase:            phase,
	})
}

// Compatibility aliases. Keep public API stable while implementation is unified.
type AgentDispatcher = HybridDispatcher
type LocalDispatcher = HybridDispatcher

func NewHybridDispatcher(registry *modules.Registry) *HybridDispatcher {
	return &HybridDispatcher{
		Registry:      registry,
		Client:        &http.Client{Timeout: 30 * time.Second},
		results:       map[string]Result{},
		outputOffsets: map[string]outputOffset{},
		taskMeta:      map[string]taskMeta{},
	}
}

func NewHybridDispatcherWithRouters(local, remote Dispatcher) *HybridDispatcher {
	d := NewHybridDispatcher(nil)
	d.localRouter = local
	d.remoteRouter = remote
	return d
}

func NewAgentDispatcher(baseURL string) *AgentDispatcher {
	d := NewHybridDispatcher(nil)
	d.BaseURL = strings.TrimSpace(baseURL)
	return d
}

var _ Dispatcher = (*HybridDispatcher)(nil)
var _ AsyncDispatcher = (*HybridDispatcher)(nil)

func (d *HybridDispatcher) Dispatch(ctx context.Context, task Task) (Result, error) {
	if d == nil {
		return Result{}, fmt.Errorf("hybrid dispatcher is nil")
	}
	if strings.TrimSpace(task.ID) == "" {
		task.ID = fmt.Sprintf("task-%d", time.Now().UTC().UnixNano())
	}
	if d.useRemote(task) {
		if d.remoteRouter != nil && d.remoteRouter != d {
			return d.remoteRouter.Dispatch(ctx, task)
		}
		return d.dispatchRemote(ctx, task)
	}
	if d.localRouter != nil && d.localRouter != d {
		return d.localRouter.Dispatch(ctx, task)
	}
	return d.dispatchLocal(ctx, task)
}

func (d *HybridDispatcher) Submit(ctx context.Context, task Task, opts SubmitOptions) (SubmitResult, error) {
	if d == nil {
		return SubmitResult{}, fmt.Errorf("hybrid dispatcher is nil")
	}
	if strings.TrimSpace(task.ID) == "" {
		task.ID = fmt.Sprintf("task-%d", time.Now().UTC().UnixNano())
	}
	if strings.TrimSpace(task.RunID) == "" {
		task.RunID = task.ID
	}
	if d.useRemote(task) {
		return d.submitRemote(ctx, task, opts)
	}
	result, err := d.dispatchLocal(ctx, task)
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = "success"
	}
	return SubmitResult{TaskID: result.TaskID, RunID: task.RunID, Status: status}, err
}

func (d *HybridDispatcher) Get(ctx context.Context, taskID string) (Result, error) {
	if d == nil {
		return Result{}, fmt.Errorf("hybrid dispatcher is nil")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Result{}, fmt.Errorf("task_id is required")
	}

	meta := d.getTaskMeta(taskID)
	if !meta.remote {
		d.mu.Lock()
		defer d.mu.Unlock()
		result, ok := d.results[taskID]
		if !ok {
			return Result{}, ErrTaskNotFound
		}
		return result, nil
	}

	baseURL := strings.TrimSpace(meta.baseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(d.BaseURL)
	}
	if baseURL == "" {
		return Result{}, fmt.Errorf("agent dispatcher base url is required")
	}

	result, err := d.fetchStatus(ctx, baseURL, taskID, meta.token, d.DispatchTimeout)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "404") ||
			strings.Contains(strings.ToLower(err.Error()), "not found") {
			return Result{}, ErrTaskNotFound
		}
		return result, err
	}
	d.emitOutputDelta(taskID, result.Output)
	status := strings.ToLower(strings.TrimSpace(result.Status))
	if status != "running" {
		d.clearTaskMeta(taskID)
	}
	return result, nil
}

func (d *HybridDispatcher) Cancel(ctx context.Context, taskID string) error {
	if d == nil {
		return fmt.Errorf("hybrid dispatcher is nil")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("task_id is required")
	}

	meta := d.getTaskMeta(taskID)
	if !meta.remote {
		d.mu.Lock()
		defer d.mu.Unlock()
		if _, ok := d.results[taskID]; !ok {
			return ErrTaskNotFound
		}
		return nil
	}

	baseURL := strings.TrimSpace(meta.baseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(d.BaseURL)
	}
	if baseURL == "" {
		return fmt.Errorf("agent dispatcher base url is required")
	}
	url := strings.TrimRight(baseURL, "/") + "/cancel"
	payload := struct {
		TaskID string `json:"task_id"`
	}{TaskID: taskID}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(meta.token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Runner-Token", token)
		req.Header.Set("X-Agent-Auth", token)
	}
	for k, v := range d.Headers {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		req.Header.Set(k, v)
	}
	logAgentRequestAuth("agent cancel request auth", meta.token, req,
		zap.String("task_id", taskID),
		zap.String("url", url),
	)
	client := d.clientWithTimeout(d.DispatchTimeout, 30*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		logging.L().Warn("agent cancel transport failed",
			withAgentAuthFields([]zap.Field{
				zap.String("task_id", taskID),
				zap.String("url", url),
				zap.Error(err),
			}, meta.token, req)...,
		)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := readLimitedBody(resp.Body)
		logging.L().Warn("agent cancel rejected",
			withAgentAuthFields([]zap.Field{
				zap.String("task_id", taskID),
				zap.String("url", url),
				zap.Int("status_code", resp.StatusCode),
				zap.String("body", body),
			}, meta.token, req)...,
		)
		if body != "" {
			return fmt.Errorf("agent cancel failed: %s (%s)", resp.Status, body)
		}
		return fmt.Errorf("agent cancel failed: %s", resp.Status)
	}
	d.clearTaskMeta(taskID)
	return nil
}

func (d *HybridDispatcher) useRemote(task Task) bool {
	hostName := strings.ToLower(strings.TrimSpace(task.Host.Name))
	if hostName == "local" {
		return false
	}

	addr := strings.ToLower(strings.TrimSpace(task.Host.Address))
	if addr == "local" || addr == "127.0.0.1" {
		return false
	}
	return true
}

func (d *HybridDispatcher) dispatchLocal(ctx context.Context, task Task) (Result, error) {
	if d.Registry == nil {
		return Result{}, fmt.Errorf("registry is nil")
	}
	logging.L().Debug("dispatch task",
		zap.String("task_id", task.ID),
		zap.String("run_id", task.RunID),
		zap.String("step", task.Step.Name),
		zap.String("action", task.Step.Action),
		zap.String("host", task.Host.Name),
		zap.String("fsm_job_id", task.FSMJobID),
		zap.String("fsm_step_name", task.FSMStepName),
		zap.Int("fsm_step_index", task.FSMStepIndex),
		zap.String("fsm_waiting_token", task.FSMWaitingToken),
	)
	module, ok := d.Registry.Get(task.Step.Action)
	if !ok {
		return Result{}, fmt.Errorf("module %q not registered", task.Step.Action)
	}

	res, err := module.Apply(ctx, modules.Request{
		Step: task.Step,
		Host: task.Host,
		Vars: task.Vars,
	})
	if err != nil {
		logging.L().Debug("dispatch task failed",
			zap.String("task_id", task.ID),
			zap.String("run_id", task.RunID),
			zap.Error(err),
		)
		result := Result{
			TaskID: task.ID,
			Status: "failed",
			Output: res.Output,
			Error:  err.Error(),
		}
		result = attachRunnerDebug(result, runnerDebug{
			Mode:        "local",
			TaskID:      task.ID,
			RunID:       task.RunID,
			Step:        task.Step.Name,
			Action:      task.Step.Action,
			Host:        task.Host.Name,
			HostAddress: strings.TrimSpace(task.Host.Address),
			Phase:       "dispatch",
		})
		d.persistResult(result)
		return result, err
	}

	result := Result{
		TaskID: task.ID,
		Status: "success",
		Output: res.Output,
	}
	result = attachRunnerDebug(result, runnerDebug{
		Mode:        "local",
		TaskID:      task.ID,
		RunID:       task.RunID,
		Step:        task.Step.Name,
		Action:      task.Step.Action,
		Host:        task.Host.Name,
		HostAddress: strings.TrimSpace(task.Host.Address),
		Phase:       "dispatch",
	})
	d.persistResult(result)
	return result, nil
}

func (d *HybridDispatcher) submitRemote(ctx context.Context, task Task, opts SubmitOptions) (SubmitResult, error) {
	baseURL := d.resolveBaseURL(task)
	if baseURL == "" {
		return SubmitResult{}, fmt.Errorf("agent dispatcher base url is required")
	}
	cfg := d.resolveRemoteConfig(task)
	d.setTaskMeta(task.ID, task.RunID, task.Step.Name, task.Step.Action, task.Host.Name, strings.TrimSpace(task.Host.Address), baseURL, cfg.token, cfg.heartbeat, true)

	attempts := cfg.retryMax + 1
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if ctx.Err() != nil {
			return SubmitResult{}, ctx.Err()
		}
		logging.L().Info("agent submit attempt",
			zap.String("run_id", task.RunID),
			zap.String("task_id", task.ID),
			zap.String("step", task.Step.Name),
			zap.String("action", task.Step.Action),
			zap.String("host", task.Host.Name),
			zap.String("resolved_address", baseURL),
			zap.Int("attempt", attempt+1),
			zap.Bool("heartbeat_enabled", cfg.heartbeat),
		)
		if cfg.heartbeat {
			if err := d.sendHeartbeat(ctx, baseURL, cfg); err != nil {
				lastErr = err
				if attempt < attempts-1 {
					if waitErr := sleepWithContextFor(ctx, cfg.retryDelay); waitErr != nil {
						return SubmitResult{}, waitErr
					}
					continue
				}
				return SubmitResult{}, err
			}
		}

		result, err := d.dispatchOnce(ctx, baseURL, task, opts.Wait, cfg.token, cfg.dispatchTimeout, attempt+1, cfg.heartbeat)
		if err != nil {
			lastErr = err
			if attempt < attempts-1 {
				if waitErr := sleepWithContextFor(ctx, cfg.retryDelay); waitErr != nil {
					return SubmitResult{}, waitErr
				}
				continue
			}
			return SubmitResult{}, err
		}
		if strings.TrimSpace(result.TaskID) == "" {
			result.TaskID = task.ID
		}
		d.setTaskMeta(result.TaskID, task.RunID, task.Step.Name, task.Step.Action, task.Host.Name, strings.TrimSpace(task.Host.Address), baseURL, cfg.token, cfg.heartbeat, true)

		status := strings.TrimSpace(result.Status)
		if status == "" {
			if opts.Wait {
				status = "success"
			} else {
				status = "running"
			}
		}
		if opts.Wait && strings.EqualFold(status, "running") {
			finalResult, err := d.pollStatus(ctx, baseURL, result.TaskID, cfg)
			if err != nil {
				return SubmitResult{}, err
			}
			status = strings.TrimSpace(finalResult.Status)
			if status == "" {
				status = "success"
			}
		}
		return SubmitResult{TaskID: result.TaskID, RunID: task.RunID, Status: status}, nil
	}
	if lastErr != nil {
		return SubmitResult{}, lastErr
	}
	return SubmitResult{}, fmt.Errorf("agent submit failed")
}

func (d *HybridDispatcher) dispatchRemote(ctx context.Context, task Task) (Result, error) {
	baseURL := d.resolveBaseURL(task)
	if baseURL == "" {
		return Result{}, fmt.Errorf("agent dispatcher base url is required")
	}
	cfg := d.resolveRemoteConfig(task)
	d.setTaskMeta(task.ID, task.RunID, task.Step.Name, task.Step.Action, task.Host.Name, strings.TrimSpace(task.Host.Address), baseURL, cfg.token, cfg.heartbeat, true)
	remoteTaskID := ""
	defer func() {
		d.clearTaskMeta(task.ID)
		if strings.TrimSpace(remoteTaskID) != "" && remoteTaskID != task.ID {
			d.clearTaskMeta(remoteTaskID)
		}
	}()

	attempts := cfg.retryMax + 1
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	var lastResult Result

	for attempt := 0; attempt < attempts; attempt++ {
		if ctx.Err() != nil {
			return lastResult, ctx.Err()
		}
		logging.L().Info("agent dispatch attempt",
			zap.String("run_id", task.RunID),
			zap.String("task_id", task.ID),
			zap.String("step", task.Step.Name),
			zap.String("action", task.Step.Action),
			zap.String("host", task.Host.Name),
			zap.String("resolved_address", baseURL),
			zap.Int("attempt", attempt+1),
			zap.Bool("heartbeat_enabled", cfg.heartbeat),
		)
		if cfg.heartbeat {
			if err := d.sendHeartbeat(ctx, baseURL, cfg); err != nil {
				lastErr = err
				lastResult = remoteFailedResult(task, baseURL, attempt+1, cfg.heartbeat, "heartbeat", err.Error())
				if attempt < attempts-1 {
					logging.L().Warn("agent heartbeat failed, retrying",
						zap.String("host", baseURL),
						zap.Int("attempt", attempt+1),
						zap.Error(err),
					)
					if waitErr := sleepWithContextFor(ctx, cfg.retryDelay); waitErr != nil {
						return lastResult, waitErr
					}
					continue
				}
				return lastResult, err
			}
		}

		result, err := d.dispatchOnce(ctx, baseURL, task, true, cfg.token, cfg.dispatchTimeout, attempt+1, cfg.heartbeat)
		if err == nil {
			if strings.TrimSpace(result.TaskID) == "" {
				result.TaskID = task.ID
			}
			remoteTaskID = result.TaskID
			d.setTaskMeta(result.TaskID, task.RunID, task.Step.Name, task.Step.Action, task.Host.Name, strings.TrimSpace(task.Host.Address), baseURL, cfg.token, cfg.heartbeat, true)
			if strings.EqualFold(result.Status, "running") {
				return d.pollStatus(ctx, baseURL, result.TaskID, cfg)
			}
			return result, nil
		}
		lastErr = err
		lastResult = result
		if attempt < attempts-1 {
			logging.L().Warn("agent dispatch failed, retrying",
				zap.String("task_id", task.ID),
				zap.String("run_id", task.RunID),
				zap.Int("attempt", attempt+1),
				zap.Error(err),
			)
			if waitErr := sleepWithContextFor(ctx, cfg.retryDelay); waitErr != nil {
				return lastResult, waitErr
			}
		}
	}
	if lastErr != nil {
		return lastResult, lastErr
	}
	return lastResult, fmt.Errorf("agent dispatch failed")
}

func (d *HybridDispatcher) resolveRemoteConfig(task Task) remoteConfig {
	cfg := remoteConfig{
		token:            strings.TrimSpace(d.Token),
		heartbeat:        d.Heartbeat,
		retryMax:         d.RetryMax,
		retryDelay:       d.RetryDelay,
		dispatchTimeout:  d.DispatchTimeout,
		heartbeatTimeout: d.HeartbeatTimeout,
		asyncTimeout:     d.AsyncTimeout,
		pollInterval:     d.PollInterval,
	}
	if len(task.Vars) == 0 {
		if token, ok := readString(task.Host.Vars, "RUNNER_AGENT_TOKEN"); ok {
			cfg.token = token
		}
		return cfg
	}

	if token, ok := readString(task.Host.Vars, "RUNNER_AGENT_TOKEN"); ok {
		cfg.token = token
	}
	if heartbeat, ok := readBool(task.Vars, "RUNNER_AGENT_HEARTBEAT"); ok {
		cfg.heartbeat = heartbeat
	}
	if retryMax, ok := readInt(task.Vars, "RUNNER_AGENT_RETRY_MAX"); ok {
		cfg.retryMax = retryMax
	}
	if delaySec, ok := readInt(task.Vars, "RUNNER_AGENT_RETRY_DELAY_SEC"); ok && delaySec > 0 {
		cfg.retryDelay = time.Duration(delaySec) * time.Second
	}
	if timeoutSec, ok := readInt(task.Vars, "RUNNER_AGENT_DISPATCH_TIMEOUT_SEC"); ok && timeoutSec > 0 {
		cfg.dispatchTimeout = time.Duration(timeoutSec) * time.Second
	}
	if timeoutSec, ok := readInt(task.Vars, "RUNNER_AGENT_HEARTBEAT_TIMEOUT_SEC"); ok && timeoutSec > 0 {
		cfg.heartbeatTimeout = time.Duration(timeoutSec) * time.Second
	}
	if timeoutSec, ok := readInt(task.Vars, "RUNNER_AGENT_ASYNC_TIMEOUT_SEC"); ok && timeoutSec > 0 {
		cfg.asyncTimeout = time.Duration(timeoutSec) * time.Second
	}
	if pollSec, ok := readInt(task.Vars, "RUNNER_AGENT_POLL_INTERVAL_SEC"); ok && pollSec > 0 {
		cfg.pollInterval = time.Duration(pollSec) * time.Second
	}
	return cfg
}

func (d *HybridDispatcher) sendHeartbeat(ctx context.Context, baseURL string, cfg remoteConfig) error {
	path := strings.TrimSpace(d.HeartbeatPath)
	if path == "" {
		path = "/heartbeat"
	}
	url := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	if token := strings.TrimSpace(cfg.token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Runner-Token", token)
		req.Header.Set("X-Agent-Auth", token)
	}
	for k, v := range d.Headers {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		req.Header.Set(k, v)
	}
	logAgentRequestAuth("agent heartbeat request auth", cfg.token, req,
		zap.String("url", url),
	)
	client := d.clientWithTimeout(cfg.heartbeatTimeout, 10*time.Second)
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		logging.L().Warn("agent heartbeat transport failed",
			withAgentAuthFields([]zap.Field{
				zap.String("url", url),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err),
			}, cfg.token, req)...,
		)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := readLimitedBody(resp.Body)
		logging.L().Warn("agent heartbeat failed",
			withAgentAuthFields([]zap.Field{
				zap.String("status", resp.Status),
				zap.String("body", body),
				zap.String("url", url),
			}, cfg.token, req)...,
		)
		if body != "" {
			return fmt.Errorf("agent heartbeat failed: %s (%s)", resp.Status, body)
		}
		return fmt.Errorf("agent heartbeat failed: %s", resp.Status)
	}
	logging.L().Info("agent heartbeat ok",
		zap.String("url", url),
		zap.Int("status_code", resp.StatusCode),
		zap.Duration("duration", time.Since(start)),
	)
	return nil
}

func (d *HybridDispatcher) dispatchOnce(ctx context.Context, baseURL string, task Task, wait bool, token string, timeout time.Duration, attempt int, heartbeat bool) (Result, error) {
	url := strings.TrimRight(baseURL, "/") + "/run"
	payload := struct {
		Task Task  `json:"task"`
		Wait *bool `json:"wait,omitempty"`
	}{Task: task}
	if !wait {
		value := false
		payload.Wait = &value
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token = strings.TrimSpace(token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Runner-Token", token)
		req.Header.Set("X-Agent-Auth", token)
	}
	for k, v := range d.Headers {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		req.Header.Set(k, v)
	}
	logAgentRequestAuth("agent dispatch request auth", token, req,
		zap.String("run_id", task.RunID),
		zap.String("task_id", task.ID),
		zap.String("step", task.Step.Name),
		zap.String("action", task.Step.Action),
		zap.String("host", task.Host.Name),
		zap.String("resolved_address", baseURL),
		zap.String("url", url),
		zap.Bool("wait", wait),
		zap.Int("attempt", attempt),
	)

	client := d.clientWithTimeout(timeout, 30*time.Second)
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		logging.L().Warn("agent dispatch transport failed",
			withAgentAuthFields([]zap.Field{
				zap.String("run_id", task.RunID),
				zap.String("task_id", task.ID),
				zap.String("step", task.Step.Name),
				zap.String("action", task.Step.Action),
				zap.String("host", task.Host.Name),
				zap.String("resolved_address", baseURL),
				zap.String("url", url),
				zap.Bool("wait", wait),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err),
			}, token, req)...,
		)
		result := remoteFailedResult(task, baseURL, attempt, heartbeat, "dispatch", err.Error())
		return result, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := readLimitedBody(resp.Body)
		logging.L().Warn("agent dispatch rejected",
			withAgentAuthFields([]zap.Field{
				zap.String("run_id", task.RunID),
				zap.String("task_id", task.ID),
				zap.String("step", task.Step.Name),
				zap.String("action", task.Step.Action),
				zap.String("host", task.Host.Name),
				zap.String("resolved_address", baseURL),
				zap.String("url", url),
				zap.Int("status_code", resp.StatusCode),
				zap.String("body", body),
				zap.Duration("duration", time.Since(start)),
			}, token, req)...,
		)
		errMsg := fmt.Sprintf("agent dispatch failed: %s", resp.Status)
		if body != "" {
			errMsg = fmt.Sprintf("agent dispatch failed: %s (%s)", resp.Status, body)
		}
		result := remoteFailedResult(task, baseURL, attempt, heartbeat, "dispatch", errMsg)
		return result, fmt.Errorf("%s", errMsg)
	}

	var decoded struct {
		Result Result `json:"result"`
		Error  string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		result := remoteFailedResult(task, baseURL, attempt, heartbeat, "dispatch", err.Error())
		return result, err
	}
	if decoded.Result.TaskID == "" {
		decoded.Result.TaskID = task.ID
	}
	decoded.Result = attachRunnerDebug(decoded.Result, runnerDebug{
		Mode:             "remote",
		TaskID:           decoded.Result.TaskID,
		RunID:            task.RunID,
		Step:             task.Step.Name,
		Action:           task.Step.Action,
		Host:             task.Host.Name,
		HostAddress:      strings.TrimSpace(task.Host.Address),
		ResolvedAddress:  baseURL,
		HeartbeatEnabled: heartbeat,
		Attempt:          attempt,
		Phase:            "dispatch",
	})
	if decoded.Error != "" {
		decoded.Result.Status = "failed"
		decoded.Result.Error = decoded.Error
		logging.L().Warn("agent dispatch returned error",
			zap.String("run_id", task.RunID),
			zap.String("task_id", decoded.Result.TaskID),
			zap.String("step", task.Step.Name),
			zap.String("action", task.Step.Action),
			zap.String("host", task.Host.Name),
			zap.String("resolved_address", baseURL),
			zap.String("result_error", decoded.Result.Error),
			zap.Duration("duration", time.Since(start)),
		)
		return decoded.Result, fmt.Errorf("%s", decoded.Error)
	}
	if strings.EqualFold(strings.TrimSpace(decoded.Result.Status), "failed") && strings.TrimSpace(decoded.Result.Error) == "" {
		decoded.Result.Error = "agent returned failed status without error"
		logging.L().Warn("agent returned failed status without error",
			zap.String("run_id", task.RunID),
			zap.String("task_id", decoded.Result.TaskID),
			zap.String("step", task.Step.Name),
			zap.String("host", task.Host.Name),
			zap.String("resolved_address", baseURL),
			zap.Duration("duration", time.Since(start)),
		)
	} else if strings.EqualFold(strings.TrimSpace(decoded.Result.Status), "failed") {
		logging.L().Warn("agent dispatch reported failed status",
			zap.String("run_id", task.RunID),
			zap.String("task_id", decoded.Result.TaskID),
			zap.String("step", task.Step.Name),
			zap.String("action", task.Step.Action),
			zap.String("host", task.Host.Name),
			zap.String("resolved_address", baseURL),
			zap.String("result_error", decoded.Result.Error),
			zap.Duration("duration", time.Since(start)),
		)
	}
	logging.L().Info("agent dispatch completed",
		zap.String("run_id", task.RunID),
		zap.String("task_id", decoded.Result.TaskID),
		zap.String("step", task.Step.Name),
		zap.String("action", task.Step.Action),
		zap.String("host", task.Host.Name),
		zap.String("resolved_address", baseURL),
		zap.String("result_status", decoded.Result.Status),
		zap.Duration("duration", time.Since(start)),
	)
	return decoded.Result, nil
}

func (d *HybridDispatcher) pollStatus(ctx context.Context, baseURL, taskID string, cfg remoteConfig) (Result, error) {
	if strings.TrimSpace(taskID) == "" {
		return Result{}, fmt.Errorf("task_id is required for status polling")
	}
	timeout := cfg.asyncTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	interval := cfg.pollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}

	pollCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		pollCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	for {
		result, err := d.fetchStatus(pollCtx, baseURL, taskID, cfg.token, cfg.dispatchTimeout)
		if err != nil {
			return result, err
		}
		d.emitOutputDelta(taskID, result.Output)
		if !strings.EqualFold(result.Status, "running") {
			return result, nil
		}
		if err := sleepWithContextFor(pollCtx, interval); err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return Result{}, fmt.Errorf("agent task %s polling timeout: %w", taskID, err)
			}
			return Result{}, err
		}
	}
}

func (d *HybridDispatcher) emitOutputDelta(taskID string, output map[string]any) {
	if d.OnOutput == nil || strings.TrimSpace(taskID) == "" {
		return
	}
	meta := d.getTaskMeta(taskID)
	d.outputMu.Lock()
	offsets := d.outputOffsets[taskID]
	d.outputMu.Unlock()

	emit := func(stream string, text string, previous int) int {
		if strings.TrimSpace(text) == "" {
			return previous
		}
		if previous < 0 {
			previous = 0
		}
		if previous > len(text) {
			previous = len(text)
		}
		if previous == len(text) {
			return previous
		}
		chunk := text[previous:]
		if strings.TrimSpace(chunk) == "" {
			return len(text)
		}
		d.OnOutput(taskID, meta.step, meta.host, stream, chunk)
		return len(text)
	}

	stdout := ""
	if raw, ok := output["stdout"]; ok {
		stdout = fmt.Sprint(raw)
	}
	stderr := ""
	if raw, ok := output["stderr"]; ok {
		stderr = fmt.Sprint(raw)
	}

	nextStdout := emit("stdout", stdout, offsets.stdout)
	nextStderr := emit("stderr", stderr, offsets.stderr)

	d.outputMu.Lock()
	d.outputOffsets[taskID] = outputOffset{stdout: nextStdout, stderr: nextStderr}
	d.outputMu.Unlock()
}

func (d *HybridDispatcher) setTaskMeta(taskID, runID, step, action, host, hostAddress, baseURL, token string, heartbeat, remote bool) {
	if strings.TrimSpace(taskID) == "" {
		return
	}
	d.taskMetaMu.Lock()
	d.taskMeta[taskID] = taskMeta{
		runID:       runID,
		step:        step,
		action:      action,
		host:        host,
		hostAddress: hostAddress,
		baseURL:     baseURL,
		token:       token,
		heartbeat:   heartbeat,
		remote:      remote,
	}
	d.taskMetaMu.Unlock()
}

func (d *HybridDispatcher) getTaskMeta(taskID string) taskMeta {
	d.taskMetaMu.Lock()
	defer d.taskMetaMu.Unlock()
	return d.taskMeta[taskID]
}

func (d *HybridDispatcher) clearTaskMeta(taskID string) {
	d.outputMu.Lock()
	delete(d.outputOffsets, taskID)
	d.outputMu.Unlock()

	d.taskMetaMu.Lock()
	delete(d.taskMeta, taskID)
	d.taskMetaMu.Unlock()
}

func (d *HybridDispatcher) fetchStatus(ctx context.Context, baseURL, taskID, token string, timeout time.Duration) (Result, error) {
	meta := d.getTaskMeta(taskID)
	path := strings.TrimSpace(d.StatusPath)
	if path == "" {
		path = "/status"
	}
	url := strings.TrimRight(baseURL, "/") + path
	payload := struct {
		TaskID string `json:"task_id"`
	}{TaskID: taskID}

	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token = strings.TrimSpace(token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Runner-Token", token)
		req.Header.Set("X-Agent-Auth", token)
	}
	for k, v := range d.Headers {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		req.Header.Set(k, v)
	}
	logAgentRequestAuth("agent status request auth", token, req,
		zap.String("run_id", meta.runID),
		zap.String("task_id", taskID),
		zap.String("step", meta.step),
		zap.String("action", meta.action),
		zap.String("host", meta.host),
		zap.String("resolved_address", baseURL),
		zap.String("url", url),
	)

	client := d.clientWithTimeout(timeout, 30*time.Second)
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		logging.L().Warn("agent status transport failed",
			withAgentAuthFields([]zap.Field{
				zap.String("run_id", meta.runID),
				zap.String("task_id", taskID),
				zap.String("step", meta.step),
				zap.String("action", meta.action),
				zap.String("host", meta.host),
				zap.String("resolved_address", baseURL),
				zap.String("url", url),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err),
			}, token, req)...,
		)
		return remoteFailedResultFromMeta(meta, taskID, baseURL, 0, "status", err.Error()), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := readLimitedBody(resp.Body)
		errMsg := fmt.Sprintf("agent status failed: %s", resp.Status)
		if body != "" {
			errMsg = fmt.Sprintf("agent status failed: %s (%s)", resp.Status, body)
		}
		logging.L().Warn("agent status rejected",
			withAgentAuthFields([]zap.Field{
				zap.String("run_id", meta.runID),
				zap.String("task_id", taskID),
				zap.String("step", meta.step),
				zap.String("action", meta.action),
				zap.String("host", meta.host),
				zap.String("resolved_address", baseURL),
				zap.String("url", url),
				zap.Int("status_code", resp.StatusCode),
				zap.String("body", body),
				zap.Duration("duration", time.Since(start)),
			}, token, req)...,
		)
		return remoteFailedResultFromMeta(meta, taskID, baseURL, 0, "status", errMsg), fmt.Errorf("%s", errMsg)
	}

	var decoded struct {
		Result Result `json:"result"`
		Error  string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		logging.L().Warn("agent status decode failed",
			zap.String("run_id", meta.runID),
			zap.String("task_id", taskID),
			zap.String("step", meta.step),
			zap.String("action", meta.action),
			zap.String("host", meta.host),
			zap.String("resolved_address", baseURL),
			zap.Duration("duration", time.Since(start)),
			zap.Error(err),
		)
		return remoteFailedResultFromMeta(meta, taskID, baseURL, 0, "status", err.Error()), err
	}
	if decoded.Result.TaskID == "" {
		decoded.Result.TaskID = taskID
	}
	decoded.Result = attachRunnerDebug(decoded.Result, runnerDebug{
		Mode:             "remote",
		TaskID:           decoded.Result.TaskID,
		RunID:            meta.runID,
		Step:             meta.step,
		Action:           meta.action,
		Host:             meta.host,
		HostAddress:      meta.hostAddress,
		ResolvedAddress:  baseURL,
		HeartbeatEnabled: meta.heartbeat,
		Phase:            "status",
	})
	if decoded.Error != "" {
		decoded.Result.Status = "failed"
		decoded.Result.Error = decoded.Error
		logging.L().Warn("agent status returned error",
			zap.String("run_id", meta.runID),
			zap.String("task_id", decoded.Result.TaskID),
			zap.String("step", meta.step),
			zap.String("action", meta.action),
			zap.String("host", meta.host),
			zap.String("resolved_address", baseURL),
			zap.String("result_error", decoded.Result.Error),
			zap.Duration("duration", time.Since(start)),
		)
		return decoded.Result, fmt.Errorf("%s", decoded.Error)
	}
	if strings.EqualFold(strings.TrimSpace(decoded.Result.Status), "failed") && strings.TrimSpace(decoded.Result.Error) == "" {
		decoded.Result.Error = "agent returned failed status without error during status polling"
		logging.L().Warn("agent status returned failed status without error",
			zap.String("run_id", meta.runID),
			zap.String("task_id", decoded.Result.TaskID),
			zap.String("step", meta.step),
			zap.String("action", meta.action),
			zap.String("host", meta.host),
			zap.String("resolved_address", baseURL),
			zap.Duration("duration", time.Since(start)),
		)
	} else if strings.EqualFold(strings.TrimSpace(decoded.Result.Status), "failed") {
		logging.L().Warn("agent status reported failed status",
			zap.String("run_id", meta.runID),
			zap.String("task_id", decoded.Result.TaskID),
			zap.String("step", meta.step),
			zap.String("action", meta.action),
			zap.String("host", meta.host),
			zap.String("resolved_address", baseURL),
			zap.String("result_error", decoded.Result.Error),
			zap.Duration("duration", time.Since(start)),
		)
	}
	logging.L().Info("agent status completed",
		zap.String("run_id", meta.runID),
		zap.String("task_id", decoded.Result.TaskID),
		zap.String("step", meta.step),
		zap.String("action", meta.action),
		zap.String("host", meta.host),
		zap.String("resolved_address", baseURL),
		zap.String("result_status", decoded.Result.Status),
		zap.Duration("duration", time.Since(start)),
	)
	return decoded.Result, nil
}

func (d *HybridDispatcher) clientWithTimeout(timeout, fallback time.Duration) *http.Client {
	if d.Client != nil {
		return d.Client
	}
	if timeout <= 0 {
		timeout = fallback
	}
	return &http.Client{Timeout: timeout}
}

func logAgentRequestAuth(message, resolvedToken string, req *http.Request, fields ...zap.Field) {
	logging.L().Info(message, withAgentAuthFields(fields, resolvedToken, req)...)
}

func withAgentAuthFields(fields []zap.Field, resolvedToken string, req *http.Request) []zap.Field {
	return append(fields, agentAuthFields(resolvedToken, req)...)
}

func agentAuthFields(resolvedToken string, req *http.Request) []zap.Field {
	authHeader := ""
	runnerToken := ""
	agentAuth := ""
	if req != nil {
		authHeader = strings.TrimSpace(req.Header.Get("Authorization"))
		runnerToken = strings.TrimSpace(req.Header.Get("X-Runner-Token"))
		agentAuth = strings.TrimSpace(req.Header.Get("X-Agent-Auth"))
	}
	resolvedToken = strings.TrimSpace(resolvedToken)
	return []zap.Field{
		zap.Bool("token_present", resolvedToken != "" || authHeader != "" || runnerToken != "" || agentAuth != ""),
		zap.String("resolved_token", resolvedToken),
		zap.String("authorization_header", authHeader),
		zap.String("authorization_token", trimBearerToken(authHeader)),
		zap.String("x_runner_token", runnerToken),
		zap.String("x_agent_auth", agentAuth),
	}
}

func trimBearerToken(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= len("Bearer ") && strings.EqualFold(value[:len("Bearer ")], "Bearer ") {
		return strings.TrimSpace(value[len("Bearer "):])
	}
	return value
}

func (d *HybridDispatcher) resolveBaseURL(task Task) string {
	addr := strings.TrimSpace(task.Host.Address)
	if addr != "" && !strings.EqualFold(addr, "local") {
		return normalizeAgentURL(addr)
	}
	if name := strings.TrimSpace(task.Host.Name); name != "" && !strings.EqualFold(name, "local") {
		return normalizeAgentURL(name)
	}
	return normalizeAgentURL(d.BaseURL)
}

func normalizeAgentURL(raw string) string {
	addr := strings.TrimSpace(raw)
	if addr == "" {
		return ""
	}
	lower := strings.ToLower(addr)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return addr
	}
	return "http://" + addr
}

func (d *HybridDispatcher) persistResult(result Result) {
	if strings.TrimSpace(result.TaskID) == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.results[result.TaskID] = result
}

func readLimitedBody(reader io.Reader) string {
	if reader == nil {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(reader, 2048))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

func sleepWithContextFor(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func attachRunnerDebug(result Result, debug runnerDebug) Result {
	if strings.TrimSpace(result.TaskID) == "" {
		result.TaskID = debug.TaskID
	}
	if result.Output == nil {
		result.Output = map[string]any{}
	}
	result.Output[runnerDebugKey] = map[string]any{
		"mode":              strings.TrimSpace(debug.Mode),
		"task_id":           strings.TrimSpace(debug.TaskID),
		"run_id":            strings.TrimSpace(debug.RunID),
		"step":              strings.TrimSpace(debug.Step),
		"action":            strings.TrimSpace(debug.Action),
		"host":              strings.TrimSpace(debug.Host),
		"host_address":      strings.TrimSpace(debug.HostAddress),
		"resolved_address":  strings.TrimSpace(debug.ResolvedAddress),
		"heartbeat_enabled": debug.HeartbeatEnabled,
		"attempt":           debug.Attempt,
		"phase":             strings.TrimSpace(debug.Phase),
	}
	return result
}

func readString(vars map[string]any, key string) (string, bool) {
	raw, ok := readVar(vars, key)
	if !ok || raw == nil {
		return "", false
	}
	value := strings.TrimSpace(fmt.Sprint(raw))
	if value == "" {
		return "", false
	}
	return value, true
}

func readBool(vars map[string]any, key string) (bool, bool) {
	raw, ok := readVar(vars, key)
	if !ok || raw == nil {
		return false, false
	}
	switch value := raw.(type) {
	case bool:
		return value, true
	case string:
		trimmed := strings.ToLower(strings.TrimSpace(value))
		switch trimmed {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}

func readInt(vars map[string]any, key string) (int, bool) {
	raw, ok := readVar(vars, key)
	if !ok || raw == nil {
		return 0, false
	}
	switch value := raw.(type) {
	case int:
		return value, true
	case int8:
		return int(value), true
	case int16:
		return int(value), true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case uint:
		return int(value), true
	case uint8:
		return int(value), true
	case uint16:
		return int(value), true
	case uint32:
		return int(value), true
	case uint64:
		return int(value), true
	case float32:
		return int(value), true
	case float64:
		return int(value), true
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 0, false
		}
		var parsed int
		if _, err := fmt.Sscanf(trimmed, "%d", &parsed); err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func readVar(vars map[string]any, key string) (any, bool) {
	if len(vars) == 0 {
		return nil, false
	}
	if raw, ok := vars[key]; ok {
		return raw, true
	}
	env, ok := vars["env"]
	if !ok || env == nil {
		return nil, false
	}
	switch typed := env.(type) {
	case map[string]any:
		raw, ok := typed[key]
		return raw, ok
	case map[string]string:
		raw, ok := typed[key]
		if !ok {
			return nil, false
		}
		return raw, true
	case map[any]any:
		for nestedKey, nestedValue := range typed {
			if strings.TrimSpace(fmt.Sprint(nestedKey)) == key {
				return nestedValue, true
			}
		}
	}
	return nil, false
}
