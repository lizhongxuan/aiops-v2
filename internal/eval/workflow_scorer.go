package eval

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	WorkflowGenerationCaseSchemaVersion   = "aiops.workflow-generation.eval-case.v1"
	WorkflowGenerationOutputSchemaVersion = "aiops.workflow-generation.eval-output.v1"
	WorkflowGenerationReportSchemaVersion = "aiops.workflow-generation.eval-report.v1"
	WorkflowFinalValidThreshold           = .95
	WorkflowFirstPreviewValidThreshold    = .75
)

func ScoreWorkflowGenerationCase(caseID string, fixture WorkflowGenerationFixture, attempts []WorkflowGenerationAttempt) (WorkflowGenerationCaseScore, error) {
	if fixture.SchemaVersion != WorkflowGenerationCaseSchemaVersion {
		return WorkflowGenerationCaseScore{}, fmt.Errorf("%s: unsupported workflow eval case schema %q", caseID, fixture.SchemaVersion)
	}
	if len(attempts) == 0 {
		return WorkflowGenerationCaseScore{}, fmt.Errorf("%s: no workflow generation attempts", caseID)
	}
	result := WorkflowGenerationCaseScore{CaseID: caseID, Passed: true, Attempts: attempts}
	var allChecks [][]CheckResult
	for i, attempt := range attempts {
		if attempt.SchemaVersion != WorkflowGenerationOutputSchemaVersion {
			return result, fmt.Errorf("%s attempt %d: unsupported output schema %q", caseID, i+1, attempt.SchemaVersion)
		}
		checks, placeholders := scoreWorkflowAttempt(fixture, attempt)
		score := workflowChecksScore(checks)
		result.AttemptScores = append(result.AttemptScores, score)
		result.PlaceholderCount += placeholders
		allChecks = append(allChecks, checks)
		if !allWorkflowChecksPass(checks) {
			result.Passed = false
			result.FailedIterations = append(result.FailedIterations, i+1)
		}
		if fixture.Validation.RateEligible {
			result.RateEligible++
			if attempt.FirstPreview.Validation.Valid {
				result.FirstPreviewValid++
			}
			if attempt.Final.Validation.Valid {
				result.FinalValid++
			}
		}
	}
	result.MeanScore, result.ScoreVariance = meanVariance(result.AttemptScores)
	result.Checks = aggregateWorkflowChecks(allChecks)
	if fixture.RequireDeterministic {
		check := scoreWorkflowDeterminism(attempts)
		result.Checks = append(result.Checks, check)
		if !check.Passed {
			result.Passed = false
		}
	}
	return result, nil
}

func scoreWorkflowAttempt(f WorkflowGenerationFixture, a WorkflowGenerationAttempt) ([]CheckResult, int) {
	nodes := map[string]WorkflowEvalNode{}
	nodeTypes := map[string]bool{}
	actions := map[string]bool{}
	for _, n := range a.Final.Graph.Nodes {
		nodes[n.ID] = n
		nodeTypes[n.Type] = true
		if n.Action != "" {
			actions[n.Action] = true
		}
	}
	checks := []CheckResult{
		workflowNodeTypeCheck(nodeTypes, f.RequiredNodeTypes, f.AllowedNodeTypes), workflowActionCheck(actions, f.RequiredActions, f.AllowedActions),
		workflowArgsCheck(nodes, f.ArgumentSchemas), workflowPortsCheck(a.Final.Graph, f.RequiredPorts), workflowPathsCheck("requiredPaths", a.Final, f.RequiredPaths, true), workflowPathsCheck("forbiddenPaths", a.Final, f.ForbiddenPaths, false),
	}
	placeholderCheck, count := workflowPlaceholderCheck(a.Final.Graph, f.ForbiddenPlaceholders)
	checks = append(checks, placeholderCheck)
	checks = append(checks, workflowValidationCheck(a, f.Validation), workflowRiskCheck(a.Final, f.Risk), workflowInvariantCheck(a, f.Invariants), workflowBudgetCheck(a, f.Budgets), workflowGraphTextBoundary(a))
	return checks, count
}

