package visual

import (
	"fmt"
	"sort"
	"strings"

	"runner/workflow"
)

type Issue struct {
	Severity   string `json:"severity,omitempty"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	NodeID     string `json:"node_id,omitempty"`
	EdgeID     string `json:"edge_id,omitempty"`
	Field      string `json:"field,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

type ValidationError struct {
	Issues []Issue `json:"issues"`
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "visual workflow validation failed"
	}
	messages := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		messages = append(messages, issue.Message)
	}
	return "visual workflow validation failed: " + strings.Join(messages, "; ")
}

func ValidateGraph(g Graph) error {
	_, err := validateGraph(g)
	return err
}

func validateGraph(g Graph) (graphIndex, error) {
	idx := newGraphIndex(g)
	var issues []Issue

	if strings.TrimSpace(g.Version) == "" {
		issues = append(issues, issue("graph_version_required", "graph version is required", "", "", "version"))
	}
	if g.Version != "" && g.Version != GraphVersion {
		issues = append(issues, issue("graph_version_unsupported", fmt.Sprintf("graph version %q is not supported", g.Version), "", "", "version"))
	}
	if len(g.Nodes) == 0 {
		issues = append(issues, issue("nodes_required", "graph nodes must not be empty", "", "", "nodes"))
	}

	startCount := 0
	stepBackedCount := 0
	stepNames := map[string]string{}
	stepIDs := map[string]string{}
	handlerNames := map[string]string{}
	for i, node := range g.Nodes {
		field := fmt.Sprintf("nodes[%d]", i)
		id := strings.TrimSpace(node.ID)
		if id == "" {
			issues = append(issues, issue("node_id_required", fmt.Sprintf("%s id is required", field), "", "", field+".id"))
			continue
		}
		if first, exists := idx.nodeIDs[id]; exists && first != i {
			issues = append(issues, issue("node_id_duplicate", fmt.Sprintf("node id %q is duplicated", id), id, "", field+".id"))
		}
		if !validNodeType(node.Type) {
			issues = append(issues, issue("node_type_invalid", fmt.Sprintf("node %q type %q is not supported", id, node.Type), id, "", field+".type"))
		}
		if node.Type == NodeTypeStart {
			startCount++
		}
		if stepBackedNodeType(node.Type) {
			stepBackedCount++
			step := nodeStep(node)
			if strings.TrimSpace(step.Name) == "" {
				issues = append(issues, issue("step_name_required", fmt.Sprintf("node %q step name is required", id), id, "", field+".step.name"))
			} else if owner, exists := stepNames[step.Name]; exists {
				issues = append(issues, issue("step_name_duplicate", fmt.Sprintf("step name %q is duplicated by nodes %q and %q", step.Name, owner, id), id, "", field+".step.name"))
			} else {
				stepNames[step.Name] = id
			}
			if strings.TrimSpace(step.Action) == "" {
				issues = append(issues, issue("step_action_required", fmt.Sprintf("node %q step action is required", id), id, "", field+".step.action"))
			}
			stepID := strings.TrimSpace(firstNonEmpty(step.ID, node.StepID, id))
			if owner, exists := stepIDs[stepID]; exists {
				issues = append(issues, issue("step_id_duplicate", fmt.Sprintf("step id %q is duplicated by nodes %q and %q", stepID, owner, id), id, "", field+".step_id"))
			} else {
				stepIDs[stepID] = id
			}
		}
		if node.Type == NodeTypeHandler {
			handler := nodeHandler(node)
			if strings.TrimSpace(handler.Name) == "" {
				issues = append(issues, issue("handler_name_required", fmt.Sprintf("node %q handler name is required", id), id, "", field+".handler.name"))
			} else if owner, exists := handlerNames[handler.Name]; exists {
				issues = append(issues, issue("handler_name_duplicate", fmt.Sprintf("handler name %q is duplicated by nodes %q and %q", handler.Name, owner, id), id, "", field+".handler.name"))
			} else {
				handlerNames[handler.Name] = id
			}
			if strings.TrimSpace(handler.Action) == "" {
				issues = append(issues, issue("handler_action_required", fmt.Sprintf("node %q handler action is required", id), id, "", field+".handler.action"))
			}
		}
		if node.Type == NodeTypeManualApproval {
			if node.Approval == nil {
				issues = append(issues, issue("approval_required", fmt.Sprintf("node %q approval spec is required", id), id, "", field+".approval"))
			} else {
				if len(node.Approval.Subjects) == 0 {
					issues = append(issues, issue("approval_subjects_required", fmt.Sprintf("node %q approval subjects are required", id), id, "", field+".approval.subjects"))
				}
				if strings.TrimSpace(node.Approval.Timeout) == "" {
					issues = append(issues, issue("approval_timeout_required", fmt.Sprintf("node %q approval timeout is required", id), id, "", field+".approval.timeout"))
				}
			}
		}
		if node.Type == NodeTypeSubflow {
			if node.Subflow == nil || strings.TrimSpace(node.Subflow.WorkflowName) == "" {
				issues = append(issues, issue("subflow_workflow_required", fmt.Sprintf("node %q subflow workflow_name is required", id), id, "", field+".subflow.workflow_name"))
			}
		}
		if node.Type == NodeTypeJoin {
			strategy := ""
			if node.Join != nil {
				strategy = strings.TrimSpace(node.Join.Strategy)
			}
			switch strategy {
			case "", "all_success", "any_success", "always", "failure_threshold":
			default:
				issues = append(issues, issue("join_strategy_invalid", fmt.Sprintf("node %q join strategy %q is not supported", id, strategy), id, "", field+".join.strategy"))
			}
		}
		if node.Type == NodeTypeLoop {
			issues = append(issues, validateLoopSpec(id, field, node.Loop)...)
		}
		parent, hasParent := idx.nodes[strings.TrimSpace(node.ParentID)]
		if hasParent && parent.Type == NodeTypeLoop && stepBackedNodeType(node.Type) && len(nodeStep(node).Loop) > 0 {
			issues = append(issues, issue("loop_body_step_loop_unsupported", fmt.Sprintf("node %q is inside graph loop %q and cannot also use legacy step.loop", id, node.ParentID), id, "", field+".step.loop"))
		}
		issues = append(issues, validateNodeInputs(id, field, node.Inputs)...)
		issues = append(issues, validateNodeOutputs(id, field, node.Outputs)...)
	}
	if startCount == 0 {
		issues = append(issues, issue("start_required", "graph must contain a start node", "", "", "nodes"))
	}
	if startCount > 1 {
		issues = append(issues, issue("start_duplicate", "graph must contain only one start node", "", "", "nodes"))
	}
	if stepBackedCount == 0 {
		issues = append(issues, issue("executable_node_required", "graph must contain at least one executable node", "", "", "nodes"))
	}

	edgeIDs := map[string]int{}
	for i, edge := range g.Edges {
		field := fmt.Sprintf("edges[%d]", i)
		id := strings.TrimSpace(edge.ID)
		if id == "" {
			issues = append(issues, issue("edge_id_required", fmt.Sprintf("%s id is required", field), "", "", field+".id"))
		} else if first, exists := edgeIDs[id]; exists {
			issues = append(issues, issue("edge_id_duplicate", fmt.Sprintf("edge id %q is duplicated by edges[%d] and edges[%d]", id, first, i), "", id, field+".id"))
		}
		edgeIDs[id] = i
		if !validEdgeKind(edge.Kind) {
			issues = append(issues, issue("edge_kind_invalid", fmt.Sprintf("edge %q kind %q is not supported", id, edge.Kind), "", id, field+".kind"))
		}
		if strings.TrimSpace(edge.Source) == "" {
			issues = append(issues, issue("edge_source_required", fmt.Sprintf("edge %q source is required", id), "", id, field+".source"))
		} else if _, exists := idx.nodes[edge.Source]; !exists {
			issues = append(issues, issue("edge_source_missing", fmt.Sprintf("edge %q source node %q was not found", id, edge.Source), edge.Source, id, field+".source"))
		} else if strings.TrimSpace(edge.SourcePort) != "" && !idx.hasPort(edge.Source, edge.SourcePort) {
			issues = append(issues, issue("edge_source_port_missing", fmt.Sprintf("edge %q source port %q was not found on node %q", id, edge.SourcePort, edge.Source), edge.Source, id, field+".source_port"))
		}
		if strings.TrimSpace(edge.Target) == "" {
			issues = append(issues, issue("edge_target_required", fmt.Sprintf("edge %q target is required", id), "", id, field+".target"))
		} else if _, exists := idx.nodes[edge.Target]; !exists {
			issues = append(issues, issue("edge_target_missing", fmt.Sprintf("edge %q target node %q was not found", id, edge.Target), edge.Target, id, field+".target"))
		} else if strings.TrimSpace(edge.TargetPort) != "" && !idx.hasPort(edge.Target, edge.TargetPort) {
			issues = append(issues, issue("edge_target_port_missing", fmt.Sprintf("edge %q target port %q was not found on node %q", id, edge.TargetPort, edge.Target), edge.Target, id, field+".target_port"))
		}
		if edge.Source != "" && edge.Source == edge.Target {
			issues = append(issues, issue("edge_self_loop", fmt.Sprintf("edge %q cannot point node %q to itself", id, edge.Source), edge.Source, id, field))
		}
		if edge.Kind == EdgeKindCondition && strings.TrimSpace(edge.Condition) == "" {
			sourceNode, hasSource := idx.nodes[edge.Source]
			if !hasSource || sourceNode.Type != NodeTypeCondition {
				issues = append(issues, issue("condition_required", fmt.Sprintf("condition edge %q must include a condition expression", id), edge.Target, id, field+".condition"))
			}
		}
	}

	if len(issues) == 0 {
		if cycle := detectCycle(idx); len(cycle) > 0 {
			issues = append(issues, issue("graph_cycle", "graph must be a DAG; cycle detected: "+strings.Join(cycle, " -> "), cycle[0], "", "edges"))
		}
		for _, orphan := range unreachableExecutableNodes(idx) {
			issues = append(issues, issue("node_unreachable", fmt.Sprintf("node %q is not reachable from start", orphan), orphan, "", "nodes"))
		}
		for _, node := range g.Nodes {
			if strings.TrimSpace(node.ParentID) == "" && continuationRequiredNodeType(node.Type) && idx.continuationOutDegree(node.ID) == 0 {
				issues = append(issues, issue("node_outgoing_required", fmt.Sprintf("node %q must connect to a following executable or end node", node.ID), node.ID, "", "edges"))
			}
			if node.Type != NodeTypeJoin {
			} else if len(idx.incoming[node.ID]) < 2 {
				issues = append(issues, issue("join_requires_multiple_inputs", fmt.Sprintf("join node %q must have at least two incoming edges", node.ID), node.ID, "", "edges"))
			}
		}
	}
	if len(issues) == 0 {
		issues = append(issues, validateVariableReferences(g)...)
	}

	if len(issues) > 0 {
		sort.SliceStable(issues, func(i, j int) bool {
			if issues[i].Code != issues[j].Code {
				return issues[i].Code < issues[j].Code
			}
			if issues[i].NodeID != issues[j].NodeID {
				return issues[i].NodeID < issues[j].NodeID
			}
			return issues[i].EdgeID < issues[j].EdgeID
		})
		return idx, &ValidationError{Issues: issues}
	}
	return idx, nil
}

