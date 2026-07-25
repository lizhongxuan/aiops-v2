package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"aiops-v2/internal/workfloweditor"
	"runner/server/service"
	edit "runner/workflow/edit"
	"runner/workflow/visual"
)

const runnerStudioWorkflowAIPrefix = "/api/runner-studio/workflow-ai/"

var retiredRunnerStudioWorkflowAIActions = map[string]struct{}{
	"sessions": {}, "step": {}, "describe": {}, "plan": {}, "patch": {},
	"effect": {}, "undo": {}, "manual-candidate": {}, "manual-update": {}, "create-draft": {},
}

var compatibleRunnerStudioWorkflowAIActions = map[string]struct{}{
	"snapshot": {}, "validate": {}, "preview": {}, "apply": {},
}

type workflowLegacyCompatibilityApplication interface {
	ReadLegacySnapshot(context.Context, edit.WorkflowKey) (service.WorkflowLegacySnapshotResult, error)
	ValidateLegacyPatch(context.Context, service.WorkflowLegacyPatchRequest) (service.WorkflowLegacyPatchValidationResult, error)
	PreviewLegacyPatch(context.Context, service.WorkflowLegacyPatchRequest) (service.WorkflowEditPreviewResult, error)
	ApplyLegacyPatch(context.Context, service.WorkflowLegacyPatchRequest) (service.WorkflowManualCommandResult, error)
}

func (s *HTTPServer) handleRunnerStudioWorkflowAI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWorkflowCommandError(w, http.StatusMethodNotAllowed, edit.ErrorSchemaViolation, "method not allowed", "send a POST request")
		return
	}
	action := strings.Trim(strings.TrimPrefix(r.URL.EscapedPath(), runnerStudioWorkflowAIPrefix), "/")
	if _, retired := retiredRunnerStudioWorkflowAIActions[action]; retired {
		writeLegacyWorkflowMigrationRequired(w)
		return
	}
	if _, compatible := compatibleRunnerStudioWorkflowAIActions[action]; !compatible {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner studio workflow ai endpoint not found"})
		return
	}
	application, ok := s.workflowCommandApplication.(workflowLegacyCompatibilityApplication)
	if !ok || application == nil || s.workflowCommandScopeResolver == nil {
		if s.workflowEditor != nil {
			// An explicitly injected legacy service must never revive its separate
			// memory/local store as a compatibility fallback.
			writeLegacyWorkflowMigrationRequired(w)
			return
		}
		writeWorkflowCommandError(w, http.StatusServiceUnavailable, edit.ErrorMissingConfiguration, "workflow compatibility application is not configured", "retry after the Runner application is available")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWorkflowCommandBodyBytes)
	switch action {
	case "snapshot":
		s.handleLegacyWorkflowSnapshot(w, r, application)
	case "validate":
		s.handleLegacyWorkflowValidate(w, r, application)
	case "preview":
		s.handleLegacyWorkflowPreview(w, r, application)
	case "apply":
		s.handleLegacyWorkflowApply(w, r, application)
	}
}

func (s *HTTPServer) handleLegacyWorkflowSnapshot(w http.ResponseWriter, r *http.Request, application workflowLegacyCompatibilityApplication) {
	var request workfloweditor.GetSnapshotRequest
	if !decodeLegacyWorkflowRequest(w, r, &request) {
		return
	}
	key, _, ok := s.resolveLegacyWorkflowScope(w, r, request.WorkflowID)
	if !ok {
		return
	}
	result, err := application.ReadLegacySnapshot(r.Context(), key)
	if err != nil {
		writeLegacyWorkflowApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, legacyWorkflowSnapshot(result))
}

func (s *HTTPServer) handleLegacyWorkflowValidate(w http.ResponseWriter, r *http.Request, application workflowLegacyCompatibilityApplication) {
	var request workfloweditor.ValidatePatchRequest
	if !decodeLegacyWorkflowRequest(w, r, &request) {
		return
	}
	legacy, ok := s.legacyWorkflowPatchRequest(w, r, request.WorkflowID, request.BaseRevision, request.Patch, request.Patch.ID, firstLegacyReason(request.Patch.Reason, request.Patch.Summary, "validate legacy workflow patch"), false, "")
	if !ok {
		return
	}
	if request.AllowFullGraphReplacement || request.SecondConfirmationID != "" {
		writeLegacyWorkflowSchemaError(w, "full graph replacement is not supported by the compatibility adapter")
		return
	}
	result, err := application.ValidateLegacyPatch(r.Context(), legacy)
	if err != nil {
		writeLegacyWorkflowApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, legacyWorkflowValidation(result.Validation))
}

