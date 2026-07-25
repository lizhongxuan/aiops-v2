package edit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"runner/workflow/visual"
)

type ApplyResult struct {
	Graph       visual.Graph         `json:"graph"`
	ClientRefs  map[string]string    `json:"clientRefs"`
	Diff        GraphDiff            `json:"diff"`
	Inverse     InversePlan          `json:"inverse"`
	Diagnostics []WorkflowDiagnostic `json:"diagnostics,omitempty"`
}

var nodePatchFieldMap = map[string]string{"Type": "Type", "Position": "Position", "StepName": "StepName", "StepID": "StepID", "Step": "Step", "HandlerName": "HandlerName", "Handler": "Handler", "Approval": "Approval", "Condition": "Condition", "Subflow": "Subflow", "Aggregator": "Aggregator", "Join": "Join", "Loop": "Loop", "ParentID": "ParentID", "Label": "Label", "Collapsed": "Collapsed", "Ports": "Ports", "Inputs": "Inputs", "Outputs": "Outputs"}
var edgePatchFieldMap = map[string]string{"Source": "Source", "SourcePort": "SourcePort", "Target": "Target", "TargetPort": "TargetPort", "Kind": "Kind", "Condition": "Condition"}
var nodeJSONFields = map[string]string{"stepName": "step_name", "stepId": "step_id", "handlerName": "handler_name", "parentId": "parent_id"}
var edgeJSONFields = map[string]string{"sourcePort": "source_port", "targetPort": "target_port"}

func ApplyOperations(base visual.Graph, operations []WorkflowOperation) (ApplyResult, error) {
	working, err := cloneEditGraph(base)
	if err != nil {
		return ApplyResult{}, err
	}
	result := ApplyResult{ClientRefs: map[string]string{}}
	nodeRefs, err := allocateClientRefs(working, operations, result.ClientRefs)
	if err != nil {
		return result, err
	}
	var inverse []WorkflowOperation
	for _, operation := range operations {
		if err := operation.Validate(); err != nil {
			return result, invalidPatch(err.Error(), nil)
		}
		chunk, applyErr := applyOne(&working, operation, nodeRefs, result.ClientRefs)
		if applyErr != nil {
			if errors.Is(applyErr, errOperationNotImplemented) {
				result.Diagnostics = []WorkflowDiagnostic{{Code: "OPERATION_NOT_IMPLEMENTED", Severity: "error", Message: applyErr.Error(), Suggestion: "wait for the operation capability to be activated"}}
			}
			return result, applyErr
		}
		inverse = append(chunk, inverse...)
	}
	if err := visual.ValidateGraph(working); err != nil {
		result.Diagnostics = validationDiagnostics(err)
		return result, invalidPatch("candidate graph is invalid", diagnosticPaths(result.Diagnostics))
	}
	result.Diff = DiffGraphs(base, working)
	result.Inverse, err = buildInversePlan(inverse, working, result.Diff.TouchedPaths)
	if err != nil {
		return ApplyResult{}, err
	}
	result.Graph, err = cloneEditGraph(working)
	return result, err
}

var errOperationNotImplemented = errors.New("operation is planned but not implemented")