func validateNodeInputs(nodeID, field string, inputs []InputParamSpec) []Issue {
	var issues []Issue
	seen := map[string]struct{}{}
	for i, input := range inputs {
		inputField := fmt.Sprintf("%s.inputs[%d]", field, i)
		key := strings.TrimSpace(input.Key)
		if key == "" {
			issues = append(issues, issue("input_key_required", fmt.Sprintf("node %q input key is required", nodeID), nodeID, "", inputField+".key"))
		} else if _, exists := seen[key]; exists {
			issues = append(issues, issue("input_key_duplicate", fmt.Sprintf("node %q input key %q is duplicated", nodeID, key), nodeID, "", inputField+".key"))
		} else {
			seen[key] = struct{}{}
		}
		if typ := strings.TrimSpace(input.Type); typ != "" && !validParamType(typ) {
			issues = append(issues, issue("input_type_invalid", fmt.Sprintf("node %q input %q type %q is not supported", nodeID, key, typ), nodeID, "", inputField+".type"))
		}
		issues = append(issues, validateValueSource(nodeID, key, inputField+".value_source", input.ValueSource)...)
	}
	return issues
}

func validateNodeOutputs(nodeID, field string, outputs []OutputParamSpec) []Issue {
	var issues []Issue
	seen := map[string]struct{}{}
	for i, output := range outputs {
		outputField := fmt.Sprintf("%s.outputs[%d]", field, i)
		key := strings.TrimSpace(output.Key)
		if key == "" {
			issues = append(issues, issue("output_key_required", fmt.Sprintf("node %q output key is required", nodeID), nodeID, "", outputField+".key"))
		} else if _, exists := seen[key]; exists {
			issues = append(issues, issue("output_key_duplicate", fmt.Sprintf("node %q output key %q is duplicated", nodeID, key), nodeID, "", outputField+".key"))
		} else {
			seen[key] = struct{}{}
		}
		if typ := strings.TrimSpace(output.Type); typ != "" && !validParamType(typ) {
			issues = append(issues, issue("output_type_invalid", fmt.Sprintf("node %q output %q type %q is not supported", nodeID, key, typ), nodeID, "", outputField+".type"))
		}
		issues = append(issues, validateExtractSource(nodeID, key, outputField+".extract_source", output.ExtractSource)...)
	}
	return issues
}

