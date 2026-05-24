package opsmanual

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const paramResolutionArtifactType = "ops_manual_param_resolution"

func (s *Service) ResolveOpsManualParams(req ResolveOpsManualParamsRequest) (ParamResolutionResult, error) {
	if s.repo == nil {
		return ParamResolutionResult{}, fmt.Errorf("manual repository is nil")
	}
	manualID := strings.TrimSpace(req.ManualID)
	if manualID == "" {
		return ParamResolutionResult{}, fmt.Errorf("manual_id is required")
	}
	manual, err := s.GetManual(manualID)
	if err != nil {
		return ParamResolutionResult{}, err
	}
	frame := req.OperationFrame
	if operationFrameEmptyValue(frame) && strings.TrimSpace(req.RequestText) != "" {
		frame = BuildOperationFrame(req.RequestText, req.Metadata)
	}
	frame = normalizeResolutionFrame(frame, manual, req.RequestText)
	workflowParams := workflowParamRequirementsFromMetadata(manual.Metadata["workflow_parameters"])
	requirements := BuildParamRequirements(manual, workflowParams)
	ledger := s.buildResolutionLedger(context.Background(), req, frame, requirements)
	result := ResolveParamsForManualWithHints(context.Background(), manual, frame, requirements, ledger, discoveryFromRequest(req.Metadata, s.discovery), s.hintProvider)
	result.ManualID = manual.ID
	result.WorkflowID = firstNonEmpty(strings.TrimSpace(req.WorkflowID), strings.TrimSpace(manual.WorkflowRef.WorkflowID))
	result.OpsManualFlowID = BuildOpsManualFlowIDFromMetadata(req.Metadata, result.ManualID, result.WorkflowID, frame)
	result.OperationFrame = frame
	result.ArtifactType = paramResolutionArtifactType
	result.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	if repo, ok := s.repo.(ParamResolutionEventRepository); ok {
		_ = repo.SaveParamResolutionEvent(ParamResolutionEvent{
			ID:              "param-resolution-" + time.Now().UTC().Format("20060102T150405.000000000Z"),
			SessionID:       metadataString(req.Metadata, "session_id"),
			TurnID:          metadataString(req.Metadata, "turn_id"),
			OpsManualFlowID: result.OpsManualFlowID,
			ManualID:        result.ManualID,
			WorkflowID:      result.WorkflowID,
			OperationFrame:  frame,
			Result:          result,
			CreatedAt:       result.CreatedAt,
		})
	}
	s.writeResolvedSessionFacts(context.Background(), req, requirements, result)
	return result, nil
}

func ResolveParamsForManual(ctx context.Context, manual OpsManual, frame OperationFrame, requirements []ParamRequirement, ledger OperationContextLedger, discovery ResourceDiscovery) ParamResolutionResult {
	return ResolveParamsForManualWithHints(ctx, manual, frame, requirements, ledger, discovery, nil)
}