func applyOne(graph *visual.Graph, operation WorkflowOperation, nodeRefs, allRefs map[string]string) ([]WorkflowOperation, error) {
	switch value := operation.Value.(type) {
	case *AddNodeOperation:
		id := operationID(value.Node.ID, value.ClientRef, allRefs)
		node := visual.Node{ID: id}
		if err := mergePatch(&node, value.Node.NodePatch, nodeJSONFields); err != nil {
			return nil, invalidPatch(err.Error(), nil)
		}
		graph.Nodes = append(graph.Nodes, node)
		return []WorkflowOperation{NewOperation(&DeleteNodeOperation{NodeID: id, CascadeEdges: false})}, nil
	case *UpdateNodeOperation:
		nodeID := resolveRef(value.NodeID, nodeRefs)
		index := nodeIndex(graph.Nodes, nodeID)
		if index < 0 {
			return nil, invalidPatch("node not found", []string{"/nodes/" + escapePointer(nodeID)})
		}
		before := graph.Nodes[index]
		if err := mergePatch(&graph.Nodes[index], value.Set, nodeJSONFields); err != nil {
			return nil, invalidPatch(err.Error(), nil)
		}
		inverse, reversible := inverseNodePatch(before, value.Set)
		if !reversible {
			return nil, invalidPatch("update cannot be represented by a reversible v2 patch", nil)
		}
		return []WorkflowOperation{NewOperation(&UpdateNodeOperation{NodeID: nodeID, Set: inverse})}, nil
	case *DeleteNodeOperation:
		resolved := *value
		resolved.NodeID = resolveRef(value.NodeID, nodeRefs)
		return deleteNode(graph, &resolved)
	case *AddEdgeOperation:
		id := operationID(value.Edge.ID, value.ClientRef, allRefs)
		edge := visual.Edge{ID: id}
		if err := mergePatch(&edge, value.Edge.EdgePatch, edgeJSONFields); err != nil {
			return nil, invalidPatch(err.Error(), nil)
		}
		edge.Source, edge.Target = resolveRef(edge.Source, nodeRefs), resolveRef(edge.Target, nodeRefs)
		graph.Edges = append(graph.Edges, edge)
		return []WorkflowOperation{NewOperation(&DeleteEdgeOperation{EdgeID: id})}, nil
	case *UpdateEdgeOperation:
		edgeID := resolveRef(value.EdgeID, allRefs)
		index := edgeIndex(graph.Edges, edgeID)
		if index < 0 {
			return nil, invalidPatch("edge not found", []string{"/edges/" + escapePointer(edgeID)})
		}
		before := graph.Edges[index]
		if err := mergePatch(&graph.Edges[index], value.Set, edgeJSONFields); err != nil {
			return nil, invalidPatch(err.Error(), nil)
		}
		graph.Edges[index].Source = resolveRef(graph.Edges[index].Source, nodeRefs)
		graph.Edges[index].Target = resolveRef(graph.Edges[index].Target, nodeRefs)
		inverse, _ := inverseEdgePatch(before, value.Set)
		return []WorkflowOperation{NewOperation(&UpdateEdgeOperation{EdgeID: edgeID, Set: inverse})}, nil
	case *DeleteEdgeOperation:
		edgeID := resolveRef(value.EdgeID, allRefs)
		index := edgeIndex(graph.Edges, edgeID)
		if index < 0 {
			return nil, invalidPatch("edge not found", []string{"/edges/" + escapePointer(edgeID)})
		}
		before := graph.Edges[index]
		graph.Edges = append(graph.Edges[:index], graph.Edges[index+1:]...)
		return []WorkflowOperation{NewOperation(&AddEdgeOperation{Edge: editableEdge(before)})}, nil
	default:
		return nil, fmt.Errorf("%w: %s", errOperationNotImplemented, operation.Value.Kind())
	}
}
func allocateClientRefs(graph visual.Graph, operations []WorkflowOperation, refs map[string]string) (map[string]string, error) {
	nodeRefs := map[string]string{}
	nodeIDs, edgeIDs := map[string]bool{}, map[string]bool{}
	for _, node := range graph.Nodes {
		nodeIDs[node.ID] = true
	}
	for _, edge := range graph.Edges {
		edgeIDs[edge.ID] = true
	}
	for _, operation := range operations {
		var clientRef, id string
		var used map[string]bool
		switch value := operation.Value.(type) {
		case *AddNodeOperation:
			clientRef, id, used = value.ClientRef, value.Node.ID, nodeIDs
		case *AddEdgeOperation:
			clientRef, id, used = value.ClientRef, value.Edge.ID, edgeIDs
		default:
			continue
		}
		if id == "" {
			id = clientRef
		}
		if strings.TrimSpace(id) == "" || used[id] || (clientRef != "" && refs[clientRef] != "") {
			return nil, invalidPatch("duplicate or empty operation id/clientRef", nil)
		}
		used[id], refs[clientRef] = true, id
		if _, ok := operation.Value.(*AddNodeOperation); ok && clientRef != "" {
			nodeRefs[clientRef] = id
		}
	}
	return nodeRefs, nil
}
func deleteNode(graph *visual.Graph, operation *DeleteNodeOperation) ([]WorkflowOperation, error) {
	index := nodeIndex(graph.Nodes, operation.NodeID)
	if index < 0 {
		return nil, invalidPatch("node not found", []string{"/nodes/" + escapePointer(operation.NodeID)})
	}
	var incident []visual.Edge
	for _, edge := range graph.Edges {
		if edge.Source == operation.NodeID || edge.Target == operation.NodeID {
			incident = append(incident, edge)
		}
	}
	if len(incident) > 0 && !operation.CascadeEdges {
		return nil, invalidPatch("node has incident edges; cascadeEdges must be explicit", []string{"/nodes/" + escapePointer(operation.NodeID)})
	}
	before := graph.Nodes[index]
	graph.Nodes = append(graph.Nodes[:index], graph.Nodes[index+1:]...)
	kept := graph.Edges[:0]
	for _, edge := range graph.Edges {
		if edge.Source != operation.NodeID && edge.Target != operation.NodeID {
			kept = append(kept, edge)
		}
	}
	graph.Edges = kept
	// A restored node already has its durable ID. Carry the same value as a
	// stable clientRef so strict external ChangeSet consumers can project Undo
	// candidates without inventing identity.
	inverse := []WorkflowOperation{NewOperation(&AddNodeOperation{ClientRef: before.ID, Node: editableNode(before)})}
	for _, edge := range incident {
		inverse = append(inverse, NewOperation(&AddEdgeOperation{Edge: editableEdge(edge)}))
	}
	return inverse, nil
}
func mergePatch(target, patch any, fieldNames map[string]string) error {
	targetObject, patchObject := jsonObject(target), jsonObject(patch)
	patchJSON, _ := json.Marshal(patch)
	if bytes.Contains(patchJSON, []byte(`"ui":`)) || bytes.Contains(patchJSON, []byte(`"state":`)) {
		return fmt.Errorf("ui/state fields are not editable")
	}
	for key, value := range patchObject {
		if mapped := fieldNames[key]; mapped != "" {
			key = mapped
		}
		targetObject[key] = value
	}
	raw, err := json.Marshal(targetObject)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}
