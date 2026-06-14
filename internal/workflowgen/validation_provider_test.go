package workflowgen

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"runner/workflow/visual"
)

func TestStaticValidationProviderPassesGeneratedGraph(t *testing.T) {
	builder := DeterministicPlanBuilder{}
	plan, err := builder.BuildPlan(context.Background(), BuildPlanRequest{
		Requirement: "每天早上8点抓取AI新闻，提取三条关键内容直接返回给我",
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	graph, err := (RunnerGraphGenerator{}).GenerateGraph(context.Background(), GenerateGraphRequest{
		SessionID: "wfgen-static",
		Plan:      *plan,
	})
	if err != nil {
		t.Fatalf("GenerateGraph() error = %v", err)
	}
	result, err := (StaticValidationProvider{}).Validate(context.Background(), ValidationRequest{
		SessionID: "wfgen-static",
		Graph:     graph,
		Scenario:  "news-summary-return-only",
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Status != "passed" {
		t.Fatalf("status = %q, want passed: %#v", result.Status, result)
	}
}

func TestDockerValidatorSkipsWhenDockerUnavailable(t *testing.T) {
	result, err := (DockerValidator{
		LookPath: func(string) (string, error) {
			return "", exec.ErrNotFound
		},
	}).Validate(context.Background(), ValidationRequest{Scenario: "news-summary-return-only"})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Status != "skipped" || result.SkippedReason == "" {
		t.Fatalf("result = %#v, want skipped with reason", result)
	}
}

func TestDockerValidatorReturnsFailedSummaryFromCommand(t *testing.T) {
	result, err := (DockerValidator{
		DockerBinary: "false",
		Command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c", "echo failed >&2; exit 3")
		},
	}).Validate(context.Background(), ValidationRequest{Scenario: "news-summary-return-only"})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Status != "failed" || result.ExitCode != 3 {
		t.Fatalf("result = %#v, want failed exit 3", result)
	}
	if result.StderrSummary == "" {
		t.Fatalf("stderr summary is empty: %#v", result)
	}
}

func TestDockerValidatorExtractsNodeExecutionResults(t *testing.T) {
	builder := DeterministicPlanBuilder{}
	plan, err := builder.BuildPlan(context.Background(), BuildPlanRequest{
		Requirement: "每天早上8点抓取AI新闻，提取三条关键内容直接返回给我",
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	graph, err := (RunnerGraphGenerator{}).GenerateGraph(context.Background(), GenerateGraphRequest{
		SessionID: "wfgen-docker",
		Plan:      *plan,
	})
	if err != nil {
		t.Fatalf("GenerateGraph() error = %v", err)
	}

	result, err := (DockerValidator{
		DockerBinary: "docker",
		Command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c", `printf '%s\n' '{"node_results":[{"node_id":"search-news","action":"script.python","status":"passed","stdout_summary":"AIOPS_NODE_RESULT_BEGIN"}]}'`)
		},
	}).Validate(context.Background(), ValidationRequest{Graph: graph, Scenario: "news-summary-return-only"})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if len(result.NodeResults) != 1 {
		t.Fatalf("NodeResults = %#v, want one node result", result.NodeResults)
	}
	if got := result.NodeResults[0]; got.NodeID != "search-news" || got.Action != "script.python" || got.Status != "passed" {
		t.Fatalf("NodeResults[0] = %#v, want search-news script.python passed", got)
	}
}

func TestDockerValidatorUsesConfiguredImage(t *testing.T) {
	var capturedArgs []string
	result, err := (DockerValidator{
		DockerBinary: "docker",
		Image:        "python:3.12-bookworm",
		Command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			capturedArgs = append([]string(nil), args...)
			return exec.CommandContext(ctx, "sh", "-c", `printf '%s\n' '{"node_results":[{"node_id":"node-1","action":"script.python","status":"passed"}]}'`)
		},
	}).Validate(context.Background(), ValidationRequest{
		Graph: visual.Graph{
			Nodes: []visual.Node{{ID: "node-1"}},
			Edges: []visual.Edge{{ID: "edge-1", Source: "node-1", Target: "node-2"}},
		},
		AllowedImages: []string{"python:3.12-slim"},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Image != "python:3.12-bookworm" {
		t.Fatalf("Image = %q, want configured image", result.Image)
	}
	if !containsString(capturedArgs, "python:3.12-bookworm") {
		t.Fatalf("docker args = %#v, want configured image", capturedArgs)
	}
	if containsString(capturedArgs, "python:3.12-slim") {
		t.Fatalf("docker args = %#v, should not use fallback allowed image", capturedArgs)
	}
}

func TestDockerVerificationScriptUsesPythonBooleanLiterals(t *testing.T) {
	script := verificationScript(ValidationRequest{
		Graph: visual.Graph{
			Nodes: []visual.Node{{ID: "node-1"}},
			Edges: []visual.Edge{{ID: "edge-1", Source: "node-1", Target: "node-2"}},
		},
	}, nil)

	if strings.Contains(script, `"passed": true`) || strings.Contains(script, `"passed": false`) {
		t.Fatalf("verification script contains JSON boolean literals that are invalid in Python:\n%s", script)
	}
	if !strings.Contains(script, `"passed": True`) {
		t.Fatalf("verification script does not contain Python boolean literals:\n%s", script)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestDockerValidatorResolveDockerBinaryUsesLookPath(t *testing.T) {
	_, err := (DockerValidator{
		LookPath: func(string) (string, error) {
			return "", errors.New("missing")
		},
	}).resolveDockerBinary()
	if err == nil {
		t.Fatal("resolveDockerBinary() error = nil, want missing error")
	}
}
