package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/agentrpc"
	"aiops-v2/internal/hostagent"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
	"runner/modules"
	"runner/modules/script"
	"runner/scheduler"
)

const agentVersion = "v0.1.0"

type runRequest struct {
	Task scheduler.Task `json:"task"`
	Wait *bool          `json:"wait,omitempty"`
}

type runResponse struct {
	Result scheduler.Result `json:"result"`
	RunID  string           `json:"run_id,omitempty"`
	Error  string           `json:"error,omitempty"`
}

type hostSystemInfo struct {
	OS            string
	Arch          string
	OSRelease     string
	KernelVersion string
	CPUCores      int
	MemoryBytes   uint64
}

type hostAgentEventPayloadInput struct {
	HostID        string
	Hostname      string
	ListenAddress string
	Capabilities  []string
	Labels        map[string]string
	System        hostSystemInfo
	Registration  bool
	Timestamp     string
}

type statusRequest struct {
	TaskID string `json:"task_id"`
}

type controlMessage struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
	Time    int64           `json:"time"`
}

type agentExecRequest struct {
	Command        string   `json:"command"`
	Args           []string `json:"args,omitempty"`
	WorkingDir     string   `json:"workingDir,omitempty"`
	TimeoutMs      int      `json:"timeoutMs,omitempty"`
	MaxOutputBytes int      `json:"maxOutputBytes,omitempty"`
}

type agentExecResponse struct {
	Status   string `json:"status"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exitCode,omitempty"`
	Error    string `json:"error,omitempty"`
}

type taskEntry struct {
	Task       scheduler.Task
	Result     scheduler.Result
	Done       bool
	StartedAt  time.Time
	FinishedAt time.Time
	Cancel     context.CancelFunc
	Stdout     *outputBuffer
	Stderr     *outputBuffer
}

type outputBuffer struct {
	mu      sync.Mutex
	maxSize int
	data    []byte
}

type agentOptions struct {
	AsyncThreshold time.Duration
	MaxOutputBytes int
}

type agentDiagnostics struct {
	mu        sync.Mutex
	startedAt time.Time
	cfg       hostagent.Config
	register  agentDiagnosticCheck
	heartbeat agentDiagnosticCheck
	grpc      agentDiagnosticCheck
}

type agentDiagnosticCheck struct {
	LastAttemptAt      time.Time
	LastSuccessAt      time.Time
	LastErrorAt        time.Time
	LastError          string
	LastCategory       string
	LastStatusCode     int
	LastLatency        time.Duration
	ConsecutiveFailure int
}

type agentDiagnosticsSnapshot struct {
	Status          string                       `json:"status"`
	HostID          string                       `json:"host_id"`
	Version         string                       `json:"version"`
	ConnectionMode  string                       `json:"connection_mode"`
	ListenAddr      string                       `json:"listen_addr"`
	ServerURL       string                       `json:"server_url,omitempty"`
	GRPCURL         string                       `json:"grpc_url,omitempty"`
	TokenConfigured bool                         `json:"token_configured"`
	Capabilities    []string                     `json:"capabilities,omitempty"`
	StartedAt       string                       `json:"started_at"`
	UptimeMs        int64                        `json:"uptime_ms"`
	ConfigWarnings  []string                     `json:"config_warnings,omitempty"`
	Register        agentDiagnosticCheckSnapshot `json:"register"`
	Heartbeat       agentDiagnosticCheckSnapshot `json:"heartbeat"`
	GRPC            agentDiagnosticCheckSnapshot `json:"grpc"`
}

type agentDiagnosticCheckSnapshot struct {
	LastAttemptAt       string `json:"last_attempt_at,omitempty"`
	LastSuccessAt       string `json:"last_success_at,omitempty"`
	LastErrorAt         string `json:"last_error_at,omitempty"`
	LastError           string `json:"last_error,omitempty"`
	LastCategory        string `json:"last_category,omitempty"`
	LastStatusCode      int    `json:"last_status_code,omitempty"`
	LastLatencyMs       int64  `json:"last_latency_ms,omitempty"`
	ConsecutiveFailures int    `json:"consecutive_failures,omitempty"`
}

type agentHTTPStatusError struct {
	Path       string
	StatusCode int
}

func (e *agentHTTPStatusError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s returned status %d", e.Path, e.StatusCode)
}

func newOutputBuffer(maxSize int) *outputBuffer {
	return &outputBuffer{maxSize: maxSize}
}

