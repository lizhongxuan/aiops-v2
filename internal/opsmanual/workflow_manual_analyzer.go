package opsmanual

import (
	"fmt"
	"sort"
	"strings"

	"runner/workflow"
)

func AnalyzeWorkflowForManual(req WorkflowManualGenerationRequest) (WorkflowManualAnalysis, error) {
	workflowID := strings.TrimSpace(req.WorkflowID)
	if workflowID == "" {
		return WorkflowManualAnalysis{}, fmt.Errorf("workflow_id is required")
	}
	if len(req.RawYAML) == 0 {
		return WorkflowManualAnalysis{}, fmt.Errorf("workflow yaml is required")
	}
	wf, err := workflow.Load(req.RawYAML)
	if err != nil {
		return WorkflowManualAnalysis{}, err
	}

	actionSpecs := actionSpecSummaryMap(req.ActionSpecs)
	xOpsManual := workflowXOpsManual(wf)
	version := firstNonEmpty(req.WorkflowVersion, wf.Version)
	digest := firstNonEmpty(req.WorkflowDigest, DigestWorkflowYAML(string(req.RawYAML)))
	analysis := WorkflowManualAnalysis{
		WorkflowID:      workflowID,
		WorkflowVersion: version,
		WorkflowDigest:  digest,
		StorageURI:      strings.TrimSpace(req.StorageURI),
		Name:            firstNonEmpty(wf.Name, workflowID),
		Description:     strings.TrimSpace(wf.Description),
		ParameterRules:  map[string]ParameterRule{},
		Evidence:        map[string][]string{},
		XOpsManual:      cloneMap(xOpsManual),
		RecentRuns:      cloneRunRecords(req.RecentRuns),
	}

	analysis.Operation = inferWorkflowOperation(wf, xOpsManual, &analysis)
	analysis.Applicability = inferWorkflowApplicability(wf, xOpsManual)
	analysis.ParameterRules = workflowParameterRules(wf, actionSpecs, &analysis)
	analysis.RequiredContext.RequiredInputs = requiredInputsFromParameterRules(analysis.ParameterRules)
	analysis.Steps = summarizeWorkflowSteps(wf, actionSpecs, &analysis)
	analysis.GraphStages = summarizeWorkflowGraphStages(wf)
	analysis.ValidationHints = workflowValidationHints(wf, analysis)
	analysis.CannotUseHints = metadataStringSlice(xOpsManual, "cannot_use_when")
	analysis.Operation.RiskLevel = inferWorkflowRiskLevel(wf, analysis, actionSpecs)
	if analysis.Operation.RiskLevel == "" {
		analysis.Operation.RiskLevel = "medium"
	}
	analysis.RequiredContext.RequiredEvidence = metadataStringSlice(xOpsManual, "required_evidence")
	for _, input := range metadataStringSlice(xOpsManual, "required_inputs") {
		analysis.RequiredContext.RequiredInputs = appendUnique(analysis.RequiredContext.RequiredInputs, input)
	}
	for _, hint := range metadataStringSlice(xOpsManual, "validation") {
		analysis.ValidationHints = appendUnique(analysis.ValidationHints, hint)
	}
	return analysis, nil
}

func actionSpecSummaryMap(specs []ActionSpecSummary) map[string]ActionSpecSummary {
	out := make(map[string]ActionSpecSummary, len(specs))
	for _, spec := range specs {
		action := strings.TrimSpace(spec.Action)
		if action == "" {
			continue
		}
		spec.RequiredArgs = cloneStrings(spec.RequiredArgs)
		spec.Outputs = cloneStrings(spec.Outputs)
		out[action] = spec
	}
	return out
}

func workflowXOpsManual(wf workflow.Workflow) map[string]any {
	if wf.Extensions == nil {
		return nil
	}
	raw, ok := wf.Extensions["x_ops_manual"]
	if !ok {
		return nil
	}
	if typed, ok := raw.(map[string]any); ok {
		return typed
	}
	return nil
}

