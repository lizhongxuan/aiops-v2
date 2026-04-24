// Package client 提供 runner-web 的 Go SDK，封装了提交工作流、轮询状态、
// YAML host 注入、环境变量构造等常用操作，让调用方无需关心 HTTP 细节。
//
// 用法示例:
//
//	c := client.New("http://127.0.0.1:8090", "your-token")
//	result, err := c.SubmitAndWait(ctx, yamlText, client.RunOptions{
//	    Env:         map[string]string{"BACKUP_DIR": "/data/backup"},
//	    Hosts:       map[string]string{"backup-host": "http://10.0.0.5:9990"},
//	    TriggeredBy: "pg-backup",
//	})
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"runner/workflow"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// 默认值
// ---------------------------------------------------------------------------

const (
	DefaultPollInterval = 5 * time.Second
	DefaultTimeout      = 24 * time.Hour
	DefaultSubmitTimeout = 30 * time.Second
	DefaultPollReqTimeout = 15 * time.Second
)

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

// Client 是 runner-web 的 HTTP 客户端
type Client struct {
	baseURL      string
	token        string
	pollInterval time.Duration
	timeout      time.Duration
	httpClient   *http.Client
}

// Option 用于配置 Client
type Option func(*Client)

// WithPollInterval 设置轮询间隔
func WithPollInterval(d time.Duration) Option {
	return func(c *Client) { c.pollInterval = d }
}

// WithTimeout 设置最大等待时间
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}

