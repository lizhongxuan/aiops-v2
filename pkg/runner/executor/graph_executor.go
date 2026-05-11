package executor

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"
	"runner/logging"
	"runner/state"
	"runner/workflow"
)

type GraphExecutor struct {
	Runner    HostRunner
	Observer  Observer
	Approvals ApprovalRuntime
	Subflows  SubflowRuntime
}

type GraphObserver interface {
	GraphNodeStart(nodeID string)
	GraphNodeFinish(nodeID, status, message string)
	GraphEdgeSelected(edge workflow.GraphEdgeSpec)
}

type GraphIterationObserver interface {
	GraphNodeIterationStart(nodeID string, iteration int, item any)
	GraphNodeIterationFinish(nodeID string, iteration int, status, message string)
}

type ApprovalRuntime interface {
	WaitForApproval(ctx context.Context, wf workflow.Workflow, node workflow.GraphNodeSpec) (ApprovalDecision, error)
}

type ApprovalDecision struct {
	Status  string
	Actor   string
	Comment string
}

type SubflowRuntime interface {
	LoadSubflow(ctx context.Context, parent workflow.Workflow, node workflow.GraphNodeSpec, request SubflowRequest) (workflow.Workflow, error)
}

type SubflowRequest struct {
	WorkflowName string
	Vars         map[string]any
}

type graphApprovalObserver interface {
	GraphApprovalWaiting(nodeID string)
	GraphApprovalResolved(nodeID, status, message string)
}

type graphNodeResult struct {
	id      string
	status  string
	exports map[string]any
	allowed map[string]any
	err     error
}

func (e *GraphExecutor) Run(ctx context.Context, wf workflow.Workflow) error {
	if e.Runner == nil {
		return fmt.Errorf("executor runner is nil")
	}
	if wf.XRunnerGraph == nil {
		return fmt.Errorf("graph workflow requires x_runner_graph")
	}
	idx, err := newExecutionGraph(wf)
	if err != nil {
		return err
	}

	logging.L().Debug("graph executor run start",
		zap.String("workflow", wf.Name),
		zap.Int("nodes", len(idx.nodes)),
		zap.Int("edges", len(idx.edges)),
	)

	runtimeVars := mergeVars(wf.Vars, nil)
	allowedVars := map[string]any{}
	var varsMu sync.Mutex

	statuses := map[string]string{}
	selectedIncoming := map[string]map[string]workflow.GraphEdgeSpec{}
	executed := map[string]bool{}
	var firstFailure error

	completeNode := func(result graphNodeResult) error {
		if strings.TrimSpace(result.status) == "" {
			result.status = state.RunStatusSuccess
		}
		statuses[result.id] = result.status
		executed[result.id] = true
		if len(result.exports) > 0 || len(result.allowed) > 0 {
			varsMu.Lock()
			runtimeVars = mergeExportedVars(runtimeVars, result.exports)
			allowedVars = mergeVars(allowedVars, result.allowed)
			varsMu.Unlock()
		}
		if result.err != nil && firstFailure == nil && !idx.hasContinuation(result.id, result.status) {
			firstFailure = result.err
		}
		for _, edge := range idx.selectedOutgoing(result.id, result.status, snapshotVars(&varsMu, runtimeVars)) {
			e.notifyGraphEdgeSelected(edge)
			if selectedIncoming[edge.Target] == nil {
				selectedIncoming[edge.Target] = map[string]workflow.GraphEdgeSpec{}
			}
			selectedIncoming[edge.Target][edge.ID] = edge
		}
		return nil
	}

	e.notifyGraphNodeStart(idx.startID)
	e.notifyGraphNodeFinish(idx.startID, state.RunStatusSuccess, "")
	if err := completeNode(graphNodeResult{id: idx.startID, status: state.RunStatusSuccess}); err != nil {
		return err
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ready := idx.readyNodes(selectedIncoming, statuses, executed)
		ready = idx.topLevelOnly(ready)
		if len(ready) == 0 {
			break
		}

		resultCh := make(chan graphNodeResult, len(ready))
		var wg sync.WaitGroup
		for _, node := range ready {
			node := node
			wg.Add(1)
			go func() {
				defer wg.Done()
				resultCh <- e.runGraphNode(ctx, wf, idx, node, selectedIncoming[node.ID], statuses, snapshotVars(&varsMu, runtimeVars), snapshotVars(&varsMu, allowedVars))
			}()
		}
		wg.Wait()
		close(resultCh)

		for result := range resultCh {
			if err := completeNode(result); err != nil {
				return err
			}
		}
		if firstFailure != nil && !idx.hasPendingContinuation(selectedIncoming, statuses, executed) {
			return firstFailure
		}
	}

	if pending := idx.pendingSelectedNodes(selectedIncoming, executed); len(pending) > 0 {
		return fmt.Errorf("graph execution stalled; pending nodes: %s", strings.Join(pending, ", "))
	}
	if firstFailure != nil {
		return firstFailure
	}
	logging.L().Debug("graph executor run done", zap.String("workflow", wf.Name))
	return nil
}