func workflowNodeTypeCheck(actual map[string]bool, required, allowed []string) CheckResult {
	var missing []string
	for _, value := range required {
		if !actual[value] {
			missing = append(missing, value)
		}
	}
	if len(allowed) == 0 {
		allowed = required
	}
	set := stringSet(allowed)
	var unexpected []string
	for value := range actual {
		if !set[value] {
			unexpected = append(unexpected, value)
		}
	}
	sort.Strings(unexpected)
	return CheckResult{Name: "nodeTypes", Passed: len(missing) == 0 && len(unexpected) == 0, Missing: missing, Unexpected: unexpected, Detail: fmt.Sprintf("%d required typed node kinds checked", len(required))}
}
func workflowActionCheck(actual map[string]bool, required, allowed []string) CheckResult {
	set := stringSet(allowed)
	var missing []string
	for _, value := range required {
		if !actual[value] {
			missing = append(missing, value)
		}
	}
	var unexpected []string
	for value := range actual {
		if !set[value] {
			unexpected = append(unexpected, value)
		}
	}
	sort.Strings(unexpected)
	return CheckResult{Name: "catalogActions", Passed: len(missing) == 0 && len(unexpected) == 0, Missing: missing, Unexpected: unexpected, Detail: "actions must resolve to the versioned fixture catalog"}
}
func workflowArgsCheck(nodes map[string]WorkflowEvalNode, schemas map[string]WorkflowEvalArgumentSchema) CheckResult {
	var issues []string
	for _, n := range nodes {
		if n.Action == "" {
			continue
		}
		s, ok := schemas[n.Action]
		if !ok {
			issues = append(issues, n.ID+":missing_schema")
			continue
		}
		for _, k := range s.Required {
			if _, ok := n.Args[k]; !ok {
				issues = append(issues, n.ID+":"+k+":required")
			}
		}
		for k, v := range n.Args {
			typ, ok := s.Properties[k]
			if !ok && !s.AdditionalProperties {
				issues = append(issues, n.ID+":"+k+":unknown")
				continue
			}
			if ok && !workflowJSONType(v, typ) {
				issues = append(issues, n.ID+":"+k+":"+typ)
			}
		}
	}
	return CheckResult{Name: "argumentSchemas", Passed: len(issues) == 0, Unexpected: issues, Detail: "action args checked against fixture Catalog JSON schemas"}
}
func workflowPortsCheck(graph WorkflowEvalGraph, expected map[string]WorkflowEvalPortExpectation) CheckResult {
	nodes := map[string]WorkflowEvalNode{}
	edges := map[string]bool{}
	var missing []string
	for _, node := range graph.Nodes {
		if node.ID == "" || nodes[node.ID].ID != "" {
			missing = append(missing, "node:"+node.ID+":unique_id")
		}
		nodes[node.ID] = node
	}
	for id, want := range expected {
		n, ok := nodes[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		for _, p := range want.Inputs {
			if !hasString(n.InputPorts, p) {
				missing = append(missing, id+":input:"+p)
			}
		}
		for _, p := range want.Outputs {
			if !hasString(n.OutputPorts, p) {
				missing = append(missing, id+":output:"+p)
			}
		}
	}
	for _, edge := range graph.Edges {
		if edge.ID == "" || edges[edge.ID] {
			missing = append(missing, "edge:"+edge.ID+":unique_id")
		}
		edges[edge.ID] = true
		source, sourceOK := nodes[edge.Source]
		target, targetOK := nodes[edge.Target]
		if !sourceOK {
			missing = append(missing, edge.ID+":source_node")
		} else if edge.SourcePort == "" {
			missing = append(missing, edge.ID+":source_port")
		} else if !hasString(source.OutputPorts, edge.SourcePort) {
			missing = append(missing, edge.ID+":source_port:"+edge.SourcePort)
		}
		if !targetOK {
			missing = append(missing, edge.ID+":target_node")
		} else if edge.TargetPort == "" {
			missing = append(missing, edge.ID+":target_port")
		} else if !hasString(target.InputPorts, edge.TargetPort) {
			missing = append(missing, edge.ID+":target_port:"+edge.TargetPort)
		}
	}
	sort.Strings(missing)
	return CheckResult{Name: "ports", Passed: len(missing) == 0, Missing: missing, Detail: "required node ports checked"}
}
func workflowPathsCheck(name string, value any, paths []string, required bool) CheckResult {
	decoded := jsonValue(value)
	var failures []string
	for _, p := range paths {
		exists := len(workflowPathValues(decoded, p)) > 0
		if exists != required {
			failures = append(failures, p)
		}
	}
	result := CheckResult{Name: name, Passed: len(failures) == 0, Detail: "versioned structured paths checked"}
	if required {
		result.Missing = failures
	} else {
		result.Unexpected = failures
	}
	return result
}
func workflowPlaceholderCheck(graph WorkflowEvalGraph, forbidden []string) (CheckResult, int) {
	patterns := append([]string{"script.python", "<todo>", "replace_me", "placeholder"}, forbidden...)
	var found []string
	for _, node := range graph.Nodes {
		if node.Type == "action" && strings.TrimSpace(node.Action) == "" {
			found = append(found, node.ID+":missing_action")
		}
	}
	walkWorkflowStrings(jsonValue(graph), func(s string) {
		lower := strings.ToLower(s)
		for _, p := range patterns {
			if p != "" && strings.Contains(lower, strings.ToLower(p)) {
				found = append(found, p)
				break
			}
		}
	})
	found = uniqueStrings(found)
	return CheckResult{Name: "placeholders", Passed: len(found) == 0, Unexpected: found, Detail: "placeholder action/script values forbidden"}, len(found)
}
func workflowValidationCheck(a WorkflowGenerationAttempt, expected WorkflowEvalValidationExpectation) CheckResult {
	var missing []string
	if a.FirstPreview.Validation.Status != expected.FirstPreviewStatus || a.FirstPreview.Validation.Valid != workflowStatusIsValid(expected.FirstPreviewStatus) {
		missing = append(missing, "first_preview:"+expected.FirstPreviewStatus)
	}
	if a.Final.Validation.Status != expected.FinalStatus || a.Final.Validation.Valid != workflowStatusIsValid(expected.FinalStatus) {
		missing = append(missing, "final:"+expected.FinalStatus)
	}
	return CheckResult{Name: "validation", Passed: len(missing) == 0, Missing: missing, Detail: "first preview and final validation status checked"}
}
func workflowStatusIsValid(status string) bool {
	return status == "valid" || status == "valid_with_warnings"
}
func workflowRiskCheck(final WorkflowEvalSnapshot, expected WorkflowEvalRiskExpectation) CheckResult {
	var missing []string
	if final.Risk != expected.Level {
		missing = append(missing, "risk:"+expected.Level)
	}
	if final.Approval.Required != expected.RequiresApproval {
		missing = append(missing, "approval_required")
	}
	if expected.MustBeGranted && !final.Approval.Granted {
		missing = append(missing, "approval_granted")
	}
	return CheckResult{Name: "riskApproval", Passed: len(missing) == 0, Missing: missing, Detail: "risk and approval contract checked"}
}
func workflowInvariantCheck(a WorkflowGenerationAttempt, invariants []WorkflowEvalInvariant) CheckResult {
	var missing []string
	for _, inv := range invariants {
		if !workflowInvariantPass(a, inv) {
			key := inv.Kind
			if inv.Path != "" {
				key += ":" + inv.Path
			}
			missing = append(missing, key)
		}
	}
	return CheckResult{Name: "semanticInvariants", Passed: len(missing) == 0, Missing: missing, Detail: "graph semantic invariants checked"}
}
func workflowInvariantPass(a WorkflowGenerationAttempt, inv WorkflowEvalInvariant) bool {
	switch inv.Kind {
	case "acyclic":
		return workflowAcyclic(a.Final.Graph)
	case "all_paths_end":
		return workflowAllPathsEnd(a.Final.Graph)
	case "edge":
		for _, e := range a.Final.Graph.Edges {
			if e.Source == inv.Source && e.SourcePort == inv.SourcePort && e.Target == inv.Target && (inv.TargetPort == "" || e.TargetPort == inv.TargetPort) {
				return true
			}
		}
		return false
	case "preserve_path":
		if a.Baseline == nil {
			return false
		}
		before := workflowPathValues(jsonValue(*a.Baseline), inv.Path)
		after := workflowPathValues(jsonValue(a.Final), inv.Path)
		return len(before) > 0 && len(after) > 0 && workflowValuesEqual(before, after)
	case "fact":
		return a.SemanticFacts[inv.Name]
	case "bounded_loop":
		for _, n := range a.Final.Graph.Nodes {
			if n.Type == "loop" {
				if v, ok := numberValue(n.Args["maxIterations"]); !ok || v <= 0 {
					return false
				}
			}
		}
		return true
	default:
		return false
	}
}
func workflowBudgetCheck(a WorkflowGenerationAttempt, b WorkflowEvalBudgets) CheckResult {
	var unexpected []string
	m := a.Metrics
	maxRepairCount := b.MaxRepairCount
	if maxRepairCount == 0 {
		maxRepairCount = 2
	}
	if maxRepairCount >= 0 && m.RepairCount > maxRepairCount {
		unexpected = append(unexpected, "repair_count")
	}
	if b.MaxFirstFeedbackMS > 0 && m.FirstFeedbackMS > b.MaxFirstFeedbackMS {
		unexpected = append(unexpected, "first_feedback_ms")
	}
	if b.MaxPreviewP95MS > 0 && m.PreviewMS > b.MaxPreviewP95MS {
		unexpected = append(unexpected, "preview_ms")
	}
	if b.MaxValidateP95MS > 0 && m.ValidateMS > b.MaxValidateP95MS {
		unexpected = append(unexpected, "validate_ms")
	}
	if b.MaxInputTokens > 0 && m.InputTokens > b.MaxInputTokens {
		unexpected = append(unexpected, "input_tokens")
	}
	if b.MaxContextTokens > 0 && m.ContextTokens > b.MaxContextTokens {
		unexpected = append(unexpected, "context_tokens")
	}
	return CheckResult{Name: "budgets", Passed: len(unexpected) == 0, Unexpected: unexpected, Detail: "feedback, repair, latency and token budgets checked"}
}
func workflowGraphTextBoundary(a WorkflowGenerationAttempt) CheckResult {
	copies := 0
	for _, text := range a.TextContent {
		if workflowTextContainsGraph(text, a.Final.Graph) {
			copies++
		}
	}
	return CheckResult{Name: "graphTextBoundary", Passed: copies == 0, Detail: "full Graph must not be copied into TextContent", Unexpected: func() []string {
		if copies > 0 {
			return []string{"full_graph_in_text_content"}
		}
		return nil
	}()}
}

func workflowTextContainsGraph(text string, graph WorkflowEvalGraph) bool {
	want, _ := json.Marshal(jsonValue(graph))
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "```json"))
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "```"))
	}
	var value any
	if json.Unmarshal([]byte(trimmed), &value) != nil {
		return len(want) > 2 && strings.Contains(text, string(want))
	}
	return workflowValueContainsJSON(value, string(want))
}

func workflowValueContainsJSON(value any, want string) bool {
	data, _ := json.Marshal(value)
	if string(data) == want {
		return true
	}
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			if workflowValueContainsJSON(child, want) {
				return true
			}
		}
	case map[string]any:
		for _, child := range typed {
			if workflowValueContainsJSON(child, want) {
				return true
			}
		}
	}
	return false
}
