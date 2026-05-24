package service

import (
	"context"
	"fmt"
	"strings"

	"runner/workflow"
	"runner/workflow/visual"
)

type VisualWorkflowAIDraftRequest struct {
	WorkflowName   string       `json:"workflow_name,omitempty"`
	WorkflowStatus string       `json:"workflow_status,omitempty"`
	Instruction    string       `json:"instruction"`
	Graph          visual.Graph `json:"graph"`
}

type VisualWorkflowGraphPatch struct {
	Operations []map[string]any `json:"operations"`
}

type VisualWorkflowAIDraftResponse struct {
	GraphPatch     VisualWorkflowGraphPatch `json:"graph_patch"`
	CandidateGraph visual.Graph             `json:"candidate_graph"`
	DiffSummary    map[string]any           `json:"diff_summary"`
}

func (s *VisualWorkflowService) GenerateAIDraft(_ context.Context, req VisualWorkflowAIDraftRequest) (*VisualWorkflowAIDraftResponse, error) {
	if strings.TrimSpace(req.Instruction) == "" {
		return nil, fmt.Errorf("%w: instruction is required", ErrInvalid)
	}
	if status := strings.ToLower(strings.TrimSpace(req.WorkflowStatus)); status != "" && status != WorkflowStatusDraft {
		return nil, fmt.Errorf("%w: AI draft is only allowed for draft workflows", ErrConflict)
	}

	name := strings.TrimSpace(req.WorkflowName)
	if name == "" {
		name = strings.TrimSpace(req.Graph.Workflow.Name)
	}
	if name == "" {
		name = "host-resource-check"
	}

	graph := hostResourceCheckDraftGraph(name, req.Instruction)
	return &VisualWorkflowAIDraftResponse{
		GraphPatch: VisualWorkflowGraphPatch{
			Operations: []map[string]any{
				{
					"op":          "replace_graph",
					"workflow":    name,
					"description": "generate host resource inspection workflow draft",
					"nodes":       len(graph.Nodes),
					"edges":       len(graph.Edges),
				},
			},
		},
		CandidateGraph: graph,
		DiffSummary: map[string]any{
			"semantic_changes": []map[string]any{
				{
					"title":  "生成主机资源检查工作流",
					"detail": "创建顺序执行的资源检查步骤，覆盖负载、CPU、内存、磁盘和关键进程快照。",
				},
			},
			"nodes_added": len(graph.Nodes),
			"edges_added": len(graph.Edges),
		},
	}, nil
}

func hostResourceCheckDraftGraph(name, instruction string) visual.Graph {
	description := strings.TrimSpace(instruction)
	if description == "" {
		description = "检查主机资源使用情况"
	}
	return visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version:     "v0.1",
			Name:        name,
			Description: description,
			Vars: map[string]any{
				"target": "local",
			},
			Inventory: workflow.Inventory{
				Hosts: map[string]workflow.Host{
					"local": {Address: "local"},
				},
			},
		},
		Layout: visual.Layout{Direction: "LR"},
		Nodes: []visual.Node{
			{
				ID:       "start",
				Type:     visual.NodeTypeStart,
				Position: visual.Position{X: 80, Y: 180},
				Label:    "Start",
				Ports:    startNodePorts(),
			},
			{
				ID:       "check-host-resources",
				Type:     visual.NodeTypeAction,
				Position: visual.Position{X: 340, Y: 160},
				Label:    "检查主机资源",
				Ports:    actionNodePorts(),
				Step: &workflow.Step{
					Name:    "check-host-resources",
					Action:  "script.shell",
					Targets: []string{"local"},
					Timeout: "2m",
					Args: map[string]any{
						"script": strings.Join([]string{
							"set -e",
							"echo '== host =='",
							"hostname || true",
							"echo '== uptime/load =='",
							"uptime || true",
							"echo '== disk =='",
							"df -h || true",
							"echo '== memory =='",
							"free -m || vm_stat || true",
							"echo '== cpu/process snapshot =='",
							"(top -b -n 1 | head -30) 2>/dev/null || (ps aux --sort=-%cpu | head -12) || true",
						}, "\n"),
						"export_vars": true,
					},
				},
				Inputs: []visual.InputParamSpec{
					{Key: "target", Type: "string", Label: "目标主机", Required: true, Default: "local"},
				},
				Outputs: []visual.OutputParamSpec{
					{Key: "stdout", Type: "string", Label: "检查输出"},
					{Key: "stderr", Type: "string", Label: "错误输出"},
					{Key: "exit_code", Type: "number", Label: "退出码"},
				},
			},
			{
				ID:       "summarize-result",
				Type:     visual.NodeTypeAction,
				Position: visual.Position{X: 640, Y: 160},
				Label:    "汇总结果",
				Ports:    actionNodePorts(),
				Step: &workflow.Step{
					Name:    "summarize-result",
					Action:  "script.shell",
					Targets: []string{"local"},
					Timeout: "30s",
					Args: map[string]any{
						"script": "echo resource_check_completed",
					},
				},
				Outputs: []visual.OutputParamSpec{
					{Key: "stdout", Type: "string", Label: "汇总输出"},
				},
			},
			{
				ID:       "end",
				Type:     visual.NodeTypeEnd,
				Position: visual.Position{X: 900, Y: 180},
				Label:    "End",
				Ports:    endNodePorts(),
			},
		},
		Edges: []visual.Edge{
			{ID: "start-check-host-resources", Source: "start", Target: "check-host-resources", Kind: visual.EdgeKindNext, SourcePort: "next", TargetPort: "in"},
			{ID: "check-host-resources-summarize-result", Source: "check-host-resources", Target: "summarize-result", Kind: visual.EdgeKindNext, SourcePort: "next", TargetPort: "in"},
			{ID: "summarize-result-end", Source: "summarize-result", Target: "end", Kind: visual.EdgeKindNext, SourcePort: "next", TargetPort: "in"},
		},
	}
}

func startNodePorts() []visual.Port {
	return []visual.Port{{ID: "next", Type: "output", Label: "下一步"}}
}

func endNodePorts() []visual.Port {
	return []visual.Port{{ID: "in", Type: "input", Label: "输入"}}
}

func actionNodePorts() []visual.Port {
	return []visual.Port{
		{ID: "in", Type: "input", Label: "输入"},
		{ID: "next", Type: "output", Label: "下一步"},
		{ID: "failure", Type: "output", Label: "失败"},
	}
}