func (b *outputBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if b.maxSize > 0 && len(b.data) > b.maxSize {
		b.data = b.data[len(b.data)-b.maxSize:]
	}
	return len(p), nil
}

func (b *outputBuffer) String() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}

func newAgentDiagnostics(cfg hostagent.Config) *agentDiagnostics {
	return &agentDiagnostics{
		startedAt: time.Now().UTC(),
		cfg:       cfg,
	}
}

func resolveAgentDiagnostics(cfg hostagent.Config, diagnostics []*agentDiagnostics) *agentDiagnostics {
	if len(diagnostics) > 0 && diagnostics[0] != nil {
		return diagnostics[0]
	}
	return newAgentDiagnostics(cfg)
}

func (d *agentDiagnostics) recordSuccess(name string, started time.Time, statusCode int) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	check := d.checkLocked(name)
	if check == nil {
		return
	}
	now := time.Now().UTC()
	check.LastAttemptAt = started.UTC()
	check.LastSuccessAt = now
	check.LastError = ""
	check.LastCategory = ""
	check.LastStatusCode = statusCode
	check.LastLatency = now.Sub(started)
	check.ConsecutiveFailure = 0
}

func (d *agentDiagnostics) recordFailure(name string, started time.Time, err error) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	check := d.checkLocked(name)
	if check == nil {
		return
	}
	now := time.Now().UTC()
	check.LastAttemptAt = started.UTC()
	check.LastErrorAt = now
	check.LastError = strings.TrimSpace(errString(err))
	check.LastCategory = classifyAgentDiagnosticError(err)
	check.LastStatusCode = agentDiagnosticStatusCode(err)
	check.LastLatency = now.Sub(started)
	check.ConsecutiveFailure++
}

func (d *agentDiagnostics) checkLocked(name string) *agentDiagnosticCheck {
	switch name {
	case "register":
		return &d.register
	case "heartbeat":
		return &d.heartbeat
	case "grpc":
		return &d.grpc
	default:
		return nil
	}
}

func (d *agentDiagnostics) snapshot() agentDiagnosticsSnapshot {
	if d == nil {
		return agentDiagnosticsSnapshot{Status: "ok", Version: agentVersion}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now().UTC()
	return agentDiagnosticsSnapshot{
		Status:          "ok",
		HostID:          d.cfg.HostID,
		Version:         agentVersion,
		ConnectionMode:  hostagent.NormalizeConnectionMode(d.cfg.ConnectionMode, d.cfg.ServerURL, d.cfg.GRPCURL),
		ListenAddr:      d.cfg.ListenAddr,
		ServerURL:       d.cfg.ServerURL,
		GRPCURL:         d.cfg.GRPCURL,
		TokenConfigured: strings.TrimSpace(d.cfg.Token) != "",
		Capabilities:    append([]string(nil), d.cfg.Capabilities...),
		StartedAt:       formatDiagnosticTime(d.startedAt),
		UptimeMs:        now.Sub(d.startedAt).Milliseconds(),
		ConfigWarnings:  agentDiagnosticConfigWarnings(d.cfg),
		Register:        snapshotDiagnosticCheck(d.register),
		Heartbeat:       snapshotDiagnosticCheck(d.heartbeat),
		GRPC:            snapshotDiagnosticCheck(d.grpc),
	}
}

func (d *agentDiagnostics) lastBeatTime() time.Time {
	if d == nil {
		return time.Now().UTC()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.heartbeat.LastSuccessAt.IsZero() {
		return d.heartbeat.LastSuccessAt
	}
	if !d.register.LastSuccessAt.IsZero() {
		return d.register.LastSuccessAt
	}
	if !d.startedAt.IsZero() {
		return d.startedAt
	}
	return time.Now().UTC()
}

func snapshotDiagnosticCheck(check agentDiagnosticCheck) agentDiagnosticCheckSnapshot {
	return agentDiagnosticCheckSnapshot{
		LastAttemptAt:       formatDiagnosticTime(check.LastAttemptAt),
		LastSuccessAt:       formatDiagnosticTime(check.LastSuccessAt),
		LastErrorAt:         formatDiagnosticTime(check.LastErrorAt),
		LastError:           check.LastError,
		LastCategory:        check.LastCategory,
		LastStatusCode:      check.LastStatusCode,
		LastLatencyMs:       check.LastLatency.Milliseconds(),
		ConsecutiveFailures: check.ConsecutiveFailure,
	}
}

func formatDiagnosticTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func agentDiagnosticStatusCode(err error) int {
	var statusErr *agentHTTPStatusError
	if errors.As(err, &statusErr) && statusErr != nil {
		return statusErr.StatusCode
	}
	return 0
}

func classifyAgentDiagnosticError(err error) string {
	if err == nil {
		return ""
	}
	statusCode := agentDiagnosticStatusCode(err)
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return "unauthorized"
	case statusCode == http.StatusNotFound:
		return "not_found"
	case statusCode >= 500:
		return "server_error"
	case statusCode > 0:
		return "http_error"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "connection refused"):
		return "tcp_refused"
	case strings.Contains(message, "no such host"):
		return "dns_failed"
	case strings.Contains(message, "network is unreachable"):
		return "network_unreachable"
	case strings.Contains(message, "i/o timeout"), strings.Contains(message, "deadline exceeded"):
		return "timeout"
	default:
		return "network_error"
	}
}

