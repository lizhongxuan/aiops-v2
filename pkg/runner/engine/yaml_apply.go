package engine

//
//import (
//	"context"
//	"fmt"
//	"os"
//	"path/filepath"
//	"strconv"
//	"strings"
//	"time"
//
//	"runner/scheduler"
//	"runner/state"
//	"runner/workflow"
//)
//
//const (
//	runtimeRefPrefix  = "runtime://"
//	defaultRuntimeDir = "runtime"
//)
//
//// ApplyYAML executes workflow YAML directly with built-in engine/registry.
//// yamlTextOrRef supports:
//// 1) raw yaml text
//// 2) filesystem yaml path
//// 3) runtime://<relative-path> reference
//func ApplyYAML(ctx context.Context, yamlTextOrRef string, params map[string]any) error {
//	_, err := ApplyYAMLWithRun(ctx, yamlTextOrRef, params)
//	return err
//}
//
//// ApplyYAMLWithRun executes workflow YAML and returns run snapshot.
//// It performs load -> inject -> validate -> run in one place.
//func ApplyYAMLWithRun(ctx context.Context, yamlTextOrRef string, params map[string]any) (state.RunState, error) {
//	wf, err := loadWorkflowFromInput(yamlTextOrRef, params)
//	if err != nil {
//		return state.RunState{}, err
//	}
//	if err := injectWorkflowParams(&wf, params); err != nil {
//		return state.RunState{}, err
//	}
//	if err := wf.Validate(); err != nil {
//		return state.RunState{}, err
//	}
//
//	eng := NewDefault()
//	if dispatcher, err := buildDispatcherFromParams(params); err != nil {
//		return state.RunState{}, err
//	} else if dispatcher != nil {
//		eng.Dispatcher = dispatcher
//	}
//
//	runOpts := RunOptions{}
//	if runID := readStringParam(params, "run_id"); runID != "" {
//		runOpts.RunID = runID
//	}
//	return eng.ApplyWithRun(ctx, wf, runOpts)
//}
//
//func loadWorkflowFromInput(yamlTextOrRef string, params map[string]any) (workflow.Workflow, error) {
//	raw := strings.TrimSpace(yamlTextOrRef)
//	if raw == "" {
//		return workflow.Workflow{}, fmt.Errorf("runner yaml is empty")
//	}
//
//	if strings.HasPrefix(raw, runtimeRefPrefix) {
//		path, err := resolveRuntimeRef(raw, params)
//		if err != nil {
//			return workflow.Workflow{}, err
//		}
//		return workflow.LoadFile(path)
//	}
//
//	// file path mode (single-line existing file)
//	if !strings.Contains(raw, "\n") && !strings.Contains(raw, "\r") {
//		if stat, err := os.Stat(raw); err == nil && !stat.IsDir() {
//			return workflow.LoadFile(raw)
//		}
//	}
//	return workflow.Load([]byte(raw))
//}
//
//func resolveRuntimeRef(ref string, params map[string]any) (string, error) {
//	rel := strings.TrimSpace(strings.TrimPrefix(ref, runtimeRefPrefix))
//	rel = strings.TrimPrefix(rel, "/")
//	if rel == "" {
//		return "", fmt.Errorf("invalid runtime ref: %q", ref)
//	}
//	clean := filepath.Clean(rel)
//	if clean == "." || strings.HasPrefix(clean, "..") {
//		return "", fmt.Errorf("invalid runtime ref path: %q", ref)
//	}
//	return filepath.Join(resolveRuntimeBaseDir(params), clean), nil
//}
//
//func resolveRuntimeBaseDir(params map[string]any) string {
//	if base := strings.TrimSpace(readStringParam(params, "runtime_base_dir")); base != "" {
//		return base
//	}
//	if base := strings.TrimSpace(os.Getenv("RUNNER_RUNTIME_PATH")); base != "" {
//		return base
//	}
//	if base := strings.TrimSpace(os.Getenv("KME_RUNTIME_PATH")); base != "" {
//		return base
//	}
//	return defaultRuntimeDir
//}
//
//func injectWorkflowParams(wf *workflow.Workflow, params map[string]any) error {
//	if wf == nil || len(params) == 0 {
//		return nil
//	}
//
//	if varsRaw, ok := params["vars"]; ok {
//		varsMap, ok := toStringAnyMap(varsRaw)
//		if !ok {
//			return fmt.Errorf("params.vars must be map")
//		}
//		wf.Vars = mergeAnyMap(wf.Vars, varsMap)
//	}
//
//	if hostsRaw, ok := params["hosts"]; ok {
//		hostsMap, ok := toStringAnyMap(hostsRaw)
//		if !ok {
//			return fmt.Errorf("params.hosts must be map")
//		}
//		if wf.Inventory.Hosts == nil {
//			wf.Inventory.Hosts = map[string]workflow.Host{}
//		}
//		for hostName, hostRaw := range hostsMap {
//			host := wf.Inventory.Hosts[hostName]
//			if err := applyHostOverride(&host, hostRaw); err != nil {
//				return fmt.Errorf("apply host %q override failed: %w", hostName, err)
//			}
//			wf.Inventory.Hosts[hostName] = host
//		}
//	}
//
//	stepArgs, err := readStepArgsOverride(params, "step_args")
//	if err != nil {
//		return err
//	}
//	stepScripts, err := readStepScriptOverride(params, "step_script")
//	if err != nil {
//		return err
//	}
//	stepExpectVars, err := readStepExpectVarsOverride(params, "step_expect_vars")
//	if err != nil {
//		return err
//	}
//
//	for i := range wf.Steps {
//		step := &wf.Steps[i]
//		if args, ok := stepArgs[step.Name]; ok {
//			step.Args = mergeAnyMap(step.Args, args)
//		}
//		if script, ok := stepScripts[step.Name]; ok {
//			if strings.TrimSpace(step.Action) == "" {
//				step.Action = "shell.run"
//			}
//			if step.Action != "shell.run" {
//				return fmt.Errorf("step %q action must be shell.run when step_script is set", step.Name)
//			}
//			if step.Args == nil {
//				step.Args = map[string]any{}
//			}
//			step.Args["script"] = script
//		}
//		if expectVars, ok := stepExpectVars[step.Name]; ok {
//			step.ExpectVars = expectVars
//		}
//	}
//
//	return nil
//}
//
//func applyHostOverride(host *workflow.Host, hostRaw any) error {
//	if host == nil {
//		return fmt.Errorf("host is nil")
//	}
//	switch v := hostRaw.(type) {
//	case string:
//		host.Address = v
//		return nil
//	case map[string]any:
//		return applyHostOverrideFromMap(host, v)
//	case map[any]any:
//		vm := make(map[string]any, len(v))
//		for k, val := range v {
//			vm[fmt.Sprint(k)] = val
//		}
//		return applyHostOverrideFromMap(host, vm)
//	default:
//		return fmt.Errorf("invalid host override type %T", hostRaw)
//	}
//}
//
//func applyHostOverrideFromMap(host *workflow.Host, m map[string]any) error {
//	if m == nil {
//		return nil
//	}
//	if addr, ok := readString(m, "address"); ok {
//		host.Address = addr
//	}
//	if varsRaw, ok := m["vars"]; ok {
//		varsMap, ok := toStringAnyMap(varsRaw)
//		if !ok {
//			return fmt.Errorf("host vars must be map")
//		}
//		host.Vars = mergeAnyMap(host.Vars, varsMap)
//	}
//	return nil
//}
//
//func readStepArgsOverride(params map[string]any, key string) (map[string]map[string]any, error) {
//	result := map[string]map[string]any{}
//	if len(params) == 0 {
//		return result, nil
//	}
//	raw, ok := params[key]
//	if !ok {
//		return result, nil
//	}
//	stepMap, ok := toStringAnyMap(raw)
//	if !ok {
//		return nil, fmt.Errorf("params.%s must be map", key)
//	}
//	for stepName, stepRaw := range stepMap {
//		args, ok := toStringAnyMap(stepRaw)
//		if !ok {
//			return nil, fmt.Errorf("params.%s.%s must be map", key, stepName)
//		}
//		result[stepName] = args
//	}
//	return result, nil
//}
//
//func readStepScriptOverride(params map[string]any, key string) (map[string]string, error) {
//	result := map[string]string{}
//	if len(params) == 0 {
//		return result, nil
//	}
//	raw, ok := params[key]
//	if !ok {
//		return result, nil
//	}
//	stepMap, ok := toStringAnyMap(raw)
//	if !ok {
//		return nil, fmt.Errorf("params.%s must be map", key)
//	}
//	for stepName, stepRaw := range stepMap {
//		script := strings.TrimSpace(fmt.Sprint(stepRaw))
//		if script == "" {
//			return nil, fmt.Errorf("params.%s.%s is empty", key, stepName)
//		}
//		result[stepName] = script
//	}
//	return result, nil
//}
//
//func readStepExpectVarsOverride(params map[string]any, key string) (map[string][]string, error) {
//	result := map[string][]string{}
//	if len(params) == 0 {
//		return result, nil
//	}
//	raw, ok := params[key]
//	if !ok {
//		return result, nil
//	}
//	stepMap, ok := toStringAnyMap(raw)
//	if !ok {
//		return nil, fmt.Errorf("params.%s must be map", key)
//	}
//	for stepName, stepRaw := range stepMap {
//		values, ok := toStringSlice(stepRaw)
//		if !ok {
//			return nil, fmt.Errorf("params.%s.%s must be string array", key, stepName)
//		}
//		result[stepName] = values
//	}
//	return result, nil
//}
//
//func buildDispatcherFromParams(params map[string]any) (scheduler.Dispatcher, error) {
//	if len(params) == 0 {
//		return nil, nil
//	}
//	raw, ok := params["dispatch"]
//	if !ok {
//		return nil, nil
//	}
//	dispatchMap, ok := toStringAnyMap(raw)
//	if !ok {
//		return nil, fmt.Errorf("params.dispatch must be map")
//	}
//	typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(dispatchMap["type"])))
//	if typ == "" || typ == "local" {
//		return nil, nil
//	}
//	if typ != "agent" {
//		return nil, fmt.Errorf("unsupported dispatch type %q", typ)
//	}
//
//	baseURL := readStringParam(dispatchMap, "base_url")
//	token := readStringParam(dispatchMap, "token")
//	dispatcher := scheduler.NewAgentDispatcherWithToken(baseURL, token)
//	if retryMax, ok := readIntParam(dispatchMap, "retry_max"); ok {
//		dispatcher.RetryMax = retryMax
//	}
//	if delaySec, ok := readInt64Param(dispatchMap, "retry_delay_sec"); ok {
//		dispatcher.RetryDelay = time.Duration(delaySec) * time.Second
//	}
//	if timeoutSec, ok := readInt64Param(dispatchMap, "async_timeout_sec"); ok {
//		dispatcher.AsyncTimeout = time.Duration(timeoutSec) * time.Second
//	}
//	if pollSec, ok := readInt64Param(dispatchMap, "poll_interval_sec"); ok {
//		dispatcher.PollInterval = time.Duration(pollSec) * time.Second
//	}
//	return dispatcher, nil
//}
//
//func toStringAnyMap(raw any) (map[string]any, bool) {
//	switch v := raw.(type) {
//	case map[string]any:
//		return v, true
//	case map[any]any:
//		m := make(map[string]any, len(v))
//		for k, vv := range v {
//			m[fmt.Sprint(k)] = vv
//		}
//		return m, true
//	default:
//		return nil, false
//	}
//}
//
//func toStringSlice(raw any) ([]string, bool) {
//	switch v := raw.(type) {
//	case []string:
//		out := make([]string, 0, len(v))
//		for _, s := range v {
//			s = strings.TrimSpace(s)
//			if s != "" {
//				out = append(out, s)
//			}
//		}
//		return out, true
//	case []any:
//		out := make([]string, 0, len(v))
//		for _, item := range v {
//			s := strings.TrimSpace(fmt.Sprint(item))
//			if s != "" {
//				out = append(out, s)
//			}
//		}
//		return out, true
//	default:
//		return nil, false
//	}
//}
//
//func mergeAnyMap(base, overlay map[string]any) map[string]any {
//	if len(base) == 0 && len(overlay) == 0 {
//		return nil
//	}
//	out := map[string]any{}
//	for k, v := range base {
//		out[k] = v
//	}
//	for k, v := range overlay {
//		out[k] = v
//	}
//	return out
//}
//
//func readStringParam(params map[string]any, key string) string {
//	v, ok := readString(params, key)
//	if !ok {
//		return ""
//	}
//	return strings.TrimSpace(v)
//}
//
//func readString(m map[string]any, key string) (string, bool) {
//	if len(m) == 0 {
//		return "", false
//	}
//	raw, ok := m[key]
//	if !ok {
//		return "", false
//	}
//	switch v := raw.(type) {
//	case string:
//		return v, true
//	default:
//		return fmt.Sprint(v), true
//	}
//}
//
//func readIntParam(m map[string]any, key string) (int, bool) {
//	if len(m) == 0 {
//		return 0, false
//	}
//	raw, ok := m[key]
//	if !ok {
//		return 0, false
//	}
//	switch v := raw.(type) {
//	case int:
//		return v, true
//	case int8:
//		return int(v), true
//	case int16:
//		return int(v), true
//	case int32:
//		return int(v), true
//	case int64:
//		return int(v), true
//	case uint:
//		return int(v), true
//	case uint8:
//		return int(v), true
//	case uint16:
//		return int(v), true
//	case uint32:
//		return int(v), true
//	case uint64:
//		return int(v), true
//	case float32:
//		return int(v), true
//	case float64:
//		return int(v), true
//	case string:
//		n, err := strconv.Atoi(strings.TrimSpace(v))
//		if err != nil {
//			return 0, false
//		}
//		return n, true
//	default:
//		return 0, false
//	}
//}
//
//func readInt64Param(m map[string]any, key string) (int64, bool) {
//	if len(m) == 0 {
//		return 0, false
//	}
//	raw, ok := m[key]
//	if !ok {
//		return 0, false
//	}
//	switch v := raw.(type) {
//	case int:
//		return int64(v), true
//	case int8:
//		return int64(v), true
//	case int16:
//		return int64(v), true
//	case int32:
//		return int64(v), true
//	case int64:
//		return v, true
//	case uint:
//		return int64(v), true
//	case uint8:
//		return int64(v), true
//	case uint16:
//		return int64(v), true
//	case uint32:
//		return int64(v), true
//	case uint64:
//		return int64(v), true
//	case float32:
//		return int64(v), true
//	case float64:
//		return int64(v), true
//	case string:
//		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
//		if err != nil {
//			return 0, false
//		}
//		return n, true
//	default:
//		return 0, false
//	}
//}