func (e *GraphExecutor) runGraphNode(ctx context.Context, wf workflow.Workflow, idx executionGraph, node workflow.GraphNodeSpec, incoming map[string]workflow.GraphEdgeSpec, statuses map[string]string, runtimeVars, allowedVars map[string]any) graphNodeResult {
	e.notifyGraphNodeStart(node.ID)
	finish := func(result graphNodeResult) graphNodeResult {
		message := ""
		if result.err != nil {
			message = result.err.Error()
		}
		e.notifyGraphNodeFinish(node.ID, result.status, message)
		return result
	}
	switch strings.TrimSpace(node.Type) {
	case "parallel", "start":
		return finish(graphNodeResult{id: node.ID, status: state.RunStatusSuccess})
	case "join":
		status, err := idx.evaluateJoin(node, incoming, statuses)
		return finish(graphNodeResult{id: node.ID, status: status, err: err})
	case "loop":
		exports, allowed, err := e.runLoopNode(ctx, wf, idx, node, runtimeVars, allowedVars)
		if err != nil {
			return finish(graphNodeResult{id: node.ID, status: state.RunStatusFailed, exports: exports, allowed: allowed, err: err})
		}
		return finish(graphNodeResult{id: node.ID, status: state.RunStatusSuccess, exports: exports, allowed: allowed})
	case "variable_aggregator":
		exports, allowed, err := aggregateGraphVariables(node, runtimeVars)
		if err != nil {
			return finish(graphNodeResult{id: node.ID, status: state.RunStatusFailed, err: err})
		}
		return finish(graphNodeResult{id: node.ID, status: state.RunStatusSuccess, exports: exports, allowed: allowed})
	case "end":
		return finish(graphNodeResult{id: node.ID, status: state.RunStatusSuccess})
	case "manual_approval":
		if e.Approvals == nil {
			return finish(graphNodeResult{id: node.ID, status: state.RunStatusFailed, err: fmt.Errorf("manual approval graph nodes require approval runtime")})
		}
		e.notifyGraphApprovalWaiting(node.ID)
		decision, err := e.Approvals.WaitForApproval(ctx, wf, node)
		if err != nil {
			result := finish(graphNodeResult{id: node.ID, status: state.RunStatusFailed, err: err})
			e.notifyGraphApprovalResolved(node.ID, result.status, err.Error())
			return result
		}
		status, err := normalizeApprovalDecisionStatus(decision.Status)
		if err != nil {
			result := finish(graphNodeResult{id: node.ID, status: state.RunStatusFailed, err: err})
			e.notifyGraphApprovalResolved(node.ID, result.status, err.Error())
			return result
		}
		message := approvalDecisionMessage(decision, status)
		result := graphNodeResult{id: node.ID, status: status}
		if status == state.RunStatusFailed {
			result.err = fmt.Errorf("%s", message)
		}
		result = finish(result)
		e.notifyGraphApprovalResolved(node.ID, result.status, message)
		return result
	case "subflow":
		if e.Subflows == nil {
			return finish(graphNodeResult{id: node.ID, status: state.RunStatusFailed, err: fmt.Errorf("subflow graph nodes require subflow runtime")})
		}
		step, _ := idx.stepForNode(node)
		request, err := buildSubflowRequest(node, step, runtimeVars)
		if err != nil {
			return finish(graphNodeResult{id: node.ID, status: state.RunStatusFailed, err: err})
		}
		child, err := e.Subflows.LoadSubflow(ctx, wf, node, request)
		if err != nil {
			return finish(graphNodeResult{id: node.ID, status: state.RunStatusFailed, err: err})
		}
		child.Vars = mergeVars(child.Vars, request.Vars)
		if err := e.runSubflow(ctx, child); err != nil {
			return finish(graphNodeResult{id: node.ID, status: state.RunStatusFailed, err: err})
		}
		return finish(graphNodeResult{id: node.ID, status: state.RunStatusSuccess})
	}

	step, ok := idx.stepForNode(node)
	if !ok {
		return finish(graphNodeResult{id: node.ID, status: state.RunStatusFailed, err: fmt.Errorf("graph node %q has no executable step", node.ID)})
	}
	if strings.TrimSpace(step.Action) == "condition.evaluate" {
		return finish(graphNodeResult{id: node.ID, status: state.RunStatusSuccess})
	}
	status, exports, allowed, err := e.runStep(ctx, wf, step, runtimeVars, allowedVars)
	return finish(graphNodeResult{id: node.ID, status: status, exports: exports, allowed: allowed, err: err})
}

func (e *GraphExecutor) runSubflow(ctx context.Context, wf workflow.Workflow) error {
	if strings.TrimSpace(wf.Plan.Strategy) == "graph" || wf.XRunnerGraph != nil {
		return (&GraphExecutor{
			Runner:    e.Runner,
			Observer:  e.Observer,
			Approvals: e.Approvals,
			Subflows:  e.Subflows,
		}).Run(ctx, wf)
	}
	return (&Executor{Runner: e.Runner, Observer: e.Observer}).Run(ctx, wf)
}

func (e *GraphExecutor) runLoopNode(ctx context.Context, wf workflow.Workflow, idx executionGraph, node workflow.GraphNodeSpec, runtimeVars, allowedVars map[string]any) (map[string]any, map[string]any, error) {
	loop := node.Data.Loop
	if loop == nil {
		return nil, nil, fmt.Errorf("loop node %q loop spec is required", node.ID)
	}
	maxIterations := loop.MaxIterations
	if maxIterations <= 0 {
		return nil, nil, fmt.Errorf("loop node %q max_iterations must be greater than zero", node.ID)
	}
	items, err := idx.loopItems(loop, runtimeVars)
	if err != nil {
		return nil, nil, err
	}
	currentVars := mergeVars(runtimeVars, nil)
	exports := map[string]any{}
	allowed := map[string]any{}

	for iteration := 0; iteration < maxIterations; iteration++ {
		if ctx.Err() != nil {
			return exports, allowed, ctx.Err()
		}
		item, ok, err := nextLoopIteration(loop, iteration, items, currentVars)
		if err != nil {
			return exports, allowed, err
		}
		if !ok {
			break
		}
		iterationVars := loopIterationVars(loop, currentVars, iteration, item)
		e.notifyGraphNodeIterationStart(node.ID, iteration, item)
		iterExports, iterAllowed, err := e.runLoopBody(ctx, wf, idx, node, iterationVars, allowedVars)
		status := state.RunStatusSuccess
		message := ""
		if err != nil {
			status = state.RunStatusFailed
			message = err.Error()
		}
		e.notifyGraphNodeIterationFinish(node.ID, iteration, status, message)
		if err != nil {
			return exports, allowed, err
		}
		if len(iterExports) > 0 {
			exports = mergeVars(exports, iterExports)
			currentVars = mergeExportedVars(currentVars, iterExports)
		}
		if len(iterAllowed) > 0 {
			allowed = mergeVars(allowed, iterAllowed)
		}
	}
	return exports, allowed, nil
}