func agentDiagnosticConfigWarnings(cfg hostagent.Config) []string {
	var warnings []string
	if hostagent.NormalizeConnectionMode(cfg.ConnectionMode, cfg.ServerURL, cfg.GRPCURL) != hostagent.ConnectionModeNodePushGRPC {
		return warnings
	}
	if host := parsedURLHost(cfg.ServerURL); isLoopbackHost(host) {
		warnings = append(warnings, "server_url_loopback")
	}
	if serverURLLooksLikeNodeEndpoint(cfg.ServerURL) {
		warnings = append(warnings, "server_url_looks_like_node_endpoint")
	}
	return warnings
}

func serverURLLooksLikeNodeEndpoint(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return false
	}
	if u.Port() == "7072" {
		return true
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	return path == "/exec" || path == "/run" || path == "/health" || path == "/diagnostics" || strings.HasPrefix(path, "/terminal")
}

func parsedURLHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return ""
	}
	return u.Hostname()
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func main() {
	fs := flag.NewFlagSet("host-agent", flag.ExitOnError)
	configPath := fs.String("config", "/etc/aiops/host-agent.yaml", "host-agent config path")
	asyncThresholdSec := fs.Int("async-threshold-sec", 4, "auto async threshold in seconds when wait is omitted")
	defaultMaxOutputBytes := fs.Int("max-output-bytes", 65536, "default max stdout/stderr bytes kept in memory")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cfg, err := hostagent.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	opts := agentOptions{
		AsyncThreshold: time.Duration(*asyncThresholdSec) * time.Second,
		MaxOutputBytes: *defaultMaxOutputBytes,
	}
	if opts.AsyncThreshold <= 0 {
		opts.AsyncThreshold = 4 * time.Second
	}
	if opts.MaxOutputBytes <= 0 {
		opts.MaxOutputBytes = 65536
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := runHostAgent(ctx, cfg, opts, &http.Client{Timeout: 10 * time.Second}, os.Stderr, nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type hostAgentServeFunc func(http.Handler) error

func runHostAgent(ctx context.Context, cfg hostagent.Config, opts agentOptions, client *http.Client, stderr io.Writer, serve hostAgentServeFunc) error {
	cfg.ConnectionMode = hostagent.NormalizeConnectionMode(cfg.ConnectionMode, cfg.ServerURL, cfg.GRPCURL)
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if serve == nil {
		serve = func(handler http.Handler) error {
			return http.ListenAndServe(cfg.ListenAddr, handler)
		}
	}
	diagnostics := newAgentDiagnostics(cfg)
	if cfg.ConnectionMode == hostagent.ConnectionModeNodePushGRPC {
		if err := register(ctx, client, cfg, diagnostics); err != nil {
			fmt.Fprintf(stderr, "register host-agent: %v\n", err)
		}
		go heartbeatLoop(ctx, client, cfg, diagnostics)
		if strings.TrimSpace(cfg.GRPCURL) != "" {
			go grpcControlLoop(ctx, cfg, opts, diagnostics)
		}
	}

	fmt.Fprintf(stderr, "host-agent listening on %s\n", cfg.ListenAddr)
	return serve(newAgentHandler(cfg, opts, diagnostics))
}

func grpcControlLoop(ctx context.Context, cfg hostagent.Config, opts agentOptions, diagnostics *agentDiagnostics) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		started := time.Now().UTC()
		if err := runGRPCControlSession(ctx, cfg, opts); err != nil && ctx.Err() == nil {
			diagnostics.recordFailure("grpc", started, err)
			fmt.Fprintf(os.Stderr, "host-agent grpc control: %v\n", err)
		} else if ctx.Err() == nil {
			diagnostics.recordSuccess("grpc", started, 0)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
}

func runGRPCControlSession(ctx context.Context, cfg hostagent.Config, opts agentOptions) error {
	dialCtx, dialCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dialCancel()
	conn, err := grpc.DialContext(dialCtx, strings.TrimSpace(cfg.GRPCURL), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return err
	}
	defer conn.Close()

	stream, err := agentrpc.NewAgentServiceClient(conn).Connect(ctx)
	if err != nil {
		return err
	}
	sendMu := &sync.Mutex{}
	send := func(msg controlMessage) error {
		msg.Time = time.Now().UnixMilli()
		envelope, err := controlMessageToStruct(msg)
		if err != nil {
			return err
		}
		sendMu.Lock()
		defer sendMu.Unlock()
		return stream.Send(envelope)
	}
	hostname, _ := os.Hostname()
	registerPayloadMap := buildHostAgentEventPayload(hostAgentEventPayloadInput{
		HostID:        cfg.HostID,
		Hostname:      hostname,
		ListenAddress: cfg.ListenAddr,
		Capabilities:  cfg.Capabilities,
		Labels:        cfg.Labels,
		System:        collectHostSystemInfo(),
		Registration:  true,
	})
	registerPayloadMap["token"] = cfg.Token
	registerPayload, _ := json.Marshal(registerPayloadMap)
	if err := send(controlMessage{Type: "register", ID: cfg.HostID, Payload: registerPayload}); err != nil {
		return err
	}

	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go func() {
		ticker := time.NewTicker(cfg.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				payload, _ := json.Marshal(buildHostAgentEventPayload(hostAgentEventPayloadInput{
					HostID:       cfg.HostID,
					Capabilities: cfg.Capabilities,
					System:       collectHostSystemInfo(),
					Timestamp:    time.Now().UTC().Format(time.RFC3339),
				}))
				if err := send(controlMessage{Type: "heartbeat", ID: cfg.HostID, Payload: payload}); err != nil {
					fmt.Fprintf(os.Stderr, "host-agent grpc heartbeat: %v\n", err)
					return
				}
			}
		}
	}()

	for {
		envelope, err := stream.Recv()
		if err != nil {
			return err
		}
		msg, err := structToControlMessage(envelope)
		if err != nil {
			_ = send(controlMessage{Type: "error", Error: err.Error()})
			continue
		}
		switch msg.Type {
		case "ack":
			continue
		case "exec":
			go handleGRPCExec(ctx, opts, msg, send)
		default:
			_ = send(controlMessage{Type: "error", ID: msg.ID, Error: "unknown message type: " + msg.Type})
		}
	}
}

func handleGRPCExec(ctx context.Context, opts agentOptions, msg controlMessage, send func(controlMessage) error) {
	var req agentExecRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		_ = sendExecResponse(send, msg.ID, agentExecResponse{Status: "failed", ExitCode: -1, Error: err.Error()})
		return
	}
	result := runLocalExecCommand(ctx, req, opts.MaxOutputBytes)
	_ = sendExecResponse(send, msg.ID, result)
}

func sendExecResponse(send func(controlMessage) error, id string, result agentExecResponse) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return send(controlMessage{Type: "exec", ID: id, Payload: data, Error: result.Error})
}