func (s *HTTPServer) handleLegacyWorkflowPreview(w http.ResponseWriter, r *http.Request, application workflowLegacyCompatibilityApplication) {
	var request workfloweditor.PreviewPatchRequest
	if !decodeLegacyWorkflowRequest(w, r, &request) {
		return
	}
	legacy, ok := s.legacyWorkflowPatchRequest(w, r, request.WorkflowID, request.BaseRevision, request.Patch, request.Patch.ID, firstLegacyReason(request.Patch.Reason, request.Patch.Summary, "preview legacy workflow patch"), false, "")
	if !ok {
		return
	}
	result, err := application.PreviewLegacyPatch(r.Context(), legacy)
	if err != nil {
		writeLegacyWorkflowApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workfloweditor.WorkflowPatchPreview{PatchID: request.Patch.ID, Graph: result.Graph, Effect: legacyWorkflowEffect(result)})
}

func (s *HTTPServer) handleLegacyWorkflowApply(w http.ResponseWriter, r *http.Request, application workflowLegacyCompatibilityApplication) {
	var request workfloweditor.ApplyPatchRequest
	if !decodeLegacyWorkflowRequest(w, r, &request) {
		return
	}
	if request.AllowFullGraphReplacement || request.SecondConfirmationID != "" {
		writeLegacyWorkflowSchemaError(w, "full graph replacement is not supported by the compatibility adapter")
		return
	}
	legacy, ok := s.legacyWorkflowPatchRequest(w, r, request.WorkflowID, request.BaseRevision, request.Patch, request.PatchID, request.Reason, strings.TrimSpace(request.UserConfirmationID) != "", request.DrawerSessionID)
	if !ok {
		return
	}
	if strings.TrimSpace(request.UserConfirmationID) == "" || strings.TrimSpace(request.UserConfirmationID) != request.UserConfirmationID {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "legacy workflow apply requires explicit confirmation", "confirm the reviewed patch before applying")
		return
	}
	result, err := application.ApplyLegacyPatch(r.Context(), legacy)
	if err != nil {
		writeLegacyWorkflowApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, legacyWorkflowPatchResult(request.PatchID, result))
}

func (s *HTTPServer) legacyWorkflowPatchRequest(w http.ResponseWriter, r *http.Request, workflowID, baseRevision string, patch workfloweditor.WorkflowPatch, idempotencyKey, reason string, confirmed bool, suppliedCanvasSession string) (service.WorkflowLegacyPatchRequest, bool) {
	workflowID, baseRevision, idempotencyKey, reason = strings.TrimSpace(workflowID), strings.TrimSpace(baseRevision), strings.TrimSpace(idempotencyKey), strings.TrimSpace(reason)
	if workflowID == "" || baseRevision == "" || idempotencyKey == "" || reason == "" || (patch.WorkflowID != "" && strings.TrimSpace(patch.WorkflowID) != workflowID) || (patch.BaseRevision != "" && strings.TrimSpace(patch.BaseRevision) != baseRevision) || strings.TrimSpace(patch.ID) != idempotencyKey {
		writeLegacyWorkflowSchemaError(w, "legacy workflow patch identity and base revision must match")
		return service.WorkflowLegacyPatchRequest{}, false
	}
	key, scope, ok := s.resolveLegacyWorkflowScope(w, r, workflowID)
	if !ok {
		return service.WorkflowLegacyPatchRequest{}, false
	}
	if suppliedCanvasSession != "" && strings.TrimSpace(suppliedCanvasSession) != scope.CanvasSessionID {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "legacy canvas session does not match trusted scope", "reopen the workflow editor session")
		return service.WorkflowLegacyPatchRequest{}, false
	}
	operations, err := legacyWorkflowOperations(patch.Operations)
	if err != nil {
		writeLegacyWorkflowSchemaError(w, err.Error())
		return service.WorkflowLegacyPatchRequest{}, false
	}
	return service.WorkflowLegacyPatchRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: key, ExpectedRevision: baseRevision,
		IdempotencyKey: idempotencyKey, ActorID: scope.ActorID, CanvasSessionID: scope.CanvasSessionID,
		SessionID: scope.SessionID, Reason: reason, Confirmed: confirmed, Operations: operations,
	}, true
}

