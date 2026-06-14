package hostops

import (
	"context"
	"errors"
	"testing"

	"runner/scheduler"
)

func TestExecutionAdapterDispatchesOnlyToBoundHostAgent(t *testing.T) {
	dispatcher := &fakeHostDispatcher{}
	adapter := NewExecutionAdapter(dispatcher)
	ctx := ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"}
	_, err := adapter.RunShell(context.Background(), ctx, HostCommandRequest{
		HostID: "host-a",
		Script: "uptime",
	})
	if err != nil {
		t.Fatalf("RunShell() error = %v", err)
	}
	if dispatcher.lastHostID != "host-a" {
		t.Fatalf("lastHostID = %q, want host-a", dispatcher.lastHostID)
	}
	if dispatcher.lastTask.Step.Action != "script.shell" {
		t.Fatalf("action = %q, want script.shell", dispatcher.lastTask.Step.Action)
	}
	if dispatcher.lastTask.Step.Args["script"] != "uptime" {
		t.Fatalf("script arg = %#v, want uptime", dispatcher.lastTask.Step.Args["script"])
	}
}

func TestExecutionAdapterRejectsCrossHostDispatch(t *testing.T) {
	dispatcher := &fakeHostDispatcher{}
	adapter := NewExecutionAdapter(dispatcher)
	ctx := ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"}
	_, err := adapter.RunShell(context.Background(), ctx, HostCommandRequest{
		HostID: "host-b",
		Script: "uptime",
	})
	if !errors.Is(err, ErrCrossHostDenied) {
		t.Fatalf("err = %v, want ErrCrossHostDenied", err)
	}
	if dispatcher.lastHostID != "" {
		t.Fatalf("dispatcher was called for cross-host request: %q", dispatcher.lastHostID)
	}
}

func TestExecutionAdapterRejectsManagerDirectCommand(t *testing.T) {
	dispatcher := &fakeHostDispatcher{}
	adapter := NewExecutionAdapter(dispatcher)
	_, err := adapter.RunShell(context.Background(), ToolContext{AgentKind: AgentKindManager}, HostCommandRequest{
		HostID: "host-a",
		Script: "uptime",
	})
	if !errors.Is(err, ErrManagerDirectHostDenied) {
		t.Fatalf("err = %v, want ErrManagerDirectHostDenied", err)
	}
}

type fakeHostDispatcher struct {
	lastHostID string
	lastTask   scheduler.Task
}

func (d *fakeHostDispatcher) Dispatch(_ context.Context, task scheduler.Task) (scheduler.Result, error) {
	d.lastHostID = task.Host.Name
	d.lastTask = task
	return scheduler.Result{
		TaskID: task.ID,
		Status: "success",
		Output: map[string]any{"stdout": "ok", "exitCode": 0},
	}, nil
}
