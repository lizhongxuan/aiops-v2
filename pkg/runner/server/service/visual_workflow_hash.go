package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"runner/workflow/visual"
)

type WorkflowGraphHashes struct {
	SemanticHash string `json:"semantic_hash"`
	LayoutHash   string `json:"layout_hash"`
}

// Workflow hash field classification:
//
//   - semantic/contract: workflow runtime fields; node identity, type, action,
//     arguments, conditions, bindings, ports and control specs; edge identity,
//     endpoints, source/target handles, kind and condition.
//   - layout/presentation: graph layout/viewport, node position, label,
//     collapsed state and presentation UI, plus edge presentation UI.
//   - excluded/volatile: resource version, run state and duplicated compiled
//     x_runner graph/UI projections. Nested UI maps never define semantics.
//
// Tests in visual_workflow_hash_test.go enforce the boundary for every field
// class that can invalidate validation or dry-run artifacts.
func VisualWorkflowGraphHashes(graph visual.Graph) (WorkflowGraphHashes, error) {
	graph = normalizeGraph(graph)
	workflowExtensions := cloneAnyMap(graph.Workflow.Extensions)
	semanticGraph, err := cloneVisualGraphForHash(graph)
	if err != nil {
		return WorkflowGraphHashes{}, err
	}
	scrubVisualGraphSemanticHashInput(&semanticGraph)
	semanticHash, err := canonicalWorkflowHash(visualGraphSemanticHash{
		Graph:              semanticGraph,
		WorkflowExtensions: workflowExtensions,
	})
	if err != nil {
		return WorkflowGraphHashes{}, err
	}

	layoutHash, err := canonicalWorkflowHash(visualGraphLayoutHashInput(graph))
	if err != nil {
		return WorkflowGraphHashes{}, err
	}
	return WorkflowGraphHashes{SemanticHash: semanticHash, LayoutHash: layoutHash}, nil
}

type visualGraphSemanticHash struct {
	visual.Graph
	WorkflowExtensions map[string]any `json:"workflow_extensions,omitempty"`
}

func workflowYAMLGraphHashes(raw []byte) (WorkflowGraphHashes, error) {
	graph, err := visual.ParseYAMLToGraph(raw)
	if err != nil {
		return WorkflowGraphHashes{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return VisualWorkflowGraphHashes(graph)
}

// legacyVisualWorkflowSemanticHash reproduces the pre-control-plane hash that
// included compiled Workflow.Steps/Handlers alongside their authoritative
// visual node projections. It is read-only migration support for already
// committed validation bases; new revisions are always written with the
// canonical hash from VisualWorkflowGraphHashes.
func legacyVisualWorkflowSemanticHash(graph visual.Graph) (string, error) {
	graph = normalizeGraph(graph)
	workflowExtensions := cloneAnyMap(graph.Workflow.Extensions)
	semanticGraph, err := cloneVisualGraphForHash(graph)
	if err != nil {
		return "", err
	}
	steps, handlers := semanticGraph.Workflow.Steps, semanticGraph.Workflow.Handlers
	scrubVisualGraphSemanticHashInput(&semanticGraph)
	semanticGraph.Workflow.Steps, semanticGraph.Workflow.Handlers = steps, handlers
	return canonicalWorkflowHash(visualGraphSemanticHash{
		Graph:              semanticGraph,
		WorkflowExtensions: workflowExtensions,
	})
}

func cloneVisualGraphForHash(graph visual.Graph) (visual.Graph, error) {
	raw, err := json.Marshal(graph)
	if err != nil {
		return visual.Graph{}, err
	}
	var out visual.Graph
	if err := json.Unmarshal(raw, &out); err != nil {
		return visual.Graph{}, err
	}
	return out, nil
}

func scrubVisualGraphSemanticHashInput(graph *visual.Graph) {
	graph.Layout = visual.Layout{}
	graph.UI = nil
	graph.Workflow.Description = ""
	graph.Workflow.XRunnerUI = nil
	graph.Workflow.XRunnerGraph = nil
	// Steps and handlers are compiled projections of the typed visual nodes.
	// A committed YAML round-trip populates these fields, while a Canvas edit
	// updates the authoritative node first. Hashing both copies makes a valid
	// node edit look stale until the next compile regenerates the projection.
	graph.Workflow.Steps = nil
	graph.Workflow.Handlers = nil
	for index := range graph.Nodes {
		node := &graph.Nodes[index]
		node.Position = visual.Position{}
		node.Label = ""
		node.Collapsed = false
		node.UI = nil
		node.State = nil
		for portIndex := range node.Ports {
			node.Ports[portIndex].UI = nil
		}
		if node.Approval != nil {
			node.Approval.UI = nil
		}
		if node.Condition != nil {
			node.Condition.UI = nil
			for branchIndex := range node.Condition.Elif {
				node.Condition.Elif[branchIndex].UI = nil
			}
		}
		if node.Subflow != nil {
			node.Subflow.UI = nil
		}
		if node.Aggregator != nil {
			node.Aggregator.UI = nil
		}
		if node.Join != nil {
			node.Join.UI = nil
		}
		if node.Loop != nil {
			node.Loop.UI = nil
		}
		for inputIndex := range node.Inputs {
			node.Inputs[inputIndex].UI = nil
		}
		for outputIndex := range node.Outputs {
			node.Outputs[outputIndex].UI = nil
		}
	}
	for index := range graph.Edges {
		graph.Edges[index].UI = nil
		graph.Edges[index].State = nil
	}
}

type visualGraphLayoutHash struct {
	Layout visual.Layout               `json:"layout,omitempty"`
	Nodes  []visualGraphLayoutHashNode `json:"nodes,omitempty"`
	Edges  []visualGraphLayoutHashEdge `json:"edges,omitempty"`
	UI     map[string]any              `json:"ui,omitempty"`
}

type visualGraphLayoutHashNode struct {
	ID        string          `json:"id"`
	Position  visual.Position `json:"position"`
	Label     string          `json:"label,omitempty"`
	Collapsed bool            `json:"collapsed,omitempty"`
	UI        map[string]any  `json:"ui,omitempty"`
}

type visualGraphLayoutHashEdge struct {
	ID string         `json:"id"`
	UI map[string]any `json:"ui,omitempty"`
}

func visualGraphLayoutHashInput(graph visual.Graph) visualGraphLayoutHash {
	out := visualGraphLayoutHash{
		Layout: graph.Layout,
		UI:     withoutGraphResourceVersion(graph.UI),
		Nodes:  make([]visualGraphLayoutHashNode, 0, len(graph.Nodes)),
		Edges:  make([]visualGraphLayoutHashEdge, 0, len(graph.Edges)),
	}
	for _, node := range graph.Nodes {
		out.Nodes = append(out.Nodes, visualGraphLayoutHashNode{
			ID:        strings.TrimSpace(node.ID),
			Position:  node.Position,
			Label:     strings.TrimSpace(node.Label),
			Collapsed: node.Collapsed,
			UI:        cloneAnyMap(node.UI),
		})
	}
	for _, edge := range graph.Edges {
		out.Edges = append(out.Edges, visualGraphLayoutHashEdge{
			ID: strings.TrimSpace(edge.ID),
			UI: cloneAnyMap(edge.UI),
		})
	}
	return out
}

func canonicalWorkflowHash(input any) (string, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
