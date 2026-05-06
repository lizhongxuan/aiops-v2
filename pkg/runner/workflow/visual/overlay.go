package visual

import (
	"runner/state"
	"runner/workflow"
)

func OverlayRunState(g Graph, run state.RunState) Graph {
	out := cloneGraph(g)
	steps := make(map[string]state.StepState, len(run.Steps))
	for _, step := range run.Steps {
		steps[step.Name] = step
	}
	for i := range out.Nodes {
		if run.Graph != nil {
			if node, ok := run.Graph.Nodes[out.Nodes[i].ID]; ok {
				out.Nodes[i].State = nodeRunStateFromGraph(run.RunID, node)
				continue
			}
		}
		if !executableNodeType(out.Nodes[i].Type) {
			continue
		}
		stepName := nodeStepName(out.Nodes[i])
		step, ok := steps[stepName]
		if !ok {
			continue
		}
		out.Nodes[i].State = &NodeRunState{
			RunID:      run.RunID,
			Status:     step.Status,
			Message:    step.Message,
			StartedAt:  step.StartedAt,
			FinishedAt: step.FinishedAt,
			Hosts:      cloneHosts(step.Hosts),
		}
	}
	if run.Graph != nil {
		for i := range out.Edges {
			if edge, ok := run.Graph.Edges[out.Edges[i].ID]; ok {
				out.Edges[i].State = edgeRunStateFromGraph(run.RunID, edge)
			}
		}
	}
	return out
}

func nodeRunStateFromGraph(runID string, node state.NodeState) *NodeRunState {
	return &NodeRunState{
		RunID:      runID,
		Status:     node.Status,
		Message:    node.Message,
		StartedAt:  node.StartedAt,
		FinishedAt: node.FinishedAt,
		Hosts:      cloneHosts(node.Hosts),
		Output:     cloneMap(node.Output),
	}
}

func edgeRunStateFromGraph(runID string, edge state.EdgeState) *EdgeRunState {
	return &EdgeRunState{
		RunID:      runID,
		Status:     edge.Status,
		Message:    edge.Message,
		SelectedAt: edge.SelectedAt,
	}
}

func cloneGraph(g Graph) Graph {
	out := g
	out.Workflow = cloneWorkflow(g.Workflow)
	out.Layout.UI = cloneMap(g.Layout.UI)
	out.UI = cloneMap(g.UI)
	out.Nodes = make([]Node, len(g.Nodes))
	for i, node := range g.Nodes {
		out.Nodes[i] = node
		out.Nodes[i].UI = cloneMap(node.UI)
		out.Nodes[i].State = cloneNodeRunState(node.State)
		out.Nodes[i].Approval = cloneApproval(node.Approval)
		out.Nodes[i].Subflow = cloneSubflow(node.Subflow)
		out.Nodes[i].Join = cloneJoin(node.Join)
		out.Nodes[i].Ports = clonePorts(node.Ports)
		if node.Step != nil {
			step := *node.Step
			step.Args = cloneMap(step.Args)
			out.Nodes[i].Step = &step
		}
		if node.Handler != nil {
			handler := *node.Handler
			handler.Args = cloneMap(handler.Args)
			out.Nodes[i].Handler = &handler
		}
	}
	out.Edges = make([]Edge, len(g.Edges))
	for i, edge := range g.Edges {
		out.Edges[i] = edge
		out.Edges[i].UI = cloneMap(edge.UI)
		out.Edges[i].State = cloneEdgeRunState(edge.State)
	}
	return out
}

func cloneWorkflow(wf workflow.Workflow) workflow.Workflow {
	out := wf
	out.EnvPackages = append([]string(nil), wf.EnvPackages...)
	out.Vars = cloneMap(wf.Vars)
	out.Inventory.Groups = cloneGroups(wf.Inventory.Groups)
	out.Inventory.Hosts = cloneHostsInventory(wf.Inventory.Hosts)
	out.Inventory.Vars = cloneMap(wf.Inventory.Vars)
	out.Steps = make([]workflow.Step, len(wf.Steps))
	for i, step := range wf.Steps {
		out.Steps[i] = step
		out.Steps[i].Targets = append([]string(nil), step.Targets...)
		out.Steps[i].Args = cloneMap(step.Args)
		out.Steps[i].MustVars = append([]string(nil), step.MustVars...)
		out.Steps[i].Loop = append([]any(nil), step.Loop...)
		out.Steps[i].ExpectVars = append([]string(nil), step.ExpectVars...)
		out.Steps[i].Notify = append([]string(nil), step.Notify...)
	}
	out.Handlers = make([]workflow.Handler, len(wf.Handlers))
	for i, handler := range wf.Handlers {
		out.Handlers[i] = handler
		out.Handlers[i].Args = cloneMap(handler.Args)
	}
	out.Tests = make([]workflow.Test, len(wf.Tests))
	for i, test := range wf.Tests {
		out.Tests[i] = test
		out.Tests[i].Args = cloneMap(test.Args)
	}
	return out
}

func cloneGroups(input map[string]workflow.Group) map[string]workflow.Group {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]workflow.Group, len(input))
	for name, group := range input {
		group.Hosts = append([]string(nil), group.Hosts...)
		group.Vars = cloneMap(group.Vars)
		out[name] = group
	}
	return out
}

func cloneHostsInventory(input map[string]workflow.Host) map[string]workflow.Host {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]workflow.Host, len(input))
	for name, host := range input {
		host.Vars = cloneMap(host.Vars)
		out[name] = host
	}
	return out
}

func cloneNodeRunState(input *NodeRunState) *NodeRunState {
	if input == nil {
		return nil
	}
	out := *input
	out.Hosts = cloneHosts(input.Hosts)
	out.Output = cloneMap(input.Output)
	return &out
}

func cloneEdgeRunState(input *EdgeRunState) *EdgeRunState {
	if input == nil {
		return nil
	}
	out := *input
	return &out
}

func cloneHosts(input map[string]state.HostResult) map[string]state.HostResult {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]state.HostResult, len(input))
	for host, result := range input {
		result.Output = cloneMap(result.Output)
		out[host] = result
	}
	return out
}
