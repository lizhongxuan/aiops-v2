package state

import (
	"sort"
	"strings"
	"time"
)

func CloneRunState(input RunState) RunState {
	out := input
	out.Args = cloneMap(input.Args)
	if len(input.Resources) > 0 {
		out.Resources = make(map[string]ResourceState, len(input.Resources))
		for k, v := range input.Resources {
			out.Resources[k] = cloneResource(v)
		}
	}
	if len(input.Steps) > 0 {
		out.Steps = make([]StepState, 0, len(input.Steps))
		for _, step := range input.Steps {
			out.Steps = append(out.Steps, cloneStep(step))
		}
	}
	if input.Graph != nil {
		out.Graph = cloneGraph(input.Graph)
	}
	return out
}

func SynthesizeStepStatesFromGraph(graph *GraphRunState) []StepState {
	if graph == nil || len(graph.Nodes) == 0 {
		return nil
	}
	nodes := make([]NodeState, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if !graphNodeMapsToStep(node) {
			continue
		}
		nodes = append(nodes, node)
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		left := nodes[i]
		right := nodes[j]
		if !left.StartedAt.IsZero() || !right.StartedAt.IsZero() {
			if left.StartedAt.IsZero() {
				return false
			}
			if right.StartedAt.IsZero() {
				return true
			}
			if !left.StartedAt.Equal(right.StartedAt) {
				return left.StartedAt.Before(right.StartedAt)
			}
		}
		return left.ID < right.ID
	})
	steps := make([]StepState, 0, len(nodes))
	for _, node := range nodes {
		name := strings.TrimSpace(node.Name)
		if name == "" {
			name = strings.TrimSpace(node.ID)
		}
		steps = append(steps, StepState{
			Name:       name,
			Status:     strings.TrimSpace(node.Status),
			StartedAt:  node.StartedAt,
			FinishedAt: node.FinishedAt,
			Message:    strings.TrimSpace(node.Message),
			Hosts:      cloneHostsMap(node.Hosts),
		})
	}
	return steps
}

func graphNodeMapsToStep(node NodeState) bool {
	switch strings.TrimSpace(node.Type) {
	case "action", "condition", "manual_approval", "subflow":
		return strings.TrimSpace(node.Name) != "" || strings.TrimSpace(node.ID) != ""
	default:
		return false
	}
}

func (r *RunState) UpsertStepStart(stepName string, now time.Time) {
	step := r.ensureStep(stepName)
	if step.StartedAt.IsZero() {
		step.StartedAt = now
	}
	step.Status = RunStatusRunning
}

func (r *RunState) UpsertStepFinish(stepName, status, message string, now time.Time) {
	step := r.ensureStep(stepName)
	if step.StartedAt.IsZero() {
		step.StartedAt = now
	}
	step.Status = status
	step.Message = message
	step.FinishedAt = now
}

func (r *RunState) UpsertHostResult(stepName string, host HostResult) {
	r.UpsertStepHostResult(stepName, host)
	if id, ok := r.graphNodeIDForStep(stepName, ""); ok {
		node := r.Graph.Nodes[id]
		if node.Hosts == nil {
			node.Hosts = map[string]HostResult{}
		}
		node.Hosts[host.Host] = cloneHost(host)
		r.Graph.Nodes[id] = node
	}
}

func (r *RunState) UpsertStepHostResult(stepName string, host HostResult) {
	step := r.ensureStep(stepName)
	if step.Hosts == nil {
		step.Hosts = map[string]HostResult{}
	}
	step.Hosts[host.Host] = cloneHost(host)
}

func (r *RunState) ensureStep(name string) *StepState {
	for i := range r.Steps {
		if r.Steps[i].Name == name {
			return &r.Steps[i]
		}
	}
	r.Steps = append(r.Steps, StepState{
		Name:  name,
		Hosts: map[string]HostResult{},
	})
	return &r.Steps[len(r.Steps)-1]
}

func (r *RunState) UpsertGraphNodeStart(stepName, stepID string, now time.Time) {
	id, ok := r.graphNodeIDForStep(stepName, stepID)
	if !ok {
		return
	}
	node := r.Graph.Nodes[id]
	if node.StartedAt.IsZero() {
		node.StartedAt = now
	}
	node.Status = RunStatusRunning
	r.Graph.Nodes[id] = node
}

