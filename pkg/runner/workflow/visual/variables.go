package visual

import (
	"fmt"
	"sort"
	"strings"
)

type VariableResolveResult struct {
	NodeID string                     `json:"node_id,omitempty"`
	Scopes []VariableScope            `json:"scopes,omitempty"`
	Nodes  map[string][]VariableScope `json:"nodes,omitempty"`
}

type VariableScope struct {
	Scope     string              `json:"scope"`
	Title     string              `json:"title,omitempty"`
	Variables []VariableCandidate `json:"variables"`
}

type VariableCandidate struct {
	Scope       string `json:"scope"`
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Path        string `json:"path,omitempty"`
	NodeID      string `json:"node_id,omitempty"`
	ParentID    string `json:"parent_id,omitempty"`
	Source      string `json:"source,omitempty"`
	Description string `json:"description,omitempty"`
}

func ResolveVariableScopes(g Graph, nodeID string) VariableResolveResult {
	idx := newGraphIndex(g)
	visibility := resolveVariableVisibility(g, idx)
	nodeID = strings.TrimSpace(nodeID)
	if nodeID != "" {
		return VariableResolveResult{
			NodeID: nodeID,
			Scopes: groupVariableScopes(visibility[nodeID]),
		}
	}
	nodes := make(map[string][]VariableScope, len(g.Nodes))
	for _, node := range g.Nodes {
		nodes[node.ID] = groupVariableScopes(visibility[node.ID])
	}
	return VariableResolveResult{Nodes: nodes}
}

func VariablesForNode(g Graph, nodeID string) []VariableScope {
	return ResolveVariableScopes(g, nodeID).Scopes
}

func resolveVariableVisibility(g Graph, idx graphIndex) map[string]variableSet {
	global := globalVariableSet(g)
	visible := make(map[string]variableSet, len(idx.nodes))
	mergeIncoming := map[string][]variableSet{}
	for id := range idx.nodes {
		visible[id] = global.clone()
	}

	order := topologicalNodeIDs(idx)
	if len(order) != len(idx.nodes) {
		order = make([]string, 0, len(g.Nodes))
		for _, node := range g.Nodes {
			order = append(order, node.ID)
		}
	}

	for _, id := range order {
		node, ok := idx.nodes[id]
		if !ok {
			continue
		}
		current := visible[id].clone()
		if len(idx.incoming[id]) > 1 && len(mergeIncoming[id]) > 0 {
			current = global.clone()
			current.merge(intersectVariableSets(mergeIncoming[id]))
			visible[id] = current.clone()
		}
		current.merge(nodeOutputVariableSet(node))
		for _, edge := range idx.outgoing[id] {
			target, ok := idx.nodes[edge.Target]
			if !ok {
				continue
			}
			next := filterVariablesForTarget(current, target)
			if len(idx.incoming[target.ID]) > 1 {
				mergeIncoming[target.ID] = append(mergeIncoming[target.ID], next)
				continue
			}
			if _, ok := visible[target.ID]; !ok {
				visible[target.ID] = global.clone()
			}
			visible[target.ID].merge(next)
		}
	}
	return visible
}

type variableSet map[string]VariableCandidate

func (s variableSet) add(v VariableCandidate) {
	v.Scope = strings.TrimSpace(v.Scope)
	v.Name = strings.TrimSpace(v.Name)
	if v.Scope == "" || v.Name == "" {
		return
	}
	if v.Type == "" {
		v.Type = "any"
	}
	s[variableKey(v)] = v
}

func (s variableSet) merge(other variableSet) {
	for key, value := range other {
		s[key] = value
	}
}

func (s variableSet) clone() variableSet {
	out := variableSet{}
	for key, value := range s {
		out[key] = value
	}
	return out
}

func variableKey(v VariableCandidate) string {
	return strings.Join([]string{v.Scope, v.NodeID, v.Name, v.Path}, "\x00")
}

func globalVariableSet(g Graph) variableSet {
	out := variableSet{}
	for _, item := range []VariableCandidate{
		{Scope: "system", Name: "run_id", Type: "string", Path: "system.run_id", Source: "system"},
		{Scope: "system", Name: "workflow_name", Type: "string", Path: "system.workflow_name", Source: "system"},
		{Scope: "system", Name: "operator", Type: "string", Path: "system.operator", Source: "system"},
		{Scope: "system", Name: "timestamp", Type: "string", Path: "system.timestamp", Source: "system"},
	} {
		out.add(item)
	}
	for _, node := range g.Nodes {
		if node.Type != NodeTypeStart {
			continue
		}
		for _, output := range node.Outputs {
			out.add(variableFromOutput("workflow_input", node, output, "workflow_input"))
		}
		for _, input := range node.Inputs {
			out.add(VariableCandidate{
				Scope:       "workflow_input",
				Name:        input.Key,
				Type:        input.Type,
				Path:        "workflow.input." + input.Key,
				Source:      "workflow_input",
				Description: input.Description,
			})
		}
	}
	for _, key := range sortedAnyMapKeys(g.Workflow.Vars) {
		out.add(VariableCandidate{
			Scope:  "workflow_var",
			Name:   key,
			Type:   inferVariableType(g.Workflow.Vars[key]),
			Path:   "vars." + key,
			Source: "workflow_var",
		})
	}
	for _, key := range sortedAnyMapKeys(g.Workflow.Inventory.Vars) {
		out.add(VariableCandidate{
			Scope:  "inventory",
			Name:   key,
			Type:   inferVariableType(g.Workflow.Inventory.Vars[key]),
			Path:   "inventory.vars." + key,
			Source: "inventory",
		})
	}
	for _, name := range sortedHostNames(g) {
		out.add(VariableCandidate{Scope: "inventory", Name: name, Type: "host", Path: "inventory.hosts." + name, Source: "inventory"})
	}
	for _, name := range sortedGroupNames(g) {
		out.add(VariableCandidate{Scope: "inventory", Name: name, Type: "group", Path: "inventory.groups." + name, Source: "inventory"})
	}
	return out
}