func validateValueSource(nodeID, key, field string, source ValueSource) []Issue {
	sourceType := strings.TrimSpace(source.Type)
	if sourceType == "" {
		return nil
	}
	switch sourceType {
	case "literal":
		return nil
	case "variable":
		if source.Variable == nil || strings.TrimSpace(source.Variable.Scope) == "" || strings.TrimSpace(source.Variable.Name) == "" {
			return []Issue{issue("input_variable_required", fmt.Sprintf("node %q input %q variable source requires scope and name", nodeID, key), nodeID, "", field+".variable")}
		}
	case "expression":
		if strings.TrimSpace(source.Expression) == "" {
			return []Issue{issue("input_expression_required", fmt.Sprintf("node %q input %q expression source requires expression", nodeID, key), nodeID, "", field+".expression")}
		}
	case "secret":
		if strings.TrimSpace(source.SecretRef) == "" {
			return []Issue{issue("input_secret_ref_required", fmt.Sprintf("node %q input %q secret source requires secret_ref", nodeID, key), nodeID, "", field+".secret_ref")}
		}
	case "env":
		if strings.TrimSpace(source.EnvKey) == "" {
			return []Issue{issue("input_env_key_required", fmt.Sprintf("node %q input %q env source requires env_key", nodeID, key), nodeID, "", field+".env_key")}
		}
	default:
		return []Issue{issue("input_value_source_invalid", fmt.Sprintf("node %q input %q value source %q is not supported", nodeID, key, sourceType), nodeID, "", field+".type")}
	}
	return nil
}