func runLocalExecCommand(ctx context.Context, req agentExecRequest, fallbackMaxOutputBytes int) agentExecResponse {
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return agentExecResponse{Status: "failed", ExitCode: -1, Error: "command is required"}
	}
	timeout := 15 * time.Second
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
		if timeout > 60*time.Second {
			timeout = 60 * time.Second
		}
	}
	maxOutputBytes := req.MaxOutputBytes
	if maxOutputBytes <= 0 {
		maxOutputBytes = fallbackMaxOutputBytes
	}
	if maxOutputBytes <= 0 {
		maxOutputBytes = 65536
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, command, req.Args...)
	if strings.TrimSpace(req.WorkingDir) != "" {
		cmd.Dir = strings.TrimSpace(req.WorkingDir)
	}
	stdout := newOutputBuffer(maxOutputBytes)
	stderr := newOutputBuffer(maxOutputBytes / 2)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	result := agentExecResponse{
		Status:   "success",
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if runCtx.Err() != nil {
		result.Status = "failed"
		result.ExitCode = -1
		result.Error = runCtx.Err().Error()
		return result
	}
	if err != nil {
		result.Status = "failed"
		if result.ExitCode == 0 {
			result.ExitCode = -1
		}
		result.Error = err.Error()
	}
	return result
}