// WithHTTPClient 设置自定义 http.Client
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// New 创建一个 runner-web 客户端
func New(baseURL, token string, opts ...Option) *Client {
	c := &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		token:        token,
		pollInterval: DefaultPollInterval,
		timeout:      DefaultTimeout,
		httpClient:   &http.Client{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ---------------------------------------------------------------------------
// 请求/响应结构
// ---------------------------------------------------------------------------

// RunOptions 提交工作流时的选项
type RunOptions struct {
	// Env 环境变量，会自动包装为 {"env": {...}} 注入到 workflow vars
	Env map[string]string
	// Hosts 需要注入到 YAML inventory.hosts 中的主机地址，key=hostName, value=address
	Hosts map[string]string
	// TriggeredBy 触发来源标识
	TriggeredBy string
	// IdempotencyKey 幂等键（可选）
	IdempotencyKey string
	// ExtraVars 额外的 vars（会与 env 合并，env 优先）
	ExtraVars map[string]any
}

// RunResult 运行结果
type RunResult struct {
	RunID   string `json:"run_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// submitResponse POST /api/v1/runs 的响应
type submitResponse struct {
	RunID        string `json:"run_id"`
	Status       string `json:"status"`
	WorkflowName string `json:"workflow_name"`
}

// runDetail GET /api/v1/runs/{id} 的响应
type runDetail struct {
	RunID   string `json:"run_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// 核心方法
// ---------------------------------------------------------------------------

// Submit 提交工作流并立即返回 run_id，不等待完成
func (c *Client) Submit(ctx context.Context, yamlText string, opts RunOptions) (string, error) {
	// 注入 hosts
	patched := injectHosts(yamlText, opts.Hosts)

	// 构造 vars
	vars := buildVars(opts)

	body := map[string]any{
		"workflow_yaml": patched,
		"vars":          vars,
		"triggered_by":  opts.TriggeredBy,
	}
	if opts.IdempotencyKey != "" {
		body["idempotency_key"] = opts.IdempotencyKey
	}

	resp, err := c.doPost(ctx, "/api/v1/runs", body)
	if err != nil {
		return "", fmt.Errorf("submit: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("submit: status=%d body=%s", resp.StatusCode, string(raw))
	}

	var sr submitResponse
	if err := json.Unmarshal(raw, &sr); err != nil {
		return "", fmt.Errorf("submit: parse response: %w", err)
	}
	if strings.TrimSpace(sr.RunID) == "" {
		return "", fmt.Errorf("submit: empty run_id in response")
	}
	return sr.RunID, nil
}

// Wait 等待指定 run 到达终态
func (c *Client) Wait(ctx context.Context, runID string) (*RunResult, error) {
	deadline := time.Now().Add(c.timeout)
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context canceled while waiting for run %s", runID)
		default:
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for run %s after %v", runID, c.timeout)
		}

		result, done, err := c.pollOnce(ctx, runID)
		if err != nil {
			// 网络错误等，继续重试
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-ticker.C:
			}
			continue
		}
		if done {
			return result, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// SubmitAndWait 提交工作流并同步等待完成。成功返回 RunResult，失败返回 error。
// 这是最常用的方法，一行搞定。
func (c *Client) SubmitAndWait(ctx context.Context, yamlText string, opts RunOptions) (*RunResult, error) {
	runID, err := c.Submit(ctx, yamlText, opts)
	if err != nil {
		return nil, err
	}
	result, err := c.Wait(ctx, runID)
	if err != nil {
		return nil, err
	}
	if result.Status != "success" {
		return result, fmt.Errorf("run %s %s: %s", runID, result.Status, result.Message)
	}
	return result, nil
}

// GetRun 获取运行详情
func (c *Client) GetRun(ctx context.Context, runID string) (*RunResult, error) {
	resp, err := c.doGet(ctx, "/api/v1/runs/"+runID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get run: status=%d body=%s", resp.StatusCode, string(raw))
	}

	var detail runDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return nil, fmt.Errorf("get run: parse: %w", err)
	}
	return &RunResult{
		RunID:   detail.RunID,
		Status:  detail.Status,
		Message: detail.Message,
	}, nil
}

// CancelRun 取消运行
func (c *Client) CancelRun(ctx context.Context, runID string) error {
	resp, err := c.doPost(ctx, "/api/v1/runs/"+runID+"/cancel", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cancel run: status=%d body=%s", resp.StatusCode, string(raw))
	}
	return nil
}

// ---------------------------------------------------------------------------
// 内部方法
// ---------------------------------------------------------------------------

func (c *Client) pollOnce(ctx context.Context, runID string) (*RunResult, bool, error) {
	resp, err := c.doGet(ctx, "/api/v1/runs/"+runID)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("poll: status=%d", resp.StatusCode)
	}

	var detail runDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return nil, false, fmt.Errorf("poll: parse: %w", err)
	}

	result := &RunResult{
		RunID:   detail.RunID,
		Status:  detail.Status,
		Message: detail.Message,
	}

	switch detail.Status {
	case "success", "failed", "canceled", "interrupted":
		return result, true, nil
	default:
		return result, false, nil
	}
}

func (c *Client) doPost(ctx context.Context, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = strings.NewReader(string(data))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

func (c *Client) doGet(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

// ---------------------------------------------------------------------------
// 工具函数（公开，调用方也可以单独使用）
// ---------------------------------------------------------------------------

// InjectHosts 将多个 host 地址注入到 YAML 的 inventory.hosts 中。
// 解析失败时原样返回。
func InjectHosts(yamlText string, hosts map[string]string) string {
	return injectHosts(yamlText, hosts)
}

func injectHosts(yamlText string, hosts map[string]string) string {
	if len(hosts) == 0 {
		return yamlText
	}
	wf, err := workflow.Load([]byte(yamlText))
	if err != nil {
		return yamlText
	}
	if wf.Inventory.Hosts == nil {
		wf.Inventory.Hosts = make(map[string]workflow.Host)
	}
	for name, addr := range hosts {
		wf.Inventory.Hosts[name] = workflow.Host{Address: addr}
	}
	data, err := yaml.Marshal(wf)
	if err != nil {
		return yamlText
	}
	return string(data)
}

// BuildEnvVars 将 string map 包装为 runner-web 需要的 {"env": {...}} 结构。
func BuildEnvVars(env map[string]string) map[string]any {
	m := make(map[string]any, len(env))
	for k, v := range env {
		m[k] = v
	}
	return map[string]any{"env": m}
}

func buildVars(opts RunOptions) map[string]any {
	vars := make(map[string]any)
	// 先放 extra vars
	for k, v := range opts.ExtraVars {
		vars[k] = v
	}
	// env 覆盖
	if len(opts.Env) > 0 {
		env := make(map[string]any, len(opts.Env))
		for k, v := range opts.Env {
			env[k] = v
		}
		vars["env"] = env
	}
	return vars
}