func editableNode(node visual.Node) EditableNode {
	return EditableNode{ID: node.ID, NodePatch: NodePatch{
		Type: &node.Type, Position: &node.Position, StepName: &node.StepName, StepID: &node.StepID, Step: node.Step,
		HandlerName: &node.HandlerName, Handler: node.Handler, Approval: node.Approval, Condition: node.Condition,
		Subflow: node.Subflow, Aggregator: node.Aggregator, Join: node.Join, Loop: node.Loop, ParentID: &node.ParentID,
		Label: &node.Label, Collapsed: &node.Collapsed, Ports: &node.Ports, Inputs: &node.Inputs, Outputs: &node.Outputs,
	}}
}
func editableEdge(edge visual.Edge) EditableEdge {
	return EditableEdge{ID: edge.ID, EdgePatch: EdgePatch{Source: &edge.Source, SourcePort: &edge.SourcePort, Target: &edge.Target, TargetPort: &edge.TargetPort, Kind: &edge.Kind, Condition: &edge.Condition}}
}
func operationID(id, clientRef string, refs map[string]string) string {
	if id != "" {
		return id
	}
	return refs[clientRef]
}
func resolveRef(id string, refs map[string]string) string {
	if refs[id] != "" {
		return refs[id]
	}
	return id
}
func nodeIndex(nodes []visual.Node, id string) int {
	for i := range nodes {
		if nodes[i].ID == id {
			return i
		}
	}
	return -1
}
func edgeIndex(edges []visual.Edge, id string) int {
	for i := range edges {
		if edges[i].ID == id {
			return i
		}
	}
	return -1
}
func cloneEditGraph(graph visual.Graph) (visual.Graph, error) {
	raw, err := json.Marshal(graph)
	if err != nil {
		return visual.Graph{}, err
	}
	var cloned visual.Graph
	err = json.Unmarshal(raw, &cloned)
	return cloned, err
}
func invalidPatch(message string, paths []string) *DomainError {
	sort.Strings(paths)
	return &DomainError{Kind: "error", Code: ErrorInvalidPatch, Message: message, ConflictingPaths: paths}
}
func validationDiagnostics(err error) []WorkflowDiagnostic {
	var validation *visual.ValidationError
	if !errors.As(err, &validation) {
		return []WorkflowDiagnostic{{Code: "GRAPH_INVALID", Severity: "error", Message: err.Error()}}
	}
	result := make([]WorkflowDiagnostic, 0, len(validation.Issues))
	for _, issue := range validation.Issues {
		result = append(result, WorkflowDiagnostic{Code: issue.Code, Severity: issue.Severity, Message: issue.Message, NodeID: issue.NodeID, EdgeID: issue.EdgeID, Field: issue.Field, Suggestion: issue.Suggestion})
	}
	return result
}
func diagnosticPaths(diagnostics []WorkflowDiagnostic) []string {
	paths := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if diagnostic.Field != "" {
			paths = append(paths, diagnostic.Field)
		}
	}
	return paths
}