func nodeOutputVariableSet(node Node) variableSet {
	out := variableSet{}
	if node.Type == NodeTypeStart {
		return out
	}
	for _, output := range node.Outputs {
		out.add(variableFromOutput("node_output", node, output, "node_output"))
		if node.Type == NodeTypeSubflow {
			out.add(variableFromOutput("subflow", node, output, "subflow"))
		}
	}
	if node.Type == NodeTypeVariableAggregator && node.Aggregator != nil && strings.TrimSpace(node.Aggregator.OutputKey) != "" {
		output := OutputParamSpec{Key: strings.TrimSpace(node.Aggregator.OutputKey), Type: "any"}
		out.add(variableFromOutput("node_output", node, output, "node_output"))
	}
	if node.Type == NodeTypeManualApproval {
		for _, item := range []VariableCandidate{
			{Scope: "approval", Name: "decision", Type: "string", Path: "approval." + node.ID + ".decision", NodeID: node.ID, Source: "approval"},
			{Scope: "approval", Name: "actor", Type: "string", Path: "approval." + node.ID + ".actor", NodeID: node.ID, Source: "approval"},
			{Scope: "approval", Name: "comment", Type: "string", Path: "approval." + node.ID + ".comment", NodeID: node.ID, Source: "approval"},
			{Scope: "approval", Name: "resolved_at", Type: "string", Path: "approval." + node.ID + ".resolved_at", NodeID: node.ID, Source: "approval"},
		} {
			item.ParentID = node.ParentID
			out.add(item)
		}
	}
	return out
}

func variableFromOutput(scope string, node Node, output OutputParamSpec, source string) VariableCandidate {
	path := output.ExtractSource.Path
	if strings.TrimSpace(path) == "" {
		path = fmt.Sprintf("nodes.%s.outputs.%s", node.ID, output.Key)
	}
	return VariableCandidate{
		Scope:       scope,
		Name:        output.Key,
		Type:        output.Type,
		Path:        path,
		NodeID:      node.ID,
		ParentID:    node.ParentID,
		Source:      source,
		Description: firstNonEmpty(output.Description, output.Label),
	}
}

func filterVariablesForTarget(input variableSet, target Node) variableSet {
	out := variableSet{}
	for key, value := range input {
		if value.ParentID != "" && value.ParentID != target.ParentID {
			continue
		}
		out[key] = value
	}
	return out
}

func intersectVariableSets(inputs []variableSet) variableSet {
	if len(inputs) == 0 {
		return variableSet{}
	}
	out := variableSet{}
	for key, value := range inputs[0] {
		keep := true
		for _, other := range inputs[1:] {
			if _, ok := other[key]; !ok {
				keep = false
				break
			}
		}
		if keep {
			out[key] = value
		}
	}
	return out
}

func groupVariableScopes(input variableSet) []VariableScope {
	byScope := map[string][]VariableCandidate{}
	for _, item := range input {
		byScope[item.Scope] = append(byScope[item.Scope], item)
	}
	order := []string{"system", "workflow_input", "workflow_var", "inventory", "node_output", "approval", "subflow"}
	var scopes []VariableScope
	for _, scope := range order {
		items := byScope[scope]
		if len(items) == 0 {
			continue
		}
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].NodeID != items[j].NodeID {
				return items[i].NodeID < items[j].NodeID
			}
			return items[i].Name < items[j].Name
		})
		scopes = append(scopes, VariableScope{Scope: scope, Title: variableScopeTitle(scope), Variables: items})
	}
	return scopes
}

func variableScopeTitle(scope string) string {
	switch scope {
	case "system":
		return "System"
	case "workflow_input":
		return "Workflow Input"
	case "workflow_var":
		return "Workflow Vars"
	case "inventory":
		return "Inventory"
	case "node_output":
		return "Node Output"
	case "approval":
		return "Approval"
	case "subflow":
		return "Subflow"
	default:
		return scope
	}
}

func sortedAnyMapKeys(input map[string]any) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func sortedHostNames(g Graph) []string {
	keys := make([]string, 0, len(g.Workflow.Inventory.Hosts))
	for key := range g.Workflow.Inventory.Hosts {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func sortedGroupNames(g Graph) []string {
	keys := make([]string, 0, len(g.Workflow.Inventory.Groups))
	for key := range g.Workflow.Inventory.Groups {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func inferVariableType(value any) string {
	switch value.(type) {
	case bool:
		return "boolean"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "integer"
	case float32, float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "any"
	}
}