func ResolveParamsForManualWithHints(ctx context.Context, manual OpsManual, frame OperationFrame, requirements []ParamRequirement, ledger OperationContextLedger, discovery ResourceDiscovery, hints HintProvider) ParamResolutionResult {
	registry := NewParamResolverRegistry(discovery, hints)
	resolved := map[string]ResolvedParam{}
	result := ParamResolutionResult{
		Status: ParamResolutionUnresolved,
		Graph:  buildParamResolutionGraph(requirements),
	}
	for i := range result.Graph.Nodes {
		node := &result.Graph.Nodes[i]
		if dependenciesUnresolved(node.Requirement, resolved) {
			node.Status = "waiting_dependency"
			continue
		}
		resolverResult, logs := registry.Resolve(ctx, ParamResolverRequest{
			Requirement:       node.Requirement,
			OperationFrame:    frame,
			Manual:            manual,
			Ledger:            ledger,
			AlreadyResolved:   resolved,
			ResourceDiscovery: discovery,
		})
		node.ResolverLog = logs
		candidates := dedupeParamCandidates(resolverResult.Candidates)
		switch {
		case len(candidates) == 1 && candidates[0].Confidence >= 0.85:
			param := ResolvedParam{
				ID:         node.Requirement.ID,
				Value:      candidates[0].Value,
				Source:     candidates[0].Source,
				Confidence: candidates[0].Confidence,
				Evidence:   candidates[0].Evidence,
			}
			node.Status = string(ParamResolutionResolved)
			node.Resolved = &param
			resolved[param.ID] = param
			result.ResolvedParams = append(result.ResolvedParams, param)
		case len(candidates) > 1:
			ambiguous := AmbiguousParam{ParamRequirement: node.Requirement, Reason: "multiple candidates", Candidates: candidates}
			node.Status = string(ParamResolutionAmbiguous)
			node.Ambiguous = &ambiguous
			result.AmbiguousParams = append(result.AmbiguousParams, ambiguous)
			result.Fields = append(result.Fields, formFieldFromAmbiguous(ambiguous))
		case len(candidates) > 0 && candidatesRequireCurrentConfirmation(candidates):
			missing := MissingParam{ParamRequirement: node.Requirement, Reason: "requires current confirmation/discovery", Candidates: candidates}
			node.Status = string(ParamResolutionNeedUserInput)
			node.Missing = &missing
			result.MissingParams = append(result.MissingParams, missing)
			result.Fields = append(result.Fields, formFieldFromMissing(missing))
		case node.Requirement.Required:
			missing := MissingParam{ParamRequirement: node.Requirement, Reason: firstNonEmpty(resolverResult.Message, "no candidate")}
			node.Status = string(ParamResolutionNeedUserInput)
			node.Missing = &missing
			result.MissingParams = append(result.MissingParams, missing)
			result.Fields = append(result.Fields, formFieldFromMissing(missing))
		default:
			node.Status = "skipped"
		}
	}
	switch {
	case len(result.AmbiguousParams) > 0:
		result.Status = ParamResolutionAmbiguous
		result.NextAction = "ask_user"
	case len(result.MissingParams) > 0:
		result.Status = ParamResolutionNeedUserInput
		result.NextAction = "ask_user"
	default:
		result.Status = ParamResolutionResolved
		result.NextAction = "run_preflight"
	}
	result.Fields = dedupeParamResolutionFormFields(result.Fields)
	return result
}

func candidatesRequireCurrentConfirmation(candidates []ParamCandidate) bool {
	if len(candidates) == 0 {
		return false
	}
	for _, candidate := range candidates {
		if candidate.Metadata == nil || candidate.Metadata["requires_current_confirmation"] != true {
			return false
		}
	}
	return true
}

func buildParamResolutionGraph(requirements []ParamRequirement) ParamResolutionGraph {
	graph := ParamResolutionGraph{Nodes: make([]ParamResolutionNode, 0, len(requirements))}
	seen := map[string]bool{}
	requirementIDs := map[string]bool{}
	for _, requirement := range requirements {
		if strings.TrimSpace(requirement.ID) != "" {
			requirementIDs[strings.TrimSpace(requirement.ID)] = true
		}
	}
	for _, requirement := range requirements {
		if requirement.ID == "" || seen[requirement.ID] {
			continue
		}
		seen[requirement.ID] = true
		if len(requirement.DependsOn) == 0 {
			requirement.DependsOn = defaultParamDependencies(requirement, requirementIDs)
		}
		node := ParamResolutionNode{
			ID:           requirement.ID,
			Requirement:  requirement,
			Status:       string(ParamResolutionUnresolved),
			Dependencies: cloneStrings(requirement.DependsOn),
		}
		graph.Nodes = append(graph.Nodes, node)
		for _, dep := range requirement.DependsOn {
			graph.Edges = append(graph.Edges, ParamResolutionEdge{From: dep, To: requirement.ID})
		}
	}
	return graph
}

