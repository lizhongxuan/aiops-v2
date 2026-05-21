package opsmanual

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const preflightArtifactType = "ops_manual_preflight_result"

func (s *Service) RunPreflight(req PreflightRequest) (PreflightResult, error) {
	if s.repo == nil {
		return PreflightResult{}, fmt.Errorf("manual repository is nil")
	}
	manualID := strings.TrimSpace(req.ManualID)
	if manualID == "" {
		return PreflightResult{}, fmt.Errorf("manual_id is required")
	}
	manual, err := s.GetManual(manualID)
	if err != nil {
		return PreflightResult{}, err
	}
	result := basePreflightResult(manual, req)
	probe := effectivePreflightProbe(manual)
	result.ProbeID = strings.TrimSpace(probe.ID)
	result.EnvironmentDiffs = environmentDiffsForManual(manual, req.OperationFrame)
	if len(result.EnvironmentDiffs) > 0 {
		result.Status = PreflightStatusBlocked
		result.Ready = false
		result.Reason = "operation environment differs from manual applicability"
		result.NextAction = "generate_workflow_variant"
		return result, nil
	}
	missingParams := missingPreflightParams(manual, req)
	if len(missingParams) > 0 {
		result.Status = PreflightStatusBlocked
		result.Ready = false
		result.Reason = "required parameters missing: " + strings.Join(missingParams, ", ")
		result.NextAction = "collect_required_context"
		return result, nil
	}
	if probe.ID == "" && len(probe.RequiredOutputs) == 0 {
		result.Status = PreflightStatusNotApplicable
		result.Ready = true
		result.Reason = "manual has no preflight probe"
		result.NextAction = "confirm_execution"
		return s.applyWorkflowPlanCheck(context.Background(), result, manual, req), nil
	}
	if truthyParam(req.Parameters, "simulate_permission_missing") {
		result.Status = PreflightStatusBlocked
		result.Ready = false
		result.Reason = "preflight probe permission is missing"
		result.MissingPermissions = appendUnique(result.MissingPermissions, firstNonEmpty(probe.ID, "preflight_probe"))
		result.NextAction = "request_permission"
		return result, nil
	}
	if truthyParam(req.Parameters, "simulate_provider_unavailable") {
		result.Status = PreflightStatusBlocked
		result.Ready = false
		result.Reason = "preflight provider is unavailable"
		result.NextAction = "fix_provider"
		result.Evidence = append(result.Evidence, PreflightEvidence{Name: "provider_available", Status: "failed", Value: false, Note: "provider unavailable"})
		return result, nil
	}
	if truthyParam(req.Parameters, "simulate_target_missing") {
		result.Status = PreflightStatusBlocked
		result.Ready = false
		result.Reason = "target instance is not reachable"
		result.NextAction = "fallback_guide"
		result.Evidence = append(result.Evidence, PreflightEvidence{Name: "target_reachable", Status: "failed", Value: false})
		return result, nil
	}
	result.Status = PreflightStatusPassed
	result.Ready = true
	result.NextAction = "confirm_execution"
	outputs := probe.RequiredOutputs
	if len(outputs) == 0 {
		outputs = []string{firstNonEmpty(probe.ID, "preflight_probe")}
	}
	for _, output := range outputs {
		result.Evidence = append(result.Evidence, PreflightEvidence{
			Name:   strings.TrimSpace(output),
			Status: "passed",
			Value:  true,
		})
	}
	return s.applyWorkflowPlanCheck(context.Background(), result, manual, req), nil
}

func basePreflightResult(manual OpsManual, req PreflightRequest) PreflightResult {
	workflowID := firstNonEmpty(strings.TrimSpace(req.WorkflowID), strings.TrimSpace(manual.WorkflowRef.WorkflowID))
	flowID := strings.TrimSpace(req.OpsManualFlowID)
	if flowID == "" {
		flowID = BuildOpsManualFlowID(OpsManualFlowIDInput{
			ManualID:       manual.ID,
			WorkflowID:     workflowID,
			OperationFrame: req.OperationFrame,
		})
	}
	return PreflightResult{
		Status:          PreflightStatusUnknown,
		OpsManualFlowID: flowID,
		Ready:           false,
		ManualID:        manual.ID,
		WorkflowID:      workflowID,
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
		ArtifactType:    preflightArtifactType,
	}
}

