package server

import (
	"encoding/json"
	"fmt"
	"io"

	"aiops-v2/internal/appui"
)

const (
	assistantTransportCommandAddMessage       = "add-message"
	assistantTransportCommandRetry            = "aiops.retry"
	assistantTransportCommandStop             = "aiops.stop"
	assistantTransportCommandApprovalDecision = "aiops.approval-decision"
	assistantTransportCommandChoiceAnswer     = "aiops.choice-answer"
	assistantTransportCommandMCPAction        = "aiops.mcp-action"
	assistantTransportCommandMCPRefresh       = "aiops.mcp-refresh"
	assistantTransportCommandMCPPin           = "aiops.mcp-pin"
)

type assistantTransportRequest struct {
	State        appui.AiopsTransportState      `json:"state"`
	Commands     []assistantTransportCommand    `json:"commands"`
	System       string                         `json:"system,omitempty"`
	Tools        map[string]any                 `json:"tools,omitempty"`
	ThreadID     string                         `json:"threadId,omitempty"`
	ParentID     string                         `json:"parentId,omitempty"`
	CallSettings assistantTransportCallSettings `json:"callSettings,omitempty"`
	Config       map[string]any                 `json:"config,omitempty"`
	Model        string                         `json:"model,omitempty"`
	Settings     map[string]any                 `json:"settings,omitempty"`
}

type assistantTransportCallSettings struct {
	Model    string         `json:"model,omitempty"`
	Settings map[string]any `json:"settings,omitempty"`
}

type assistantTransportCommand interface {
	Type() string
}

type assistantTransportAddMessageCommand struct {
	CommandType string          `json:"type"`
	Message     json.RawMessage `json:"message"`
}

func (c *assistantTransportAddMessageCommand) Type() string {
	return c.CommandType
}