func defaultParamDependencies(req ParamRequirement, requirementIDs map[string]bool) []string {
	if req.ID == "target_host" {
		return nil
	}
	switch NormalizeParamType(req.ID, req.Type) {
	case "resource_ref":
		return []string{"target_host"}
	case "execution_surface":
		if requirementIDs["target_instance"] {
			return []string{"target_host", "target_instance"}
		}
		return []string{"target_host"}
	default:
		return nil
	}
}

func dependenciesUnresolved(req ParamRequirement, resolved map[string]ResolvedParam) bool {
	for _, dep := range req.DependsOn {
		if strings.TrimSpace(dep) == "" {
			continue
		}
		if _, ok := resolved[dep]; !ok {
			return true
		}
	}
	return false
}

func formFieldFromMissing(missing MissingParam) ParamResolutionFormField {
	placeholder := placeholderForRequirement(missing.ParamRequirement)
	if strings.TrimSpace(missing.Reason) != "" && strings.TrimSpace(missing.Reason) != "no candidate" {
		placeholder = missing.Reason
	}
	return ParamResolutionFormField{
		ID:          missing.ID,
		Label:       firstNonEmpty(missing.Label, DefaultParamLabel(missing.ID)),
		Type:        NormalizeParamType(missing.ID, missing.Type),
		Required:    missing.Required,
		Sensitive:   missing.Sensitive,
		UIControl:   firstNonEmpty(missing.UIControl, DefaultParamUIControl(missing.ParamRequirement)),
		Placeholder: placeholder,
		Default:     missing.DefaultValue,
		Candidates:  cloneParamCandidates(missing.Candidates),
	}
}

func formFieldFromAmbiguous(ambiguous AmbiguousParam) ParamResolutionFormField {
	field := formFieldFromMissing(MissingParam{ParamRequirement: ambiguous.ParamRequirement})
	field.Candidates = cloneParamCandidates(ambiguous.Candidates)
	field.UIControl = "select"
	return field
}

func placeholderForRequirement(req ParamRequirement) string {
	switch NormalizeParamType(req.ID, req.Type) {
	case "host_ref":
		return "留空使用当前选择主机"
	case "resource_ref":
		return "选择或填写目标实例"
	case "path":
		return "例如 /data/backups"
	default:
		return ""
	}
}

func normalizeResolutionFrame(frame OperationFrame, manual OpsManual, text string) OperationFrame {
	if frame.RawText == "" {
		frame.RawText = text
	}
	if frame.ObjectType == "" {
		frame.ObjectType = firstNonEmpty(frame.Target.Type, manual.Operation.TargetType, manual.Applicability.Middleware)
	}
	if frame.Target.Type == "" {
		frame.Target.Type = frame.ObjectType
	}
	if frame.Operation.TargetType == "" {
		frame.Operation.TargetType = frame.ObjectType
	}
	if frame.OperationType == "" {
		frame.OperationType = firstNonEmpty(frame.Operation.Action, manual.Operation.Action)
	}
	if frame.Operation.Action == "" {
		frame.Operation.Action = frame.OperationType
	}
	return frame
}

func buildResolutionLedger(req ResolveOpsManualParamsRequest, frame OperationFrame) OperationContextLedger {
	ledger := NewOperationContextLedger()
	ledger.Merge(LedgerFromOperationFrame(frame))
	ledger.Merge(LedgerFromKnownParams(req.KnownParams, "user_form"))
	if host := metadataString(req.Metadata, "aiops.target.hostId"); host != "" {
		ledger.AddFact(OperationContextFact{Key: "target_host", Value: host, Source: "tool_execution_host", Confidence: 0.9})
	}
	if host := firstNonEmpty(metadataString(req.Metadata, "selected_host"), metadataString(req.Metadata, "current_host")); host != "" {
		ledger.AddFact(OperationContextFact{Key: "target_host", Value: host, Source: "selected_host", Confidence: 0.95})
	}
	if backupPath := extractBackupPath(firstNonEmpty(req.RequestText, frame.RawText)); backupPath != "" {
		ledger.AddFact(OperationContextFact{Key: "backup_path", Value: backupPath, Source: "conversation", Confidence: 0.78})
	}
	return ledger
}

