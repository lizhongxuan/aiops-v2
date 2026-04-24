package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ApplyOutputLimits(req Request, stdout, stderr string) (string, string) {
	maxBytes := readOutputLimit(req)
	if maxBytes > 0 {
		if len(stdout) > maxBytes {
			stdout = stdout[:maxBytes] + "...(truncated)"
		}
		if len(stderr) > maxBytes {
			stderr = stderr[:maxBytes] + "...(truncated)"
		}
	}

	path := readOutputPath(req)
	if path == "" {
		return stdout, stderr
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return stdout, stderr
	}
	ts := time.Now().UTC().Format("20060102-150405")
	stepName := strings.ReplaceAll(req.Step.Name, " ", "_")
	if stepName == "" {
		stepName = "step"
	}
	stdoutFile := filepath.Join(path, fmt.Sprintf("%s-%s-stdout.log", stepName, ts))
	stderrFile := filepath.Join(path, fmt.Sprintf("%s-%s-stderr.log", stepName, ts))
	_ = os.WriteFile(stdoutFile, []byte(stdout), 0o644)
	_ = os.WriteFile(stderrFile, []byte(stderr), 0o644)
	return stdout, stderr
}

func readOutputLimit(req Request) int {
	if req.Step.Args == nil {
		return 0
	}
	raw, ok := req.Step.Args["max_output_bytes"]
	if !ok || raw == nil {
		return 0
	}
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0
		}
		var out int
		_, _ = fmt.Sscanf(trimmed, "%d", &out)
		return out
	default:
		return 0
	}
}

func readOutputPath(req Request) string {
	if req.Step.Args == nil {
		return ""
	}
	raw, ok := req.Step.Args["output_path"]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