func inferWorkflowOperation(wf workflow.Workflow, meta map[string]any, analysis *WorkflowManualAnalysis) OperationProfile {
	op := OperationProfile{
		TargetType: metadataString(meta, "target_type"),
		Action:     metadataString(meta, "action"),
		RiskLevel:  metadataString(meta, "risk_level"),
	}
	if op.TargetType != "" && op.Action != "" {
		recordAnalysisEvidence(analysis, "operation", "x_ops_manual target_type/action")
		return op
	}
	text := workflowSemanticText(wf)
	lower := strings.ToLower(text)
	switch {
	case containsAny(lower, "postgresql", "postgres", "pg-") && strings.Contains(lower, "restore"):
		op.TargetType = firstNonEmpty(op.TargetType, "postgresql")
		op.Action = firstNonEmpty(op.Action, "restore")
		recordAnalysisEvidence(analysis, "operation", "workflow text mentions postgresql restore")
	case containsAny(lower, "postgresql", "postgres", "pg-") && containsAny(lower, "backup", "dump"):
		op.TargetType = firstNonEmpty(op.TargetType, "postgresql")
		op.Action = firstNonEmpty(op.Action, "backup")
		recordAnalysisEvidence(analysis, "operation", "workflow text mentions postgresql backup")
	case strings.Contains(lower, "mysql") && containsAny(lower, "backup", "dump"):
		op.TargetType = firstNonEmpty(op.TargetType, "mysql")
		op.Action = firstNonEmpty(op.Action, "backup")
		recordAnalysisEvidence(analysis, "operation", "workflow text mentions mysql backup")
	case strings.Contains(lower, "kubelet"):
		op.TargetType = firstNonEmpty(op.TargetType, "kubelet")
		op.Action = firstNonEmpty(op.Action, "repair")
		recordAnalysisEvidence(analysis, "operation", "workflow text mentions kubelet repair")
	case containsAny(lower, "dns", "tcp", "tls", "ssl", "http") && containsAny(lower, "probe", "inspect", "check"):
		op.TargetType = firstNonEmpty(op.TargetType, "network_service")
		op.Action = firstNonEmpty(op.Action, "inspect")
		recordAnalysisEvidence(analysis, "operation", "workflow text mentions network probes")
	case containsAny(lower, "incident", "itsm", "ticket", "chatops"):
		op.TargetType = firstNonEmpty(op.TargetType, "incident")
		op.Action = firstNonEmpty(op.Action, "create_or_notify")
		recordAnalysisEvidence(analysis, "operation", "workflow text mentions incident/chatops")
	case strings.Contains(lower, "redis") && containsAny(lower, "memory", "rca", "diagnos", "dry run", "策略"):
		op.TargetType = firstNonEmpty(op.TargetType, "redis")
		op.Action = firstNonEmpty(op.Action, "rca_or_repair")
		recordAnalysisEvidence(analysis, "operation", "workflow text mentions redis memory operation")
	}
	return op
}

func workflowSemanticText(wf workflow.Workflow) string {
	parts := []string{wf.Name, wf.Description, wf.Plan.Mode, wf.Plan.Strategy}
	for _, step := range wf.Steps {
		parts = append(parts, step.Name, step.Action, mapValuesText(step.Args))
	}
	for _, test := range wf.Tests {
		parts = append(parts, test.Name, test.Action, mapValuesText(test.Args))
	}
	if wf.XRunnerUI != nil {
		for _, node := range wf.XRunnerUI.Nodes {
			parts = append(parts, node.ID, node.Type, node.Label, node.Step, node.StepName)
		}
	}
	if wf.XRunnerGraph != nil {
		for _, node := range wf.XRunnerGraph.Nodes {
			parts = append(parts, node.ID, node.Type, node.Label, node.Step, node.StepName)
		}
	}
	return strings.Join(parts, " ")
}