func effectivePreflightProbe(manual OpsManual) PreflightProbe {
	probe := manual.PreflightProbe
	if probe.ID != "" || probe.Type != "" || probe.Action != "" || len(probe.RequiredOutputs) > 0 {
		return probe
	}
	raw, ok := manual.Metadata["preflight_probe"]
	if !ok {
		return PreflightProbe{}
	}
	rawMap, ok := raw.(map[string]any)
	if !ok {
		return PreflightProbe{}
	}
	return PreflightProbe{
		ID:              strings.TrimSpace(fmt.Sprint(firstAny(rawMap["id"], rawMap["probe_id"]))),
		Type:            strings.TrimSpace(fmt.Sprint(rawMap["type"])),
		Action:          strings.TrimSpace(fmt.Sprint(rawMap["action"])),
		ReadOnly:        metadataBool(rawMap, "read_only"),
		TimeoutSeconds:  int(metadataFloat(rawMap, "timeout_seconds")),
		RequiredOutputs: metadataStringSliceFromAny(firstAny(rawMap["required_outputs"], rawMap["requiredOutputs"])),
	}
}

func (s *Service) applyWorkflowPlanCheck(ctx context.Context, result PreflightResult, manual OpsManual, req PreflightRequest) PreflightResult {
	if s == nil || s.workflowPlanChecker == nil || strings.TrimSpace(result.WorkflowID) == "" {
		return result
	}
	plan, err := s.workflowPlanChecker.CheckWorkflowPlan(ctx, WorkflowPlanCheckRequest{
		Manual:         manual,
		WorkflowID:     result.WorkflowID,
		OperationFrame: req.OperationFrame,
		Parameters:     req.Parameters,
		RequestedBy:    req.RequestedBy,
		TriggeredBy:    req.TriggeredBy,
	})
	if err != nil {
		result.Status = PreflightStatusBlocked
		result.Ready = false
		result.Reason = "workflow plan check failed: " + err.Error()
		result.NextAction = "fallback_guide"
		return result
	}
	result.WorkflowDigest = firstNonEmpty(strings.TrimSpace(plan.WorkflowDigest), strings.TrimSpace(manual.WorkflowRef.WorkflowDigest))
	result.ExecutionPlan = PreflightExecutionPlan{
		Summary:          strings.TrimSpace(plan.Summary),
		WorkflowStatus:   strings.TrimSpace(plan.WorkflowStatus),
		TargetHosts:      cloneStrings(plan.TargetHosts),
		ActionsUsed:      cloneStrings(plan.ActionsUsed),
		RequiresApproval: plan.RequiresApproval,
		RiskLevel:        firstNonEmpty(strings.TrimSpace(plan.RiskLevel), strings.TrimSpace(manual.Operation.RiskLevel)),
		Warnings:         append([]PreflightPlanWarning(nil), plan.Warnings...),
	}
	planStatus := strings.TrimSpace(plan.Status)
	if strings.EqualFold(planStatus, "blocked") || strings.EqualFold(planStatus, "failed") {
		result.Status = PreflightStatusBlocked
		result.Ready = false
		result.Reason = firstNonEmpty(strings.TrimSpace(plan.Summary), "workflow plan check did not pass")
		result.NextAction = "fallback_guide"
		return result
	}
	if plan.RequiresApproval || manual.RunnableConditions.RequiresApproval || riskLevelRank(firstNonEmpty(plan.RiskLevel, manual.Operation.RiskLevel)) >= riskLevelRank("high") {
		result.NextAction = "request_approval"
	}
	return result
}

func missingPreflightParams(manual OpsManual, req PreflightRequest) []string {
	required := cloneStrings(manual.RunnableConditions.RequiredParams)
	if len(required) == 0 {
		required = cloneStrings(manual.RequiredContext.RequiredInputs)
	}
	missing := []string{}
	for _, name := range required {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !preflightParamAvailable(name, req) {
			missing = appendUnique(missing, name)
		}
	}
	return missing
}

func preflightParamAvailable(name string, req PreflightRequest) bool {
	if name == "target_instance" {
		if strings.TrimSpace(req.OperationFrame.Target.Name) != "" {
			return true
		}
		if len(req.OperationFrame.TargetScope.Hosts) > 0 {
			return true
		}
	}
	if valuePresent(req.Parameters[name]) {
		return true
	}
	if valuePresent(req.OperationFrame.RequiredParams[name]) {
		return true
	}
	if valuePresent(req.OperationFrame.Metadata[name]) {
		return true
	}
	return false
}

func valuePresent(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	default:
		return true
	}
}

func truthyParam(params map[string]any, key string) bool {
	raw, ok := params[key]
	if !ok {
		return false
	}
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "y", "on":
			return true
		default:
			return false
		}
	default:
		return fmt.Sprint(typed) == "1"
	}
}

func metadataBool(meta map[string]any, key string) bool {
	raw, ok := meta[key]
	if !ok {
		return false
	}
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}
