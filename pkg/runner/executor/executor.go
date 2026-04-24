package executor

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"runner/logging"
	"runner/workflow"
)

type RunResult struct {
	Output map[string]any
}

type HostRunner interface {
	Run(ctx context.Context, step workflow.Step, host workflow.HostSpec, vars map[string]any) (RunResult, error)
}

type Observer interface {
	StepStart(step workflow.Step, targets []workflow.HostSpec)
	StepFinish(step workflow.Step, status string)
}

type Executor struct {
	Runner   HostRunner
	Observer Observer
}

func (e *Executor) Run(ctx context.Context, wf workflow.Workflow) error {
	if e.Runner == nil {
		return fmt.Errorf("executor runner is nil")
	}
	logging.L().Debug("executor run start",
		zap.String("workflow", wf.Name),
		zap.Int("steps", len(wf.Steps)),
	)

	hosts := wf.Inventory.ResolveHosts()
	handlers := map[string]workflow.Handler{}
	for _, handler := range wf.Handlers {
		handlers[handler.Name] = handler
	}

	runtimeVars := mergeVars(wf.Vars, nil)
	allowedVars := map[string]any{}
	for _, step := range wf.Steps {
		shouldRun, err := evalWhen(step.When, runtimeVars)
		if err != nil {
			return err
		}
		if !shouldRun {
			continue
		}

		targets, err := resolveTargets(step, hosts, wf.Inventory)
		if err != nil {
			return err
		}

		logging.L().Debug("executor step start",
			zap.String("step", step.Name),
			zap.String("action", step.Action),
			zap.Int("targets", len(targets)),
		)
		if e.Observer != nil {
			e.Observer.StepStart(step, targets)
		}

		if err := validateMustVars(step.MustVars, targets, allowedVars); err != nil {
			logging.L().Debug("executor step missing required vars",
				zap.String("step", step.Name),
				zap.Error(err),
			)
			if e.Observer != nil {
				e.Observer.StepFinish(step, "failed")
			}
			return err
		}

		loopItems := step.Loop
		if len(loopItems) == 0 {
			loopItems = []any{nil}
		}

		stepFailed := false
		stepExports := map[string]any{}
		for _, item := range loopItems {
			stepVars, err := e.runOnTargets(ctx, step, targets, runtimeVars, item)
			if err != nil {
				logging.L().Debug("executor step failed", zap.String("step", step.Name), zap.Error(err))
				if step.ContinueOnError {
					stepFailed = true
					break
				}
				if e.Observer != nil {
					e.Observer.StepFinish(step, "failed")
				}
				return err
			}
			if len(stepVars) > 0 {
				stepExports = mergeVars(stepExports, stepVars)
				runtimeVars = mergeExportedVars(runtimeVars, stepVars)
			}
			if len(step.Notify) > 0 {
				if err := e.runHandlers(ctx, handlers, step.Notify, targets, runtimeVars, item); err != nil {
					logging.L().Debug("executor handler failed", zap.String("step", step.Name), zap.Error(err))
					if step.ContinueOnError {
						stepFailed = true
						break
					}
					if e.Observer != nil {
						e.Observer.StepFinish(step, "failed")
					}
					return err
				}
			}
		}

		if !stepFailed && len(step.ExpectVars) > 0 {
			if err := validateExpectedVars(step.ExpectVars, stepExports); err != nil {
				logging.L().Debug("executor step expected vars missing",
					zap.String("step", step.Name),
					zap.Error(err),
				)
				if step.ContinueOnError {
					stepFailed = true
				} else {
					if e.Observer != nil {
						e.Observer.StepFinish(step, "failed")
					}
					return err
				}
			} else if len(stepExports) > 0 {
				allowedVars = mergeVars(allowedVars, selectExpectedVars(stepExports, step.ExpectVars))
			}
		}

		if e.Observer != nil {
			if stepFailed {
				e.Observer.StepFinish(step, "failed")
			} else {
				e.Observer.StepFinish(step, "success")
			}
		}
		logging.L().Debug("executor step done", zap.String("step", step.Name), zap.Bool("failed", stepFailed))
	}

	logging.L().Debug("executor run done", zap.String("workflow", wf.Name))
	return nil
}

