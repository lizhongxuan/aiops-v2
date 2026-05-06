package visual

import (
	"fmt"
	"strings"

	"runner/workflow"

	"gopkg.in/yaml.v3"
)

const (
	legacyStepStartX       = 360.0
	legacyStepStartY       = 180.0
	legacyStepColumnGap    = 280.0
	legacyStepRowGap       = 160.0
	legacyStepsPerRow      = 5
	legacyHandlerLaneGap   = 180.0
	legacyHandlerStartX    = 360.0
	legacyHandlerColumnGap = 280.0
)

type ParseErrorKind string

const (
	ParseErrorKindYAMLSyntax         ParseErrorKind = "yaml_syntax"
	ParseErrorKindWorkflowValidation ParseErrorKind = "workflow_validation"
	ParseErrorKindGraphValidation    ParseErrorKind = "graph_validation"
)

type ParseError struct {
	Kind ParseErrorKind
	Err  error
}

func (e *ParseError) Error() string {
	if e == nil || e.Err == nil {
		return string(e.Kind)
	}
	return fmt.Sprintf("%s: %v", e.Kind, e.Err)
}

func (e *ParseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func ParseYAMLToGraph(rawYAML []byte) (Graph, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(rawYAML, &root); err != nil {
		return Graph{}, parseError(ParseErrorKindYAMLSyntax, err)
	}

	wf, err := workflow.Load(rawYAML)
	if err != nil {
		return Graph{}, parseError(ParseErrorKindYAMLSyntax, err)
	}
	if err := wf.Validate(); err != nil {
		return Graph{}, parseError(ParseErrorKindWorkflowValidation, err)
	}
	graphNode := findTopLevelMapValue(&root, "x_runner_graph")
	uiNode := findTopLevelMapValue(&root, "x_runner_ui")
	if graphNode == nil && uiNode == nil {
		g := graphFromWorkflow(wf, nil)
		return g, parseGraphValidationError(ValidateGraph(g))
	}

	var ui *xRunnerUI
	if uiNode != nil {
		var decoded xRunnerUI
		if err := uiNode.Decode(&decoded); err != nil {
			return Graph{}, parseError(ParseErrorKindGraphValidation, fmt.Errorf("decode x_runner_ui: %w", err))
		}
		ui = &decoded
	}
	if graphNode != nil {
		var graph xRunnerGraph
		if err := graphNode.Decode(&graph); err != nil {
			return Graph{}, parseError(ParseErrorKindGraphValidation, fmt.Errorf("decode x_runner_graph: %w", err))
		}
		g := graphFromWorkflowAndGraph(wf, graph, ui)
		return g, parseGraphValidationError(ValidateGraph(g))
	}
	g := graphFromWorkflowAndUI(wf, *ui)
	return g, parseGraphValidationError(ValidateGraph(g))
}

func ParseYAMLStringToGraph(rawYAML string) (Graph, error) {
	return ParseYAMLToGraph([]byte(rawYAML))
}

func parseError(kind ParseErrorKind, err error) error {
	if err == nil {
		return nil
	}
	return &ParseError{Kind: kind, Err: err}
}

func parseGraphValidationError(err error) error {
	return parseError(ParseErrorKindGraphValidation, err)
}

func graphFromWorkflow(wf workflow.Workflow, ui *xRunnerUI) Graph {
	g := Graph{
		Version:  GraphVersion,
		Workflow: wf,
		Layout: Layout{
			Direction: "LR",
			Viewport:  Viewport{Zoom: 1},
		},
		Nodes: []Node{{
			ID:       "start",
			Type:     NodeTypeStart,
			Position: Position{X: 80, Y: 180},
			Label:    "Start",
		}},
	}
	if ui != nil {
		g.Version = defaultString(ui.Version, GraphVersion)
		g.Layout = ui.Layout
		g.UI = cloneMap(ui.UI)
	}

	previous := "start"
	stepNodeIDs := map[string]string{}
	for i, step := range wf.Steps {
		stepCopy := step
		id := stableNodeID("step", step.Name, i)
		stepNodeIDs[step.Name] = id
		g.Nodes = append(g.Nodes, Node{
			ID:       id,
			Type:     nodeTypeForStep(step),
			Position: legacyStepPosition(i),
			StepName: step.Name,
			StepID:   firstNonEmpty(step.ID, id),
			Step:     &stepCopy,
			Label:    step.Name,
		})
		g.Edges = append(g.Edges, Edge{
			ID:     stableEdgeID(previous, id, i),
			Source: previous,
			Target: id,
			Kind:   EdgeKindNext,
		})
		previous = id
	}
	handlerNodeIDs := map[string]string{}
	handlerY := legacyHandlerLaneY(len(wf.Steps))
	for i, handler := range wf.Handlers {
		handlerCopy := handler
		id := stableNodeID("handler", handler.Name, i)
		handlerNodeIDs[handler.Name] = id
		g.Nodes = append(g.Nodes, Node{
			ID:          id,
			Type:        NodeTypeHandler,
			Position:    legacyHandlerPosition(i, handlerY),
			HandlerName: handler.Name,
			Handler:     &handlerCopy,
			Label:       handler.Name,
		})
	}
	addNotifyEdges(&g, wf.Steps, stepNodeIDs, handlerNodeIDs)
	ensureTerminalEnd(&g)
	moveGeneratedEndForLegacy(&g, len(wf.Steps))
	return g
}

func graphFromWorkflowAndGraph(wf workflow.Workflow, graph xRunnerGraph, ui *xRunnerUI) Graph {
	g := graphFromWorkflowAndUI(wf, xRunnerUI{
		Version: graph.Version,
		Nodes:   graph.Nodes,
		Edges:   graph.Edges,
		UI:      graph.UI,
	})
	if ui != nil {
		g.Layout = ui.Layout
		for id, position := range positionsByNodeID(ui.Nodes) {
			for i := range g.Nodes {
				if g.Nodes[i].ID == id {
					g.Nodes[i].Position = position
					break
				}
			}
		}
		for id, edgeUI := range edgeUIByID(ui.Edges) {
			for i := range g.Edges {
				if g.Edges[i].ID == id {
					g.Edges[i].UI = edgeUI
					break
				}
			}
		}
	}
	return g
}

func graphFromWorkflowAndUI(wf workflow.Workflow, ui xRunnerUI) Graph {
	stepByName := map[string]workflow.Step{}
	stepByID := map[string]workflow.Step{}
	for _, step := range wf.Steps {
		stepByName[step.Name] = step
		if strings.TrimSpace(step.ID) != "" {
			stepByID[step.ID] = step
		}
	}
	handlerByName := map[string]workflow.Handler{}
	for _, handler := range wf.Handlers {
		handlerByName[handler.Name] = handler
	}

	g := Graph{
		Version:  defaultString(ui.Version, GraphVersion),
		Workflow: wf,
		Layout:   ui.Layout,
		Nodes:    make([]Node, 0, len(ui.Nodes)),
		Edges:    make([]Edge, 0, len(ui.Edges)),
		UI:       cloneMap(ui.UI),
	}
	seenSteps := map[string]bool{}
	seenHandlers := map[string]bool{}
	for _, uiNode := range ui.Nodes {
		node := uiNode.toNode()
		if executableNodeType(node.Type) && node.StepName != "" {
			if step, ok := stepByName[node.StepName]; ok {
				stepCopy := step
				node.Step = &stepCopy
				node.StepName = step.Name
				if strings.TrimSpace(node.StepID) == "" {
					node.StepID = firstNonEmpty(step.ID, node.ID)
				}
				seenSteps[step.Name] = true
			}
		} else if executableNodeType(node.Type) && node.StepID != "" {
			if step, ok := stepByID[node.StepID]; ok {
				stepCopy := step
				node.Step = &stepCopy
				node.StepName = step.Name
				seenSteps[step.Name] = true
			}
		}
		if node.Type == NodeTypeHandler && node.HandlerName != "" {
			if handler, ok := handlerByName[node.HandlerName]; ok {
				handlerCopy := handler
				node.Handler = &handlerCopy
				seenHandlers[handler.Name] = true
			}
		}
		if node.Loop == nil && uiNode.Data.Loop != nil {
			node.Loop = cloneLoop(uiNode.Data.Loop)
		}
		g.Nodes = append(g.Nodes, node)
	}
	if !containsNodeType(g.Nodes, NodeTypeStart) {
		g.Nodes = append([]Node{{
			ID:       "start",
			Type:     NodeTypeStart,
			Position: Position{X: 80, Y: 180},
			Label:    "Start",
		}}, g.Nodes...)
	}
	for i, step := range wf.Steps {
		if seenSteps[step.Name] {
			continue
		}
		stepCopy := step
		id := uniqueNodeID(g.Nodes, stableNodeID("step", step.Name, i))
		g.Nodes = append(g.Nodes, Node{
			ID:       id,
			Type:     nodeTypeForStep(step),
			Position: legacyStepPosition(i),
			StepName: step.Name,
			StepID:   firstNonEmpty(step.ID, id),
			Step:     &stepCopy,
			Label:    step.Name,
		})
	}
	handlerY := legacyHandlerLaneY(len(wf.Steps))
	for i, handler := range wf.Handlers {
		if seenHandlers[handler.Name] {
			continue
		}
		handlerCopy := handler
		g.Nodes = append(g.Nodes, Node{
			ID:          uniqueNodeID(g.Nodes, stableNodeID("handler", handler.Name, i)),
			Type:        NodeTypeHandler,
			Position:    legacyHandlerPosition(i, handlerY),
			HandlerName: handler.Name,
			Handler:     &handlerCopy,
			Label:       handler.Name,
		})
	}
	for _, uiEdge := range ui.Edges {
		g.Edges = append(g.Edges, uiEdge.toEdge())
	}
	if len(g.Edges) == 0 {
		g.Edges = sequentialEdges(g.Nodes)
	}
	addNotifyEdges(&g, wf.Steps, stepNodeIDsByName(g.Nodes), handlerNodeIDsByName(g.Nodes))
	ensureTerminalEnd(&g)
	return g
}

func positionsByNodeID(nodes []xRunnerNode) map[string]Position {
	out := map[string]Position{}
	for _, node := range nodes {
		out[node.ID] = node.Position
	}
	return out
}

func edgeUIByID(edges []xRunnerEdge) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, edge := range edges {
		out[edge.ID] = cloneMap(edge.UI)
	}
	return out
}