func validateExtractSource(nodeID, key, field string, source ExtractSource) []Issue {
	sourceType := strings.TrimSpace(source.Type)
	if sourceType == "" {
		return nil
	}
	switch sourceType {
	case "stdout", "stderr", "exit_code":
		return nil
	case "stdout_jsonpath", "stderr_jsonpath", "jsonpath":
		if strings.TrimSpace(source.Path) == "" {
			return []Issue{issue("output_extract_path_required", fmt.Sprintf("node %q output %q extract source %q requires path", nodeID, key, sourceType), nodeID, "", field+".path")}
		}
	case "expression":
		if strings.TrimSpace(source.Expression) == "" {
			return []Issue{issue("output_extract_expression_required", fmt.Sprintf("node %q output %q expression source requires expression", nodeID, key), nodeID, "", field+".expression")}
		}
	case "literal":
		return nil
	default:
		return []Issue{issue("output_extract_source_invalid", fmt.Sprintf("node %q output %q extract source %q is not supported", nodeID, key, sourceType), nodeID, "", field+".type")}
	}
	return nil
}

func validateLoopSpec(nodeID, field string, loop *LoopSpec) []Issue {
	if loop == nil {
		return []Issue{issue("loop_required", fmt.Sprintf("node %q loop spec is required", nodeID), nodeID, "", field+".loop")}
	}
	var issues []Issue
	mode := strings.TrimSpace(loop.Mode)
	switch mode {
	case "":
		issues = append(issues, issue("loop_mode_required", fmt.Sprintf("node %q loop mode is required", nodeID), nodeID, "", field+".loop.mode"))
	case "for_each":
		if len(loop.Items) == 0 && strings.TrimSpace(loop.ItemsVariable) == "" {
			issues = append(issues, issue("loop_items_required", fmt.Sprintf("node %q for_each loop requires items or items_variable", nodeID), nodeID, "", field+".loop.items"))
		}
	case "while_condition":
		if strings.TrimSpace(loop.WhileCondition) == "" {
			issues = append(issues, issue("loop_while_condition_required", fmt.Sprintf("node %q while_condition loop requires while_condition", nodeID), nodeID, "", field+".loop.while_condition"))
		}
	default:
		issues = append(issues, issue("loop_mode_invalid", fmt.Sprintf("node %q loop mode %q is not supported", nodeID, mode), nodeID, "", field+".loop.mode"))
	}
	if loop.MaxIterations <= 0 {
		issues = append(issues, issue("loop_max_iterations_required", fmt.Sprintf("node %q loop max_iterations must be greater than zero", nodeID), nodeID, "", field+".loop.max_iterations"))
	}
	return issues
}

