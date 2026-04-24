package script

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"runner/modules"
)

type Module struct {
	language string
}

func New(language string) *Module {
	return &Module{language: language}
}

func (m *Module) Check(ctx context.Context, req modules.Request) (modules.Result, error) {
	source, err := readScript(req)
	if err != nil {
		return modules.Result{}, err
	}
	return modules.Result{
		Changed: true,
		Diff: map[string]any{
			"language": m.language,
			"script":   source,
		},
	}, nil
}

func (m *Module) Apply(ctx context.Context, req modules.Request) (modules.Result, error) {
	script, err := readScript(req)
	if err != nil {
		return modules.Result{}, err
	}
	args, err := readArgs(req)
	if err != nil {
		return modules.Result{}, err
	}

	var execCmd *exec.Cmd
	switch m.language {
	case "shell":
		execCmd = exec.CommandContext(ctx, "/bin/sh", append([]string{"-s", "--"}, args...)...)
	case "python":
		execCmd = exec.CommandContext(ctx, "python3", append([]string{"-"}, args...)...)
	default:
		return modules.Result{}, fmt.Errorf("unsupported script language: %s", m.language)
	}

	if dir, ok := readString(req, "dir"); ok {
		execCmd.Dir = dir
	}
	if env, ok := readEnv(req); ok {
		execCmd.Env = append(os.Environ(), env...)
	}

	execCmd.Stdin = strings.NewReader(script)
	var stdout, stderr bytes.Buffer
	stdoutWriter := io.Writer(&stdout)
	stderrWriter := io.Writer(&stderr)
	if req.Stdout != nil {
		stdoutWriter = io.MultiWriter(stdoutWriter, req.Stdout)
	}
	if req.Stderr != nil {
		stderrWriter = io.MultiWriter(stderrWriter, req.Stderr)
	}
	execCmd.Stdout = stdoutWriter
	execCmd.Stderr = stderrWriter

	err = execCmd.Run()
	stdoutText, stderrText := modules.ApplyOutputLimits(req, stdout.String(), stderr.String())
	result := modules.Result{
		Changed: true,
		Output: map[string]any{
			"stdout": stdoutText,
			"stderr": stderrText,
		},
	}
	if modules.ExportVarsEnabled(req) {
		if exports := modules.ParseExportVars(stdoutText); len(exports) > 0 {
			result.Output["vars"] = exports
		}
	}
	if err != nil {
		return result, fmt.Errorf("script.%s failed: %w", m.language, err)
	}
	return result, nil
}

func (m *Module) Rollback(ctx context.Context, req modules.Request) (modules.Result, error) {
	return modules.Result{}, fmt.Errorf("script.%s rollback not supported", m.language)
}

func readScript(req modules.Request) (string, error) {
	if _, ok := readString(req, "script_ref"); ok {
		return "", fmt.Errorf("script_ref is no longer supported; use args.script")
	}
	script, ok := readString(req, "script")
	if !ok {
		return "", fmt.Errorf("script requires args.script")
	}
	if strings.TrimSpace(script) == "" {
		return "", fmt.Errorf("script content is empty")
	}
	return script, nil
}

func readArgs(req modules.Request) ([]string, error) {
	if req.Step.Args == nil {
		return nil, nil
	}
	raw, ok := req.Step.Args["args"]
	if !ok || raw == nil {
		return nil, nil
	}

	switch v := raw.(type) {
	case []string:
		return append([]string{}, v...), nil
	case []any:
		args := make([]string, 0, len(v))
		for _, item := range v {
			args = append(args, fmt.Sprint(item))
		}
		return args, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		return []string{v}, nil
	default:
		return nil, fmt.Errorf("args must be list or string")
	}
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

func readEnv(req modules.Request) ([]string, bool) {
	merged := map[string]string{}

	if req.Vars != nil {
		if raw, ok := req.Vars["env"]; ok {
			mergeEnvMap(merged, raw)
		}
	}
	if req.Step.Args != nil {
		if raw, ok := req.Step.Args["env"]; ok {
			mergeEnvMap(merged, raw)
		}
	}

	if len(merged) == 0 {
		return nil, false
	}

	result := make([]string, 0, len(merged))
	for k, v := range merged {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result, true
}

func mergeEnvMap(dst map[string]string, raw any) {
	switch env := raw.(type) {
	case map[string]any:
		for k, v := range env {
			dst[k] = fmt.Sprint(v)
		}
	case map[any]any:
		for k, v := range env {
			dst[fmt.Sprint(k)] = fmt.Sprint(v)
		}
	case map[string]string:
		for k, v := range env {
			dst[k] = v
		}
	}
}