func (r *RunState) UpsertGraphNodeFinish(stepName, stepID, status, message string, now time.Time) {
	id, ok := r.graphNodeIDForStep(stepName, stepID)
	if !ok {
		return
	}
	node := r.Graph.Nodes[id]
	if node.StartedAt.IsZero() {
		node.StartedAt = now
	}
	node.Status = status
	node.Message = message
	node.FinishedAt = now
	r.Graph.Nodes[id] = node
}

func (r *RunState) UpsertGraphNodeStartByID(nodeID string, now time.Time) {
	if r == nil || r.Graph == nil {
		return
	}
	node, ok := r.Graph.Nodes[nodeID]
	if !ok {
		return
	}
	if node.StartedAt.IsZero() {
		node.StartedAt = now
	}
	node.Status = RunStatusRunning
	r.Graph.Nodes[nodeID] = node
}

func (r *RunState) UpsertGraphNodeFinishByID(nodeID, status, message string, now time.Time) {
	if r == nil || r.Graph == nil {
		return
	}
	node, ok := r.Graph.Nodes[nodeID]
	if !ok {
		return
	}
	if node.StartedAt.IsZero() {
		node.StartedAt = now
	}
	node.Status = status
	node.Message = message
	node.FinishedAt = now
	r.Graph.Nodes[nodeID] = node
}

func (r *RunState) UpsertGraphNodeWaitingByID(nodeID string, now time.Time) {
	if r == nil || r.Graph == nil {
		return
	}
	node, ok := r.Graph.Nodes[nodeID]
	if !ok {
		return
	}
	if node.StartedAt.IsZero() {
		node.StartedAt = now
	}
	node.Status = "waiting"
	node.Message = "waiting for approval"
	r.Graph.Nodes[nodeID] = node
}

func (r *RunState) UpsertGraphNodeIterationStartByID(nodeID string, index int, item any, now time.Time) {
	if r == nil || r.Graph == nil {
		return
	}
	node, ok := r.Graph.Nodes[nodeID]
	if !ok {
		return
	}
	if node.StartedAt.IsZero() {
		node.StartedAt = now
	}
	node.Status = RunStatusRunning
	upsertNodeIteration(&node, NodeIterationState{
		Index:     index,
		Status:    RunStatusRunning,
		Item:      item,
		StartedAt: now,
	})
	r.Graph.Nodes[nodeID] = node
}

func (r *RunState) UpsertGraphNodeIterationFinishByID(nodeID string, index int, status, message string, now time.Time) {
	if r == nil || r.Graph == nil {
		return
	}
	node, ok := r.Graph.Nodes[nodeID]
	if !ok {
		return
	}
	upsertNodeIteration(&node, NodeIterationState{
		Index:      index,
		Status:     strings.TrimSpace(status),
		Message:    strings.TrimSpace(message),
		FinishedAt: now,
	})
	r.Graph.Nodes[nodeID] = node
}

func (r *RunState) UpsertGraphNodeIterationNodeStartByID(loopID string, index int, nodeID string, now time.Time) {
	if r == nil || r.Graph == nil {
		return
	}
	loop, ok := r.Graph.Nodes[loopID]
	if !ok {
		return
	}
	child, ok := r.Graph.Nodes[nodeID]
	if !ok {
		return
	}
	if child.StartedAt.IsZero() {
		child.StartedAt = now
	}
	child.Status = RunStatusRunning
	upsertNodeIterationChild(&loop, index, child)
	r.Graph.Nodes[loopID] = loop
}

func (r *RunState) UpsertGraphNodeIterationNodeFinishByID(loopID string, index int, nodeID, status, message string, now time.Time) {
	if r == nil || r.Graph == nil {
		return
	}
	loop, ok := r.Graph.Nodes[loopID]
	if !ok {
		return
	}
	child, ok := r.Graph.Nodes[nodeID]
	if !ok {
		return
	}
	if child.StartedAt.IsZero() {
		child.StartedAt = now
	}
	child.Status = strings.TrimSpace(status)
	child.Message = strings.TrimSpace(message)
	child.FinishedAt = now
	upsertNodeIterationChild(&loop, index, child)
	r.Graph.Nodes[loopID] = loop
}

