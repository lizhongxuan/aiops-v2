package workflowgen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type DockerValidator struct {
	DockerBinary string
	Image        string
	LookPath     func(string) (string, error)
	Command      func(context.Context, string, ...string) *exec.Cmd
}

func (v DockerValidator) Name() ValidationProvider {
	return ValidationProviderDocker
}

func (v DockerValidator) Validate(ctx context.Context, req ValidationRequest) (*ValidationResult, error) {
	started := time.Now().UTC()
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	docker, err := v.resolveDockerBinary()
	if err != nil {
		return skippedDockerValidation(started, req, "Docker 不可用，已完成静态验证；动态验证跳过。"), nil
	}
	image := firstNonEmpty(v.Image, firstAllowedImage(req.AllowedImages), "python:3.12-slim")
	tmpDir, err := os.MkdirTemp("", "aiops-workflow-validation-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	nodeScripts := dockerNodeScripts(req)
	if len(nodeScripts) > 0 {
		nodesDir := filepath.Join(tmpDir, "nodes")
		if err := os.MkdirAll(nodesDir, 0o700); err != nil {
			return nil, err
		}
		for _, node := range nodeScripts {
			if err := os.WriteFile(filepath.Join(nodesDir, node.FileName), []byte(node.Script), 0o600); err != nil {
				return nil, err
			}
		}
	}
	verifyPath := filepath.Join(tmpDir, "verify.py")
	if err := os.WriteFile(verifyPath, []byte(verificationScript(req, nodeScripts)), 0o600); err != nil {
		return nil, err
	}

	args := []string{
		"run", "--rm",
		"--network", dockerNetwork(req.NetworkPolicy),
		"--cpus", "1",
		"--memory", "512m",
		"-v", tmpDir + ":/workspace:ro",
		"-w", "/workspace",
		image,
		"python", "verify.py",
	}
	cmdFn := v.Command
	if cmdFn == nil {
		cmdFn = exec.CommandContext
	}
	cmd := cmdFn(ctx, docker, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	ended := time.Now().UTC()
	result := &ValidationResult{
		ID:            "docker-" + started.Format("20060102150405.000000000"),
		Provider:      ValidationProviderDocker,
		Status:        "passed",
		Scenario:      req.Scenario,
		Summary:       "Docker validation passed.",
		Image:         image,
		StdoutSummary: limitString(stdout.String(), 4096),
		StderrSummary: limitString(stderr.String(), 4096),
		NodeResults:   parseDockerNodeResults(stdout.String()),
		StartedAt:     started,
		EndedAt:       ended,
		DurationMs:    ended.Sub(started).Milliseconds(),
	}
	if err != nil {
		result.Status = "failed"
		result.Summary = "Docker validation failed."
		if exitErr := new(exec.ExitError); errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() != nil {
			result.Summary = "Docker validation timed out."
		}
	}
	return result, nil
}

func (v DockerValidator) resolveDockerBinary() (string, error) {
	if strings.TrimSpace(v.DockerBinary) != "" {
		return strings.TrimSpace(v.DockerBinary), nil
	}
	lookPath := v.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	return lookPath("docker")
}

func skippedDockerValidation(started time.Time, req ValidationRequest, reason string) *ValidationResult {
	ended := time.Now().UTC()
	return &ValidationResult{
		ID:            "docker-skipped-" + started.Format("20060102150405.000000000"),
		Provider:      ValidationProviderDocker,
		Status:        "skipped",
		Scenario:      req.Scenario,
		Summary:       reason,
		StartedAt:     started,
		EndedAt:       ended,
		DurationMs:    ended.Sub(started).Milliseconds(),
		SkippedReason: reason,
	}
}

type dockerNodeScript struct {
	NodeID   string `json:"node_id"`
	Action   string `json:"action"`
	FileName string `json:"file_name"`
	Path     string `json:"path"`
	Script   string `json:"-"`
}

func dockerNodeScripts(req ValidationRequest) []dockerNodeScript {
	var scripts []dockerNodeScript
	for _, node := range req.Graph.Nodes {
		if node.Step == nil {
			continue
		}
		action := strings.TrimSpace(node.Step.Action)
		if action != "script.python" {
			continue
		}
		rawScript, ok := node.Step.Args["script"].(string)
		if !ok || strings.TrimSpace(rawScript) == "" {
			continue
		}
		nodeID := firstNonEmpty(node.ID, node.Step.ID, node.Step.Name)
		fileName := sanitizeID(nodeID) + ".py"
		if fileName == ".py" {
			fileName = fmt.Sprintf("node-%d.py", len(scripts)+1)
		}
		scripts = append(scripts, dockerNodeScript{
			NodeID:   nodeID,
			Action:   action,
			FileName: fileName,
			Path:     "/workspace/nodes/" + fileName,
			Script:   rawScript,
		})
	}
	return scripts
}

func verificationScript(req ValidationRequest, scripts []dockerNodeScript) string {
	nodeCount := len(req.Graph.Nodes)
	edgeCount := len(req.Graph.Edges)
	if scripts == nil {
		scripts = []dockerNodeScript{}
	}
	scriptPayload, _ := json.Marshal(scripts)
	return fmt.Sprintf(`import json
import subprocess
import sys
import time

scripts = %s
node_results = []
for item in scripts:
    started = time.time()
    try:
        proc = subprocess.run(
            [sys.executable, item["path"]],
            capture_output=True,
            text=True,
            timeout=20,
        )
        stdout = proc.stdout or ""
        stderr = proc.stderr or ""
        status = "passed" if proc.returncode == 0 and "AIOPS_NODE_RESULT_BEGIN" in stdout and "AIOPS_NODE_RESULT_END" in stdout else "failed"
        node_results.append({
            "node_id": item["node_id"],
            "action": item.get("action", ""),
            "status": status,
            "summary": "节点脚本在 Docker 中执行成功。" if status == "passed" else "节点脚本在 Docker 中执行失败。",
            "exit_code": proc.returncode,
            "stdout_summary": stdout[:2000],
            "stderr_summary": stderr[:2000],
            "duration_ms": int((time.time() - started) * 1000),
        })
    except Exception as exc:
        node_results.append({
            "node_id": item["node_id"],
            "action": item.get("action", ""),
            "status": "failed",
            "summary": "节点脚本在 Docker 中执行异常。",
            "stderr_summary": str(exc)[:2000],
            "duration_ms": int((time.time() - started) * 1000),
        })

result = {
    "scenario": %q,
    "node_count": %d,
    "edge_count": %d,
    "assertions": [
        {"name": "graph_has_nodes", "passed": %s},
        {"name": "graph_has_edges", "passed": %s},
    ],
    "node_results": node_results,
}
print(json.dumps(result, ensure_ascii=False))
if not all(item["passed"] for item in result["assertions"]) or any(item["status"] != "passed" for item in node_results):
    raise SystemExit(2)
`, string(scriptPayload), req.Scenario, nodeCount, edgeCount, pyBool(nodeCount > 0), pyBool(edgeCount > 0))
}

func parseDockerNodeResults(stdout string) []NodeValidationSummary {
	lines := strings.Split(stdout, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var payload struct {
			NodeResults []NodeValidationSummary `json:"node_results"`
		}
		if err := json.Unmarshal([]byte(line), &payload); err == nil && len(payload.NodeResults) > 0 {
			return payload.NodeResults
		}
	}
	return nil
}

func firstAllowedImage(images []string) string {
	for _, image := range images {
		if strings.TrimSpace(image) != "" {
			return strings.TrimSpace(image)
		}
	}
	return ""
}

func dockerNetwork(policy string) string {
	switch strings.TrimSpace(strings.ToLower(policy)) {
	case "", "none", "mock":
		return "none"
	case "egress", "bridge":
		return "bridge"
	default:
		return "none"
	}
}

func limitString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}
