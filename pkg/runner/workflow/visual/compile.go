package visual

import (
	"fmt"
	"strings"

	"runner/workflow"

	"gopkg.in/yaml.v3"
)

func CompileGraphToYAML(g Graph) ([]byte, error) {
	if g.Version == "" {
		g.Version = GraphVersion
	}
	idx, err := validateGraph(g)
	if err != nil {
		return nil, err
	}

	wf := g.Workflow
	wf.Steps = compileSteps(g, idx)
	wf.Handlers = compileHandlers(g)
	wf.XRunnerUI = workflowUISpecFromGraph(g)
	wf.XRunnerGraph = workflowGraphSpecFromGraph(g)
	if err := wf.Validate(); err != nil {
		return nil, fmt.Errorf("compiled workflow is invalid: %w", err)
	}

	doc := workflowYAMLDocument{
		Version:       wf.Version,
		Name:          wf.Name,
		Description:   wf.Description,
		EnvPackages:   wf.EnvPackages,
		ValidationEnv: wf.ValidationEnv,
		XRunnerUI:     wf.XRunnerUI,
		XRunnerGraph:  wf.XRunnerGraph,
		Inventory:     wf.Inventory,
		Vars:          wf.Vars,
		Plan:          wf.Plan,
		Steps:         wf.Steps,
		Handlers:      wf.Handlers,
		Tests:         wf.Tests,
		Extensions:    workflowDocumentExtensions(wf.Extensions),
	}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	if _, err := workflow.Load(raw); err != nil {
		return nil, fmt.Errorf("compiled workflow cannot be loaded: %w", err)
	}
	return raw, nil
}

