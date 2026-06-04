package hostops

import (
	"context"
	"fmt"
	"strings"
	"time"

	"runner/scheduler"
	"runner/workflow"
)

type HostDispatcher interface {
	Dispatch(ctx context.Context, task scheduler.Task) (scheduler.Result, error)
}

type ExecutionAdapter struct {
	dispatcher HostDispatcher
}

func NewExecutionAdapter(dispatcher HostDispatcher) *ExecutionAdapter {
	return &ExecutionAdapter{dispatcher: dispatcher}
}

type HostCommandRequest struct {
	HostID      string
	HostAddress string
	Script      string
	Timeout     string
	Env         map[string]any
}

type HostCommandResult struct {
	TaskID   string
	Status   string
	Stdout   string
	Stderr   string
	ExitCode int
	Output   map[string]any
	Error    string
}

func (a *ExecutionAdapter) RunShell(ctx context.Context, toolCtx ToolContext, req HostCommandRequest) (HostCommandResult, error) {
	hostID := strings.TrimSpace(req.HostID)
	if hostID == "" {
		hostID = strings.TrimSpace(toolCtx.BoundHostID)
	}
	if err := EnforceHostBinding(toolCtx, hostID); err != nil {
		return HostCommandResult{}, err
	}
	if a == nil || a.dispatcher == nil {
		return HostCommandResult{}, fmt.Errorf("host dispatcher is unavailable")
	}
	script := strings.TrimSpace(req.Script)
	if script == "" {
		return HostCommandResult{}, fmt.Errorf("script is required")
	}
	task := scheduler.Task{
		ID:    fmt.Sprintf("host-shell-%d", time.Now().UTC().UnixNano()),
		RunID: fmt.Sprintf("host-shell-run-%d", time.Now().UTC().UnixNano()),
		Step: workflow.Step{
			Name:    "host shell",
			Action:  "script.shell",
			Targets: []string{hostID},
			Args: map[string]any{
				"script": script,
			},
			Timeout: strings.TrimSpace(req.Timeout),
		},
		Host: workflow.HostSpec{
			Name:    hostID,
			Address: strings.TrimSpace(req.HostAddress),
			Vars:    cloneAnyMap(req.Env),
		},
	}
	if task.Host.Address == "" {
		task.Host.Address = hostID
	}
	result, err := a.dispatcher.Dispatch(ctx, task)
	if err != nil {
		return HostCommandResult{}, err
	}
	return hostCommandResultFromScheduler(result), nil
}

func hostCommandResultFromScheduler(result scheduler.Result) HostCommandResult {
	return HostCommandResult{
		TaskID:   result.TaskID,
		Status:   result.Status,
		Stdout:   stringOutputValue(result.Output, "stdout"),
		Stderr:   stringOutputValue(result.Output, "stderr"),
		ExitCode: intOutputValue(result.Output, "exitCode"),
		Output:   cloneAnyMap(result.Output),
		Error:    result.Error,
	}
}

func stringOutputValue(output map[string]any, key string) string {
	if len(output) == 0 {
		return ""
	}
	value := output[key]
	if text, ok := value.(string); ok {
		return text
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func intOutputValue(output map[string]any, key string) int {
	if len(output) == 0 {
		return 0
	}
	switch value := output[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}