func (s *Service) buildResolutionLedger(ctx context.Context, req ResolveOpsManualParamsRequest, frame OperationFrame, requirements []ParamRequirement) OperationContextLedger {
	ledger := buildResolutionLedger(req, frame)
	if s == nil || s.sessionContext == nil {
		return ledger
	}
	sessionID := metadataString(req.Metadata, "session_id")
	if strings.TrimSpace(sessionID) == "" {
		return ledger
	}
	facts, err := s.sessionContext.ListFacts(ctx, sessionID, SessionOpsFactFilter{Now: time.Now().UTC()})
	if err != nil {
		return ledger
	}
	ledger.Merge(LedgerFromSessionOpsFacts(facts, requirements, frame))
	return ledger
}

func LedgerFromSessionOpsFacts(facts []SessionOpsFact, requirements []ParamRequirement, frame OperationFrame) OperationContextLedger {
	ledger := NewOperationContextLedger()
	relevant := relevantSessionFactKeys(requirements, frame)
	for _, fact := range facts {
		if !sessionFactRelevant(fact, relevant) {
			continue
		}
		if sessionFactAutoResolveBlocked(fact.Key) {
			continue
		}
		converted, ok := operationContextFactFromSessionFact(fact)
		if !ok {
			continue
		}
		ledger.AddFact(converted)
	}
	return ledger
}

func relevantSessionFactKeys(requirements []ParamRequirement, frame OperationFrame) map[string]bool {
	keys := map[string]bool{}
	for _, req := range requirements {
		id := strings.TrimSpace(req.ID)
		if id == "" {
			continue
		}
		keys[id] = true
		switch NormalizeParamType(req.ID, req.Type) {
		case "host_ref":
			keys[SessionOpsFactTargetHost] = true
		case "resource_ref":
			keys[SessionOpsFactTargetInstance] = true
			switch strings.TrimSpace(firstNonEmpty(frame.ObjectType, frame.Target.Type, frame.Operation.TargetType)) {
			case "redis":
				keys["redis_instance"] = true
			case "postgresql":
				keys["pg_instance"] = true
			case "mysql":
				keys["mysql_instance"] = true
			}
		}
	}
	return keys
}

func sessionFactRelevant(fact SessionOpsFact, relevant map[string]bool) bool {
	key := strings.TrimSpace(fact.Key)
	return key != "" && relevant[key]
}

func sessionFactAutoResolveBlocked(key string) bool {
	switch strings.TrimSpace(key) {
	case "backup_path", "approval_reason", "change_window", "delete_object", "restore_target":
		return true
	default:
		return false
	}
}

func operationContextFactFromSessionFact(fact SessionOpsFact) (OperationContextFact, bool) {
	key := strings.TrimSpace(fact.Key)
	if key == "" {
		return OperationContextFact{}, false
	}
	value := fact.Value
	if fact.Sensitive {
		if strings.TrimSpace(fact.SecretRef) == "" || !fact.ConfirmedByUser {
			return OperationContextFact{}, false
		}
		value = fact.SecretRef
	}
	if !valuePresent(value) {
		return OperationContextFact{}, false
	}
	confidence := 0.74
	if fact.ConfirmedByUser {
		confidence = 0.87
	}
	if fact.Confidence > 0 && fact.Confidence < confidence {
		confidence = fact.Confidence
	}
	return OperationContextFact{
		Key:             key,
		Value:           value,
		Source:          "session_fact",
		Confidence:      confidence,
		ConfirmedByUser: fact.ConfirmedByUser,
		Sensitive:       fact.Sensitive,
		CreatedAt:       fact.CreatedAt.Format(time.RFC3339),
	}, true
}

