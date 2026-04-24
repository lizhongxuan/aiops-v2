package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"runner/engine"
	"runner/logging"
	"runner/scheduler"
	"runner/state"
	"runner/workflow"
)

//go:embed static/*
var staticFS embed.FS

type startRequest struct {
	YAML  string `json:"yaml"`
	Token string `json:"token"`
	Mode  string `json:"mode"` // "agent" or "local"
}

type startResponse struct {
	RunID string     `json:"run_id"`
	Steps []stepView `json:"steps,omitempty"`
	Error string     `json:"error,omitempty"`
}

type stepView struct {
	Name            string         `json:"name"`
	Action          string         `json:"action"`
	Targets         []string       `json:"targets,omitempty"`
	Args            map[string]any `json:"args,omitempty"`
	MustVars        []string       `json:"must_vars,omitempty"`
	When            string         `json:"when,omitempty"`
	Retries         int            `json:"retries,omitempty"`
	Timeout         string         `json:"timeout,omitempty"`
	ContinueOnError bool           `json:"continue_on_error,omitempty"`
	ExpectVars      []string       `json:"expect_vars,omitempty"`
	Notify          []string       `json:"notify,omitempty"`
}

type logEvent struct {
	Type      string `json:"type"`
	Step      string `json:"step,omitempty"`
	StepNo    int    `json:"step_no,omitempty"`
	StepTotal int    `json:"step_total,omitempty"`
	Host      string `json:"host,omitempty"`
	Status    string `json:"status,omitempty"`
	Stream    string `json:"stream,omitempty"`
	Chunk     string `json:"chunk,omitempty"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	Message   string `json:"message,omitempty"`
}

type streamRecorder struct {
	ch        chan logEvent
	stepIndex map[string]int
	total     int
}

func (r *streamRecorder) StepStart(step workflow.Step, targets []workflow.HostSpec) {
	idx := r.stepIndex[step.Name]
	r.ch <- logEvent{
		Type:      "step_start",
		Step:      step.Name,
		StepNo:    idx,
		StepTotal: r.total,
	}
}

func (r *streamRecorder) StepFinish(step workflow.Step, status string) {
	idx := r.stepIndex[step.Name]
	r.ch <- logEvent{
		Type:      "step_finish",
		Step:      step.Name,
		Status:    status,
		StepNo:    idx,
		StepTotal: r.total,
	}
}

func (r *streamRecorder) HostResult(step workflow.Step, host workflow.HostSpec, result scheduler.Result) {
	idx := r.stepIndex[step.Name]
	event := logEvent{
		Type:      "host_result",
		Step:      step.Name,
		Host:      host.Name,
		Status:    result.Status,
		StepNo:    idx,
		StepTotal: r.total,
	}
	if output := result.Output; output != nil {
		if stdout, ok := output["stdout"]; ok {
			event.Stdout = fmt.Sprint(stdout)
		}
		if stderr, ok := output["stderr"]; ok {
			event.Stderr = fmt.Sprint(stderr)
		}
	}
	r.ch <- event
}

type runHub struct {
	mu   sync.Mutex
	runs map[string]chan logEvent
}

func newRunHub() *runHub {
	return &runHub{runs: map[string]chan logEvent{}}
}

func (h *runHub) create(runID string) chan logEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	if strings.TrimSpace(runID) == "" {
		runID = state.NewRunID()
	}
	ch := make(chan logEvent, 64)
	h.runs[runID] = ch
	return ch
}

func (h *runHub) get(runID string) (chan logEvent, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch, ok := h.runs[runID]
	return ch, ok
}

func (h *runHub) remove(runID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.runs, runID)
}

func main() {
	_, _ = logging.Init(logging.Config{LogLevel: "info", LogFormat: "console"})
	hub := newRunHub()
	runStore := state.NewInMemoryRunStore()

	http.Handle("/", http.FileServer(http.FS(staticFS)))

	http.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req startRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(startResponse{Error: err.Error()})
			return
		}
		if strings.TrimSpace(req.YAML) == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(startResponse{Error: "yaml is required"})
			return
		}

		wf, err := workflow.Load([]byte(req.YAML))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(startResponse{Error: err.Error()})
			return
		}
		if err := wf.Validate(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(startResponse{Error: err.Error()})
			return
		}

		stepIndex := map[string]int{}
		for i := range wf.Steps {
			name := strings.TrimSpace(wf.Steps[i].Name)
			if name == "" {
				name = fmt.Sprintf("step-%d", i+1)
				wf.Steps[i].Name = name
			}
			if _, ok := stepIndex[name]; !ok {
				stepIndex[name] = i + 1
			}
		}
		stepViews := buildStepViews(wf)

		runID := state.NewRunID()
		ch := hub.create(runID)
		total := len(wf.Steps)

		go func() {
			defer close(ch)
			eng := engine.New()
			eng.RunStore = runStore

			mode := strings.ToLower(strings.TrimSpace(req.Mode))
			if mode == "agent" || mode == "" {
				hosts := wf.Inventory.ResolveHosts()
				useAgent := false
				for _, host := range hosts {
					if strings.HasPrefix(host.Address, "http://") || strings.HasPrefix(host.Address, "https://") {
						useAgent = true
						break
					}
				}
				if useAgent {
					if wf.Vars == nil {
						wf.Vars = map[string]any{}
					}
					wf.Vars["RUNNER_AGENT_HEARTBEAT"] = true
					wf.Vars["RUNNER_AGENT_ASYNC_TIMEOUT_SEC"] = int((10 * time.Minute).Seconds())
					wf.Vars["RUNNER_AGENT_POLL_INTERVAL_SEC"] = int((2 * time.Second).Seconds())
					if strings.TrimSpace(req.Token) != "" {
						wf.Vars["RUNNER_AGENT_TOKEN"] = strings.TrimSpace(req.Token)
					}
				}
			}

			rec := &streamRecorder{ch: ch, stepIndex: stepIndex, total: total}
			ctx := engine.WithRecorder(context.Background(), rec)
			ch <- logEvent{Type: "run_start", StepTotal: total}
			runSnapshot, err := eng.ApplyWithRun(ctx, wf, engine.RunOptions{
				RunID: runID,
				Store: runStore,
			})
			if err != nil {
				ch <- logEvent{Type: "error", Message: err.Error()}
				return
			}
			ch <- logEvent{Type: "done", Status: runSnapshot.Status}
		}()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(startResponse{
			RunID: runID,
			Steps: stepViews,
		})
	})

	http.HandleFunc("/run-status", runStatusHandler(runStore))

	http.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
		if runID == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ch, ok := hub.get(runID)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		for event := range ch {
			payload, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
		hub.remove(runID)
	})

	addr := ":8088"
	logging.L().Info("runner web ui listening", zap.String("addr", addr))
	if err := http.ListenAndServe(addr, nil); err != nil {
		logging.L().Error("web ui server failed", zap.Error(err))
	}
}

func buildStepViews(wf workflow.Workflow) []stepView {
	if len(wf.Steps) == 0 {
		return nil
	}
	out := make([]stepView, 0, len(wf.Steps))
	for _, step := range wf.Steps {
		view := stepView{
			Name:            strings.TrimSpace(step.Name),
			Action:          strings.TrimSpace(step.Action),
			Targets:         append([]string{}, step.Targets...),
			Args:            cloneAnyMap(step.Args),
			MustVars:        append([]string{}, step.MustVars...),
			When:            strings.TrimSpace(step.When),
			Retries:         step.Retries,
			Timeout:         strings.TrimSpace(step.Timeout),
			ContinueOnError: step.ContinueOnError,
			ExpectVars:      append([]string{}, step.ExpectVars...),
			Notify:          append([]string{}, step.Notify...),
		}
		out = append(out, view)
	}
	return out
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
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
