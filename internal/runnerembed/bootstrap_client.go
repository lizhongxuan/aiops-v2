package runnerembed

import (
	"context"
	"strings"

	"aiops-v2/internal/appui"
	"runner/server/service"
	"runner/state"
	"runner/workflow/visual"
)

type BootstrapClient struct {
	runtime *Runtime
}

func NewBootstrapClient(runtime *Runtime) *BootstrapClient {
	return &BootstrapClient{runtime: runtime}
}

func (c *BootstrapClient) SubmitHostInstallGraph(ctx context.Context, graph visual.Graph, vars map[string]any, idempotencyKey string) (appui.HostInstallRun, error) {
	resp, err := c.runtime.SubmitGraphRun(ctx, graph, vars, "host-bootstrap", idempotencyKey)
	if err != nil {
		return appui.HostInstallRun{}, err
	}
	hostID, _ := vars["host_id"].(string)
	agentVersion, _ := vars["agent_version"].(string)
	return appui.HostInstallRun{
		HostID:       strings.TrimSpace(hostID),
		RunID:        resp.RunID,
		WorkflowID:   resp.WorkflowName,
		Status:       resp.Status,
		AgentVersion: strings.TrimSpace(agentVersion),
	}, nil
}

func (c *BootstrapClient) GetHostInstallRun(ctx context.Context, runID string) (appui.HostInstallRun, error) {
	detail, err := c.runtime.GetRun(ctx, runID)
	if err != nil {
		return appui.HostInstallRun{}, err
	}
	return mapRunDetail(detail), nil
}

func mapRunDetail(detail *service.RunDetail) appui.HostInstallRun {
	if detail == nil {
		return appui.HostInstallRun{}
	}
	return appui.HostInstallRun{
		HostID:       stringFromVars(detail.Vars, "host_id"),
		RunID:        detail.RunID,
		WorkflowID:   firstNonEmpty(detail.WorkflowID, detail.WorkflowName),
		Status:       detail.Status,
		CurrentStep:  currentRunStep(detail),
		LastError:    firstNonEmpty(detail.LastError, detail.Message),
		Platform:     stringFromVars(detail.Vars, "platform"),
		AgentVersion: stringFromVars(detail.Vars, "agent_version"),
	}
}

func currentRunStep(detail *service.RunDetail) string {
	for i := len(detail.Steps) - 1; i >= 0; i-- {
		step := detail.Steps[i]
		if step.Name == "" {
			continue
		}
		if step.Status != state.RunStatusQueued || i == len(detail.Steps)-1 {
			return step.Name
		}
	}
	if detail.Graph != nil {
		for _, node := range detail.Graph.Nodes {
			if node.Name != "" && node.Status != state.RunStatusQueued {
				return node.Name
			}
		}
	}
	return ""
}

func stringFromVars(vars map[string]any, key string) string {
	if len(vars) == 0 {
		return ""
	}
	if value, ok := vars[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