func (e *Executor) runOnTargets(ctx context.Context, step workflow.Step, targets []workflow.HostSpec, baseVars map[string]any, item any) (map[string]any, error) {
	timeout, err := parseTimeout(step.Timeout)
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(targets))
	outCh := make(chan map[string]any, len(targets))

	for _, target := range targets {
		target := target
		wg.Add(1)
		go func() {
			defer wg.Done()

			vars := mergeVars(target.Vars, baseVars)
			if item != nil {
				vars = mergeVars(vars, map[string]any{"item": item})
			}

			result, err := runWithRetry(ctx, e.Runner, step, target, vars, timeout)
			if err != nil {
				errCh <- err
				return
			}
			if exported := extractExportedVars(result.Output); len(exported) > 0 {
				outCh <- exported
			}
		}()
	}

	wg.Wait()
	close(errCh)
	close(outCh)

	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}

	merged := map[string]any{}
	for vars := range outCh {
		merged = mergeVars(merged, vars)
	}
	if len(merged) == 0 {
		return nil, nil
	}
	return merged, nil
}

func (e *Executor) runHandlers(ctx context.Context, handlers map[string]workflow.Handler, notify []string, targets []workflow.HostSpec, baseVars map[string]any, item any) error {
	seen := map[string]struct{}{}
	for _, name := range notify {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}

		handler, ok := handlers[name]
		if !ok {
			return fmt.Errorf("handler %q not found", name)
		}
		logging.L().Debug("executor handler start", zap.String("handler", handler.Name))

		handlerVars := baseVars
		if item != nil {
			handlerVars = mergeVars(handlerVars, map[string]any{"item": item})
		}
		shouldRun, err := evalWhen(handler.When, handlerVars)
		if err != nil {
			return err
		}
		if !shouldRun {
			continue
		}

		step := workflow.Step{
			Name:    handler.Name,
			Action:  handler.Action,
			Args:    handler.Args,
			Retries: handler.Retries,
			Timeout: handler.Timeout,
		}

		if _, err := e.runOnTargets(ctx, step, targets, baseVars, item); err != nil {
			return err
		}
	}
	return nil
}

func runWithRetry(ctx context.Context, runner HostRunner, step workflow.Step, host workflow.HostSpec, vars map[string]any, timeout time.Duration) (RunResult, error) {
	attempts := step.Retries + 1
	var lastErr error
	var lastResult RunResult

	for i := 0; i < attempts; i++ {
		if ctx.Err() != nil {
			return lastResult, ctx.Err()
		}

		runCtx := ctx
		cancel := func() {}
		if timeout > 0 {
			runCtx, cancel = context.WithTimeout(ctx, timeout)
		}

		result, err := runner.Run(runCtx, step, host, vars)
		cancel()

		if err == nil {
			return result, nil
		}
		lastResult = result
		lastErr = err
		logging.L().Debug("executor retry",
			zap.String("step", step.Name),
			zap.String("host", host.Name),
			zap.Int("attempt", i+1),
			zap.Error(err),
		)
	}

	return lastResult, lastErr
}

func parseTimeout(raw string) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout %q", raw)
	}
	return parsed, nil
}

func evalWhen(expr string, vars map[string]any) (bool, error) {
	return workflow.EvalWhen(expr, vars)
}

