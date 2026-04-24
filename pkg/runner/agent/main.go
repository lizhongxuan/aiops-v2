package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"runner/logging"
	"runner/modules"
	"runner/modules/cmd"
	"runner/modules/script"
	"runner/modules/shell"
	"runner/modules/wait"
	"runner/scheduler"
)

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
	fs := flag.NewFlagSet("runner-agent", flag.ExitOnError)
	addr := fs.String("addr", ":7072", "listen address")
	token := fs.String("token", "runner-token", "auth token (empty disables auth)")
	logLevel := fs.String("log-level", "info", "log level (debug/info/warn/error)")
	logFormat := fs.String("log-format", "console", "log format (console/json)")
	asyncThresholdSec := fs.Int("async-threshold-sec", 4, "auto async threshold in seconds when wait is omitted")
	defaultMaxOutputBytes := fs.Int("max-output-bytes", 65536, "default max stdout/stderr bytes kept in memory")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if _, err := logging.Init(logging.Config{LogLevel: *logLevel, LogFormat: *logFormat}); err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}

	registry := modules.NewRegistry()
	registry.Register("cmd.run", cmd.New())
	registry.Register("shell.run", shell.New())
	registry.Register("script.shell", script.New("shell"))
	registry.Register("script.python", script.New("python"))
	registry.Register("wait.event", wait.NewEvent())
	asyncThreshold := time.Duration(*asyncThresholdSec) * time.Second
	if asyncThreshold <= 0 {
		asyncThreshold = 4 * time.Second
	}
	if *defaultMaxOutputBytes <= 0 {
		*defaultMaxOutputBytes = 65536
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
		required := strings.TrimSpace(*token)
		if required == "" {
			return true
		}
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			auth = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		}
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

		outputLimit := resolveOutputLimit(req.Task.Step.Args, *defaultMaxOutputBytes)
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

		logging.L().Info("runner agent task start",
			zap.String("task_id", req.Task.ID),
			zap.String("run_id", req.Task.RunID),
			zap.String("step", req.Task.Step.Name),
			zap.String("action", req.Task.Step.Action),
			zap.String("host", req.Task.Host.Name),
		)

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
			logging.L().Info("runner agent task finish",
				zap.String("task_id", req.Task.ID),
				zap.String("run_id", req.Task.RunID),
				zap.String("status", result.Status),
			)
			writeJSON(w, http.StatusOK, runResponse{Result: result, RunID: req.Task.RunID, Error: result.Error})
			return
		}

		select {
		case result := <-doneCh:
			logging.L().Info("runner agent task finish",
				zap.String("task_id", req.Task.ID),
				zap.String("run_id", req.Task.RunID),
				zap.String("status", result.Status),
			)
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

	mux.HandleFunc("/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		if !checkAuth(w, r) {
			return
		}
		now := time.Now().UTC()
		lastBeat.Store(now.Unix())
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"timestamp": now.Unix(),
			"last_beat": now.Format(time.RFC3339),
			"capability": []string{
				"cmd.run",
				"shell.run",
				"script.shell",
				"script.python",
				"wait.event",
			},
		})
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		last := time.Unix(lastBeat.Load(), 0)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"timestamp": now.Unix(),
			"last_beat": last.Format(time.RFC3339),
		})
	})

	logging.L().Info("runner agent listening",
		zap.String("addr", *addr),
		zap.Bool("token_required", strings.TrimSpace(*token) != ""),
		zap.Duration("async_threshold", asyncThreshold),
	)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
	case int8:
		if v > 0 {
			return int(v)
		}
	case int16:
		if v > 0 {
			return int(v)
		}
	case int32:
		if v > 0 {
			return int(v)
		}
	case int64:
		if v > 0 {
			return int(v)
		}
	case float32:
		if int(v) > 0 {
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