func validateVariableReferences(g Graph) []Issue {
	var issues []Issue
	for i, node := range g.Nodes {
		field := fmt.Sprintf("nodes[%d]", i)
		visible := VariablesForNode(g, node.ID)
		for j, input := range node.Inputs {
			source := input.ValueSource
			if strings.TrimSpace(source.Type) != "variable" || source.Variable == nil {
				continue
			}
			ref := *source.Variable
			if strings.TrimSpace(ref.Scope) == "" || strings.TrimSpace(ref.Name) == "" {
				continue
			}
			if variableRefVisible(visible, ref) {
				continue
			}
			inputKey := strings.TrimSpace(input.Key)
			issues = append(issues, issue(
				"input_variable_scope_invalid",
				fmt.Sprintf("node %q input %q references variable %q/%q outside visible scope", node.ID, inputKey, ref.Scope, ref.Name),
				node.ID,
				"",
				fmt.Sprintf("%s.inputs[%d].value_source.variable", field, j),
			))
		}
	}
	return issues
}

func variableRefVisible(scopes []VariableScope, ref VariableRef) bool {
	refScope := strings.TrimSpace(ref.Scope)
	refName := strings.TrimSpace(ref.Name)
	refNodeID := strings.TrimSpace(ref.NodeID)
	refPath := strings.TrimSpace(ref.Path)
	for _, scope := range scopes {
		if scope.Scope != refScope {
			continue
		}
		for _, candidate := range scope.Variables {
			if candidate.Name != refName {
				continue
			}
			if refNodeID != "" && candidate.NodeID != refNodeID {
				continue
			}
			if refPath != "" && candidate.Path != refPath {
				continue
			}
			return true
		}
	}
	return false
}

func validParamType(typ string) bool {
	switch strings.TrimSpace(typ) {
	case "any", "string", "number", "integer", "boolean", "object", "array", "secret", "duration":
		return true
	default:
		return false
	}
}

type graphIndex struct {
	graph     Graph
	nodes     map[string]Node
	nodeIDs   map[string]int
	ports     map[string]map[string]struct{}
	order     map[string]int
	outgoing  map[string][]Edge
	incoming  map[string][]Edge
	startNode string
}