type assistantTransportStopCommand struct {
	CommandType string `json:"type"`
	SessionID   string `json:"sessionId,omitempty"`
	TurnID      string `json:"turnId,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

func (c *assistantTransportStopCommand) Type() string {
	return c.CommandType
}

type assistantTransportRetryCommand struct {
	CommandType string `json:"type"`
	SessionID   string `json:"sessionId,omitempty"`
	TurnID      string `json:"turnId,omitempty"`
}

func (c *assistantTransportRetryCommand) Type() string {
	return c.CommandType
}

type assistantTransportApprovalDecisionCommand struct {
	CommandType string `json:"type"`
	SessionID   string `json:"sessionId,omitempty"`
	TurnID      string `json:"turnId,omitempty"`
	ApprovalID  string `json:"approvalId"`
	Decision    string `json:"decision"`
}

func (c *assistantTransportApprovalDecisionCommand) Type() string {
	return c.CommandType
}

type assistantTransportChoiceAnswerCommand struct {
	CommandType string `json:"type"`
	RequestID   string `json:"requestId"`
	Answer      string `json:"answer"`
}

func (c *assistantTransportChoiceAnswerCommand) Type() string {
	return c.CommandType
}

type assistantTransportMCPActionCommand struct {
	CommandType string         `json:"type"`
	SurfaceID   string         `json:"surfaceId,omitempty"`
	Action      string         `json:"action"`
	Target      string         `json:"target,omitempty"`
	Params      map[string]any `json:"params,omitempty"`
}

func (c *assistantTransportMCPActionCommand) Type() string {
	return c.CommandType
}

type assistantTransportMCPRefreshCommand struct {
	CommandType string `json:"type"`
	SurfaceID   string `json:"surfaceId,omitempty"`
}

func (c *assistantTransportMCPRefreshCommand) Type() string {
	return c.CommandType
}

type assistantTransportMCPPinCommand struct {
	CommandType string `json:"type"`
	SurfaceID   string `json:"surfaceId,omitempty"`
	Pinned      bool   `json:"pinned,omitempty"`
}

func (c *assistantTransportMCPPinCommand) Type() string {
	return c.CommandType
}

type assistantTransportCommandDecodeError struct {
	CommandType string
}

func (e *assistantTransportCommandDecodeError) Error() string {
	return fmt.Sprintf("assistant transport command %q is not supported", e.CommandType)
}

type assistantTransportRequestEnvelope struct {
	State        appui.AiopsTransportState      `json:"state"`
	Commands     []json.RawMessage              `json:"commands"`
	System       string                         `json:"system,omitempty"`
	Tools        map[string]any                 `json:"tools,omitempty"`
	ThreadID     string                         `json:"threadId,omitempty"`
	ParentID     string                         `json:"parentId,omitempty"`
	CallSettings assistantTransportCallSettings `json:"callSettings,omitempty"`
	Config       map[string]any                 `json:"config,omitempty"`
	Model        string                         `json:"model,omitempty"`
	Settings     map[string]any                 `json:"settings,omitempty"`
}

type assistantTransportCommandEnvelope struct {
	CommandType string `json:"type"`
}

func decodeAssistantTransportRequest(body io.Reader) (*assistantTransportRequest, error) {
	if body == nil {
		return nil, io.EOF
	}

	var envelope assistantTransportRequestEnvelope
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&envelope); err != nil {
		return nil, err
	}

	req := &assistantTransportRequest{
		State:        envelope.State,
		System:       envelope.System,
		Tools:        envelope.Tools,
		ThreadID:     envelope.ThreadID,
		ParentID:     envelope.ParentID,
		CallSettings: envelope.CallSettings,
		Config:       envelope.Config,
		Model:        envelope.Model,
		Settings:     envelope.Settings,
	}

	if req.CallSettings.Model == "" {
		req.CallSettings.Model = req.Model
	}
	if len(req.CallSettings.Settings) == 0 && len(req.Settings) > 0 {
		req.CallSettings.Settings = req.Settings
	}

	if len(envelope.Commands) == 0 {
		req.Commands = []assistantTransportCommand{}
		return req, nil
	}

	req.Commands = make([]assistantTransportCommand, 0, len(envelope.Commands))
	for _, raw := range envelope.Commands {
		command, err := decodeAssistantTransportCommand(raw)
		if err != nil {
			return nil, err
		}
		req.Commands = append(req.Commands, command)
	}

	return req, nil
}

func decodeAssistantTransportCommand(raw json.RawMessage) (assistantTransportCommand, error) {
	var envelope assistantTransportCommandEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}

	switch envelope.CommandType {
	case assistantTransportCommandAddMessage:
		var command assistantTransportAddMessageCommand
		if err := json.Unmarshal(raw, &command); err != nil {
			return nil, err
		}
		return &command, nil
	case assistantTransportCommandRetry:
		var command assistantTransportRetryCommand
		if err := json.Unmarshal(raw, &command); err != nil {
			return nil, err
		}
		return &command, nil
	case assistantTransportCommandStop:
		var command assistantTransportStopCommand
		if err := json.Unmarshal(raw, &command); err != nil {
			return nil, err
		}
		return &command, nil
	case assistantTransportCommandApprovalDecision:
		var command assistantTransportApprovalDecisionCommand
		if err := json.Unmarshal(raw, &command); err != nil {
			return nil, err
		}
		return &command, nil
	case assistantTransportCommandChoiceAnswer:
		var command assistantTransportChoiceAnswerCommand
		if err := json.Unmarshal(raw, &command); err != nil {
			return nil, err
		}
		return &command, nil
	case assistantTransportCommandMCPAction:
		var command assistantTransportMCPActionCommand
		if err := json.Unmarshal(raw, &command); err != nil {
			return nil, err
		}
		return &command, nil
	case assistantTransportCommandMCPRefresh:
		var command assistantTransportMCPRefreshCommand
		if err := json.Unmarshal(raw, &command); err != nil {
			return nil, err
		}
		return &command, nil
	case assistantTransportCommandMCPPin:
		var command assistantTransportMCPPinCommand
		if err := json.Unmarshal(raw, &command); err != nil {
			return nil, err
		}
		return &command, nil
	default:
		return nil, &assistantTransportCommandDecodeError{CommandType: envelope.CommandType}
	}
}