func controlMessageToStruct(msg controlMessage) (*structpb.Struct, error) {
	values := map[string]any{
		"type": msg.Type,
		"id":   msg.ID,
		"time": float64(msg.Time),
	}
	if len(msg.Payload) > 0 {
		values["payload"] = string(msg.Payload)
	}
	if msg.Error != "" {
		values["error"] = msg.Error
	}
	return structpb.NewStruct(values)
}

func structToControlMessage(value *structpb.Struct) (controlMessage, error) {
	if value == nil {
		return controlMessage{}, fmt.Errorf("empty control message")
	}
	fields := value.AsMap()
	msg := controlMessage{
		Type:  stringControlField(fields, "type"),
		ID:    stringControlField(fields, "id"),
		Error: stringControlField(fields, "error"),
	}
	if raw := stringControlField(fields, "payload"); raw != "" {
		if !json.Valid([]byte(raw)) {
			return controlMessage{}, fmt.Errorf("control message payload is not JSON")
		}
		msg.Payload = json.RawMessage(raw)
	}
	if timestamp, ok := fields["time"].(float64); ok && !math.IsNaN(timestamp) {
		msg.Time = int64(timestamp)
	}
	return msg, nil
}

func stringControlField(fields map[string]any, key string) string {
	value, _ := fields[key].(string)
	return value
}