func (s *HTTPServer) resolveLegacyWorkflowScope(w http.ResponseWriter, r *http.Request, workflowID string) (edit.WorkflowKey, WorkflowCommandTrustedScope, bool) {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		writeLegacyWorkflowSchemaError(w, "workflowId is required")
		return edit.WorkflowKey{}, WorkflowCommandTrustedScope{}, false
	}
	scope, err := s.workflowCommandScopeResolver.ResolveWorkflowCommandScope(r.Context(), r, workflowID)
	if err != nil {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "workflow compatibility scope is not authorized", "sign in again or request access to this workflow")
		return edit.WorkflowKey{}, WorkflowCommandTrustedScope{}, false
	}
	key := edit.WorkflowKey{TenantID: strings.TrimSpace(scope.TenantID), WorkflowID: workflowID}
	if key.Validate() != nil || strings.TrimSpace(scope.ActorID) == "" || strings.TrimSpace(scope.CanvasSessionID) == "" || strings.TrimSpace(scope.SessionID) == "" {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "trusted workflow compatibility scope is incomplete", "reopen the workflow editor session")
		return edit.WorkflowKey{}, WorkflowCommandTrustedScope{}, false
	}
	return key, scope, true
}

func decodeLegacyWorkflowRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeLegacyWorkflowSchemaError(w, "invalid legacy workflow request body")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeLegacyWorkflowSchemaError(w, "legacy workflow request must contain exactly one JSON object")
		return false
	}
	return true
}

func legacyWorkflowOperations(input []workfloweditor.WorkflowPatchOperation) ([]edit.WorkflowOperation, error) {
	if len(input) == 0 || len(input) > 200 {
		return nil, errors.New("legacy workflow patch requires 1 to 200 typed operations")
	}
	operations := make([]edit.WorkflowOperation, 0, len(input))
	for index, operation := range input {
		translated, err := legacyWorkflowOperation(operation)
		if err != nil {
			return nil, fmt.Errorf("legacy operation %d: %w", index, err)
		}
		operations = append(operations, translated)
	}
	return operations, nil
}

func legacyWorkflowOperation(operation workfloweditor.WorkflowPatchOperation) (edit.WorkflowOperation, error) {
	if operation.Metadata != nil || operation.Graph != nil {
		return edit.WorkflowOperation{}, errors.New("metadata and full graph payloads are not supported")
	}
	var translated edit.WorkflowOperation
	switch operation.Op {
	case workfloweditor.PatchAddNode:
		if operation.Node == nil || operation.NodeID != "" || operation.EdgeID != "" || operation.Edge != nil || operation.Fields != nil || legacyVisualNodeHasUIOrState(*operation.Node) {
			return translated, errors.New("add_node requires one typed node without UI or runtime state")
		}
		translated = edit.NewOperation(&edit.AddNodeOperation{Node: legacyEditableNode(*operation.Node)})
	case workfloweditor.PatchUpdateNode:
		if strings.TrimSpace(operation.NodeID) == "" || operation.Node != nil || operation.EdgeID != "" || operation.Edge != nil || operation.Fields == nil {
			return translated, errors.New("update_node requires nodeId and typed fields")
		}
		patch, err := legacyNodePatch(operation.Fields)
		if err != nil {
			return translated, err
		}
		translated = edit.NewOperation(&edit.UpdateNodeOperation{NodeID: strings.TrimSpace(operation.NodeID), Set: patch})
	case workfloweditor.PatchDeleteNode:
		if strings.TrimSpace(operation.NodeID) == "" || operation.Node != nil || operation.EdgeID != "" || operation.Edge != nil || operation.Fields != nil {
			return translated, errors.New("delete_node requires only nodeId")
		}
		translated = edit.NewOperation(&edit.DeleteNodeOperation{NodeID: strings.TrimSpace(operation.NodeID), CascadeEdges: true})
	case workfloweditor.PatchAddEdge:
		if operation.Edge == nil || operation.NodeID != "" || operation.Node != nil || operation.EdgeID != "" || operation.Fields != nil || operation.Edge.UI != nil || operation.Edge.State != nil {
			return translated, errors.New("add_edge requires one typed edge without UI or runtime state")
		}
		translated = edit.NewOperation(&edit.AddEdgeOperation{Edge: legacyEditableEdge(*operation.Edge)})
	case workfloweditor.PatchDeleteEdge:
		if strings.TrimSpace(operation.EdgeID) == "" || operation.NodeID != "" || operation.Node != nil || operation.Edge != nil || operation.Fields != nil {
			return translated, errors.New("delete_edge requires only edgeId")
		}
		translated = edit.NewOperation(&edit.DeleteEdgeOperation{EdgeID: strings.TrimSpace(operation.EdgeID)})
	default:
		return translated, fmt.Errorf("operation %q has no safe canonical compatibility mapping", operation.Op)
	}
	if err := translated.Validate(); err != nil {
		return edit.WorkflowOperation{}, errors.New("canonical operation validation failed")
	}
	return translated, nil
}