func CompileGraphToYAMLString(g Graph) (string, error) {
	raw, err := CompileGraphToYAML(g)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func compileSteps(g Graph, idx graphIndex) []workflow.Step {
	var steps []workflow.Step
	conditionByTarget := map[string]string{}
	notifyBySource := map[string][]string{}
	handlerNames := map[string]string{}
	for _, node := range g.Nodes {
		if node.Type == NodeTypeHandler {
			handlerNames[node.ID] = nodeHandlerName(node)
		}
	}
	for _, edge := range g.Edges {
		if condition := edgeConditionExpression(edge, idx.nodes[edge.Source]); condition != "" {
			conditionByTarget[edge.Target] = condition
		}
		if handlerName := strings.TrimSpace(handlerNames[edge.Target]); handlerName != "" {
			notifyBySource[edge.Source] = append(notifyBySource[edge.Source], handlerName)
		}
	}
	for _, id := range topologicalNodeIDs(idx) {
		node := idx.nodes[id]
		if !stepBackedNodeType(node.Type) {
			continue
		}
		step := nodeStep(node)
		step.Name = strings.TrimSpace(step.Name)
		if condition := conditionByTarget[node.ID]; condition != "" && strings.TrimSpace(step.When) == "" {
			step.When = condition
		}
		if notify := notifyBySource[node.ID]; len(notify) > 0 {
			step.Notify = appendUniqueStrings(step.Notify, notify...)
		}
		steps = append(steps, step)
	}
	return steps
}

func compileHandlers(g Graph) []workflow.Handler {
	handlers := make([]workflow.Handler, 0, len(g.Workflow.Handlers))
	for _, handler := range g.Workflow.Handlers {
		handlers = append(handlers, handler)
	}
	seen := map[string]int{}
	for i, handler := range handlers {
		seen[handler.Name] = i
	}
	for _, node := range g.Nodes {
		if node.Type != NodeTypeHandler {
			continue
		}
		handler := nodeHandler(node)
		if idx, exists := seen[handler.Name]; exists {
			handlers[idx] = handler
			continue
		}
		seen[handler.Name] = len(handlers)
		handlers = append(handlers, handler)
	}
	return handlers
}

type workflowYAMLDocument struct {
	Version       string                `yaml:"version,omitempty"`
	Name          string                `yaml:"name,omitempty"`
	Description   string                `yaml:"description,omitempty"`
	EnvPackages   []string              `yaml:"env_packages,omitempty"`
	ValidationEnv string                `yaml:"validation_env,omitempty"`
	XRunnerUI     *workflow.GraphUISpec `yaml:"x_runner_ui,omitempty"`
	XRunnerGraph  *workflow.GraphSpec   `yaml:"x_runner_graph,omitempty"`
	Inventory     workflow.Inventory    `yaml:"inventory,omitempty"`
	Vars          map[string]any        `yaml:"vars,omitempty"`
	Plan          workflow.Plan         `yaml:"plan,omitempty"`
	Steps         []workflow.Step       `yaml:"steps,omitempty"`
	Handlers      []workflow.Handler    `yaml:"handlers,omitempty"`
	Tests         []workflow.Test       `yaml:"tests,omitempty"`
	Extensions    map[string]any        `yaml:",inline"`
}

type xRunnerGraph struct {
	Version string         `json:"version" yaml:"version"`
	Nodes   []xRunnerNode  `json:"nodes,omitempty" yaml:"nodes,omitempty"`
	Edges   []xRunnerEdge  `json:"edges,omitempty" yaml:"edges,omitempty"`
	UI      map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type xRunnerUI struct {
	Version string         `json:"version" yaml:"version"`
	Layout  Layout         `json:"layout,omitempty" yaml:"layout,omitempty"`
	Nodes   []xRunnerNode  `json:"nodes,omitempty" yaml:"nodes,omitempty"`
	Edges   []xRunnerEdge  `json:"edges,omitempty" yaml:"edges,omitempty"`
	UI      map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type xRunnerNode struct {
	ID          string             `json:"id" yaml:"id"`
	Type        NodeType           `json:"type" yaml:"type"`
	Position    Position           `json:"position" yaml:"position"`
	Step        string             `json:"step,omitempty" yaml:"step,omitempty"`
	StepName    string             `json:"step_name,omitempty" yaml:"step_name,omitempty"`
	StepID      string             `json:"step_id,omitempty" yaml:"step_id,omitempty"`
	Handler     string             `json:"handler,omitempty" yaml:"handler,omitempty"`
	HandlerName string             `json:"handler_name,omitempty" yaml:"handler_name,omitempty"`
	ParentID    string             `json:"parent_id,omitempty" yaml:"parent_id,omitempty"`
	Label       string             `json:"label,omitempty" yaml:"label,omitempty"`
	Collapsed   bool               `json:"collapsed,omitempty" yaml:"collapsed,omitempty"`
	Ports       []Port             `json:"ports,omitempty" yaml:"ports,omitempty"`
	Inputs      *[]InputParamSpec  `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs     *[]OutputParamSpec `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	UI          map[string]any     `json:"ui,omitempty" yaml:"ui,omitempty"`
	Data        xRunnerData        `json:"data,omitempty" yaml:"data,omitempty"`
}

type xRunnerData struct {
	StepName    string             `json:"stepName,omitempty" yaml:"stepName,omitempty"`
	HandlerName string             `json:"handlerName,omitempty" yaml:"handlerName,omitempty"`
	Label       string             `json:"label,omitempty" yaml:"label,omitempty"`
	Collapsed   bool               `json:"collapsed,omitempty" yaml:"collapsed,omitempty"`
	Approval    *ApprovalSpec      `json:"approval,omitempty" yaml:"approval,omitempty"`
	Condition   *ConditionSpec     `json:"condition,omitempty" yaml:"condition,omitempty"`
	Subflow     *SubflowSpec       `json:"subflow,omitempty" yaml:"subflow,omitempty"`
	Join        *JoinSpec          `json:"join,omitempty" yaml:"join,omitempty"`
	Loop        *LoopSpec          `json:"loop,omitempty" yaml:"loop,omitempty"`
	Inputs      *[]InputParamSpec  `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs     *[]OutputParamSpec `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	UI          map[string]any     `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type xRunnerEdge struct {
	ID         string         `json:"id" yaml:"id"`
	Source     string         `json:"source" yaml:"source"`
	SourcePort string         `json:"source_port,omitempty" yaml:"source_port,omitempty"`
	Target     string         `json:"target" yaml:"target"`
	TargetPort string         `json:"target_port,omitempty" yaml:"target_port,omitempty"`
	Kind       EdgeKind       `json:"kind,omitempty" yaml:"kind,omitempty"`
	Condition  string         `json:"condition,omitempty" yaml:"condition,omitempty"`
	UI         map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

func xRunnerUIFromGraph(g Graph) xRunnerUI {
	ui := xRunnerUI{
		Version: defaultString(g.Version, GraphVersion),
		Layout:  g.Layout,
		Nodes:   make([]xRunnerNode, 0, len(g.Nodes)),
		Edges:   make([]xRunnerEdge, 0, len(g.Edges)),
		UI:      cloneMap(g.UI),
	}
	for _, node := range g.Nodes {
		ui.Nodes = append(ui.Nodes, xRunnerNode{
			ID:        node.ID,
			Type:      node.Type,
			Position:  node.Position,
			Step:      nodeStepName(node),
			StepID:    nodeStepID(node),
			Handler:   nodeHandlerName(node),
			ParentID:  node.ParentID,
			Label:     node.Label,
			Collapsed: node.Collapsed,
			Ports:     clonePorts(node.Ports),
			Inputs:    inputsPtr(node.Inputs),
			Outputs:   outputsPtr(node.Outputs),
			UI:        cloneMap(node.UI),
		})
	}
	for _, edge := range g.Edges {
		kind := edge.Kind
		if kind == "" {
			kind = EdgeKindNext
		}
		ui.Edges = append(ui.Edges, xRunnerEdge{
			ID:         edge.ID,
			Source:     edge.Source,
			SourcePort: edge.SourcePort,
			Target:     edge.Target,
			TargetPort: edge.TargetPort,
			Kind:       kind,
			Condition:  edge.Condition,
			UI:         cloneMap(edge.UI),
		})
	}
	return ui
}

func xRunnerGraphFromGraph(g Graph) xRunnerGraph {
	graph := xRunnerGraph{
		Version: defaultString(g.Version, GraphVersion),
		Nodes:   make([]xRunnerNode, 0, len(g.Nodes)),
		Edges:   make([]xRunnerEdge, 0, len(g.Edges)),
		UI:      cloneMap(g.UI),
	}
	for _, node := range g.Nodes {
		graph.Nodes = append(graph.Nodes, xRunnerNode{
			ID:        node.ID,
			Type:      node.Type,
			Step:      nodeStepName(node),
			StepID:    nodeStepID(node),
			Handler:   nodeHandlerName(node),
			ParentID:  node.ParentID,
			Label:     node.Label,
			Collapsed: node.Collapsed,
			Ports:     clonePorts(node.Ports),
			Inputs:    inputsPtr(node.Inputs),
			Outputs:   outputsPtr(node.Outputs),
			UI:        cloneMap(node.UI),
			Data: xRunnerData{
				Approval:  cloneApproval(node.Approval),
				Condition: cloneCondition(node.Condition),
				Subflow:   cloneSubflow(node.Subflow),
				Join:      cloneJoin(node.Join),
				Loop:      cloneLoop(node.Loop),
				Inputs:    inputsPtr(node.Inputs),
				Outputs:   outputsPtr(node.Outputs),
			},
		})
	}
	for _, edge := range g.Edges {
		kind := edge.Kind
		if kind == "" {
			kind = EdgeKindNext
		}
		graph.Edges = append(graph.Edges, xRunnerEdge{
			ID:         edge.ID,
			Source:     edge.Source,
			SourcePort: edge.SourcePort,
			Target:     edge.Target,
			TargetPort: edge.TargetPort,
			Kind:       kind,
			Condition:  edge.Condition,
			UI:         cloneMap(edge.UI),
		})
	}
	return graph
}

func workflowUISpecFromGraph(g Graph) *workflow.GraphUISpec {
	return &workflow.GraphUISpec{
		Version: defaultString(g.Version, GraphVersion),
		Layout: workflow.GraphLayoutSpec{
			Direction: g.Layout.Direction,
			Viewport: workflow.GraphViewport{
				X:    g.Layout.Viewport.X,
				Y:    g.Layout.Viewport.Y,
				Zoom: g.Layout.Viewport.Zoom,
			},
			UI: cloneMap(g.Layout.UI),
		},
		Nodes: workflowGraphNodesFromGraph(g),
		Edges: workflowGraphEdgesFromGraph(g),
		UI:    cloneMap(g.UI),
	}
}

func workflowGraphSpecFromGraph(g Graph) *workflow.GraphSpec {
	return &workflow.GraphSpec{
		Version: defaultString(g.Version, GraphVersion),
		Nodes:   workflowGraphNodesFromGraph(g),
		Edges:   workflowGraphEdgesFromGraph(g),
		UI:      cloneMap(g.UI),
	}
}

func workflowGraphNodesFromGraph(g Graph) []workflow.GraphNodeSpec {
	nodes := make([]workflow.GraphNodeSpec, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		nodes = append(nodes, workflow.GraphNodeSpec{
			ID:          node.ID,
			Type:        string(node.Type),
			Position:    workflow.GraphPosition{X: node.Position.X, Y: node.Position.Y},
			Step:        nodeStepName(node),
			StepName:    node.StepName,
			StepID:      nodeStepID(node),
			Handler:     nodeHandlerName(node),
			HandlerName: node.HandlerName,
			ParentID:    node.ParentID,
			Label:       node.Label,
			Collapsed:   node.Collapsed,
			Ports:       workflowPorts(node.Ports),
			Inputs:      workflowInputs(node.Inputs),
			Outputs:     workflowOutputs(node.Outputs),
			Data: workflow.GraphNodeDataSpec{
				StepName:    node.StepName,
				HandlerName: node.HandlerName,
				Label:       node.Label,
				Collapsed:   node.Collapsed,
				Approval:    workflowApproval(node.Approval),
				Condition:   workflowCondition(node.Condition),
				Subflow:     workflowSubflow(node.Subflow),
				Join:        workflowJoin(node.Join),
				Loop:        workflowLoop(node.Loop),
				Inputs:      workflowInputs(node.Inputs),
				Outputs:     workflowOutputs(node.Outputs),
				UI:          cloneMap(node.UI),
			},
			UI: cloneMap(node.UI),
		})
	}
	return nodes
}

func workflowGraphEdgesFromGraph(g Graph) []workflow.GraphEdgeSpec {
	edges := make([]workflow.GraphEdgeSpec, 0, len(g.Edges))
	for _, edge := range g.Edges {
		kind := edge.Kind
		if kind == "" {
			kind = EdgeKindNext
		}
		edges = append(edges, workflow.GraphEdgeSpec{
			ID:         edge.ID,
			Source:     edge.Source,
			SourcePort: edge.SourcePort,
			Target:     edge.Target,
			TargetPort: edge.TargetPort,
			Kind:       string(kind),
			Condition:  edge.Condition,
			UI:         cloneMap(edge.UI),
		})
	}
	return edges
}

func workflowPorts(input []Port) []workflow.GraphPortSpec {
	if len(input) == 0 {
		return nil
	}
	out := make([]workflow.GraphPortSpec, len(input))
	for i, port := range input {
		out[i] = workflow.GraphPortSpec{
			ID:       port.ID,
			Type:     port.Type,
			Label:    port.Label,
			Required: port.Required,
			UI:       cloneMap(port.UI),
		}
	}
	return out
}

func workflowInputs(input []InputParamSpec) []workflow.GraphInputSpec {
	if len(input) == 0 {
		return nil
	}
	out := make([]workflow.GraphInputSpec, len(input))
	for i, item := range input {
		out[i] = workflow.GraphInputSpec{
			Key:         item.Key,
			Type:        item.Type,
			Label:       item.Label,
			Description: item.Description,
			Required:    item.Required,
			Default:     item.Default,
			ValueSource: workflowValueSource(item.ValueSource),
			UI:          cloneMap(item.UI),
		}
	}
	return out
}

func workflowOutputs(input []OutputParamSpec) []workflow.GraphOutputSpec {
	if len(input) == 0 {
		return nil
	}
	out := make([]workflow.GraphOutputSpec, len(input))
	for i, item := range input {
		out[i] = workflow.GraphOutputSpec{
			Key:           item.Key,
			Type:          item.Type,
			Label:         item.Label,
			Description:   item.Description,
			Required:      item.Required,
			ExtractSource: workflowExtractSource(item.ExtractSource),
			UI:            cloneMap(item.UI),
		}
	}
	return out
}

func workflowValueSource(input ValueSource) workflow.GraphValueSourceSpec {
	return workflow.GraphValueSourceSpec{
		Type:       input.Type,
		Value:      input.Value,
		Variable:   workflowVariableRef(input.Variable),
		Expression: input.Expression,
		SecretRef:  input.SecretRef,
		EnvKey:     input.EnvKey,
	}
}

func workflowExtractSource(input ExtractSource) workflow.GraphExtractSourceSpec {
	return workflow.GraphExtractSourceSpec{
		Type:       input.Type,
		Path:       input.Path,
		Expression: input.Expression,
		Value:      input.Value,
	}
}

func workflowVariableRef(input *VariableRef) *workflow.GraphVariableRefSpec {
	if input == nil {
		return nil
	}
	return &workflow.GraphVariableRefSpec{
		Scope:  input.Scope,
		NodeID: input.NodeID,
		Name:   input.Name,
		Path:   input.Path,
	}
}

func workflowApproval(input *ApprovalSpec) *workflow.GraphApprovalSpec {
	if input == nil {
		return nil
	}
	return &workflow.GraphApprovalSpec{
		Subjects:  append([]string(nil), input.Subjects...),
		Timeout:   input.Timeout,
		OnTimeout: input.OnTimeout,
		UI:        cloneMap(input.UI),
	}
}

func workflowCondition(input *ConditionSpec) *workflow.GraphConditionSpec {
	if input == nil {
		return nil
	}
	return &workflow.GraphConditionSpec{
		If:   input.If,
		Elif: workflowConditionBranches(input.Elif),
		Else: input.Else,
		UI:   cloneMap(input.UI),
	}
}

func workflowConditionBranches(input []ConditionBranchSpec) []workflow.GraphConditionBranchSpec {
	if len(input) == 0 {
		return nil
	}
	out := make([]workflow.GraphConditionBranchSpec, len(input))
	for i, item := range input {
		out[i] = workflow.GraphConditionBranchSpec{
			Expression: item.Expression,
			UI:         cloneMap(item.UI),
		}
	}
	return out
}

func workflowSubflow(input *SubflowSpec) *workflow.GraphSubflowSpec {
	if input == nil {
		return nil
	}
	return &workflow.GraphSubflowSpec{
		WorkflowName: input.WorkflowName,
		Vars:         cloneMap(input.Vars),
		UI:           cloneMap(input.UI),
	}
}

func workflowJoin(input *JoinSpec) *workflow.GraphJoinSpec {
	if input == nil {
		return nil
	}
	return &workflow.GraphJoinSpec{
		Strategy:         input.Strategy,
		FailureThreshold: input.FailureThreshold,
		UI:               cloneMap(input.UI),
	}
}

func workflowLoop(input *LoopSpec) *workflow.GraphLoopSpec {
	if input == nil {
		return nil
	}
	return &workflow.GraphLoopSpec{
		Mode:           input.Mode,
		Items:          append([]any(nil), input.Items...),
		ItemsVariable:  input.ItemsVariable,
		WhileCondition: input.WhileCondition,
		MaxIterations:  input.MaxIterations,
		ItemVar:        input.ItemVar,
		IndexVar:       input.IndexVar,
		UI:             cloneMap(input.UI),
	}
}

func workflowDocumentExtensions(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		if key == "" || knownWorkflowYAMLField(key) {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func knownWorkflowYAMLField(key string) bool {
	switch key {
	case "version", "name", "description", "env_packages", "validation_env", "x_runner_ui", "x_runner_graph", "inventory", "vars", "plan", "steps", "handlers", "tests":
		return true
	default:
		return false
	}
}

func (n xRunnerNode) toNode() Node {
	stepName := firstNonEmpty(n.Step, n.StepName, n.Data.StepName)
	handlerName := firstNonEmpty(n.Handler, n.HandlerName, n.Data.HandlerName)
	label := firstNonEmpty(n.Label, n.Data.Label, stepName, handlerName)
	ui := cloneMap(n.UI)
	if len(ui) == 0 {
		ui = cloneMap(n.Data.UI)
	}
	return Node{
		ID:          n.ID,
		Type:        n.Type,
		Position:    n.Position,
		StepName:    stepName,
		StepID:      n.StepID,
		HandlerName: handlerName,
		ParentID:    n.ParentID,
		Label:       label,
		Collapsed:   n.Collapsed || n.Data.Collapsed,
		Ports:       clonePorts(n.Ports),
		Inputs:      firstInputs(n.Inputs, n.Data.Inputs),
		Outputs:     firstOutputs(n.Outputs, n.Data.Outputs),
		Approval:    cloneApproval(n.Data.Approval),
		Condition:   cloneCondition(n.Data.Condition),
		Subflow:     cloneSubflow(n.Data.Subflow),
		Join:        cloneJoin(n.Data.Join),
		Loop:        cloneLoop(n.Data.Loop),
		UI:          ui,
	}
}

func (e xRunnerEdge) toEdge() Edge {
	return Edge{
		ID:         e.ID,
		Source:     e.Source,
		SourcePort: e.SourcePort,
		Target:     e.Target,
		TargetPort: e.TargetPort,
		Kind:       e.Kind,
		Condition:  e.Condition,
		UI:         cloneMap(e.UI),
	}
}

func edgeConditionExpression(edge Edge, source Node) string {
	if condition := strings.TrimSpace(edge.Condition); condition != "" {
		return condition
	}
	switch edge.Kind {
	case EdgeKindCondition, EdgeKindIf:
		return nodeConditionExpression(source)
	case EdgeKindElse:
		if expression := nodeConditionExpression(source); expression != "" {
			return "!(" + expression + ")"
		}
	}
	return ""
}

func nodeConditionExpression(node Node) string {
	if node.Condition != nil && strings.TrimSpace(node.Condition.If) != "" {
		return strings.TrimSpace(node.Condition.If)
	}
	step := nodeStep(node)
	if expression, ok := step.Args["expression"].(string); ok && strings.TrimSpace(expression) != "" {
		return strings.TrimSpace(expression)
	}
	return strings.TrimSpace(step.When)
}

func nodeStepName(node Node) string {
	if node.Step != nil && strings.TrimSpace(node.Step.Name) != "" {
		return node.Step.Name
	}
	return node.StepName
}

func nodeStepID(node Node) string {
	if node.Step != nil && strings.TrimSpace(node.Step.ID) != "" {
		return node.Step.ID
	}
	return node.StepID
}

func nodeHandlerName(node Node) string {
	if node.Handler != nil && strings.TrimSpace(node.Handler.Name) != "" {
		return node.Handler.Name
	}
	return node.HandlerName
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func appendUniqueStrings(base []string, values ...string) []string {
	seen := map[string]struct{}{}
	for _, item := range base {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		seen[item] = struct{}{}
	}
	for _, item := range values {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		base = append(base, item)
		seen[item] = struct{}{}
	}
	return base
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func clonePorts(input []Port) []Port {
	if len(input) == 0 {
		return nil
	}
	out := make([]Port, len(input))
	for i, port := range input {
		out[i] = port
		out[i].UI = cloneMap(port.UI)
	}
	return out
}

func cloneInputs(input []InputParamSpec) []InputParamSpec {
	if len(input) == 0 {
		return nil
	}
	out := make([]InputParamSpec, len(input))
	for i, item := range input {
		out[i] = item
		out[i].ValueSource = cloneValueSource(item.ValueSource)
		out[i].UI = cloneMap(item.UI)
	}
	return out
}

func cloneOutputs(input []OutputParamSpec) []OutputParamSpec {
	if len(input) == 0 {
		return nil
	}
	out := make([]OutputParamSpec, len(input))
	for i, item := range input {
		out[i] = item
		out[i].UI = cloneMap(item.UI)
	}
	return out
}

func cloneValueSource(input ValueSource) ValueSource {
	out := input
	if input.Variable != nil {
		variable := *input.Variable
		out.Variable = &variable
	}
	return out
}

func inputsPtr(input []InputParamSpec) *[]InputParamSpec {
	if len(input) == 0 {
		return nil
	}
	out := cloneInputs(input)
	return &out
}

func outputsPtr(input []OutputParamSpec) *[]OutputParamSpec {
	if len(input) == 0 {
		return nil
	}
	out := cloneOutputs(input)
	return &out
}

func firstInputs(primary, fallback *[]InputParamSpec) []InputParamSpec {
	if primary != nil {
		return cloneInputs(*primary)
	}
	if fallback != nil {
		return cloneInputs(*fallback)
	}
	return nil
}

func firstOutputs(primary, fallback *[]OutputParamSpec) []OutputParamSpec {
	if primary != nil {
		return cloneOutputs(*primary)
	}
	if fallback != nil {
		return cloneOutputs(*fallback)
	}
	return nil
}

func cloneApproval(input *ApprovalSpec) *ApprovalSpec {
	if input == nil {
		return nil
	}
	out := *input
	out.Subjects = append([]string(nil), input.Subjects...)
	out.UI = cloneMap(input.UI)
	return &out
}

func cloneCondition(input *ConditionSpec) *ConditionSpec {
	if input == nil {
		return nil
	}
	out := *input
	out.Elif = make([]ConditionBranchSpec, len(input.Elif))
	for i, item := range input.Elif {
		out.Elif[i] = item
		out.Elif[i].UI = cloneMap(item.UI)
	}
	out.UI = cloneMap(input.UI)
	return &out
}

func cloneSubflow(input *SubflowSpec) *SubflowSpec {
	if input == nil {
		return nil
	}
	out := *input
	out.Vars = cloneMap(input.Vars)
	out.UI = cloneMap(input.UI)
	return &out
}

func cloneJoin(input *JoinSpec) *JoinSpec {
	if input == nil {
		return nil
	}
	out := *input
	out.UI = cloneMap(input.UI)
	return &out
}

func cloneLoop(input *LoopSpec) *LoopSpec {
	if input == nil {
		return nil
	}
	out := *input
	out.Items = append([]any(nil), input.Items...)
	out.UI = cloneMap(input.UI)
	return &out
}