func (e *GraphExecutor) runLoopBody(ctx context.Context, wf workflow.Workflow, idx executionGraph, loopNode workflow.GraphNodeSpec, runtimeVars, allowedVars map[string]any) (map[string]any, map[string]any, error) {
	bodyNodes := idx.loopBodyNodes(loopNode.ID)
	if len(bodyNodes) == 0 {
		return nil, nil, nil
	}
	statuses := map[string]string{loopNode.ID: state.RunStatusSuccess}
	selectedIncoming := idx.loopInitialSelectedIncoming(loopNode.ID)
	executed := map[string]bool{loopNode.ID: true}
	exports := map[string]any{}
	allowed := map[string]any{}
	currentVars := mergeVars(runtimeVars, nil)
	var firstFailure error

	for {
		if ctx.Err() != nil {
			return exports, allowed, ctx.Err()
		}
		ready := idx.readyNodes(selectedIncoming, statuses, executed)
		ready = idx.loopOnlyNodes(loopNode.ID, ready)
		if len(ready) == 0 {
			break
		}
		for _, child := range ready {
			result := e.runGraphNode(ctx, wf, idx, child, selectedIncoming[child.ID], statuses, currentVars, mergeVars(allowedVars, allowed))
			statuses[result.id] = result.status
			executed[result.id] = true
			if len(result.exports) > 0 {
				exports = mergeVars(exports, result.exports)
				currentVars = mergeExportedVars(currentVars, result.exports)
			}
			if len(result.allowed) > 0 {
				allowed = mergeVars(allowed, result.allowed)
			}
			if result.err != nil && firstFailure == nil && !idx.hasLoopContinuation(loopNode.ID, result.id, result.status) {
				firstFailure = result.err
			}
			for _, edge := range idx.selectedLoopOutgoing(loopNode.ID, result.id, result.status, currentVars) {
				e.notifyGraphEdgeSelected(edge)
				if selectedIncoming[edge.Target] == nil {
					selectedIncoming[edge.Target] = map[string]workflow.GraphEdgeSpec{}
				}
				selectedIncoming[edge.Target][edge.ID] = edge
			}
		}
		if firstFailure != nil && !idx.hasPendingContinuation(selectedIncoming, statuses, executed) {
			return exports, allowed, firstFailure
		}
	}
	if pending := idx.pendingSelectedLoopNodes(loopNode.ID, selectedIncoming, executed); len(pending) > 0 {
		return exports, allowed, fmt.Errorf("loop node %q body stalled; pending nodes: %s", loopNode.ID, strings.Join(pending, ", "))
	}
	if firstFailure != nil {
		return exports, allowed, firstFailure
	}
	return exports, allowed, nil
}

func (e *GraphExecutor) notifyGraphNodeStart(nodeID string) {
	if observer, ok := e.Observer.(GraphObserver); ok {
		observer.GraphNodeStart(nodeID)
	}
}

func (e *GraphExecutor) notifyGraphNodeFinish(nodeID, status, message string) {
	if observer, ok := e.Observer.(GraphObserver); ok {
		observer.GraphNodeFinish(nodeID, status, message)
	}
}

func (e *GraphExecutor) notifyGraphEdgeSelected(edge workflow.GraphEdgeSpec) {
	if observer, ok := e.Observer.(GraphObserver); ok {
		observer.GraphEdgeSelected(edge)
	}
}

func (e *GraphExecutor) notifyGraphApprovalWaiting(nodeID string) {
	if observer, ok := e.Observer.(graphApprovalObserver); ok {
		observer.GraphApprovalWaiting(nodeID)
	}
}

func (e *GraphExecutor) notifyGraphApprovalResolved(nodeID, status, message string) {
	if observer, ok := e.Observer.(graphApprovalObserver); ok {
		observer.GraphApprovalResolved(nodeID, status, message)
	}
}

func (e *GraphExecutor) notifyGraphNodeIterationStart(nodeID string, iteration int, item any) {
	if observer, ok := e.Observer.(GraphIterationObserver); ok {
		observer.GraphNodeIterationStart(nodeID, iteration, item)
	}
}

func (e *GraphExecutor) notifyGraphNodeIterationFinish(nodeID string, iteration int, status, message string) {
	if observer, ok := e.Observer.(GraphIterationObserver); ok {
		observer.GraphNodeIterationFinish(nodeID, iteration, status, message)
	}
}

func normalizeApprovalDecisionStatus(status string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "", "approved", "approve", state.RunStatusSuccess:
		return state.RunStatusSuccess, nil
	case "rejected", "reject", state.RunStatusFailed:
		return state.RunStatusFailed, nil
	default:
		return state.RunStatusFailed, fmt.Errorf("unsupported approval decision status %q", status)
	}
}

func approvalDecisionMessage(decision ApprovalDecision, status string) string {
	comment := strings.TrimSpace(decision.Comment)
	actor := strings.TrimSpace(decision.Actor)
	verb := "approved"
	if strings.TrimSpace(status) == state.RunStatusFailed {
		verb = "rejected"
	}
	switch {
	case actor != "" && comment != "":
		return fmt.Sprintf("approval %s by %s: %s", verb, actor, comment)
	case actor != "":
		return fmt.Sprintf("approval %s by %s", verb, actor)
	case comment != "":
		return fmt.Sprintf("approval %s: %s", verb, comment)
	default:
		return fmt.Sprintf("approval %s", verb)
	}
}