func legacyNodePatch(fields map[string]any) (edit.NodePatch, error) {
	var patch edit.NodePatch
	for field, value := range fields {
		switch field {
		case "label":
			label, ok := value.(string)
			if !ok {
				return edit.NodePatch{}, errors.New("update_node label must be a string")
			}
			patch.Label = &label
		case "type":
			raw, ok := value.(string)
			if !ok {
				return edit.NodePatch{}, errors.New("update_node type must be a string")
			}
			nodeType := visual.NodeType(raw)
			patch.Type = &nodeType
		default:
			return edit.NodePatch{}, fmt.Errorf("update_node field %q is not supported", field)
		}
	}
	if patch == (edit.NodePatch{}) {
		return edit.NodePatch{}, errors.New("update_node requires at least one field")
	}
	return patch, nil
}

func legacyEditableNode(node visual.Node) edit.EditableNode {
	return edit.EditableNode{ID: node.ID, NodePatch: edit.NodePatch{
		Type: &node.Type, Position: &node.Position, StepName: &node.StepName, StepID: &node.StepID, Step: node.Step,
		HandlerName: &node.HandlerName, Handler: node.Handler, Approval: node.Approval, Condition: node.Condition,
		Subflow: node.Subflow, Aggregator: node.Aggregator, Join: node.Join, Loop: node.Loop, ParentID: &node.ParentID,
		Label: &node.Label, Collapsed: &node.Collapsed, Ports: &node.Ports, Inputs: &node.Inputs, Outputs: &node.Outputs,
	}}
}

func legacyEditableEdge(edge visual.Edge) edit.EditableEdge {
	return edit.EditableEdge{ID: edge.ID, EdgePatch: edit.EdgePatch{Source: &edge.Source, SourcePort: &edge.SourcePort, Target: &edge.Target, TargetPort: &edge.TargetPort, Kind: &edge.Kind, Condition: &edge.Condition}}
}

func legacyVisualNodeHasUIOrState(node visual.Node) bool {
	if node.UI != nil || node.State != nil || (node.Approval != nil && node.Approval.UI != nil) || (node.Condition != nil && node.Condition.UI != nil) || (node.Subflow != nil && node.Subflow.UI != nil) || (node.Aggregator != nil && node.Aggregator.UI != nil) || (node.Join != nil && node.Join.UI != nil) || (node.Loop != nil && node.Loop.UI != nil) {
		return true
	}
	if node.Condition != nil {
		for _, branch := range node.Condition.Elif {
			if branch.UI != nil {
				return true
			}
		}
	}
	for _, port := range node.Ports {
		if port.UI != nil {
			return true
		}
	}
	for _, input := range node.Inputs {
		if input.UI != nil {
			return true
		}
	}
	for _, output := range node.Outputs {
		if output.UI != nil {
			return true
		}
	}
	return false
}