func newAgentHandler(cfg hostagent.Config, opts agentOptions, diagnostics ...*agentDiagnostics) http.Handler {
	diag := resolveAgentDiagnostics(cfg, diagnostics)
	registry := modules.NewRegistry()
	registry.Register("script.shell", script.New("shell"))
	registry.Register("script.python", script.New("python"))

	asyncThreshold := opts.AsyncThreshold
	if asyncThreshold <= 0 {
		asyncThreshold = 4 * time.Second
	}
	defaultMaxOutputBytes := opts.MaxOutputBytes
	if defaultMaxOutputBytes <= 0 {
		defaultMaxOutputBytes = 65536
	}

	var taskMu sync.Mutex
	tasks := map[string]*taskEntry{}
	waitingTokenToTaskID := map[string]string{}

	getTask := func(taskID string) (taskEntry, bool) {
		taskMu.Lock()
		defer taskMu.Unlock()
		entry, ok := tasks[taskID]
		if !ok || entry == nil {
			return taskEntry{}, false
		}
		snapshot := *entry
		snapshot.Result.Output = copyOutput(entry.Result.Output)
		return snapshot, true
	}

	setTask := func(taskID string, entry *taskEntry) {
		taskMu.Lock()
		defer taskMu.Unlock()
		tasks[taskID] = entry
		if wt := strings.TrimSpace(entry.Task.FSMWaitingToken); wt != "" {
			waitingTokenToTaskID[wt] = taskID
		}
	}

	findTaskByWaitingToken := func(waitingToken string) (taskEntry, bool) {
		taskMu.Lock()
		defer taskMu.Unlock()
		taskID, ok := waitingTokenToTaskID[strings.TrimSpace(waitingToken)]
		if !ok {
			return taskEntry{}, false
		}
		entry, ok := tasks[taskID]
		if !ok || entry == nil {
			return taskEntry{}, false
		}
		snapshot := *entry
		snapshot.Result.Output = copyOutput(entry.Result.Output)
		return snapshot, true
	}

	updateTask := func(taskID string, result scheduler.Result, done bool) {
		taskMu.Lock()
		defer taskMu.Unlock()
		entry, ok := tasks[taskID]
		if !ok {
			entry = &taskEntry{}
			tasks[taskID] = entry
		}
		entry.Result = result
		entry.Done = done
		if done {
			entry.FinishedAt = time.Now().UTC()
		}
	}

	cancelTask := func(taskID string) (scheduler.Task, bool) {
		taskMu.Lock()
		defer taskMu.Unlock()
		entry, ok := tasks[taskID]
		if !ok || entry.Done {
			return scheduler.Task{}, false
		}
		if entry.Cancel != nil {
			entry.Cancel()
		}
		entry.Done = true
		entry.FinishedAt = time.Now().UTC()
		entry.Result = scheduler.Result{
			TaskID: taskID,
			Status: "canceled",
			Output: map[string]any{
				"stdout": entry.Stdout.String(),
				"stderr": entry.Stderr.String(),
			},
			Error: "task canceled",
		}
		return entry.Task, true
	}

	writeJSON := func(w http.ResponseWriter, code int, payload any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(payload)
	}

	checkAuth := func(w http.ResponseWriter, r *http.Request) bool {
		required := strings.TrimSpace(cfg.Token)
		if required == "" {
			return true
		}
		auth := bearerToken(r.Header.Get("Authorization"))
		headerToken := strings.TrimSpace(r.Header.Get("X-Runner-Token"))
		if auth == required || headerToken == required {
			return true
		}
		writeJSON(w, http.StatusUnauthorized, runResponse{Error: "unauthorized"})
		return false
	}

	readTaskID := func(r *http.Request) (string, error) {
		taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
		if taskID != "" {
			return taskID, nil
		}
		var req statusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return "", err
		}
		return strings.TrimSpace(req.TaskID), nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/exec", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, agentExecResponse{Status: "failed", Error: "method not allowed", ExitCode: -1})
			return
		}
		if !checkAuth(w, r) {
			return
		}
		var req agentExecRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, agentExecResponse{Status: "failed", Error: err.Error(), ExitCode: -1})
			return
		}
		writeJSON(w, http.StatusOK, runLocalExecCommand(r.Context(), req, defaultMaxOutputBytes))
	})

	mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, runResponse{Error: "method not allowed"})
			return
		}
		if !checkAuth(w, r) {
			return
		}

		var req runRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, runResponse{Error: err.Error()})
			return
		}
		if strings.TrimSpace(req.Task.ID) == "" {
			req.Task.ID = fmt.Sprintf("task-%d", time.Now().UTC().UnixNano())
		}
		if strings.TrimSpace(req.Task.RunID) == "" {
			req.Task.RunID = req.Task.ID
		}
		req.Task.Step.Action = strings.TrimSpace(req.Task.Step.Action)
		if req.Task.Step.Action == "" {
			writeJSON(w, http.StatusBadRequest, runResponse{Error: "task.step.action is required"})
			return
		}

		if waitingToken := strings.TrimSpace(req.Task.FSMWaitingToken); waitingToken != "" {
			if existing, ok := findTaskByWaitingToken(waitingToken); ok {
				if existing.Done {
					writeJSON(w, http.StatusOK, runResponse{Result: existing.Result, RunID: existing.Task.RunID, Error: existing.Result.Error})
				} else {
					writeJSON(w, http.StatusOK, runResponse{Result: scheduler.Result{TaskID: existing.Task.ID, Status: "running"}, RunID: existing.Task.RunID})
				}
				return
			}
		}

		if existing, ok := getTask(req.Task.ID); ok {
			if existing.Done {
				writeJSON(w, http.StatusOK, runResponse{Result: existing.Result, RunID: req.Task.RunID, Error: existing.Result.Error})
			} else {
				writeJSON(w, http.StatusOK, runResponse{Result: scheduler.Result{TaskID: req.Task.ID, Status: "running"}, RunID: req.Task.RunID})
			}
			return
		}

		module, ok := registry.Get(req.Task.Step.Action)
		if !ok {
			writeJSON(w, http.StatusBadRequest, runResponse{Error: fmt.Sprintf("unsupported action: %s", req.Task.Step.Action)})
			return
		}

		outputLimit := resolveOutputLimit(req.Task.Step.Args, defaultMaxOutputBytes)
		runCtx, cancel := context.WithCancel(context.Background())
		entry := &taskEntry{
			Task:      req.Task,
			Result:    scheduler.Result{TaskID: req.Task.ID, Status: "running"},
			StartedAt: time.Now().UTC(),
			Cancel:    cancel,
			Stdout:    newOutputBuffer(outputLimit),
			Stderr:    newOutputBuffer(outputLimit),
		}
		setTask(req.Task.ID, entry)

		doneCh := make(chan scheduler.Result, 1)
		go func() {
			defer cancel()
			res, err := module.Apply(runCtx, modules.Request{
				Step:   req.Task.Step,
				Host:   req.Task.Host,
				Vars:   req.Task.Vars,
				Stdout: entry.Stdout,
				Stderr: entry.Stderr,
			})

			output := copyOutput(res.Output)
			if output == nil {
				output = map[string]any{}
			}
			if _, ok := output["stdout"]; !ok {
				output["stdout"] = entry.Stdout.String()
			}
			if _, ok := output["stderr"]; !ok {
				output["stderr"] = entry.Stderr.String()
			}

			result := scheduler.Result{
				TaskID: req.Task.ID,
				Status: "success",
				Output: output,
			}
			if err != nil {
				if runCtx.Err() != nil {
					result.Status = "canceled"
					result.Error = "task canceled"
				} else {
					result.Status = "failed"
					result.Error = err.Error()
				}
			}
			updateTask(req.Task.ID, result, true)
			doneCh <- result
		}()

		waitMode := true
		if req.Wait != nil {
			waitMode = *req.Wait
		}

		if req.Wait != nil && !waitMode {
			writeJSON(w, http.StatusOK, runResponse{
				Result: scheduler.Result{TaskID: req.Task.ID, Status: "running"},
				RunID:  req.Task.RunID,
			})
			return
		}
		if req.Wait != nil && waitMode {
			result := <-doneCh
			writeJSON(w, http.StatusOK, runResponse{Result: result, RunID: req.Task.RunID, Error: result.Error})
			return
		}

		select {
		case result := <-doneCh:
			writeJSON(w, http.StatusOK, runResponse{Result: result, RunID: req.Task.RunID, Error: result.Error})
		case <-time.After(asyncThreshold):
			writeJSON(w, http.StatusOK, runResponse{
				Result: scheduler.Result{TaskID: req.Task.ID, Status: "running"},
				RunID:  req.Task.RunID,
			})
		}
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, runResponse{Error: "method not allowed"})
			return
		}
		if !checkAuth(w, r) {
			return
		}
		taskID, err := readTaskID(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, runResponse{Error: err.Error()})
			return
		}
		if taskID == "" {
			writeJSON(w, http.StatusBadRequest, runResponse{Error: "task_id is required"})
			return
		}

		entry, ok := getTask(taskID)
		if !ok {
			writeJSON(w, http.StatusNotFound, runResponse{
				Result: scheduler.Result{TaskID: taskID, Status: "not_found"},
				Error:  "task not found",
			})
			return
		}

		if entry.Done {
			writeJSON(w, http.StatusOK, runResponse{Result: entry.Result, RunID: entry.Task.RunID, Error: entry.Result.Error})
			return
		}

		writeJSON(w, http.StatusOK, runResponse{
			Result: scheduler.Result{
				TaskID: taskID,
				Status: "running",
				Output: map[string]any{
					"stdout": entry.Stdout.String(),
					"stderr": entry.Stderr.String(),
				},
			},
			RunID: entry.Task.RunID,
		})
	})

	mux.HandleFunc("/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, runResponse{Error: "method not allowed"})
			return
		}
		if !checkAuth(w, r) {
			return
		}

		taskID, err := readTaskID(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, runResponse{Error: err.Error()})
			return
		}
		if taskID == "" {
			writeJSON(w, http.StatusBadRequest, runResponse{Error: "task_id is required"})
			return
		}

		task, ok := cancelTask(taskID)
		if !ok {
			writeJSON(w, http.StatusNotFound, runResponse{Error: "task not found or already done"})
			return
		}
		writeJSON(w, http.StatusOK, runResponse{
			Result: scheduler.Result{TaskID: taskID, Status: "canceled"},
			RunID:  task.RunID,
		})
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		last := diag.lastBeatTime()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":       "ok",
			"host_id":      cfg.HostID,
			"version":      agentVersion,
			"timestamp":    now.Unix(),
			"last_beat":    last.Format(time.RFC3339),
			"capabilities": cfg.Capabilities,
		})
	})

	mux.HandleFunc("/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		if !checkAuth(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, diag.snapshot())
	})

	return mux
}