func buildSubflowRequest(node workflow.GraphNodeSpec, step workflow.Step, runtimeVars map[string]any) (SubflowRequest, error) {
	request := SubflowRequest{
		WorkflowName: subflowWorkflowName(node, step),
		Vars:         mergeVars(runtimeVars, subflowVars(node, step)),
	}
	if strings.TrimSpace(request.WorkflowName) == "" {
		return SubflowRequest{}, fmt.Errorf("subflow node %q workflow name is required", node.ID)
	}
	return request, nil
}

func subflowWorkflowName(node workflow.GraphNodeSpec, step workflow.Step) string {
	if node.Data.Subflow != nil && strings.TrimSpace(node.Data.Subflow.WorkflowName) != "" {
		return strings.TrimSpace(node.Data.Subflow.WorkflowName)
	}
	for _, key := range []string{"workflow", "workflow_name", "name"} {
		if value, ok := step.Args[key]; ok && strings.TrimSpace(fmt.Sprint(value)) != "" {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func subflowVars(node workflow.GraphNodeSpec, step workflow.Step) map[string]any {
	vars := map[string]any{}
	if node.Data.Subflow != nil {
		vars = mergeVars(vars, node.Data.Subflow.Vars)
	}
	if raw, ok := step.Args["vars"]; ok {
		if casted := coerceStringMap(raw); casted != nil {
			vars = mergeVars(vars, casted)
		}
	}
	return vars
}

func aggregateGraphVariables(node workflow.GraphNodeSpec, runtimeVars map[string]any) (map[string]any, map[string]any, error) {
	aggregator := node.Data.Aggregator
	if aggregator == nil {
		return nil, nil, fmt.Errorf("variable aggregator node %q aggregator spec is required", node.ID)
	}
	outputKey := strings.TrimSpace(aggregator.OutputKey)
	if outputKey == "" {
		return nil, nil, fmt.Errorf("variable aggregator node %q output_key is required", node.ID)
	}
	values := make([]any, 0, len(aggregator.Sources))
	for _, source := range aggregator.Sources {
		value, ok := resolveAggregatorSource(source, runtimeVars)
		if ok {
			values = append(values, value)
		}
	}
	var selected any
	switch strings.TrimSpace(aggregator.Strategy) {
	case "", "first_non_empty", "prefer_success":
		for _, value := range values {
			if aggregatorValuePresent(value) {
				selected = value
				break
			}
		}
	case "array":
		selected = values
	default:
		return nil, nil, fmt.Errorf("variable aggregator node %q unsupported strategy %q", node.ID, aggregator.Strategy)
	}
	exports := map[string]any{outputKey: selected}
	return exports, exports, nil
}

func resolveAggregatorSource(source workflow.GraphVariableAggregatorSourceSpec, vars map[string]any) (any, bool) {
	for _, expression := range aggregatorSourceExpressions(source) {
		if value, ok := lookupAggregatorExpression(expression, vars); ok {
			return value, true
		}
	}
	return nil, false
}

func aggregatorSourceExpressions(source workflow.GraphVariableAggregatorSourceSpec) []string {
	expressions := []string{}
	if expression := strings.TrimSpace(source.Expression); expression != "" {
		expressions = append(expressions, expression)
	}
	if source.Variable == nil {
		return expressions
	}
	ref := source.Variable
	if path := strings.TrimSpace(ref.Path); path != "" {
		expressions = append(expressions, path)
	}
	scope := strings.TrimSpace(ref.Scope)
	nodeID := strings.TrimSpace(ref.NodeID)
	name := strings.TrimSpace(ref.Name)
	if scope != "" && nodeID != "" && name != "" {
		expressions = append(expressions, scope+"."+nodeID+"."+name)
	}
	if scope != "" && name != "" {
		expressions = append(expressions, scope+"."+name)
	}
	if name != "" {
		expressions = append(expressions, name)
	}
	return expressions
}

func lookupAggregatorExpression(expression string, vars map[string]any) (any, bool) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, false
	}
	if value, ok := vars[expression]; ok {
		return value, true
	}
	parts := splitVariablePath(expression)
	if len(parts) == 0 {
		return nil, false
	}
	if value, ok := lookupNestedAggregatorValue(vars, parts); ok {
		return value, true
	}
	switch parts[0] {
	case "env", "workflow_var", "inventory", "sys", "system", "input", "workflow_input":
		if len(parts) > 1 {
			if value, ok := lookupNestedAggregatorValue(vars, parts[1:]); ok {
				return value, true
			}
		}
	case "node", "node_output", "approval", "subflow":
		if len(parts) > 2 {
			if value, ok := lookupNestedAggregatorValue(vars, parts[2:]); ok {
				return value, true
			}
		}
	}
	if len(parts) > 1 {
		if value, ok := vars[parts[len(parts)-1]]; ok {
			return value, true
		}
	}
	return nil, false
}

func splitVariablePath(expression string) []string {
	raw := strings.Split(expression, ".")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func lookupNestedAggregatorValue(current any, parts []string) (any, bool) {
	if len(parts) == 0 {
		return current, true
	}
	switch typed := current.(type) {
	case map[string]any:
		next, ok := typed[parts[0]]
		if !ok {
			return nil, false
		}
		return lookupNestedAggregatorValue(next, parts[1:])
	case map[string]string:
		if len(parts) != 1 {
			return nil, false
		}
		next, ok := typed[parts[0]]
		return next, ok
	case map[any]any:
		next, ok := typed[parts[0]]
		if !ok {
			return nil, false
		}
		return lookupNestedAggregatorValue(next, parts[1:])
	default:
		return nil, false
	}
}

func aggregatorValuePresent(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	case []string:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	case map[string]string:
		return len(typed) > 0
	default:
		return true
	}
}

func (e *GraphExecutor) runStep(ctx context.Context, wf workflow.Workflow, step workflow.Step, runtimeVars, allowedVars map[string]any) (string, map[string]any, map[string]any, error) {
	shouldRun, err := evalWhen(step.When, runtimeVars)
	if err != nil {
		return state.RunStatusFailed, nil, nil, err
	}
	if !shouldRun {
		return state.RunStatusSuccess, nil, nil, nil
	}

	hosts := wf.Inventory.ResolveHosts()
	targets, err := resolveTargets(step, hosts, wf.Inventory)
	if err != nil {
		return state.RunStatusFailed, nil, nil, err
	}
	if e.Observer != nil {
		e.Observer.StepStart(step, targets)
	}
	if err := validateMustVars(step.MustVars, targets, allowedVars); err != nil {
		if e.Observer != nil {
			e.Observer.StepFinish(step, state.RunStatusFailed)
		}
		return state.RunStatusFailed, nil, nil, err
	}

	loopItems := step.Loop
	if len(loopItems) == 0 {
		loopItems = []any{nil}
	}

	handlers := map[string]workflow.Handler{}
	for _, handler := range wf.Handlers {
		handlers[handler.Name] = handler
	}

	stepFailed := false
	stepExports := map[string]any{}
	currentVars := mergeVars(runtimeVars, nil)
	for _, item := range loopItems {
		exports, err := (&Executor{Runner: e.Runner, Observer: e.Observer}).runOnTargets(ctx, step, targets, currentVars, item)
		if err != nil {
			if step.ContinueOnError {
				stepFailed = true
				break
			}
			if e.Observer != nil {
				e.Observer.StepFinish(step, state.RunStatusFailed)
			}
			return state.RunStatusFailed, stepExports, nil, err
		}
		if len(exports) > 0 {
			stepExports = mergeVars(stepExports, exports)
			currentVars = mergeExportedVars(currentVars, exports)
		}
		if len(step.Notify) > 0 {
			if err := (&Executor{Runner: e.Runner, Observer: e.Observer}).runHandlers(ctx, handlers, step.Notify, targets, currentVars, item); err != nil {
				if step.ContinueOnError {
					stepFailed = true
					break
				}
				if e.Observer != nil {
					e.Observer.StepFinish(step, state.RunStatusFailed)
				}
				return state.RunStatusFailed, stepExports, nil, err
			}
		}
	}

	var allowed map[string]any
	if !stepFailed && len(step.ExpectVars) > 0 {
		if err := validateExpectedVars(step.ExpectVars, stepExports); err != nil {
			if step.ContinueOnError {
				stepFailed = true
			} else {
				if e.Observer != nil {
					e.Observer.StepFinish(step, state.RunStatusFailed)
				}
				return state.RunStatusFailed, stepExports, nil, err
			}
		} else {
			allowed = selectExpectedVars(stepExports, step.ExpectVars)
		}
	}

	if e.Observer != nil {
		if stepFailed {
			e.Observer.StepFinish(step, state.RunStatusFailed)
		} else {
			e.Observer.StepFinish(step, state.RunStatusSuccess)
		}
	}
	if stepFailed {
		return state.RunStatusFailed, stepExports, allowed, fmt.Errorf("step %q failed with continue_on_error", step.Name)
	}
	return state.RunStatusSuccess, stepExports, allowed, nil
}

type executionGraph struct {
	nodes       map[string]workflow.GraphNodeSpec
	nodeOrder   map[string]int
	edges       []workflow.GraphEdgeSpec
	outgoing    map[string][]workflow.GraphEdgeSpec
	incoming    map[string][]workflow.GraphEdgeSpec
	startID     string
	stepsByName map[string]workflow.Step
	stepsByID   map[string]workflow.Step
}

func newExecutionGraph(wf workflow.Workflow) (executionGraph, error) {
	idx := executionGraph{
		nodes:       map[string]workflow.GraphNodeSpec{},
		nodeOrder:   map[string]int{},
		outgoing:    map[string][]workflow.GraphEdgeSpec{},
		incoming:    map[string][]workflow.GraphEdgeSpec{},
		stepsByName: map[string]workflow.Step{},
		stepsByID:   map[string]workflow.Step{},
	}
	for _, step := range wf.Steps {
		idx.stepsByName[step.Name] = step
		if strings.TrimSpace(step.ID) != "" {
			idx.stepsByID[step.ID] = step
		}
	}
	if wf.XRunnerGraph == nil {
		return idx, fmt.Errorf("x_runner_graph is required")
	}
	for i, node := range wf.XRunnerGraph.Nodes {
		node.ID = strings.TrimSpace(node.ID)
		if node.ID == "" {
			return idx, fmt.Errorf("graph node id is required")
		}
		if _, exists := idx.nodes[node.ID]; exists {
			return idx, fmt.Errorf("graph node id %q is duplicated", node.ID)
		}
		idx.nodes[node.ID] = node
		idx.nodeOrder[node.ID] = i
		if node.Type == "start" {
			if idx.startID != "" {
				return idx, fmt.Errorf("graph must contain only one start node")
			}
			idx.startID = node.ID
		}
	}
	if idx.startID == "" {
		return idx, fmt.Errorf("graph start node is required")
	}
	edgeIDs := map[string]struct{}{}
	for i, edge := range wf.XRunnerGraph.Edges {
		edge.ID = strings.TrimSpace(edge.ID)
		if edge.ID == "" {
			edge.ID = fmt.Sprintf("edge-%d", i+1)
		}
		if _, exists := edgeIDs[edge.ID]; exists {
			return idx, fmt.Errorf("graph edge id %q is duplicated", edge.ID)
		}
		edgeIDs[edge.ID] = struct{}{}
		if _, ok := idx.nodes[edge.Source]; !ok {
			return idx, fmt.Errorf("graph edge %q source node %q not found", edge.ID, edge.Source)
		}
		if _, ok := idx.nodes[edge.Target]; !ok {
			return idx, fmt.Errorf("graph edge %q target node %q not found", edge.ID, edge.Target)
		}
		idx.edges = append(idx.edges, edge)
		idx.outgoing[edge.Source] = append(idx.outgoing[edge.Source], edge)
		idx.incoming[edge.Target] = append(idx.incoming[edge.Target], edge)
	}
	for nodeID := range idx.outgoing {
		sort.SliceStable(idx.outgoing[nodeID], func(i, j int) bool {
			return idx.nodeOrder[idx.outgoing[nodeID][i].Target] < idx.nodeOrder[idx.outgoing[nodeID][j].Target]
		})
	}
	for _, node := range idx.nodes {
		if parent, ok := idx.nodes[strings.TrimSpace(node.ParentID)]; ok && strings.TrimSpace(parent.Type) == "loop" {
			if step, ok := idx.stepForNode(node); ok && len(step.Loop) > 0 {
				return idx, fmt.Errorf("graph loop body node %q cannot also use legacy step.loop", node.ID)
			}
		}
		if strings.TrimSpace(node.Type) != "loop" {
			continue
		}
		loop := node.Data.Loop
		if loop == nil {
			return idx, fmt.Errorf("loop node %q loop spec is required", node.ID)
		}
		if strings.TrimSpace(loop.Mode) == "" {
			return idx, fmt.Errorf("loop node %q mode is required", node.ID)
		}
		if loop.MaxIterations <= 0 {
			return idx, fmt.Errorf("loop node %q max_iterations must be greater than zero", node.ID)
		}
		switch strings.TrimSpace(loop.Mode) {
		case "for_each":
			if len(loop.Items) == 0 && strings.TrimSpace(loop.ItemsVariable) == "" {
				return idx, fmt.Errorf("loop node %q for_each requires items or items_variable", node.ID)
			}
		case "while_condition":
			if strings.TrimSpace(loop.WhileCondition) == "" {
				return idx, fmt.Errorf("loop node %q while_condition is required", node.ID)
			}
		default:
			return idx, fmt.Errorf("loop node %q unsupported mode %q", node.ID, loop.Mode)
		}
	}
	return idx, nil
}

func (idx executionGraph) stepForNode(node workflow.GraphNodeSpec) (workflow.Step, bool) {
	if strings.TrimSpace(node.StepID) != "" {
		if step, ok := idx.stepsByID[node.StepID]; ok {
			return step, true
		}
	}
	for _, key := range []string{node.Step, node.StepName, node.Data.StepName} {
		if step, ok := idx.stepsByName[strings.TrimSpace(key)]; ok {
			return step, true
		}
	}
	return workflow.Step{}, false
}

func (idx executionGraph) readyNodes(selectedIncoming map[string]map[string]workflow.GraphEdgeSpec, statuses map[string]string, executed map[string]bool) []workflow.GraphNodeSpec {
	var ready []workflow.GraphNodeSpec
	for nodeID, edges := range selectedIncoming {
		if executed[nodeID] || len(edges) == 0 {
			continue
		}
		allDone := true
		for _, edge := range edges {
			if !isTerminalStatus(statuses[edge.Source]) {
				allDone = false
				break
			}
		}
		if allDone {
			ready = append(ready, idx.nodes[nodeID])
		}
	}
	sort.SliceStable(ready, func(i, j int) bool {
		return idx.nodeOrder[ready[i].ID] < idx.nodeOrder[ready[j].ID]
	})
	return ready
}

func (idx executionGraph) topLevelOnly(nodes []workflow.GraphNodeSpec) []workflow.GraphNodeSpec {
	out := nodes[:0]
	for _, node := range nodes {
		if strings.TrimSpace(node.ParentID) == "" {
			out = append(out, node)
		}
	}
	return out
}

func (idx executionGraph) selectedOutgoing(nodeID, status string, vars map[string]any) []workflow.GraphEdgeSpec {
	var selected []workflow.GraphEdgeSpec
	failed := status == state.RunStatusFailed
	for _, edge := range idx.outgoing[nodeID] {
		if idx.isLoopInternalEdge(edge) {
			continue
		}
		kind := strings.TrimSpace(edge.Kind)
		if kind == "" {
			kind = "next"
		}
		switch kind {
		case "next", "success":
			if status == state.RunStatusSuccess {
				selected = append(selected, edge)
			}
		case "approval_approved":
			if status == state.RunStatusSuccess {
				selected = append(selected, edge)
			}
		case "approval_rejected":
			if failed {
				selected = append(selected, edge)
			}
		case "failure":
			if failed {
				selected = append(selected, edge)
			}
		case "always":
			if isTerminalStatus(status) {
				selected = append(selected, edge)
			}
		case "condition":
			if status != state.RunStatusSuccess {
				continue
			}
			ok, err := workflow.EvalWhen(idx.edgeConditionExpression(edge), vars)
			if err == nil && ok {
				selected = append(selected, edge)
			}
		case "if":
			if status != state.RunStatusSuccess {
				continue
			}
			ok, err := workflow.EvalWhen(idx.edgeConditionExpression(edge), vars)
			if err == nil && ok {
				selected = append(selected, edge)
			}
		case "else":
			if status == state.RunStatusSuccess && !idx.hasMatchedConditionalOutgoing(nodeID, edge.ID, vars) {
				selected = append(selected, edge)
			}
		}
	}
	if failed && len(selected) == 0 && idx.nodeContinueOnError(nodeID) {
		for _, edge := range idx.outgoing[nodeID] {
			if idx.isLoopInternalEdge(edge) {
				continue
			}
			kind := strings.TrimSpace(edge.Kind)
			if kind == "" {
				kind = "next"
			}
			if kind == "next" || kind == "success" {
				selected = append(selected, edge)
			}
		}
	}
	return selected
}

func (idx executionGraph) selectedLoopOutgoing(loopID, nodeID, status string, vars map[string]any) []workflow.GraphEdgeSpec {
	var selected []workflow.GraphEdgeSpec
	for _, edge := range idx.rawSelectedOutgoing(nodeID, status, vars) {
		if strings.TrimSpace(idx.nodes[edge.Target].ParentID) == loopID {
			selected = append(selected, edge)
		}
	}
	return selected
}

func (idx executionGraph) rawSelectedOutgoing(nodeID, status string, vars map[string]any) []workflow.GraphEdgeSpec {
	var selected []workflow.GraphEdgeSpec
	failed := status == state.RunStatusFailed
	for _, edge := range idx.outgoing[nodeID] {
		kind := strings.TrimSpace(edge.Kind)
		if kind == "" {
			kind = "next"
		}
		switch kind {
		case "next", "success":
			if status == state.RunStatusSuccess {
				selected = append(selected, edge)
			}
		case "approval_approved":
			if status == state.RunStatusSuccess {
				selected = append(selected, edge)
			}
		case "approval_rejected", "failure":
			if failed {
				selected = append(selected, edge)
			}
		case "always":
			if isTerminalStatus(status) {
				selected = append(selected, edge)
			}
		case "condition":
			if status == state.RunStatusSuccess {
				ok, err := workflow.EvalWhen(idx.edgeConditionExpression(edge), vars)
				if err == nil && ok {
					selected = append(selected, edge)
				}
			}
		case "if":
			if status == state.RunStatusSuccess {
				ok, err := workflow.EvalWhen(idx.edgeConditionExpression(edge), vars)
				if err == nil && ok {
					selected = append(selected, edge)
				}
			}
		case "else":
			if status == state.RunStatusSuccess && !idx.hasMatchedConditionalOutgoing(nodeID, edge.ID, vars) {
				selected = append(selected, edge)
			}
		}
	}
	return selected
}

func (idx executionGraph) edgeConditionExpression(edge workflow.GraphEdgeSpec) string {
	if strings.TrimSpace(edge.Condition) != "" {
		return edge.Condition
	}
	node := idx.nodes[edge.Source]
	if node.Data.Condition != nil && strings.TrimSpace(node.Data.Condition.If) != "" {
		return node.Data.Condition.If
	}
	return ""
}

func (idx executionGraph) hasMatchedConditionalOutgoing(nodeID, skipEdgeID string, vars map[string]any) bool {
	for _, edge := range idx.outgoing[nodeID] {
		if edge.ID == skipEdgeID {
			continue
		}
		kind := strings.TrimSpace(edge.Kind)
		if kind != "if" && kind != "condition" {
			continue
		}
		ok, err := workflow.EvalWhen(idx.edgeConditionExpression(edge), vars)
		if err == nil && ok {
			return true
		}
	}
	return false
}

func (idx executionGraph) hasLoopContinuation(loopID, nodeID, status string) bool {
	return len(idx.selectedLoopOutgoing(loopID, nodeID, status, nil)) > 0
}

func (idx executionGraph) isLoopInternalEdge(edge workflow.GraphEdgeSpec) bool {
	source := idx.nodes[edge.Source]
	target := idx.nodes[edge.Target]
	targetParent := strings.TrimSpace(target.ParentID)
	if targetParent == "" {
		return false
	}
	return targetParent == strings.TrimSpace(source.ID) || targetParent == strings.TrimSpace(source.ParentID)
}

func (idx executionGraph) nodeContinueOnError(nodeID string) bool {
	node, ok := idx.nodes[nodeID]
	if !ok {
		return false
	}
	step, ok := idx.stepForNode(node)
	return ok && step.ContinueOnError
}

func (idx executionGraph) evaluateJoin(node workflow.GraphNodeSpec, incoming map[string]workflow.GraphEdgeSpec, statuses map[string]string) (string, error) {
	strategy := "all_success"
	failureThreshold := 0
	if node.Data.Join != nil {
		if strings.TrimSpace(node.Data.Join.Strategy) != "" {
			strategy = strings.TrimSpace(node.Data.Join.Strategy)
		}
		failureThreshold = node.Data.Join.FailureThreshold
	}
	if strategy == "" {
		strategy = "all_success"
	}
	successes := 0
	failures := 0
	for _, edge := range incoming {
		sourceStatus := strings.TrimSpace(statuses[edge.Source])
		switch sourceStatus {
		case state.RunStatusSuccess:
			successes++
		default:
			failures++
		}
	}
	switch strategy {
	case "all_success":
		if failures == 0 {
			return state.RunStatusSuccess, nil
		}
	case "any_success":
		if successes > 0 {
			return state.RunStatusSuccess, nil
		}
	case "always":
		return state.RunStatusSuccess, nil
	case "failure_threshold":
		if failures <= failureThreshold {
			return state.RunStatusSuccess, nil
		}
	default:
		return state.RunStatusFailed, fmt.Errorf("unsupported join strategy %q", strategy)
	}
	return state.RunStatusFailed, fmt.Errorf("join node %q failed strategy %s", node.ID, strategy)
}

func (idx executionGraph) hasContinuation(nodeID, status string) bool {
	for _, edge := range idx.outgoing[nodeID] {
		kind := strings.TrimSpace(edge.Kind)
		if kind == "always" {
			return true
		}
		if status == state.RunStatusFailed && kind == "failure" {
			return true
		}
		if status == state.RunStatusFailed && kind == "approval_rejected" {
			return true
		}
		if status == state.RunStatusSuccess && kind == "approval_approved" {
			return true
		}
		if status == state.RunStatusFailed && idx.nodeContinueOnError(nodeID) && (kind == "" || kind == "next" || kind == "success") {
			return true
		}
	}
	return false
}

func (idx executionGraph) hasPendingContinuation(selectedIncoming map[string]map[string]workflow.GraphEdgeSpec, statuses map[string]string, executed map[string]bool) bool {
	for nodeID, edges := range selectedIncoming {
		if executed[nodeID] || len(edges) == 0 {
			continue
		}
		for _, edge := range edges {
			if !isTerminalStatus(statuses[edge.Source]) {
				return true
			}
		}
	}
	return false
}

func (idx executionGraph) pendingSelectedNodes(selectedIncoming map[string]map[string]workflow.GraphEdgeSpec, executed map[string]bool) []string {
	var pending []string
	for nodeID, edges := range selectedIncoming {
		if len(edges) > 0 && !executed[nodeID] {
			pending = append(pending, nodeID)
		}
	}
	sort.Strings(pending)
	return pending
}

func (idx executionGraph) loopItems(loop *workflow.GraphLoopSpec, vars map[string]any) ([]any, error) {
	if loop == nil {
		return nil, fmt.Errorf("loop spec is required")
	}
	if len(loop.Items) > 0 {
		return append([]any(nil), loop.Items...), nil
	}
	key := strings.TrimSpace(loop.ItemsVariable)
	if key == "" {
		return nil, nil
	}
	items, ok := coerceAnySlice(vars[key])
	if !ok {
		return nil, fmt.Errorf("loop items_variable %q must resolve to an array", key)
	}
	return items, nil
}

func nextLoopIteration(loop *workflow.GraphLoopSpec, iteration int, items []any, vars map[string]any) (any, bool, error) {
	switch strings.TrimSpace(loop.Mode) {
	case "for_each":
		if iteration >= len(items) {
			return nil, false, nil
		}
		return items[iteration], true, nil
	case "while_condition":
		ok, err := evalWhen(loop.WhileCondition, vars)
		if err != nil || !ok {
			return nil, false, err
		}
		return nil, true, nil
	default:
		return nil, false, fmt.Errorf("unsupported loop mode %q", loop.Mode)
	}
}

func loopIterationVars(loop *workflow.GraphLoopSpec, base map[string]any, iteration int, item any) map[string]any {
	vars := mergeVars(base, map[string]any{"iteration": iteration})
	indexVar := strings.TrimSpace(loop.IndexVar)
	if indexVar == "" {
		indexVar = "index"
	}
	vars[indexVar] = iteration
	itemVar := strings.TrimSpace(loop.ItemVar)
	if itemVar == "" {
		itemVar = "item"
	}
	if item != nil {
		vars[itemVar] = item
		vars["item"] = item
	}
	return vars
}

func (idx executionGraph) loopBodyNodes(loopID string) []workflow.GraphNodeSpec {
	var nodes []workflow.GraphNodeSpec
	for _, node := range idx.nodes {
		if strings.TrimSpace(node.ParentID) == loopID {
			nodes = append(nodes, node)
		}
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return idx.nodeOrder[nodes[i].ID] < idx.nodeOrder[nodes[j].ID]
	})
	return nodes
}