func legacyStepPosition(index int) Position {
	if index < 0 {
		index = 0
	}
	row := index / legacyStepsPerRow
	col := index % legacyStepsPerRow
	return Position{
		X: legacyStepStartX + float64(col)*legacyStepColumnGap,
		Y: legacyStepStartY + float64(row)*legacyStepRowGap,
	}
}

func legacyHandlerLaneY(stepCount int) float64 {
	rows := 1
	if stepCount > 0 {
		rows = (stepCount + legacyStepsPerRow - 1) / legacyStepsPerRow
	}
	return legacyStepStartY + float64(rows)*legacyStepRowGap + legacyHandlerLaneGap
}

func legacyHandlerPosition(index int, y float64) Position {
	if index < 0 {
		index = 0
	}
	return Position{
		X: legacyHandlerStartX + float64(index)*legacyHandlerColumnGap,
		Y: y,
	}
}

func stepNodeIDsByName(nodes []Node) map[string]string {
	out := map[string]string{}
	for _, node := range nodes {
		if node.Step != nil && strings.TrimSpace(node.Step.Name) != "" {
			out[node.Step.Name] = node.ID
			continue
		}
		if strings.TrimSpace(node.StepName) != "" {
			out[node.StepName] = node.ID
		}
	}
	return out
}

