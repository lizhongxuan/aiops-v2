package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"runner/engine"
	"runner/logging"
	"runner/scheduler"
	"runner/workflow"
)

type logRecorder struct {
	maxOutputLen int
	stepIndex    map[string]int
	totalSteps   int
}

func (r logRecorder) StepStart(step workflow.Step, targets []workflow.HostSpec) {
	stepID := step.Name
	fields := []zap.Field{
		zap.String("step_id", stepID),
		zap.String("step", step.Name),
		zap.String("action", step.Action),
		zap.Int("targets", len(targets)),
	}
	if idx := r.stepIndex[stepID]; idx > 0 {
		fields = append(fields, zap.Int("step_no", idx))
		if r.totalSteps > 0 {
			fields = append(fields, zap.Int("step_total", r.totalSteps))
		}
	}
	logging.L().Info("step start", fields...)
}

func (r logRecorder) StepFinish(step workflow.Step, status string) {
	stepID := step.Name
	fields := []zap.Field{
		zap.String("step_id", stepID),
		zap.String("step", step.Name),
		zap.String("status", status),
	}
	if idx := r.stepIndex[stepID]; idx > 0 {
		fields = append(fields, zap.Int("step_no", idx))
		if r.totalSteps > 0 {
			fields = append(fields, zap.Int("step_total", r.totalSteps))
		}
	}
	logging.L().Info("step finish", fields...)
}

func (r logRecorder) HostResult(step workflow.Step, host workflow.HostSpec, result scheduler.Result) {
	stepID := step.Name
	fields := []zap.Field{
		zap.String("step_id", stepID),
		zap.String("step", step.Name),
		zap.String("host", host.Name),
		zap.String("status", result.Status),
	}
	if idx := r.stepIndex[stepID]; idx > 0 {
		fields = append(fields, zap.Int("step_no", idx))
		if r.totalSteps > 0 {
			fields = append(fields, zap.Int("step_total", r.totalSteps))
		}
	}
	if stdout := extractOutputString(result.Output, "stdout"); strings.TrimSpace(stdout) != "" {
		trimmed, length := truncateOutput(stdout, r.maxOutputLen)
		fields = append(fields, zap.Int("stdout_len", length), zap.String("stdout", trimmed))
	}
	if stderr := extractOutputString(result.Output, "stderr"); strings.TrimSpace(stderr) != "" {
		trimmed, length := truncateOutput(stderr, r.maxOutputLen)
		fields = append(fields, zap.Int("stderr_len", length), zap.String("stderr", trimmed))
	}
	logging.L().Info("host result", fields...)
}

func extractOutputString(output map[string]any, key string) string {
	if output == nil {
		return ""
	}
	raw, ok := output[key]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

func truncateOutput(value string, maxLen int) (string, int) {
	length := len(value)
	if maxLen <= 0 || length <= maxLen {
		return value, length
	}
	return value[:maxLen] + "...(truncated)", length
}

func main() {
	fs := flag.NewFlagSet("agent-dispatch", flag.ExitOnError)
	token := fs.String("token", "runner-token", "auth token")
	logLevel := fs.String("log-level", "info", "log level (debug/info/warn/error)")
	logFormat := fs.String("log-format", "console", "log format (console/json)")
	maxOutputLen := fs.Int("max-output-len", 2000, "max stdout/stderr length in logs (0 = unlimited)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fmt.Println("usage: agent-dispatch [--token xxx] <yaml-file>")
		os.Exit(2)
	}

	yamlPath := fs.Arg(0)
	if _, err := logging.Init(logging.Config{LogLevel: *logLevel, LogFormat: *logFormat}); err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}

	logging.L().Info("agent dispatch start", zap.String("workflow", yamlPath))
	wf, err := workflow.LoadFile(yamlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load workflow: %v\n", err)
		os.Exit(1)
	}
	if err := wf.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "validate workflow: %v\n", err)
		os.Exit(1)
	}
	stepIndex := make(map[string]int, len(wf.Steps))
	for i := range wf.Steps {
		stepID := strings.TrimSpace(wf.Steps[i].Name)
		if stepID == "" {
			stepID = fmt.Sprintf("step-%d", i+1)
			wf.Steps[i].Name = stepID
		}
		if _, ok := stepIndex[stepID]; !ok {
			stepIndex[stepID] = i + 1
		}
	}

	eng := engine.New()
	if wf.Vars == nil {
		wf.Vars = map[string]any{}
	}
	if strings.TrimSpace(*token) != "" {
		wf.Vars["RUNNER_AGENT_TOKEN"] = strings.TrimSpace(*token)
	}
	wf.Vars["RUNNER_AGENT_HEARTBEAT"] = true
	wf.Vars["RUNNER_AGENT_RETRY_MAX"] = 2
	wf.Vars["RUNNER_AGENT_RETRY_DELAY_SEC"] = 2

	logging.L().Info("workflow apply start",
		zap.String("name", wf.Name),
		zap.Int("steps", len(wf.Steps)),
	)
	ctx := engine.WithRecorder(context.Background(), logRecorder{
		maxOutputLen: *maxOutputLen,
		stepIndex:    stepIndex,
		totalSteps:   len(wf.Steps),
	})
	if err := eng.Apply(ctx, wf); err != nil {
		fmt.Fprintf(os.Stderr, "apply workflow: %v\n", err)
		os.Exit(1)
	}
	logging.L().Info("workflow applied via agent", zap.String("name", wf.Name))
	fmt.Println("workflow applied via agent")
}
