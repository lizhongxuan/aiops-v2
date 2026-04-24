package main

import (
	"context"
	"encoding/json"
	"errors"
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
	"runner/modules/shell"
	"runner/scheduler"
	"runner/state"
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
	Canceled   bool
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
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}

func main() {
	fs := flag.NewFlagSet("agent-server", flag.ExitOnError)
	addr := fs.String("addr", ":7072", "listen address")
	token := fs.String("token", "runner-token", "auth token")
	logLevel := fs.String("log-level", "info", "log level (debug/info/warn/error)")
	logFormat := fs.String("log-format", "console", "log format (console/json)")
	stateFile := fs.String("state-file", "", "optional durable run state file path")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if _, err := logging.Init(logging.Config{LogLevel: *logLevel, LogFormat: *logFormat}); err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}

	reg := modules.NewRegistry()
	reg.Register("cmd.run", cmd.New())
	reg.Register("shell.run", shell.New())

	var runStore state.RunStateStore = state.NewInMemoryRunStore()
	if strings.TrimSpace(*stateFile) != "" {
		runStore = state.NewFileStore(strings.TrimSpace(*stateFile))
		logging.L().Info("agent run state store configured",
			zap.String("path", strings.TrimSpace(*stateFile)),
		)
	} else {
		logging.L().Warn("run state store not configured; using in-memory fallback (non-durable)")
	}
	if _, err := runStore.MarkInterruptedRunning(context.Background(), "agent-server restarted"); err != nil {
		logging.L().Warn("agent run state reconciliation failed", zap.Error(err))
	}

	var lastBeat atomic.Int64
	lastBeat.Store(time.Now().UTC().Unix())
	const asyncThreshold = 4 * time.Second

	var taskMu sync.Mutex
	tasks := map[string]*taskEntry{}
	waitingTokenToTaskID := map[string]string{}

	getTask := func(id string) (*taskEntry, bool) {
		taskMu.Lock()
		defer taskMu.Unlock()
		entry, ok := tasks[id]
		return entry, ok
	}

	setTask := func(id string, entry *taskEntry) {
		taskMu.Lock()
		defer taskMu.Unlock()
		tasks[id] = entry
		if token := strings.TrimSpace(entry.Task.FSMWaitingToken); token != "" {
			waitingTokenToTaskID[token] = id
		}
	}

	findTaskByWaitingToken := func(waitingToken string) (*taskEntry, bool) {
		taskMu.Lock()
		defer taskMu.Unlock()
		id, ok := waitingTokenToTaskID[strings.TrimSpace(waitingToken)]
		if !ok {
			return nil, false
		}
		entry, ok := tasks[id]
		return entry, ok
	}

	updateTask := func(id string, result scheduler.Result, done bool) {
		taskMu.Lock()
		defer taskMu.Unlock()
		entry, ok := tasks[id]
		if !ok {
			entry = &taskEntry{}
			tasks[id] = entry
		}
		entry.Result = result
		entry.Done = done
		if done {
			entry.FinishedAt = time.Now().UTC()
		}
	}

	cancelTask := func(id string) (scheduler.Task, bool) {
		taskMu.Lock()
		defer taskMu.Unlock()
		entry, ok := tasks[id]
		if !ok || entry.Done {
			return scheduler.Task{}, false
		}
		if entry.Cancel != nil {
			entry.Cancel()
		}
		entry.Canceled = true
		entry.Result = scheduler.Result{TaskID: id, Status: "canceled"}
		entry.Done = true
		entry.FinishedAt = time.Now().UTC()
		return entry.Task, true
	}

	normalizeRunStatus := func(status string) string {
		switch strings.ToLower(strings.TrimSpace(status)) {
		case state.RunStatusRunning:
			return state.RunStatusRunning
		case state.RunStatusSuccess:
			return state.RunStatusSuccess
		case state.RunStatusFailed:
			return state.RunStatusFailed
		case state.RunStatusCanceled:
			return state.RunStatusCanceled
		default:
			return state.RunStatusRunning
		}
	}

	updateRunState := func(task scheduler.Task, status, message string, output map[string]any) {
		runID := strings.TrimSpace(task.RunID)
		if runID == "" {
			runID = strings.TrimSpace(task.ID)
		}
		if runID == "" {
			return
		}
		now := time.Now().UTC()
		runStatus := normalizeRunStatus(status)

		run, err := runStore.GetRun(context.Background(), runID)
		if err != nil {
			if !state.IsNotFound(err) {
				logging.L().Warn("load run state failed", zap.String("run_id", runID), zap.Error(err))
				return
			}
			run = state.RunState{
				RunID:           runID,
				WorkflowName:    strings.TrimSpace(task.Step.Name),
				WorkflowVersion: "",
				Status:          state.RunStatusQueued,
				StartedAt:       now,
				UpdatedAt:       now,
				Steps:           []state.StepState{},
			}
			if createErr := runStore.CreateRun(context.Background(), run); createErr != nil {
				if errors.Is(createErr, state.ErrRunExists) {
					// continue to update path
				} else {
					logging.L().Warn("create run state failed", zap.String("run_id", runID), zap.Error(createErr))
					return
				}
			}
		}

		if run.StartedAt.IsZero() {
			run.StartedAt = now
		}
		run.WorkflowName = strings.TrimSpace(task.Step.Name)
		run.UpsertStepStart(task.Step.Name, now)
		hostResult := state.HostResult{
			Host:      task.Host.Name,
			Status:    runStatus,
			Message:   strings.TrimSpace(message),
			Output:    output,
			StartedAt: now,
		}
		if runStatus != state.RunStatusRunning {
			hostResult.FinishedAt = now
		}
		run.UpsertHostResult(task.Step.Name, hostResult)
		if runStatus != state.RunStatusRunning {
			run.UpsertStepFinish(task.Step.Name, runStatus, strings.TrimSpace(message), now)
		}

		run.Status = runStatus
		run.Message = strings.TrimSpace(message)
		if runStatus == state.RunStatusFailed && run.Message != "" {
			run.LastError = run.Message
		}
		if state.IsTerminalRunStatus(runStatus) {
			run.FinishedAt = now
		}
		run.UpdatedAt = now
		if updateErr := runStore.UpdateRun(context.Background(), run); updateErr != nil {
			logging.L().Warn("update run state failed", zap.String("run_id", runID), zap.Error(updateErr))
		}
	}

	checkAuth := func(w http.ResponseWriter, r *http.Request) bool {
		trimmed := strings.TrimSpace(*token)
		if trimmed == "" {
			return true
		}
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			auth = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		}
		headerToken := strings.TrimSpace(r.Header.Get("X-Runner-Token"))
		if auth == trimmed || headerToken == trimmed {
			return true
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("unauthorized"))
		return false
	}

	http.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !checkAuth(w, r) {
			return
		}
		var req runRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(runResponse{Error: err.Error()})
			return
		}
		if strings.TrimSpace(req.Task.ID) == "" {
			req.Task.ID = fmt.Sprintf("task-%d", time.Now().UTC().UnixNano())
		}
		if strings.TrimSpace(req.Task.RunID) == "" {
			req.Task.RunID = req.Task.ID
		}

		if waitingToken := strings.TrimSpace(req.Task.FSMWaitingToken); waitingToken != "" {
			if existing, ok := findTaskByWaitingToken(waitingToken); ok {
				w.Header().Set("Content-Type", "application/json")
				if existing.Done {
					_ = json.NewEncoder(w).Encode(runResponse{Result: existing.Result, RunID: existing.Task.RunID, Error: existing.Result.Error})
				} else {
					_ = json.NewEncoder(w).Encode(runResponse{Result: scheduler.Result{TaskID: existing.Task.ID, Status: "running"}, RunID: existing.Task.RunID})
				}
				return
			}
		}

		if existing, ok := getTask(req.Task.ID); ok {
			if existing.Done {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(runResponse{Result: existing.Result, RunID: req.Task.RunID, Error: existing.Result.Error})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(runResponse{Result: scheduler.Result{TaskID: req.Task.ID, Status: "running"}, RunID: req.Task.RunID})
			return
		}
		logging.L().Info("agent run start",
			zap.String("task_id", req.Task.ID),
			zap.String("run_id", req.Task.RunID),
			zap.String("step", req.Task.Step.Name),
			zap.String("action", req.Task.Step.Action),
			zap.String("host", req.Task.Host.Name),
		)

		module, ok := reg.Get(req.Task.Step.Action)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(runResponse{RunID: req.Task.RunID, Error: "unsupported action"})
			return
		}

		outputLimit := 65536
		if req.Task.Step.Args != nil {
			if raw, ok := req.Task.Step.Args["max_output_bytes"]; ok {
				switch v := raw.(type) {
				case int:
					outputLimit = v
				case int64:
					outputLimit = int(v)
				case float64:
					outputLimit = int(v)
				case string:
					var out int
					_, _ = fmt.Sscanf(strings.TrimSpace(v), "%d", &out)
					if out > 0 {
						outputLimit = out
					}
				}
			}
		}

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
		updateRunState(req.Task, state.RunStatusRunning, "task running", nil)

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

			result := scheduler.Result{TaskID: req.Task.ID, Status: "success", Output: res.Output}
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
			msg := result.Error
			if strings.TrimSpace(msg) == "" {
				msg = result.Status
			}
			updateRunState(req.Task, result.Status, msg, result.Output)
			doneCh <- result
		}()

		waitMode := true
		if req.Wait != nil {
			waitMode = *req.Wait
		}
		if req.Wait != nil && !waitMode {
			logging.L().Info("agent run accepted async by explicit wait=false",
				zap.String("task_id", req.Task.ID),
				zap.String("run_id", req.Task.RunID),
			)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(runResponse{
				Result: scheduler.Result{TaskID: req.Task.ID, Status: "running"},
				RunID:  req.Task.RunID,
			})
			return
		}
		if waitMode && req.Wait != nil {
			result := <-doneCh
			updateTask(req.Task.ID, result, true)
			logging.L().Info("agent run finish",
				zap.String("task_id", req.Task.ID),
				zap.String("run_id", req.Task.RunID),
				zap.String("status", result.Status),
			)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(runResponse{Result: result, RunID: req.Task.RunID, Error: result.Error})
			return
		}

		select {
		case result := <-doneCh:
			updateTask(req.Task.ID, result, true)
			logging.L().Info("agent run finish",
				zap.String("task_id", req.Task.ID),
				zap.String("run_id", req.Task.RunID),
				zap.String("status", result.Status),
			)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(runResponse{Result: result, RunID: req.Task.RunID, Error: result.Error})
		case <-time.After(asyncThreshold):
			logging.L().Info("agent run switched to async",
				zap.String("task_id", req.Task.ID),
				zap.Duration("threshold", asyncThreshold),
			)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(runResponse{Result: scheduler.Result{TaskID: req.Task.ID, Status: "running"}, RunID: req.Task.RunID})
		}
	})

	http.HandleFunc("/run-status", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		runStatusHandler(runStore).ServeHTTP(w, r)
	})

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !checkAuth(w, r) {
			return
		}
		taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
		if taskID == "" && r.Method == http.MethodPost {
			var req statusRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(runResponse{Error: err.Error()})
				return
			}
			taskID = strings.TrimSpace(req.TaskID)
		}
		if taskID == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(runResponse{Error: "task_id is required"})
			return
		}
		entry, ok := getTask(taskID)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(runResponse{
				Result: scheduler.Result{TaskID: taskID, Status: "not_found"},
				Error:  "task not found",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if entry.Done {
			_ = json.NewEncoder(w).Encode(runResponse{Result: entry.Result, RunID: entry.Task.RunID, Error: entry.Result.Error})
			return
		}
		result := scheduler.Result{TaskID: taskID, Status: "running"}
		if entry.Stdout != nil || entry.Stderr != nil {
			result.Output = map[string]any{
				"stdout": "",
				"stderr": "",
			}
			if entry.Stdout != nil {
				result.Output["stdout"] = entry.Stdout.String()
			}
			if entry.Stderr != nil {
				result.Output["stderr"] = entry.Stderr.String()
			}
		}
		_ = json.NewEncoder(w).Encode(runResponse{Result: result, RunID: entry.Task.RunID})
	})

	http.HandleFunc("/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !checkAuth(w, r) {
			return
		}
		taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
		if taskID == "" && r.Method == http.MethodPost {
			var req statusRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(runResponse{Error: err.Error()})
				return
			}
			taskID = strings.TrimSpace(req.TaskID)
		}
		if taskID == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(runResponse{Error: "task_id is required"})
			return
		}
		task, ok := cancelTask(taskID)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(runResponse{Error: "task not found or already done"})
			return
		}
		updateRunState(task, state.RunStatusCanceled, "task canceled by request", nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(runResponse{Result: scheduler.Result{TaskID: taskID, Status: "canceled"}, RunID: task.RunID})
	})

	http.HandleFunc("/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !checkAuth(w, r) {
			return
		}
		now := time.Now().UTC()
		lastBeat.Store(now.Unix())
		logging.L().Debug("agent heartbeat", zap.String("addr", *addr))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":     "ok",
			"last_beat":  now.Format(time.RFC3339),
			"timestamp":  now.Unix(),
			"capability": []string{"cmd.run"},
		})
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		now := time.Now().UTC()
		last := time.Unix(lastBeat.Load(), 0)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "ok",
			"timestamp": now.Unix(),
			"last_beat": last.Format(time.RFC3339),
		})
	})

	logging.L().Info("agent server listening",
		zap.String("addr", *addr),
		zap.Bool("token_required", strings.TrimSpace(*token) != ""),
	)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runStatusHandler(runStore state.RunStateStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
		if runID == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "run_id is required"})
			return
		}
		run, err := runStore.GetRun(r.Context(), runID)
		if err != nil {
			if state.IsNotFound(err) {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "run not found"})
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(run)
	}
}