func resolveTargets(step workflow.Step, hosts map[string]workflow.HostSpec, inv workflow.Inventory) ([]workflow.HostSpec, error) {
	selected := map[string]workflow.HostSpec{}
	if len(step.Targets) == 0 {
		if len(hosts) == 0 {
			return nil, fmt.Errorf("no hosts defined in inventory")
		}
		for name, host := range hosts {
			selected[name] = host
		}
		return stableHosts(selected), nil
	}

	for _, target := range step.Targets {
		if host, ok := hosts[target]; ok {
			selected[target] = host
			continue
		}
		if group, ok := inv.Groups[target]; ok {
			for _, hostName := range group.Hosts {
				if host, ok := hosts[hostName]; ok {
					selected[hostName] = host
				}
			}
			continue
		}
		if strings.EqualFold(strings.TrimSpace(target), "local") {
			local := builtinLocalHost(inv)
			selected[local.Name] = local
			continue
		}
		return nil, fmt.Errorf("unknown target %q", target)
	}

	if len(selected) == 0 {
		return nil, fmt.Errorf("no targets resolved for step %q", step.Name)
	}

	return stableHosts(selected), nil
}

func builtinLocalHost(inv workflow.Inventory) workflow.HostSpec {
	vars := map[string]any{}
	for k, v := range inv.Vars {
		vars[k] = v
	}
	return workflow.HostSpec{
		Name:    "local",
		Address: "127.0.0.1",
		Vars:    vars,
	}
}

func stableHosts(hosts map[string]workflow.HostSpec) []workflow.HostSpec {
	names := make([]string, 0, len(hosts))
	for name := range hosts {
		names = append(names, name)
	}
	sort.Strings(names)

	ordered := make([]workflow.HostSpec, 0, len(names))
	for _, name := range names {
		ordered = append(ordered, hosts[name])
	}
	return ordered
}

func mergeVars(base, overlay map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func validateExpectedVars(expected []string, exported map[string]any) error {
	if len(expected) == 0 {
		return nil
	}
	missing := []string{}
	for _, raw := range expected {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, ok := exported[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("expected vars missing: %s", strings.Join(missing, ", "))
}

func validateMustVars(must []string, targets []workflow.HostSpec, allowedVars map[string]any) error {
	if len(must) == 0 {
		return nil
	}
	for _, target := range targets {
		merged := mergeVars(target.Vars, allowedVars)
		missing := missingVars(must, merged)
		if len(missing) > 0 {
			return fmt.Errorf("required vars missing for host %s: %s", target.Name, strings.Join(missing, ", "))
		}
	}
	return nil
}

func selectExpectedVars(exported map[string]any, expected []string) map[string]any {
	if len(exported) == 0 || len(expected) == 0 {
		return nil
	}
	result := map[string]any{}
	for _, raw := range expected {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if val, ok := exported[key]; ok {
			result[key] = val
		}
	}
	return result
}

func missingVars(required []string, vars map[string]any) []string {
	missing := []string{}
	for _, raw := range required {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, ok := vars[key]; !ok {
			missing = append(missing, key)
		}
	}
	return missing
}

func mergeExportedVars(base map[string]any, exported map[string]any) map[string]any {
	if len(exported) == 0 {
		return base
	}
	next := mergeVars(base, exported)
	env := map[string]any{}
	if raw, ok := next["env"]; ok {
		if casted := coerceStringMap(raw); casted != nil {
			for k, v := range casted {
				env[k] = v
			}
		}
	}
	for k, v := range exported {
		env[k] = v
	}
	if len(env) > 0 {
		next["env"] = env
	}
	return next
}

func extractExportedVars(output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	raw, ok := output["vars"]
	if !ok {
		return nil
	}
	return coerceStringMap(raw)
}

func coerceStringMap(raw any) map[string]any {
	switch v := raw.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range v {
			out[k] = val
		}
		return out
	case map[string]string:
		out := map[string]any{}
		for k, val := range v {
			out[k] = val
		}
		return out
	case map[any]any:
		out := map[string]any{}
		for k, val := range v {
			out[fmt.Sprint(k)] = val
		}
		return out
	default:
		return nil
	}
}
