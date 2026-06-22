package operatorruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type WorkflowStartRequest struct {
	GuardRunID  string         `json:"guardRunId"`
	WorkflowRef string         `json:"workflowRef"`
	Inputs      map[string]any `json:"inputs,omitempty"`
}

type WorkflowRun struct {
	ID        string         `json:"id"`
	Status    string         `json:"status"`
	Inputs    map[string]any `json:"inputs,omitempty"`
	Error     string         `json:"error,omitempty"`
	StartedAt time.Time      `json:"startedAt,omitempty"`
	EndedAt   time.Time      `json:"endedAt,omitempty"`
}

const (
	WorkflowRunSucceeded = "succeeded"
	WorkflowRunFailed    = "failed"
)

type WorkflowInvoker interface {
	StartWorkflow(context.Context, WorkflowStartRequest) (WorkflowRun, error)
	GetWorkflowRun(context.Context, string) (WorkflowRun, bool, error)
	CancelWorkflowRun(context.Context, string) error
}

type MemoryWorkflowInvoker struct {
	mu   sync.RWMutex
	runs map[string]WorkflowRun
}

func NewMemoryWorkflowInvoker() *MemoryWorkflowInvoker {
	return &MemoryWorkflowInvoker{runs: map[string]WorkflowRun{}}
}

func (i *MemoryWorkflowInvoker) StartWorkflow(_ context.Context, req WorkflowStartRequest) (WorkflowRun, error) {
	now := time.Now().UTC()
	status := WorkflowRunSucceeded
	var runErr error
	errorText := ""
	if strings.Contains(strings.ToLower(req.WorkflowRef), "fail") {
		status = WorkflowRunFailed
		errorText = "workflow failed"
		runErr = errors.New(errorText)
	}
	run := WorkflowRun{
		ID:        fmt.Sprintf("workflow-run-%d", now.UnixNano()),
		Status:    status,
		Inputs:    redactInputs(req.Inputs),
		Error:     errorText,
		StartedAt: now,
		EndedAt:   now,
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.runs[run.ID] = run
	return run, runErr
}

func (i *MemoryWorkflowInvoker) GetWorkflowRun(_ context.Context, id string) (WorkflowRun, bool, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	run, ok := i.runs[id]
	return run, ok, nil
}

func (i *MemoryWorkflowInvoker) CancelWorkflowRun(_ context.Context, id string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	run, ok := i.runs[id]
	if !ok {
		return fmt.Errorf("workflow run not found")
	}
	run.Status = "cancelled"
	run.EndedAt = time.Now().UTC()
	i.runs[id] = run
	return nil
}

func redactInputs(inputs map[string]any) map[string]any {
	out := make(map[string]any, len(inputs))
	for key, value := range inputs {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") {
			continue
		}
		out[key] = value
	}
	return out
}