func newGraphIndex(g Graph) graphIndex {
	idx := graphIndex{
		graph:    g,
		nodes:    map[string]Node{},
		nodeIDs:  map[string]int{},
		ports:    map[string]map[string]struct{}{},
		order:    map[string]int{},
		outgoing: map[string][]Edge{},
		incoming: map[string][]Edge{},
	}
	for i, node := range g.Nodes {
		if _, exists := idx.nodeIDs[node.ID]; !exists {
			idx.nodeIDs[node.ID] = i
		}
		idx.nodes[node.ID] = node
		idx.order[node.ID] = i
		if len(node.Ports) > 0 {
			idx.ports[node.ID] = map[string]struct{}{}
			for _, port := range node.Ports {
				if strings.TrimSpace(port.ID) != "" {
					idx.ports[node.ID][port.ID] = struct{}{}
				}
			}
		}
		if node.Type == NodeTypeStart && idx.startNode == "" {
			idx.startNode = node.ID
		}
	}
	for _, edge := range g.Edges {
		if _, ok := idx.nodes[edge.Source]; ok {
			if _, ok := idx.nodes[edge.Target]; ok {
				idx.outgoing[edge.Source] = append(idx.outgoing[edge.Source], edge)
				idx.incoming[edge.Target] = append(idx.incoming[edge.Target], edge)
			}
		}
	}
	return idx
}

func (idx graphIndex) hasPort(nodeID, portID string) bool {
	ports := idx.ports[nodeID]
	if len(ports) == 0 {
		return false
	}
	_, ok := ports[portID]
	return ok
}

func (idx graphIndex) continuationOutDegree(nodeID string) int {
	count := 0
	for _, edge := range idx.outgoing[nodeID] {
		target, ok := idx.nodes[edge.Target]
		if !ok || target.Type == NodeTypeHandler || target.Type == NodeTypeGroup {
			continue
		}
		count++
	}
	return count
}