func register(ctx context.Context, client *http.Client, cfg hostagent.Config, diagnostics *agentDiagnostics) error {
	started := time.Now().UTC()
	hostname, _ := os.Hostname()
	payload := buildHostAgentEventPayload(hostAgentEventPayloadInput{
		HostID:        cfg.HostID,
		Hostname:      hostname,
		ListenAddress: cfg.ListenAddr,
		Capabilities:  cfg.Capabilities,
		Labels:        cfg.Labels,
		System:        collectHostSystemInfo(),
		Registration:  true,
	})
	err := postAgentEvent(ctx, client, cfg, "/api/v1/host-agents/register", payload)
	if err != nil {
		diagnostics.recordFailure("register", started, err)
		return err
	}
	diagnostics.recordSuccess("register", started, http.StatusOK)
	return nil
}

func heartbeatLoop(ctx context.Context, client *http.Client, cfg hostagent.Config, diagnostics *agentDiagnostics) {
	ticker := time.NewTicker(cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := heartbeat(ctx, client, cfg, diagnostics); err != nil {
				fmt.Fprintf(os.Stderr, "host-agent heartbeat: %v\n", err)
			}
		}
	}
}

func heartbeat(ctx context.Context, client *http.Client, cfg hostagent.Config, diagnostics *agentDiagnostics) error {
	started := time.Now().UTC()
	payload := buildHostAgentEventPayload(hostAgentEventPayloadInput{
		HostID:       cfg.HostID,
		Capabilities: cfg.Capabilities,
		System:       collectHostSystemInfo(),
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	})
	err := postAgentEvent(ctx, client, cfg, "/api/v1/host-agents/heartbeat", payload)
	if err != nil {
		diagnostics.recordFailure("heartbeat", started, err)
		return err
	}
	diagnostics.recordSuccess("heartbeat", started, http.StatusOK)
	return nil
}