func handlerNodeIDsByName(nodes []Node) map[string]string {
	out := map[string]string{}
	for _, node := range nodes {
		if node.Handler != nil && strings.TrimSpace(node.Handler.Name) != "" {
			out[node.Handler.Name] = node.ID
			continue
		}
		if strings.TrimSpace(node.HandlerName) != "" {
			out[node.HandlerName] = node.ID
		}
	}
	return out
}

func addNotifyEdges(g *Graph, steps []workflow.Step, stepNodeIDs, handlerNodeIDs map[string]string) {
	if g == nil {
		return
	}
	for _, step := range steps {
		source := strings.TrimSpace(stepNodeIDs[step.Name])
		if source == "" {
			continue
		}
		for _, notify := range step.Notify {
			target := strings.TrimSpace(handlerNodeIDs[strings.TrimSpace(notify)])
			if target == "" || graphHasEdge(*g, source, target) {
				continue
			}
			g.Edges = append(g.Edges, Edge{
				ID:     uniqueEdgeID(g.Edges, stableEdgeID(source, target, len(g.Edges))),
				Source: source,
				Target: target,
				Kind:   EdgeKindAlways,
			})
		}
	}
}

func graphHasEdge(g Graph, source, target string) bool {
	for _, edge := range g.Edges {
		if edge.Source == source && edge.Target == target {
			return true
		}
	}
	return false
}