func (r *RunState) UpsertGraphNodeIterationHostResultByID(loopID string, index int, nodeID string, host HostResult) {
	if r == nil || r.Graph == nil {
		return
	}
	loop, ok := r.Graph.Nodes[loopID]
	if !ok {
		return
	}
	child, ok := r.Graph.Nodes[nodeID]
	if !ok {
		return
	}
	if child.Hosts == nil {
		child.Hosts = map[string]HostResult{}
	}
	child.Hosts[host.Host] = cloneHost(host)
	upsertNodeIterationChildHostResult(&loop, index, child, host)
	r.Graph.Nodes[loopID] = loop
}

func (r *RunState) UpsertGraphEdgeSelected(edgeID string, now time.Time) {
	if r == nil || r.Graph == nil {
		return
	}
	edge, ok := r.Graph.Edges[edgeID]
	if !ok {
		return
	}
	edge.Status = "selected"
	edge.SelectedAt = now
	r.Graph.Edges[edgeID] = edge
}

func upsertNodeIteration(node *NodeState, next NodeIterationState) {
	for i := range node.Iterations {
		if node.Iterations[i].Index != next.Index {
			continue
		}
		if next.Item != nil {
			node.Iterations[i].Item = next.Item
		}
		if !next.StartedAt.IsZero() && node.Iterations[i].StartedAt.IsZero() {
			node.Iterations[i].StartedAt = next.StartedAt
		}
		if strings.TrimSpace(next.Status) != "" {
			node.Iterations[i].Status = next.Status
		}
		if strings.TrimSpace(next.Message) != "" {
			node.Iterations[i].Message = next.Message
		}
		if !next.FinishedAt.IsZero() {
			node.Iterations[i].FinishedAt = next.FinishedAt
		}
		if len(next.Nodes) > 0 {
			if node.Iterations[i].Nodes == nil {
				node.Iterations[i].Nodes = map[string]NodeState{}
			}
			for id, child := range next.Nodes {
				node.Iterations[i].Nodes[id] = mergeNodeState(node.Iterations[i].Nodes[id], child)
			}
		}
		return
	}
	node.Iterations = append(node.Iterations, next)
}

func upsertNodeIterationChild(loop *NodeState, index int, child NodeState) {
	if len(loop.Iterations) == 0 {
		upsertNodeIteration(loop, NodeIterationState{Index: index, Status: RunStatusRunning})
	}
	for i := range loop.Iterations {
		if loop.Iterations[i].Index != index {
			continue
		}
		if loop.Iterations[i].Nodes == nil {
			loop.Iterations[i].Nodes = map[string]NodeState{}
		}
		loop.Iterations[i].Nodes[child.ID] = mergeNodeState(loop.Iterations[i].Nodes[child.ID], child)
		return
	}
	upsertNodeIteration(loop, NodeIterationState{
		Index:  index,
		Status: RunStatusRunning,
		Nodes:  map[string]NodeState{child.ID: cloneNode(child)},
	})
}

func upsertNodeIterationChildHostResult(loop *NodeState, index int, child NodeState, host HostResult) {
	if len(loop.Iterations) == 0 {
		upsertNodeIteration(loop, NodeIterationState{Index: index, Status: RunStatusRunning})
	}
	for i := range loop.Iterations {
		if loop.Iterations[i].Index != index {
			continue
		}
		if loop.Iterations[i].Nodes == nil {
			loop.Iterations[i].Nodes = map[string]NodeState{}
		}
		existing := loop.Iterations[i].Nodes[child.ID]
		if strings.TrimSpace(existing.ID) == "" {
			existing = child
		}
		if existing.Hosts == nil {
			existing.Hosts = map[string]HostResult{}
		}
		existing.Hosts[host.Host] = cloneHost(host)
		loop.Iterations[i].Nodes[child.ID] = existing
		return
	}
	if child.Hosts == nil {
		child.Hosts = map[string]HostResult{}
	}
	child.Hosts[host.Host] = cloneHost(host)
	upsertNodeIteration(loop, NodeIterationState{
		Index:  index,
		Status: RunStatusRunning,
		Nodes:  map[string]NodeState{child.ID: cloneNode(child)},
	})
}