func (s *Service) writeResolvedSessionFacts(ctx context.Context, req ResolveOpsManualParamsRequest, requirements []ParamRequirement, result ParamResolutionResult) {
	if s == nil || s.sessionContext == nil || result.Status != ParamResolutionResolved {
		return
	}
	sessionID := metadataString(req.Metadata, "session_id")
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	requirementsByID := map[string]ParamRequirement{}
	for _, requirement := range requirements {
		requirementsByID[requirement.ID] = requirement
	}
	now := time.Now().UTC()
	for _, param := range result.ResolvedParams {
		requirement := requirementsByID[param.ID]
		if requirement.Sensitive || NormalizeParamType(requirement.ID, requirement.Type) == "secret_ref" {
			if ref := secretRefFromResolvedParam(param); ref != "" {
				_ = s.sessionContext.UpsertFact(ctx, sessionID, SessionOpsFact{
					Key:             param.ID,
					Value:           nil,
					Source:          "user_form",
					Confidence:      1,
					ConfirmedByUser: true,
					Sensitive:       true,
					SecretRef:       ref,
					ExpiresAt:       now.Add(2 * time.Hour),
					CreatedAt:       now,
					UpdatedAt:       now,
				})
			}
			continue
		}
		key := sessionFactKeyForResolvedParam(param.ID)
		if strings.TrimSpace(key) == "" || !valuePresent(param.Value) {
			continue
		}
		source, ttl, confirmed := sessionWritebackPolicy(param.Source)
		_ = s.sessionContext.UpsertFact(ctx, sessionID, SessionOpsFact{
			Key:             key,
			Value:           param.Value,
			Source:          source,
			Confidence:      param.Confidence,
			ConfirmedByUser: confirmed,
			EvidenceRef:     param.Evidence,
			ExpiresAt:       now.Add(ttl),
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}
}

func sessionFactKeyForResolvedParam(paramID string) string {
	switch strings.TrimSpace(paramID) {
	case "target_host":
		return SessionOpsFactTargetHost
	case "target_instance", "redis_instance", "pg_instance", "mysql_instance":
		return SessionOpsFactTargetInstance
	default:
		return strings.TrimSpace(paramID)
	}
}

func sessionWritebackPolicy(source string) (string, time.Duration, bool) {
	switch strings.TrimSpace(source) {
	case "user", "user_form", "known_params":
		return "user_form", 2 * time.Hour, true
	case "operation_frame":
		return "operation_frame", 2 * time.Hour, false
	case "resource_discovery", "docker", "k8s", "host_readonly", "coroot":
		return "resource_discovery", 15 * time.Minute, false
	default:
		return "param_resolution", 30 * time.Minute, false
	}
}

func secretRefFromResolvedParam(param ResolvedParam) string {
	if ref := metadataString(param.Metadata, "secret_ref"); ref != "" {
		return ref
	}
	value := strings.TrimSpace(fmt.Sprint(param.Value))
	if strings.HasPrefix(value, "secret://") || strings.HasPrefix(value, "secret_ref:") {
		return value
	}
	return ""
}

func operationFrameEmptyValue(frame OperationFrame) bool {
	return strings.TrimSpace(frame.RawText) == "" &&
		strings.TrimSpace(frame.ObjectType) == "" &&
		strings.TrimSpace(frame.Target.Type) == "" &&
		strings.TrimSpace(frame.Target.Name) == "" &&
		strings.TrimSpace(frame.Operation.Action) == "" &&
		strings.TrimSpace(frame.Operation.TargetType) == "" &&
		len(frame.TargetScope.Hosts) == 0 &&
		len(frame.RequiredParams) == 0 &&
		len(frame.Metadata) == 0
}

func workflowParamRequirementsFromMetadata(raw any) []ParamRequirement {
	return paramRequirementsFromMetadata(raw)
}

func dedupeParamResolutionFormFields(fields []ParamResolutionFormField) []ParamResolutionFormField {
	out := []ParamResolutionFormField{}
	seen := map[string]bool{}
	for _, field := range fields {
		if field.ID == "" || seen[field.ID] {
			continue
		}
		seen[field.ID] = true
		out = append(out, field)
	}
	return out
}

type metadataResourceDiscovery struct {
	resources []ResourceCandidate
	surfaces  []ParamCandidate
}

func metadataDiscoveryFromMap(meta map[string]any) ResourceDiscovery {
	if meta == nil {
		return noopResourceDiscovery{}
	}
	return metadataResourceDiscovery{
		resources: resourceCandidatesFromAny(meta["resource_candidates"]),
		surfaces:  paramCandidatesFromAny(meta["execution_surface_candidates"]),
	}
}

func discoveryFromRequest(meta map[string]any, fallback ResourceDiscovery) ResourceDiscovery {
	if metadataHasDiscovery(meta) {
		return metadataDiscoveryFromMap(meta)
	}
	if fallback != nil {
		return fallback
	}
	return noopResourceDiscovery{}
}

func metadataHasDiscovery(meta map[string]any) bool {
	if meta == nil {
		return false
	}
	return hasMetadataList(meta["resource_candidates"]) || hasMetadataList(meta["execution_surface_candidates"])
}

func hasMetadataList(raw any) bool {
	switch typed := raw.(type) {
	case []any:
		return len(typed) > 0
	case []ResourceCandidate:
		return len(typed) > 0
	case []ParamCandidate:
		return len(typed) > 0
	default:
		return false
	}
}

func (d metadataResourceDiscovery) DiscoverHostResources(context.Context, string) ([]ResourceCandidate, error) {
	return append([]ResourceCandidate(nil), d.resources...), nil
}

func (d metadataResourceDiscovery) DiscoverExecutionSurfaces(context.Context, string) ([]ParamCandidate, error) {
	out := cloneParamCandidates(d.surfaces)
	for _, resource := range d.resources {
		surface := strings.TrimSpace(resource.Surface)
		if surface == "" {
			continue
		}
		out = append(out, ParamCandidate{
			Value:      surface,
			Label:      surface,
			Source:     firstNonEmpty(resource.Source, "resource_discovery"),
			Confidence: resource.Confidence,
			Evidence:   firstNonEmpty(resource.Evidence, "read-only resource discovery"),
		})
	}
	return dedupeParamCandidates(out), nil
}

func resourceCandidatesFromAny(raw any) []ResourceCandidate {
	items, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]ResourceCandidate); ok {
			return append([]ResourceCandidate(nil), typed...)
		}
		return nil
	}
	out := []ResourceCandidate{}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, ResourceCandidate{
			ID:         metadataStringFromMap(m, "id", "value"),
			Name:       metadataStringFromMap(m, "name"),
			Type:       metadataStringFromMap(m, "type"),
			Host:       metadataStringFromMap(m, "host"),
			Surface:    metadataStringFromMap(m, "surface"),
			Source:     metadataStringFromMap(m, "source"),
			Evidence:   metadataStringFromMap(m, "evidence"),
			Confidence: metadataFloat(m, "confidence"),
		})
	}
	return out
}

func paramCandidatesFromAny(raw any) []ParamCandidate {
	items, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]ParamCandidate); ok {
			return cloneParamCandidates(typed)
		}
		return nil
	}
	out := []ParamCandidate{}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, ParamCandidate{
			Value:      firstAny(m["value"], m["id"]),
			Label:      metadataStringFromMap(m, "label"),
			Source:     metadataStringFromMap(m, "source"),
			Confidence: metadataFloat(m, "confidence"),
			Evidence:   metadataStringFromMap(m, "evidence"),
		})
	}
	return out
}

func metadataStringFromMap(m map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := m[key]
		if !ok || raw == nil {
			continue
		}
		if value := strings.TrimSpace(fmt.Sprint(raw)); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}
