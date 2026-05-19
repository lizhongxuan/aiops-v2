package opsmanual

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type OpsManualFlowIDInput struct {
	SessionID      string
	TurnID         string
	ManualID       string
	WorkflowID     string
	OperationFrame OperationFrame
}

func BuildOpsManualFlowID(input OpsManualFlowIDInput) string {
	parts := []string{
		"v1",
		stableFlowPart(input.SessionID),
		stableFlowPart(input.TurnID),
		stableFlowPart(input.ManualID),
		stableFlowPart(input.WorkflowID),
		stableFlowPart(firstNonEmpty(input.OperationFrame.ObjectType, input.OperationFrame.Target.Type, input.OperationFrame.Operation.TargetType)),
		stableFlowPart(firstNonEmpty(input.OperationFrame.Operation.Action, input.OperationFrame.OperationType, input.OperationFrame.Intent)),
		stableFlowPart(input.OperationFrame.Target.Name),
		stableFlowPart(strings.Join(input.OperationFrame.TargetScope.Hosts, ",")),
		stableFlowPart(input.OperationFrame.TargetScope.Namespace),
		stableFlowPart(input.OperationFrame.TargetScope.Cluster),
		stableFlowPart(input.OperationFrame.TargetScope.Service),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "flow-" + hex.EncodeToString(sum[:])[:20]
}

func BuildOpsManualFlowIDFromMetadata(metadata map[string]any, manualID, workflowID string, frame OperationFrame) string {
	if existing := metadataString(metadata, "ops_manual_flow_id"); existing != "" {
		return stableFlowPart(existing)
	}
	return BuildOpsManualFlowID(OpsManualFlowIDInput{
		SessionID:      metadataString(metadata, "session_id"),
		TurnID:         metadataString(metadata, "turn_id"),
		ManualID:       manualID,
		WorkflowID:     workflowID,
		OperationFrame: frame,
	})
}

func stableFlowPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) > 160 {
		value = value[:160]
	}
	return value
}

func FlowMetadata(flowID string) map[string]any {
	flowID = strings.TrimSpace(flowID)
	if flowID == "" {
		return nil
	}
	return map[string]any{"ops_manual_flow_id": flowID}
}

func flowStageArtifactID(flowID, stage string) string {
	flowID = strings.TrimSpace(flowID)
	stage = strings.TrimSpace(stage)
	if flowID == "" || stage == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", flowID, stage)
}
