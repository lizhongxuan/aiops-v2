package edit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"runner/workflow"
	"runner/workflow/visual"
)

type WorkflowPlanCatalog struct {
	Version string                      `json:"version"`
	Actions []WorkflowPlanCatalogAction `json:"actions"`
}

type WorkflowPlanCatalogAction struct {
	ID            string                    `json:"id"`
	Version       string                    `json:"version"`
	NodeType      visual.NodeType           `json:"nodeType"`
	ArgumentsJSON json.RawMessage           `json:"argumentsSchema"`
	InputPorts    []WorkflowPlanCatalogPort `json:"inputPorts,omitempty"`
	OutputPorts   []WorkflowPlanCatalogPort `json:"outputPorts,omitempty"`
}

type WorkflowPlanCatalogPort struct {
	ID       string `json:"id"`
	Required bool   `json:"required,omitempty"`
}

type WorkflowPlanCompileOptions struct {
	EnvironmentVersions map[string]string `json:"environmentVersions"`
	PositionSpacing     float64           `json:"positionSpacing,omitempty"`
}

type WorkflowPlanCompileResult struct {
	PlanHash    string                   `json:"planHash"`
	Operations  []WorkflowOperation      `json:"operations"`
	Diagnostics []WorkflowPlanDiagnostic `json:"diagnostics"`
}

func CompileWorkflowPlan(plan WorkflowPlanIR, catalog WorkflowPlanCatalog, options WorkflowPlanCompileOptions) (WorkflowPlanCompileResult, error) {
	if err := plan.Validate(); err != nil {
		return WorkflowPlanCompileResult{}, fmt.Errorf("invalid WorkflowPlanIR: %w", err)
	}
	if strings.TrimSpace(catalog.Version) == "" {
		return WorkflowPlanCompileResult{}, fmt.Errorf("catalog version is required")
	}
	result := WorkflowPlanCompileResult{PlanHash: workflowPlanCompileHash(plan, catalog, options)}
	actions, diagnostics := validateWorkflowPlanCatalog(plan, catalog, options)
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	if hasWorkflowPlanErrors(result.Diagnostics) {
		return result, nil
	}
	spacing := options.PositionSpacing
	if spacing <= 0 {
		spacing = 160
	}
	for index, item := range plan.Items {
		node, compileDiagnostics := compileWorkflowPlanNode(item, actions[item.ClientRef], float64(index)*spacing)
		result.Diagnostics = append(result.Diagnostics, compileDiagnostics...)
		if len(compileDiagnostics) > 0 {
			continue
		}
		result.Operations = append(result.Operations, NewOperation(&AddNodeOperation{ClientRef: item.ClientRef, Node: node}))
	}
	result.Operations = append(result.Operations, compileWorkflowPlanEdges(plan)...)
	sortWorkflowPlanDiagnostics(result.Diagnostics)
	if hasWorkflowPlanErrors(result.Diagnostics) {
		result.Operations = nil
	}
	return result, nil
}

