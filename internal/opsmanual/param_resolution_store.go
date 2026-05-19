package opsmanual

import "strings"

type ParamResolutionEvent struct {
	ID              string                `json:"id"`
	SessionID       string                `json:"session_id,omitempty"`
	TurnID          string                `json:"turn_id,omitempty"`
	OpsManualFlowID string                `json:"ops_manual_flow_id,omitempty"`
	ManualID        string                `json:"manual_id,omitempty"`
	WorkflowID      string                `json:"workflow_id,omitempty"`
	OperationFrame  OperationFrame        `json:"operation_frame"`
	Result          ParamResolutionResult `json:"result"`
	CreatedAt       string                `json:"created_at"`
}

type ListParamResolutionEventsRequest struct {
	OpsManualFlowID string `json:"ops_manual_flow_id,omitempty"`
	SessionID        string `json:"session_id,omitempty"`
	TurnID           string `json:"turn_id,omitempty"`
	ManualID         string `json:"manual_id,omitempty"`
	WorkflowID       string `json:"workflow_id,omitempty"`
	Limit            int    `json:"limit,omitempty"`
}

type ParamResolutionEventRepository interface {
	SaveParamResolutionEvent(ParamResolutionEvent) error
	ListParamResolutionEvents(ListParamResolutionEventsRequest) ([]ParamResolutionEvent, error)
}

func cloneParamResolutionEvent(in ParamResolutionEvent) ParamResolutionEvent {
	out := in
	out.OperationFrame = cloneOperationFrameValue(in.OperationFrame)
	out.Result = cloneParamResolutionResult(in.Result)
	return out
}

func paramResolutionEventMatchesRequest(event ParamResolutionEvent, req ListParamResolutionEventsRequest) bool {
	if req.OpsManualFlowID != "" && event.OpsManualFlowID != req.OpsManualFlowID {
		return false
	}
	if req.SessionID != "" && !strings.EqualFold(event.SessionID, req.SessionID) {
		return false
	}
	if req.TurnID != "" && !strings.EqualFold(event.TurnID, req.TurnID) {
		return false
	}
	if req.ManualID != "" && event.ManualID != req.ManualID {
		return false
	}
	if req.WorkflowID != "" && event.WorkflowID != req.WorkflowID {
		return false
	}
	return true
}