func mergeNodeState(existing NodeState, next NodeState) NodeState {
	out := cloneNode(existing)
	if strings.TrimSpace(next.ID) != "" {
		out.ID = next.ID
	}
	if strings.TrimSpace(next.Name) != "" {
		out.Name = next.Name
	}
	if strings.TrimSpace(next.Type) != "" {
		out.Type = next.Type
	}
	if strings.TrimSpace(next.ParentID) != "" {
		out.ParentID = next.ParentID
	}
	if strings.TrimSpace(next.Status) != "" {
		out.Status = next.Status
	}
	if next.Attempt != 0 {
		out.Attempt = next.Attempt
	}
	if !next.StartedAt.IsZero() && out.StartedAt.IsZero() {
		out.StartedAt = next.StartedAt
	}
	if !next.FinishedAt.IsZero() {
		out.FinishedAt = next.FinishedAt
	}
	if strings.TrimSpace(next.Message) != "" {
		out.Message = next.Message
	}
	if len(next.Hosts) > 0 {
		if out.Hosts == nil {
			out.Hosts = map[string]HostResult{}
		}
		for host, result := range next.Hosts {
			out.Hosts[host] = cloneHost(result)
		}
	}
	if len(next.Output) > 0 {
		if out.Output == nil {
			out.Output = map[string]any{}
		}
		for key, value := range next.Output {
			out.Output[key] = value
		}
	}
	if len(next.Iterations) > 0 {
		out.Iterations = cloneNode(next).Iterations
	}
	return out
}

func (r *RunState) graphNodeIDForStep(stepName, stepID string) (string, bool) {
	if r == nil || r.Graph == nil {
		return "", false
	}
	if stepID != "" {
		if _, ok := r.Graph.Nodes[stepID]; ok {
			return stepID, true
		}
	}
	for id, node := range r.Graph.Nodes {
		if node.Name == stepName || node.Name == stepID || id == stepID {
			return id, true
		}
	}
	return "", false
}

func cloneResource(input ResourceState) ResourceState {
	out := input
	out.Current = cloneMap(input.Current)
	out.Desired = cloneMap(input.Desired)
	out.Diff = cloneMap(input.Diff)
	return out
}

func cloneStep(input StepState) StepState {
	out := input
	if len(input.Hosts) > 0 {
		out.Hosts = make(map[string]HostResult, len(input.Hosts))
		for host, res := range input.Hosts {
			out.Hosts[host] = cloneHost(res)
		}
	}
	return out
}

func cloneGraph(input *GraphRunState) *GraphRunState {
	if input == nil {
		return nil
	}
	out := *input
	if len(input.Nodes) > 0 {
		out.Nodes = make(map[string]NodeState, len(input.Nodes))
		for id, node := range input.Nodes {
			out.Nodes[id] = cloneNode(node)
		}
	}
	if len(input.Edges) > 0 {
		out.Edges = make(map[string]EdgeState, len(input.Edges))
		for id, edge := range input.Edges {
			out.Edges[id] = edge
		}
	}
	return &out
}

func cloneNode(input NodeState) NodeState {
	out := input
	if len(input.Iterations) > 0 {
		out.Iterations = make([]NodeIterationState, len(input.Iterations))
		for i, iteration := range input.Iterations {
			out.Iterations[i] = iteration
			if len(iteration.Nodes) > 0 {
				out.Iterations[i].Nodes = make(map[string]NodeState, len(iteration.Nodes))
				for id, child := range iteration.Nodes {
					out.Iterations[i].Nodes[id] = cloneNode(child)
				}
			}
		}
	}
	out.Hosts = cloneHostsMap(input.Hosts)
	out.Output = cloneMap(input.Output)
	return out
}

func cloneHostsMap(input map[string]HostResult) map[string]HostResult {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]HostResult, len(input))
	for host, res := range input {
		out[host] = cloneHost(res)
	}
	return out
}

func cloneHost(input HostResult) HostResult {
	out := input
	out.Output = cloneMap(input.Output)
	return out
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
