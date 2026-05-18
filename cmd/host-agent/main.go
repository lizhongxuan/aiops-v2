package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aiops-v2/internal/hostagent"
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

type statusRequest struct {
	TaskID string `json:"task_id"`
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
	client := &http.Client{Timeout: 10 * time.Second}
	if err := register(ctx, client, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "register host-agent: %v\n", err)
		os.Exit(1)
	}
	go heartbeatLoop(ctx, client, cfg)

	fmt.Fprintf(os.Stderr, "host-agent listening on %s\n", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, newAgentHandler(cfg, opts)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newAgentHandler(cfg hostagent.Config, opts agentOptions) http.Handler {
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

	var lastBeat atomic.Int64
	lastBeat.Store(time.Now().UTC().Unix())

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
		last := time.Unix(lastBeat.Load(), 0)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":       "ok",
			"host_id":      cfg.HostID,
			"version":      agentVersion,
			"timestamp":    now.Unix(),
			"last_beat":    last.Format(time.RFC3339),
			"capabilities": cfg.Capabilities,
		})
	})

	return mux
}

func register(ctx context.Context, client *http.Client, cfg hostagent.Config) error {
	hostname, _ := os.Hostname()
	payload := map[string]any{
		"hostId":        cfg.HostID,
		"hostname":      hostname,
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"agentVersion":  agentVersion,
		"capabilities":  cfg.Capabilities,
		"labels":        cfg.Labels,
		"listenAddress": cfg.ListenAddr,
	}
	return postAgentEvent(ctx, client, cfg, "/api/v1/host-agents/register", payload)
}

func heartbeatLoop(ctx context.Context, client *http.Client, cfg hostagent.Config) {
	ticker := time.NewTicker(cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := heartbeat(ctx, client, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "host-agent heartbeat: %v\n", err)
			}
		}
	}
}

func heartbeat(ctx context.Context, client *http.Client, cfg hostagent.Config) error {
	payload := map[string]any{
		"hostId":       cfg.HostID,
		"agentVersion": agentVersion,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"capabilities": cfg.Capabilities,
	}
	return postAgentEvent(ctx, client, cfg, "/api/v1/host-agents/heartbeat", payload)
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
		return fmt.Errorf("%s returned status %d", path, resp.StatusCode)
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