func workflowPlanCompileHash(plan WorkflowPlanIR, catalog WorkflowPlanCatalog, options WorkflowPlanCompileOptions) string {
	payload, _ := json.Marshal(struct {
		Plan    WorkflowPlanIR             `json:"plan"`
		Catalog WorkflowPlanCatalog        `json:"catalog"`
		Options WorkflowPlanCompileOptions `json:"options"`
	}{plan, catalog, options})
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validateWorkflowPlanCatalog(plan WorkflowPlanIR, catalog WorkflowPlanCatalog, options WorkflowPlanCompileOptions) (map[string]WorkflowPlanCatalogAction, []WorkflowPlanDiagnostic) {
	byID := map[string]WorkflowPlanCatalogAction{}
	for _, action := range catalog.Actions {
		if action.ID == "" || action.Version == "" || len(action.ArgumentsJSON) == 0 || !json.Valid(action.ArgumentsJSON) {
			continue
		}
		byID[action.ID+"\x00"+action.Version] = action
	}
	resolved := map[string]WorkflowPlanCatalogAction{}
	var diagnostics []WorkflowPlanDiagnostic
	for _, item := range plan.Items {
		if item.Action == nil {
			continue
		}
		key := item.Action.CatalogID + "\x00" + item.Action.CatalogVersion
		action, found := byID[key]
		if !found {
			diagnostics = append(diagnostics, workflowPlanCompileDiagnostic(PlanDiagnosticUnknownAction, item.ClientRef, "/action/catalogId", "Action Catalog entry or version is unavailable"))
			continue
		}
		if action.NodeType != visual.NodeTypeAction {
			diagnostics = append(diagnostics, workflowPlanCompileDiagnostic(PlanDiagnosticUnknownAction, item.ClientRef, "/action/catalogId", "Catalog entry is not an executable action node"))
			continue
		}
		resolved[item.ClientRef] = action
		diagnostics = append(diagnostics, validateWorkflowPlanArguments(item, action)...)
		diagnostics = append(diagnostics, validateWorkflowPlanPorts(item, action)...)
	}
	for _, requirement := range plan.Environment {
		if options.EnvironmentVersions == nil || options.EnvironmentVersions[requirement.Name] != requirement.Version {
			diagnostics = append(diagnostics, workflowPlanCompileDiagnostic(PlanDiagnosticMissingEnvironment, "", "/environment", "Required execution environment version is unavailable"))
		}
	}
	for index, unresolved := range plan.Unresolved {
		code := PlanDiagnosticUnresolved
		if unresolved.Requirement == "secret_ref" {
			code = PlanDiagnosticMissingSecret
		} else if unresolved.Requirement == "environment" {
			code = PlanDiagnosticMissingEnvironment
		}
		diagnostic := workflowPlanCompileDiagnostic(code, unresolved.ClientRef, fmt.Sprintf("/unresolved/%d", index), unresolved.Reason)
		diagnostic.Requirement = &unresolved
		diagnostics = append(diagnostics, diagnostic)
	}
	sortWorkflowPlanDiagnostics(diagnostics)
	return resolved, diagnostics
}

type workflowPlanArgumentSchema struct {
	Type                 string                               `json:"type"`
	Required             []string                             `json:"required"`
	Properties           map[string]workflowPlanArgumentField `json:"properties"`
	AdditionalProperties *bool                                `json:"additionalProperties"`
}

type workflowPlanArgumentField struct {
	Type string `json:"type"`
}

func validateWorkflowPlanArguments(item WorkflowPlanItem, action WorkflowPlanCatalogAction) []WorkflowPlanDiagnostic {
	var schema workflowPlanArgumentSchema
	if err := json.Unmarshal(action.ArgumentsJSON, &schema); err != nil || schema.Type != "object" {
		return []WorkflowPlanDiagnostic{workflowPlanCompileDiagnostic(PlanDiagnosticArgumentType, item.ClientRef, "/action/arguments", "Catalog argument schema is invalid")}
	}
	var diagnostics []WorkflowPlanDiagnostic
	for _, required := range schema.Required {
		if _, found := item.Action.Arguments[required]; !found {
			diagnostics = append(diagnostics, workflowPlanCompileDiagnostic(PlanDiagnosticArgumentType, item.ClientRef, "/action/arguments/"+required, "Required action argument is missing"))
		}
	}
	for name, value := range item.Action.Arguments {
		field, found := schema.Properties[name]
		if !found {
			if schema.AdditionalProperties != nil && !*schema.AdditionalProperties {
				diagnostics = append(diagnostics, workflowPlanCompileDiagnostic(PlanDiagnosticArgumentType, item.ClientRef, "/action/arguments/"+name, "Action argument is not declared by the Catalog schema"))
			}
			continue
		}
		if !workflowPlanJSONTypeMatches(field.Type, value) {
			diagnostics = append(diagnostics, workflowPlanCompileDiagnostic(PlanDiagnosticArgumentType, item.ClientRef, "/action/arguments/"+name, "Action argument type does not match the Catalog schema"))
		}
	}
	return diagnostics
}

func workflowPlanJSONTypeMatches(want string, value any) bool {
	switch want {
	case "":
		return true
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		kind := reflect.TypeOf(value)
		return kind != nil && (kind.Kind() >= reflect.Int && kind.Kind() <= reflect.Float64)
	case "integer":
		kind := reflect.TypeOf(value)
		return kind != nil && kind.Kind() >= reflect.Int && kind.Kind() <= reflect.Uint64
	case "array":
		kind := reflect.TypeOf(value)
		return kind != nil && (kind.Kind() == reflect.Array || kind.Kind() == reflect.Slice)
	case "object":
		kind := reflect.TypeOf(value)
		return kind != nil && kind.Kind() == reflect.Map
	case "null":
		return value == nil
	default:
		return false
	}
}

func validateWorkflowPlanPorts(item WorkflowPlanItem, action WorkflowPlanCatalogAction) []WorkflowPlanDiagnostic {
	knownInputs, knownOutputs := workflowPlanCatalogPortIDs(action.InputPorts), workflowPlanCatalogPortIDs(action.OutputPorts)
	var diagnostics []WorkflowPlanDiagnostic
	boundInputs, boundOutputs := map[string]bool{}, map[string]bool{}
	for _, binding := range item.Action.InputPorts {
		boundInputs[binding.CatalogPort] = true
		if !knownInputs[binding.CatalogPort] {
			diagnostics = append(diagnostics, workflowPlanCompileDiagnostic(PlanDiagnosticPortMismatch, item.ClientRef, "/action/inputPorts", "Input port is not declared by the Catalog action"))
		}
	}
	for _, binding := range item.Action.OutputPorts {
		boundOutputs[binding.CatalogPort] = true
		if !knownOutputs[binding.CatalogPort] {
			diagnostics = append(diagnostics, workflowPlanCompileDiagnostic(PlanDiagnosticPortMismatch, item.ClientRef, "/action/outputPorts", "Output port is not declared by the Catalog action"))
		}
	}
	if len(item.Action.InputPorts) > 0 {
		for _, port := range action.InputPorts {
			if port.Required && !boundInputs[port.ID] {
				diagnostics = append(diagnostics, workflowPlanCompileDiagnostic(PlanDiagnosticPortMismatch, item.ClientRef, "/action/inputPorts", "Required input port is not bound"))
			}
		}
	}
	if len(item.Action.OutputPorts) > 0 {
		for _, port := range action.OutputPorts {
			if port.Required && !boundOutputs[port.ID] {
				diagnostics = append(diagnostics, workflowPlanCompileDiagnostic(PlanDiagnosticPortMismatch, item.ClientRef, "/action/outputPorts", "Required output port is not bound"))
			}
		}
	}
	return diagnostics
}

func workflowPlanCatalogPortIDs(ports []WorkflowPlanCatalogPort) map[string]bool {
	result := make(map[string]bool, len(ports))
	for _, port := range ports {
		if port.ID != "" {
			result[port.ID] = true
		}
	}
	return result
}

func compileWorkflowPlanNode(item WorkflowPlanItem, action WorkflowPlanCatalogAction, y float64) (EditableNode, []WorkflowPlanDiagnostic) {
	nodeType := visual.NodeTypeAction
	position := visual.Position{X: 0, Y: y}
	node := EditableNode{NodePatch: NodePatch{Type: &nodeType, Position: &position}}
	switch item.Kind {
	case PlanItemAction:
		arguments := cloneWorkflowPlanMap(item.Action.Arguments)
		step := &workflow.Step{Name: item.ClientRef, Action: action.ID, Args: arguments}
		ports := compileWorkflowPlanActionPorts(item.Action, action)
		node.StepName, node.Step, node.Ports = planStringPointer(item.ClientRef), step, &ports
	case PlanItemBranch:
		nodeType = visual.NodeTypeCondition
		condition := &visual.ConditionSpec{If: item.Branch.Cases[0].Expression, Else: item.Branch.DefaultRef != ""}
		for _, branch := range item.Branch.Cases[1:] {
			condition.Elif = append(condition.Elif, visual.ConditionBranchSpec{Expression: branch.Expression})
		}
		node.Type, node.Condition = &nodeType, condition
	case PlanItemParallel:
		nodeType = visual.NodeTypeParallel
		node.Type = &nodeType
	case PlanItemJoin:
		nodeType = visual.NodeTypeJoin
		node.Type, node.Join = &nodeType, &visual.JoinSpec{Strategy: item.Join.Strategy}
	case PlanItemLoop:
		nodeType = visual.NodeTypeLoop
		node.Type, node.Loop = &nodeType, &visual.LoopSpec{Mode: "items", ItemsVariable: item.Loop.CollectionPath, MaxIterations: item.Loop.MaxIterations}
	case PlanItemApproval:
		nodeType = visual.NodeTypeManualApproval
		node.Type, node.Approval = &nodeType, &visual.ApprovalSpec{Subjects: append([]string(nil), item.Approval.ApproverRefs...), Timeout: item.Approval.Timeout}
	case PlanItemSubflow:
		nodeType = visual.NodeTypeSubflow
		node.Type, node.Subflow = &nodeType, &visual.SubflowSpec{WorkflowName: item.Subflow.WorkflowRef, Vars: cloneWorkflowPlanMap(item.Subflow.Inputs)}
	case PlanItemAggregation:
		nodeType = visual.NodeTypeVariableAggregator
		sources := make([]visual.VariableAggregatorSourceSpec, 0, len(item.Aggregation.InputRefs))
		for _, ref := range item.Aggregation.InputRefs {
			sources = append(sources, visual.VariableAggregatorSourceSpec{Expression: ref})
		}
		node.Type, node.Aggregator = &nodeType, &visual.VariableAggregatorSpec{OutputKey: item.Aggregation.OutputVar, Strategy: item.Aggregation.Strategy, Sources: sources}
	default:
		return EditableNode{}, []WorkflowPlanDiagnostic{workflowPlanCompileDiagnostic(PlanDiagnosticUnresolved, item.ClientRef, "/kind", "Plan item kind cannot be compiled")}
	}
	return node, nil
}

func compileWorkflowPlanActionPorts(plan *WorkflowPlanAction, action WorkflowPlanCatalogAction) []visual.Port {
	bindings := append(append([]WorkflowPlanPortBind(nil), plan.InputPorts...), plan.OutputPorts...)
	if len(bindings) > 0 {
		ports := make([]visual.Port, 0, len(bindings))
		for index, binding := range bindings {
			portType := "input"
			if index >= len(plan.InputPorts) {
				portType = "output"
			}
			ports = append(ports, visual.Port{ID: binding.PlanPort, Type: portType})
		}
		return ports
	}
	ports := make([]visual.Port, 0, len(action.InputPorts)+len(action.OutputPorts))
	for _, port := range action.InputPorts {
		ports = append(ports, visual.Port{ID: port.ID, Type: "input", Required: port.Required})
	}
	for _, port := range action.OutputPorts {
		ports = append(ports, visual.Port{ID: port.ID, Type: "output", Required: port.Required})
	}
	return ports
}

func compileWorkflowPlanEdges(plan WorkflowPlanIR) []WorkflowOperation {
	type edge struct {
		source, target, sourcePort, targetPort, condition string
		kind                                              visual.EdgeKind
	}
	var edges []edge
	for _, item := range plan.Items {
		for _, dependency := range item.DependsOn {
			edges = append(edges, edge{source: dependency.ClientRef, target: item.ClientRef, sourcePort: dependency.SourcePort, targetPort: dependency.TargetPort, kind: workflowPlanOutcomeEdgeKind(dependency.Outcome)})
		}
		switch item.Kind {
		case PlanItemBranch:
			for _, branch := range item.Branch.Cases {
				edges = append(edges, edge{source: item.ClientRef, target: branch.TargetRef, condition: branch.Expression, kind: visual.EdgeKindCondition})
			}
			if item.Branch.DefaultRef != "" {
				edges = append(edges, edge{source: item.ClientRef, target: item.Branch.DefaultRef, kind: visual.EdgeKindElse})
			}
		case PlanItemParallel:
			for _, target := range item.Parallel.BranchRefs {
				edges = append(edges, edge{source: item.ClientRef, target: target, kind: visual.EdgeKindNext})
			}
		case PlanItemJoin:
			for _, source := range item.Join.WaitForRefs {
				edges = append(edges, edge{source: source, target: item.ClientRef, kind: visual.EdgeKindNext})
			}
		case PlanItemLoop:
			for _, target := range item.Loop.BodyRefs {
				edges = append(edges, edge{source: item.ClientRef, target: target, kind: visual.EdgeKindNext})
			}
		case PlanItemAggregation:
			for _, source := range item.Aggregation.InputRefs {
				edges = append(edges, edge{source: source, target: item.ClientRef, kind: visual.EdgeKindNext})
			}
		}
	}
	seen := map[string]bool{}
	var operations []WorkflowOperation
	for _, value := range edges {
		identity := strings.Join([]string{value.source, string(value.kind), value.condition, value.target, value.sourcePort, value.targetPort}, "|")
		if seen[identity] {
			continue
		}
		seen[identity] = true
		id := "edge-" + workflowPlanShortHash(identity)
		operations = append(operations, NewOperation(&AddEdgeOperation{ClientRef: id, Edge: EditableEdge{EdgePatch: EdgePatch{Source: planStringPointer(value.source), SourcePort: planStringPointer(value.sourcePort), Target: planStringPointer(value.target), TargetPort: planStringPointer(value.targetPort), Kind: planEdgeKindPointer(value.kind), Condition: planStringPointer(value.condition)}}}))
	}
	return operations
}

func workflowPlanOutcomeEdgeKind(outcome WorkflowPlanOutcome) visual.EdgeKind {
	switch outcome {
	case PlanOutcomeFailure:
		return visual.EdgeKindFailure
	case PlanOutcomeAlways:
		return visual.EdgeKindAlways
	default:
		return visual.EdgeKindSuccess
	}
}

func workflowPlanCompileDiagnostic(code WorkflowPlanDiagnosticCode, ref, path, message string) WorkflowPlanDiagnostic {
	return WorkflowPlanDiagnostic{Code: code, Severity: "error", Message: message, ClientRef: ref, Path: path, Suggestion: "resolve the diagnostic and compile the same immutable plan again"}
}

func hasWorkflowPlanErrors(diagnostics []WorkflowPlanDiagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "error" {
			return true
		}
	}
	return false
}

func sortWorkflowPlanDiagnostics(diagnostics []WorkflowPlanDiagnostic) {
	sort.SliceStable(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		return string(left.Code)+"\x00"+left.ClientRef+"\x00"+left.Path < string(right.Code)+"\x00"+right.ClientRef+"\x00"+right.Path
	})
}

func cloneWorkflowPlanMap(value map[string]any) map[string]any {
	raw, _ := json.Marshal(value)
	var cloned map[string]any
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}

func workflowPlanShortHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}

func planStringPointer(value string) *string                     { return &value }
func planEdgeKindPointer(value visual.EdgeKind) *visual.EdgeKind { return &value }