func detectCycle(idx graphIndex) []string {
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	state := map[string]int{}
	stack := []string{}
	var visit func(string) []string
	visit = func(id string) []string {
		state[id] = visiting
		stack = append(stack, id)
		edges := append([]Edge(nil), idx.outgoing[id]...)
		sort.SliceStable(edges, func(i, j int) bool {
			return idx.order[edges[i].Target] < idx.order[edges[j].Target]
		})
		for _, edge := range edges {
			target := edge.Target
			switch state[target] {
			case visiting:
				for i, entry := range stack {
					if entry == target {
						return append(append([]string(nil), stack[i:]...), target)
					}
				}
			case unvisited:
				if cycle := visit(target); len(cycle) > 0 {
					return cycle
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = visited
		return nil
	}
	for _, node := range idx.graph.Nodes {
		if state[node.ID] == unvisited {
			if cycle := visit(node.ID); len(cycle) > 0 {
				return cycle
			}
		}
	}
	return nil
}

func unreachableExecutableNodes(idx graphIndex) []string {
	if idx.startNode == "" {
		return nil
	}
	seen := map[string]bool{idx.startNode: true}
	queue := []string{idx.startNode}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, edge := range idx.outgoing[id] {
			if !seen[edge.Target] {
				seen[edge.Target] = true
				queue = append(queue, edge.Target)
			}
		}
	}
	var out []string
	for _, node := range idx.graph.Nodes {
		if node.Type == NodeTypeGroup || node.Type == NodeTypeHandler || node.Type == NodeTypeStart {
			continue
		}
		if !seen[node.ID] {
			out = append(out, node.ID)
		}
	}
	return out
}

func topologicalNodeIDs(idx graphIndex) []string {
	inDegree := make(map[string]int, len(idx.nodes))
	for id := range idx.nodes {
		inDegree[id] = 0
	}
	for _, edge := range idx.graph.Edges {
		if _, ok := idx.nodes[edge.Source]; !ok {
			continue
		}
		if _, ok := idx.nodes[edge.Target]; !ok {
			continue
		}
		inDegree[edge.Target]++
	}
	ready := make([]string, 0, len(idx.nodes))
	for _, node := range idx.graph.Nodes {
		if inDegree[node.ID] == 0 {
			ready = append(ready, node.ID)
		}
	}
	out := make([]string, 0, len(idx.nodes))
	for len(ready) > 0 {
		sort.SliceStable(ready, func(i, j int) bool {
			return idx.order[ready[i]] < idx.order[ready[j]]
		})
		id := ready[0]
		ready = ready[1:]
		out = append(out, id)
		for _, edge := range idx.outgoing[id] {
			inDegree[edge.Target]--
			if inDegree[edge.Target] == 0 {
				ready = append(ready, edge.Target)
			}
		}
	}
	return out
}

func nodeStep(node Node) workflow.Step {
	if node.Step != nil {
		step := *node.Step
		if strings.TrimSpace(step.Name) == "" {
			step.Name = strings.TrimSpace(node.StepName)
		}
		if strings.TrimSpace(step.ID) == "" {
			step.ID = strings.TrimSpace(firstNonEmpty(node.StepID, node.ID))
		}
		return step
	}
	return workflow.Step{
		ID:   strings.TrimSpace(firstNonEmpty(node.StepID, node.ID)),
		Name: strings.TrimSpace(node.StepName),
	}
}

func nodeHandler(node Node) workflow.Handler {
	if node.Handler != nil {
		handler := *node.Handler
		if strings.TrimSpace(handler.Name) == "" {
			handler.Name = strings.TrimSpace(node.HandlerName)
		}
		return handler
	}
	return workflow.Handler{Name: strings.TrimSpace(node.HandlerName)}
}

func issue(code, message, nodeID, edgeID, field string) Issue {
	return Issue{
		Severity:   "error",
		Code:       code,
		Message:    message,
		NodeID:     nodeID,
		EdgeID:     edgeID,
		Field:      field,
		Suggestion: suggestionForIssue(code),
	}
}

func suggestionForIssue(code string) string {
	switch code {
	case "graph_version_required", "graph_version_unsupported":
		return `Set graph.version to "v1".`
	case "nodes_required", "start_required", "start_duplicate":
		return "Ensure the graph has exactly one start node and at least one executable node."
	case "executable_node_required":
		return "Add at least one executable action, condition, approval, subflow, loop, join, or end node."
	case "node_id_required", "node_id_duplicate":
		return "Use a stable unique node id."
	case "node_type_invalid":
		return "Choose a supported Runner visual node type."
	case "step_name_required", "step_name_duplicate", "step_id_duplicate":
		return "Use unique step names and stable step ids for executable nodes."
	case "step_action_required":
		return "Choose an action from the Runner action catalog."
	case "handler_name_required", "handler_name_duplicate", "handler_action_required":
		return "Configure a unique handler name and a supported handler action."
	case "approval_required", "approval_subjects_required", "approval_timeout_required":
		return "Configure approvers, timeout, and timeout policy on the approval node."
	case "subflow_workflow_required":
		return "Select the child workflow this subflow node should run."
	case "join_strategy_invalid", "join_requires_multiple_inputs":
		return "Use a supported join strategy and connect at least two upstream branches."
	case "loop_required", "loop_mode_required", "loop_items_required", "loop_while_condition_required", "loop_mode_invalid", "loop_max_iterations_required", "loop_body_step_loop_unsupported":
		return "Configure loop mode, iteration limit, and loop body without nesting legacy step.loop."
	case "edge_id_required", "edge_id_duplicate":
		return "Use a stable unique edge id."
	case "edge_kind_invalid":
		return "Use a supported edge kind such as next, success, failure, if, else, or approval branches."
	case "edge_source_required", "edge_source_missing", "edge_source_port_missing":
		return "Reconnect the edge from an existing source node output port."
	case "edge_target_required", "edge_target_missing", "edge_target_port_missing":
		return "Reconnect the edge to an existing target node input port."
	case "edge_self_loop":
		return "Connect the edge to a different downstream node."
	case "condition_required":
		return "Add a condition expression to the edge or the source condition node."
	case "graph_cycle":
		return "Remove one edge from the cycle so the workflow remains a DAG."
	case "node_unreachable":
		return "Connect the node to the start path or remove the unused node."
	case "node_outgoing_required":
		return "Connect this node to a downstream executable or end node."
	case "input_key_required", "input_key_duplicate", "input_type_invalid", "input_variable_required", "input_expression_required", "input_secret_ref_required", "input_env_key_required", "input_value_source_invalid", "input_variable_scope_invalid":
		return "Fix the highlighted input parameter configuration."
	case "output_key_required", "output_key_duplicate", "output_type_invalid", "output_extract_path_required", "output_extract_expression_required", "output_extract_source_invalid":
		return "Fix the highlighted output parameter configuration."
	default:
		return "Review the highlighted graph field and update it before saving or running."
	}
}