func legacyWorkflowSnapshot(result service.WorkflowLegacySnapshotResult) workfloweditor.WorkflowSnapshot {
	return workfloweditor.WorkflowSnapshot{
		WorkflowID: result.Snapshot.Key.WorkflowID, Revision: result.Snapshot.Revision.ResourceVersion,
		RevisionDigest: result.Snapshot.Revision.SemanticHash, Graph: result.Snapshot.Graph,
		Validation: legacyWorkflowValidation(result.Validation), Describe: legacyWorkflowDescribe(result.Snapshot.Key.WorkflowID, result.Snapshot.Revision.ResourceVersion, result.Snapshot.Graph),
	}
}

func legacyWorkflowValidation(result service.WorkflowEditValidationResult) workfloweditor.WorkflowPatchValidation {
	validation := workfloweditor.WorkflowPatchValidation{Valid: result.Validation.Valid && result.Compiled && !result.Stale}
	for _, issue := range result.Validation.Errors {
		validation.Errors = append(validation.Errors, issue.Message)
	}
	for _, issue := range result.Validation.Warnings {
		validation.Warnings = append(validation.Warnings, issue.Message)
	}
	return validation
}

func legacyWorkflowEffect(preview service.WorkflowEditPreviewResult) workfloweditor.PatchEffectResult {
	delta := preview.Diff.Graph.Delta
	status := workfloweditor.EffectChanged
	if len(preview.Diff.Graph.TouchedPaths) == 0 {
		status = workfloweditor.EffectNoEffect
	}
	return workfloweditor.PatchEffectResult{Status: status, Summary: "canonical workflow patch preview", AffectedNodes: uniqueLegacyStrings(delta.AddedNodeIDs, delta.UpdatedNodeIDs, delta.DeletedNodeIDs), AffectedEdges: uniqueLegacyStrings(delta.AddedEdgeIDs, delta.UpdatedEdgeIDs, delta.DeletedEdgeIDs)}
}

func legacyWorkflowPatchResult(patchID string, result service.WorkflowManualCommandResult) workfloweditor.WorkflowPatchResult {
	after := ""
	if result.Receipt.RevisionAfter != nil {
		after = result.Receipt.RevisionAfter.ResourceVersion
	}
	return workfloweditor.WorkflowPatchResult{
		PatchID: patchID, WorkflowID: result.Receipt.Key.WorkflowID,
		RevisionBefore: result.Receipt.RevisionBefore.ResourceVersion, RevisionAfter: after,
		Effect: legacyWorkflowEffect(result.Preview), Describe: legacyWorkflowDescribe(result.Receipt.Key.WorkflowID, after, result.Preview.Graph),
		UndoCheckpoint: workfloweditor.UndoCheckpointRef{ID: result.Receipt.CheckpointID, WorkflowID: result.Receipt.Key.WorkflowID, PatchID: patchID, RevisionBefore: result.Receipt.RevisionBefore.ResourceVersion, RevisionAfter: after},
	}
}

func legacyWorkflowDescribe(workflowID, revision string, graph visual.Graph) workfloweditor.DescribeResult {
	nodeIDs := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeIDs = append(nodeIDs, node.ID)
	}
	return workfloweditor.DescribeResult{WorkflowID: workflowID, Revision: revision, Summary: "workflow graph", NodeCount: len(graph.Nodes), EdgeCount: len(graph.Edges), NodeIDs: nodeIDs}
}

func uniqueLegacyStrings(groups ...[]string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, group := range groups {
		for _, value := range group {
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

func firstLegacyReason(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "legacy workflow patch"
}

func writeLegacyWorkflowMigrationRequired(w http.ResponseWriter) {
	writeJSON(w, http.StatusGone, edit.DomainError{Kind: "error", Code: edit.ErrorMissingConfiguration, Message: "legacy workflow AI endpoint cannot be safely mapped to the canonical transaction path", RetryHint: "use AssistantTransport workflow_agent for AI edits or the workflow command endpoint for manual edits"})
}

func writeLegacyWorkflowSchemaError(w http.ResponseWriter, message string) {
	writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, message, "refresh the workflow and send a typed compatibility request")
}

func writeLegacyWorkflowApplicationError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrWorkflowEditSecret) {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "legacy workflow request contains protected credential material", "remove credential material and use secret references")
		return
	}
	writeWorkflowCommandApplicationError(w, err)
}