func moveGeneratedEndForLegacy(g *Graph, stepCount int) {
	if g == nil || stepCount < 0 {
		return
	}
	position := legacyStepPosition(stepCount)
	for i := range g.Nodes {
		if g.Nodes[i].Type == NodeTypeEnd && g.Nodes[i].ID == "end" {
			g.Nodes[i].Position = position
			return
		}
	}
}

func findTopLevelMapValue(root *yaml.Node, key string) *yaml.Node {
	if root == nil {
		return nil
	}
	node := root
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func nodeTypeForStep(step workflow.Step) NodeType {
	switch strings.TrimSpace(step.Action) {
	case "manual.approval", "manual_approval":
		return NodeTypeManualApproval
	case "workflow.run", "subflow.run":
		return NodeTypeSubflow
	case "condition.evaluate":
		return NodeTypeCondition
	default:
		return NodeTypeAction
	}
}

func sequentialEdges(nodes []Node) []Edge {
	var edges []Edge
	previous := ""
	seq := 0
	for _, node := range nodes {
		if node.Type == NodeTypeStart {
			previous = node.ID
			break
		}
	}
	if previous == "" {
		return nil
	}
	for _, node := range nodes {
		if !executableNodeType(node.Type) {
			continue
		}
		edges = append(edges, Edge{
			ID:     stableEdgeID(previous, node.ID, seq),
			Source: previous,
			Target: node.ID,
			Kind:   EdgeKindNext,
		})
		previous = node.ID
		seq++
	}
	return edges
}

func containsNodeType(nodes []Node, t NodeType) bool {
	for _, node := range nodes {
		if node.Type == t {
			return true
		}
	}
	return false
}

func stableNodeID(prefix, name string, index int) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ":", "-", ".", "-")
	slug = replacer.Replace(slug)
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = fmt.Sprintf("%d", index+1)
	}
	return prefix + "-" + slug
}

func stableEdgeID(source, target string, index int) string {
	if source != "" && target != "" {
		return "edge-" + source + "-" + target
	}
	return fmt.Sprintf("edge-%d", index+1)
}

func ensureTerminalEnd(g *Graph) {
	if g == nil || len(g.Nodes) == 0 {
		return
	}
	nodeByID := map[string]Node{}
	endID := ""
	maxX := 80.0
	for _, node := range g.Nodes {
		nodeByID[node.ID] = node
		if node.Position.X > maxX {
			maxX = node.Position.X
		}
		if node.Type == NodeTypeEnd && endID == "" {
			endID = node.ID
		}
	}

	hasContinuationSource := false
	for _, node := range g.Nodes {
		if continuationRequiredNodeType(node.Type) {
			hasContinuationSource = true
			break
		}
	}
	if !hasContinuationSource {
		return
	}

	if endID == "" {
		endID = uniqueNodeID(g.Nodes, "end")
		g.Nodes = append(g.Nodes, Node{
			ID:       endID,
			Type:     NodeTypeEnd,
			Position: Position{X: maxX + 280, Y: 180},
			Label:    "End",
		})
		nodeByID[endID] = g.Nodes[len(g.Nodes)-1]
	}

	outgoingContinuations := map[string]int{}
	for _, edge := range g.Edges {
		target, ok := nodeByID[edge.Target]
		if !ok || target.Type == NodeTypeHandler || target.Type == NodeTypeGroup {
			continue
		}
		outgoingContinuations[edge.Source]++
	}
	for _, node := range g.Nodes {
		if !continuationRequiredNodeType(node.Type) || outgoingContinuations[node.ID] > 0 {
			continue
		}
		g.Edges = append(g.Edges, Edge{
			ID:     uniqueEdgeID(g.Edges, stableEdgeID(node.ID, endID, len(g.Edges))),
			Source: node.ID,
			Target: endID,
			Kind:   EdgeKindNext,
		})
		outgoingContinuations[node.ID]++
	}
}

func uniqueNodeID(nodes []Node, base string) string {
	seen := map[string]bool{}
	for _, node := range nodes {
		seen[node.ID] = true
	}
	if !seen[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !seen[candidate] {
			return candidate
		}
	}
}

func uniqueEdgeID(edges []Edge, base string) string {
	seen := map[string]bool{}
	for _, edge := range edges {
		seen[edge.ID] = true
	}
	if !seen[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !seen[candidate] {
			return candidate
		}
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