func (idx executionGraph) loopInitialSelectedIncoming(loopID string) map[string]map[string]workflow.GraphEdgeSpec {
	selected := map[string]map[string]workflow.GraphEdgeSpec{}
	for _, edge := range idx.outgoing[loopID] {
		if strings.TrimSpace(idx.nodes[edge.Target].ParentID) != loopID {
			continue
		}
		if selected[edge.Target] == nil {
			selected[edge.Target] = map[string]workflow.GraphEdgeSpec{}
		}
		selected[edge.Target][edge.ID] = edge
	}
	if len(selected) > 0 {
		return selected
	}
	for _, node := range idx.loopBodyNodes(loopID) {
		hasLoopIncoming := false
		for _, edge := range idx.incoming[node.ID] {
			if strings.TrimSpace(idx.nodes[edge.Source].ParentID) == loopID {
				hasLoopIncoming = true
				break
			}
		}
		if !hasLoopIncoming {
			selected[node.ID] = map[string]workflow.GraphEdgeSpec{
				"loop-start-" + node.ID: {ID: "loop-start-" + node.ID, Source: loopID, Target: node.ID, Kind: "next"},
			}
		}
	}
	return selected
}

func (idx executionGraph) loopOnlyNodes(loopID string, nodes []workflow.GraphNodeSpec) []workflow.GraphNodeSpec {
	out := nodes[:0]
	for _, node := range nodes {
		if strings.TrimSpace(node.ParentID) == loopID {
			out = append(out, node)
		}
	}
	return out
}

func (idx executionGraph) pendingSelectedLoopNodes(loopID string, selectedIncoming map[string]map[string]workflow.GraphEdgeSpec, executed map[string]bool) []string {
	var pending []string
	for nodeID, edges := range selectedIncoming {
		if strings.TrimSpace(idx.nodes[nodeID].ParentID) != loopID {
			continue
		}
		if len(edges) > 0 && !executed[nodeID] {
			pending = append(pending, nodeID)
		}
	}
	sort.Strings(pending)
	return pending
}

func coerceAnySlice(raw any) ([]any, bool) {
	switch v := raw.(type) {
	case []any:
		return append([]any(nil), v...), true
	case []string:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = item
		}
		return out, true
	case []int:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = item
		}
		return out, true
	default:
		return nil, false
	}
}

func isTerminalStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case state.RunStatusSuccess, state.RunStatusFailed, state.RunStatusCanceled, state.RunStatusInterrupted:
		return true
	default:
		return false
	}
}

func snapshotVars(mu *sync.Mutex, vars map[string]any) map[string]any {
	mu.Lock()
	defer mu.Unlock()
	return mergeVars(vars, nil)
}