func buildHostAgentEventPayload(input hostAgentEventPayloadInput) map[string]any {
	system := input.System
	if system.OS == "" {
		system.OS = runtime.GOOS
	}
	if system.Arch == "" {
		system.Arch = runtime.GOARCH
	}
	payload := map[string]any{
		"hostId":        input.HostID,
		"os":            system.OS,
		"arch":          system.Arch,
		"agentVersion":  agentVersion,
		"capabilities":  input.Capabilities,
		"osRelease":     system.OSRelease,
		"kernelVersion": system.KernelVersion,
		"cpuCores":      system.CPUCores,
		"memoryBytes":   system.MemoryBytes,
	}
	if input.Timestamp != "" {
		payload["timestamp"] = input.Timestamp
	}
	if input.Registration {
		payload["hostname"] = input.Hostname
		payload["labels"] = input.Labels
		payload["listenAddress"] = input.ListenAddress
	}
	return payload
}

func collectHostSystemInfo() hostSystemInfo {
	return hostSystemInfo{
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		OSRelease:     detectOSRelease(runtime.GOOS),
		KernelVersion: detectKernelVersion(runtime.GOOS),
		CPUCores:      runtime.NumCPU(),
		MemoryBytes:   detectMemoryBytes(runtime.GOOS),
	}
}

func detectOSRelease(goos string) string {
	switch goos {
	case "linux":
		data, err := os.ReadFile("/etc/os-release")
		if err != nil {
			return ""
		}
		return parseOSReleaseName(string(data))
	case "darwin":
		out, err := exec.Command("sw_vers", "-productVersion").Output()
		if err != nil {
			return ""
		}
		version := strings.TrimSpace(string(out))
		if version == "" {
			return ""
		}
		return "macOS " + version
	default:
		return ""
	}
}

func parseOSReleaseName(data string) string {
	values := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"`)
		if key != "" && value != "" {
			values[key] = value
		}
	}
	if values["PRETTY_NAME"] != "" {
		return values["PRETTY_NAME"]
	}
	if values["NAME"] != "" && values["VERSION"] != "" {
		return values["NAME"] + " " + values["VERSION"]
	}
	return values["NAME"]
}

func detectKernelVersion(goos string) string {
	switch goos {
	case "linux", "darwin":
		out, err := exec.Command("uname", "-r").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	default:
		return ""
	}
}

func detectMemoryBytes(goos string) uint64 {
	switch goos {
	case "linux":
		data, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return 0
		}
		return parseLinuxMeminfoBytes(string(data))
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}
		value, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
		if err != nil {
			return 0
		}
		return value
	default:
		return 0
	}
}

func parseLinuxMeminfoBytes(data string) uint64 {
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.TrimSuffix(fields[0], ":") != "MemTotal" {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return value * 1024
	}
	return 0
}

func postAgentEvent(ctx context.Context, client *http.Client, cfg hostagent.Config, path string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ServerURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(cfg.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.Token))
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &agentHTTPStatusError{Path: path, StatusCode: resp.StatusCode}
	}
	return nil
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if len(header) >= 7 && strings.EqualFold(header[:7], "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return header
}

func resolveOutputLimit(args map[string]any, fallback int) int {
	limit := fallback
	if limit <= 0 {
		limit = 65536
	}
	if len(args) == 0 {
		return limit
	}
	raw, ok := args["max_output_bytes"]
	if !ok || raw == nil {
		return limit
	}
	switch v := raw.(type) {
	case int:
		if v > 0 {
			return v
		}
	case int64:
		if v > 0 {
			return int(v)
		}
	case float64:
		if int(v) > 0 {
			return int(v)
		}
	case string:
		var out int
		_, _ = fmt.Sscanf(strings.TrimSpace(v), "%d", &out)
		if out > 0 {
			return out
		}
	}
	return limit
}

func copyOutput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
