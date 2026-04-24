package wait

import (
	"context"
	"fmt"
	"strings"
	"time"

	"runner/modules"
)

type Module struct {
	mode string
}

func NewUntil() *Module {
	return &Module{mode: "until"}
}

func NewEvent() *Module {
	return &Module{mode: "event"}
}

func (m *Module) Check(ctx context.Context, req modules.Request) (modules.Result, error) {
	diff := map[string]any{"mode": m.mode}
	if m.mode == "until" {
		if duration, ok := readString(req, "duration"); ok {
			diff["duration"] = duration
		}
	}
	if m.mode == "event" {
		if event, ok := readString(req, "event"); ok {
			diff["event"] = event
		}
	}
	return modules.Result{Changed: false, Diff: diff}, nil
}

func (m *Module) Apply(ctx context.Context, req modules.Request) (modules.Result, error) {
	switch m.mode {
	case "until":
		duration, ok := readString(req, "duration")
		if !ok || strings.TrimSpace(duration) == "" {
			return modules.Result{}, fmt.Errorf("wait.until requires args.duration")
		}
		parsed, err := time.ParseDuration(duration)
		if err != nil {
			return modules.Result{}, fmt.Errorf("wait.until invalid duration: %w", err)
		}
		timer := time.NewTimer(parsed)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return modules.Result{}, ctx.Err()
		case <-timer.C:
			return modules.Result{Changed: false}, nil
		}
	case "event":
		return modules.Result{}, fmt.Errorf("wait.event is not implemented yet")
	default:
		return modules.Result{}, fmt.Errorf("unsupported wait mode")
	}
}

func (m *Module) Rollback(ctx context.Context, req modules.Request) (modules.Result, error) {
	return modules.Result{}, fmt.Errorf("wait.%s rollback not supported", m.mode)
}

func readString(req modules.Request, key string) (string, bool) {
	if req.Step.Args == nil {
		return "", false
	}
	val, ok := req.Step.Args[key]
	if !ok {
		return "", false
	}
	switch v := val.(type) {
	case string:
		return v, true
	default:
		return fmt.Sprint(v), true
	}
}