func mapValuesText(values map[string]any) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for key, value := range values {
		parts = append(parts, key, fmt.Sprint(value))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func inferWorkflowApplicability(wf workflow.Workflow, meta map[string]any) ApplicabilityProfile {
	app := ApplicabilityProfile{
		Middleware:       metadataString(meta, "middleware"),
		ExecutionSurface: metadataStringSlice(meta, "execution_surface"),
		Platform:         metadataStringSlice(meta, "platform"),
		OS:               metadataStringSlice(meta, "os"),
		Topology:         metadataStringSlice(meta, "topology"),
	}
	text := strings.ToLower(workflowSemanticText(wf))
	if app.Middleware == "" {
		switch {
		case containsAny(text, "postgresql", "postgres", "pg-"):
			app.Middleware = "postgresql"
		case strings.Contains(text, "mysql"):
			app.Middleware = "mysql"
		case strings.Contains(text, "redis"):
			app.Middleware = "redis"
		case strings.Contains(text, "kubelet"):
			app.Middleware = "kubelet"
		}
	}
	if len(app.ExecutionSurface) == 0 {
		if containsAny(text, "ssh", "systemctl", "journalctl", "shell", "script.shell") {
			app.ExecutionSurface = []string{"ssh"}
		} else if containsAny(text, "http.request", "builtin.", "probe") {
			app.ExecutionSurface = []string{"runner"}
		}
	}
	if len(app.Platform) == 0 {
		if containsAny(text, "kube", "kubectl", "pod") {
			app.Platform = []string{"kubernetes"}
		} else if len(wf.Inventory.Hosts) > 0 {
			app.Platform = []string{"vm"}
		}
	}
	if len(app.OS) == 0 && strings.Contains(text, "ubuntu") {
		app.OS = []string{"ubuntu"}
	}
	return app
}

func workflowParameterRules(wf workflow.Workflow, actionSpecs map[string]ActionSpecSummary, analysis *WorkflowManualAnalysis) map[string]ParameterRule {
	rules := map[string]ParameterRule{}
	for key, value := range wf.Vars {
		name := strings.TrimSpace(key)
		if name == "" {
			continue
		}
		rule := ParameterRule{Source: "workflow_var"}
		switch typed := value.(type) {
		case map[string]any:
			if required, ok := typed["required"].(bool); ok {
				rule.Required = required
			}
			if validation, ok := typed["validation"].(string); ok {
				rule.Validation = validation
			}
			if def, ok := typed["default"]; ok && !isSensitiveParameterKey(name) {
				rule.DefaultValue = def
			} else if ok {
				analysis.SecretFindings = append(analysis.SecretFindings, WorkflowSecretFinding{Field: name, Kind: "workflow_var", HasDefault: true, Evidence: "sensitive workflow var default"})
			}
		default:
			if !isSensitiveParameterKey(name) {
				rule.DefaultValue = value
			} else {
				analysis.SecretFindings = append(analysis.SecretFindings, WorkflowSecretFinding{Field: name, Kind: "workflow_var", HasDefault: true, Evidence: "sensitive workflow var scalar value"})
			}
		}
		rules[name] = rule
	}
	for _, step := range wf.Steps {
		if spec, ok := actionSpecs[strings.TrimSpace(step.Action)]; ok {
			for _, arg := range spec.RequiredArgs {
				arg = strings.TrimSpace(arg)
				if arg == "" {
					continue
				}
				if _, exists := rules[arg]; !exists {
					rules[arg] = ParameterRule{Source: "action_spec:" + spec.Action, Required: true}
				}
			}
		}
		for _, must := range step.MustVars {
			must = strings.TrimSpace(must)
			if must == "" {
				continue
			}
			rule := rules[must]
			rule.Source = firstNonEmpty(rule.Source, "workflow_step_must_var")
			rule.Required = true
			rules[must] = rule
		}
		collectSecretFindings("steps."+step.Name+".args", step.Args, analysis)
	}
	return rules
}

func requiredInputsFromParameterRules(rules map[string]ParameterRule) []string {
	out := []string{}
	for name, rule := range rules {
		if rule.Required {
			out = appendUnique(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func summarizeWorkflowSteps(wf workflow.Workflow, actionSpecs map[string]ActionSpecSummary, analysis *WorkflowManualAnalysis) []WorkflowStepSummary {
	out := make([]WorkflowStepSummary, 0, len(wf.Steps))
	for _, step := range wf.Steps {
		stage := inferStage(step.Name, step.Action, mapValuesText(step.Args))
		specRisk := ""
		if spec, ok := actionSpecs[strings.TrimSpace(step.Action)]; ok {
			specRisk = spec.Risk
		}
		risk := workflowStepRisk(step, specRisk)
		summary := WorkflowStepSummary{
			Name:       strings.TrimSpace(step.Name),
			Action:     strings.TrimSpace(step.Action),
			Targets:    cloneStrings(step.Targets),
			MustVars:   cloneStrings(step.MustVars),
			ExpectVars: cloneStrings(step.ExpectVars),
			ReadOnly:   stage == "precheck" || strings.EqualFold(risk, "read_only"),
			Risky:      riskLevelRank(risk) >= riskLevelRank("medium"),
			Stage:      stage,
			Evidence:   firstNonEmpty(step.Action, step.Name),
		}
		if summary.Stage != "" {
			recordAnalysisEvidence(analysis, "stage", summary.Name+" -> "+summary.Stage)
		}
		if actionRisk := workflowActionRiskSummary(step, risk); actionRisk.Action != "" {
			analysis.ActionRisks = append(analysis.ActionRisks, actionRisk)
		}
		out = append(out, summary)
	}
	return out
}

func summarizeWorkflowGraphStages(wf workflow.Workflow) []WorkflowGraphStageSummary {
	nodes := []workflow.GraphNodeSpec{}
	if wf.XRunnerUI != nil {
		nodes = append(nodes, wf.XRunnerUI.Nodes...)
	}
	if wf.XRunnerGraph != nil {
		nodes = append(nodes, wf.XRunnerGraph.Nodes...)
	}
	out := make([]WorkflowGraphStageSummary, 0, len(nodes))
	for _, node := range nodes {
		stage := inferStage(strings.Join([]string{node.ID, node.Label, node.Type, node.Step, node.StepName}, " "), "", "")
		if strings.TrimSpace(node.Type) == "manual_approval" {
			stage = "approval"
		}
		if stage == "" {
			continue
		}
		out = append(out, WorkflowGraphStageSummary{
			ID:       strings.TrimSpace(node.ID),
			Label:    strings.TrimSpace(node.Label),
			Type:     strings.TrimSpace(node.Type),
			StepName: firstNonEmpty(node.StepName, node.Step),
			Stage:    stage,
			Evidence: firstNonEmpty(node.Label, node.ID),
		})
	}
	if strings.EqualFold(strings.TrimSpace(wf.Plan.Mode), "manual-approve") {
		out = append(out, WorkflowGraphStageSummary{
			ID:       "plan.manual_approval",
			Label:    "manual approval",
			Type:     "manual_approval",
			Stage:    "approval",
			Evidence: "plan.mode=manual-approve",
		})
	}
	return out
}

func workflowValidationHints(wf workflow.Workflow, analysis WorkflowManualAnalysis) []string {
	hints := []string{}
	for _, test := range wf.Tests {
		hints = appendUnique(hints, firstNonEmpty(test.Name, test.Action, "workflow test"))
	}
	for _, step := range analysis.Steps {
		if len(step.ExpectVars) > 0 {
			hints = appendUnique(hints, "expect vars: "+strings.Join(step.ExpectVars, ", "))
		}
		if step.Stage == "validate" {
			hints = appendUnique(hints, firstNonEmpty(step.Name, step.Evidence))
		}
	}
	text := strings.ToLower(workflowSemanticText(wf))
	for _, marker := range []string{"pg_isready", "node ready", "kubelet_ready", "health endpoint", "status_code"} {
		if strings.Contains(text, marker) {
			hints = appendUnique(hints, marker)
		}
	}
	return hints
}

func inferWorkflowRiskLevel(wf workflow.Workflow, analysis WorkflowManualAnalysis, actionSpecs map[string]ActionSpecSummary) string {
	highest := "read_only"
	for _, step := range wf.Steps {
		specRisk := ""
		if spec, ok := actionSpecs[strings.TrimSpace(step.Action)]; ok {
			specRisk = spec.Risk
		}
		highest = maxRiskLevel(highest, workflowStepRisk(step, specRisk))
	}
	if strings.EqualFold(wf.Plan.Mode, "manual-approve") {
		highest = maxRiskLevel(highest, "medium")
	}
	if len(wf.Steps) == 0 {
		return ""
	}
	return highest
}

func workflowStepRisk(step workflow.Step, specRisk string) string {
	action := strings.TrimSpace(step.Action)
	text := strings.ToLower(step.Name + " " + action + " " + mapValuesText(step.Args))
	if strings.TrimSpace(specRisk) != "" {
		if strings.EqualFold(specRisk, "read_only") && stepTextLooksMutating(text) {
			return "high"
		}
		return strings.ToLower(strings.TrimSpace(specRisk))
	}
	if strings.HasPrefix(action, "builtin.") && !stepTextLooksMutating(text) {
		return "read_only"
	}
	if action == "script.shell" || action == "script.python" {
		if stage := inferStage(step.Name, step.Action, mapValuesText(step.Args)); stage == "precheck" && !stepTextLooksMutating(text) {
			return "read_only"
		}
		return "high"
	}
	if strings.Contains(action, "http.") {
		if strings.Contains(text, " method: get") || strings.Contains(text, " method get") || !containsAny(text, "post", "put", "patch", "delete") {
			return "read_only"
		}
		return "medium"
	}
	if stepTextLooksMutating(text) {
		return "high"
	}
	return "medium"
}

func workflowActionRiskSummary(step workflow.Step, risk string) WorkflowActionRiskSummary {
	text := strings.ToLower(step.Name + " " + step.Action + " " + mapValuesText(step.Args))
	if strings.TrimSpace(step.Action) == "" {
		return WorkflowActionRiskSummary{}
	}
	return WorkflowActionRiskSummary{
		Action:           strings.TrimSpace(step.Action),
		StepName:         strings.TrimSpace(step.Name),
		Risk:             risk,
		DataMutation:     stepTextLooksDataMutating(text),
		ServiceRestart:   stepTextLooksServiceRestart(text),
		RequiresApproval: riskLevelRank(risk) >= riskLevelRank("high"),
		Evidence:         firstNonEmpty(step.Name, step.Action),
	}
}

func inferStage(parts ...string) string {
	lower := strings.ToLower(strings.Join(parts, " "))
	switch {
	case containsAny(lower, "rollback", "revert"):
		return "rollback"
	case containsAny(lower, "dry run", "dry_run", "dry-run"):
		return "dry_run"
	case containsAny(lower, "approval", "approve", "审批"):
		return "approval"
	case containsAny(lower, "validate", "verify", "ready", "恢复验证", "pg_isready"):
		return "validate"
	case containsAny(lower, "execute", "执行", "restart", "restore", "repair"):
		return "execute"
	case containsAny(lower, "precheck", "pre-check", "probe", "check", "inspect", "diagnostic", "health", "capture", "dns", "tcp", "tls", "ssl"):
		return "precheck"
	default:
		return ""
	}
}

func stepTextLooksMutating(text string) bool {
	return stepTextLooksDataMutating(text) || stepTextLooksServiceRestart(text)
}

func stepTextLooksDataMutating(text string) bool {
	return containsAny(text, " rm ", "rm -", " mv ", "mv ", "tar -x", "pgbackrest", " restore", "delete", "post ", "put ", "patch ")
}

func stepTextLooksServiceRestart(text string) bool {
	return containsAny(text, "systemctl restart", "systemctl stop", "systemctl start", "service restart", "restart kubelet", "stop postgres", "start postgres")
}

func collectSecretFindings(prefix string, value any, analysis *WorkflowManualAnalysis) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			field := strings.Trim(prefix+"."+key, ".")
			if isSensitiveParameterKey(key) {
				analysis.SecretFindings = append(analysis.SecretFindings, WorkflowSecretFinding{
					Field:      field,
					Kind:       "arg",
					HasDefault: !isEmptyValue(nested),
					SecretRef:  strings.Contains(strings.ToLower(field), "secret_ref"),
					Evidence:   "sensitive argument key",
				})
			}
			collectSecretFindings(field, nested, analysis)
		}
	case []any:
		for i, nested := range typed {
			collectSecretFindings(fmt.Sprintf("%s[%d]", prefix, i), nested, analysis)
		}
	case string:
		lower := strings.ToLower(typed)
		if containsAny(lower, "secret_ref", "authorization", "api-token", "bearer ") {
			analysis.SecretFindings = append(analysis.SecretFindings, WorkflowSecretFinding{
				Field:      prefix,
				Kind:       "arg_value",
				HasDefault: true,
				SecretRef:  strings.Contains(lower, "secret_ref"),
				Evidence:   "sensitive value marker",
			})
		}
	}
}

func isEmptyValue(value any) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) == ""
	}
	return false
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func recordAnalysisEvidence(analysis *WorkflowManualAnalysis, key string, evidence string) {
	if analysis == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(evidence) == "" {
		return
	}
	if analysis.Evidence == nil {
		analysis.Evidence = map[string][]string{}
	}
	analysis.Evidence[key] = appendUnique(analysis.Evidence[key], evidence)
}

func cloneRunRecords(in []RunRecord) []RunRecord {
	if in == nil {
		return nil
	}
	out := make([]RunRecord, len(in))
	for i, record := range in {
		out[i] = cloneRunRecord(record)
	}
	return out
}
