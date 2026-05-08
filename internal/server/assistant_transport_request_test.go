package server

import (
	"errors"
	"strings"
	"testing"
)

func TestAssistantTransportRequestDecodesKnownCommands(t *testing.T) {
	body := strings.NewReader(`{
		"state": {
			"schemaVersion": "aiops.transport.v2",
			"sessionId": "sess-1",
			"threadId": "thread-1",
			"status": "idle",
			"turns": {},
			"turnOrder": [],
			"pendingApprovals": {},
			"mcpSurfaces": {},
			"artifacts": {},
			"runtimeLiveness": {},
			"seq": 7,
			"updatedAt": "2026-05-06T00:00:00Z"
		},
		"system": "You are AIOps.",
		"tools": {
			"web_search": {"description": "search the web"},
			"exec_command": {"description": "run command"}
		},
		"threadId": "thread-1",
		"parentId": "msg-parent-1",
		"callSettings": {
			"model": "gpt-4.1",
			"settings": {
				"reasoningEffort": "high"
			}
		},
		"config": {
			"mode": "chat",
			"features": {
				"approval": true
			}
		},
		"commands": [
			{
				"type": "add-message",
				"message": {
					"id": "msg-1",
					"role": "user",
					"hostId": "host-prod-07",
					"metadata": {
						"aiops.target.kind": "label",
						"aiops.target.labelKey": "env",
						"aiops.target.labelValue": "prod"
					},
					"parts": [
						{"type": "text", "text": "hello"}
					]
				}
			},
			{
				"type": "aiops.retry",
				"sessionId": "sess-1",
				"turnId": "turn-retry-1"
			},
			{
				"type": "aiops.stop",
				"sessionId": "sess-1",
				"turnId": "turn-1",
				"reason": "user requested stop"
			},
			{
				"type": "aiops.approval-decision",
				"approvalId": "approval-1",
				"decision": "accept"
			},
			{
				"type": "aiops.choice-answer",
				"requestId": "choice-1",
				"answer": "continue"
			},
			{
				"type": "aiops.mcp-action",
				"surfaceId": "surface-1",
				"action": "refresh",
				"target": "server-a",
				"params": {
					"force": true
				}
			},
			{
				"type": "aiops.mcp-refresh",
				"surfaceId": "surface-2"
			},
			{
				"type": "aiops.mcp-pin",
				"surfaceId": "surface-3",
				"pinned": true
			}
		]
	}`)

	req, err := decodeAssistantTransportRequest(body)
	if err != nil {
		t.Fatalf("decodeAssistantTransportRequest() error = %v", err)
	}

	if req.ThreadID != "thread-1" {
		t.Fatalf("ThreadID = %q, want thread-1", req.ThreadID)
	}
	if req.ParentID != "msg-parent-1" {
		t.Fatalf("ParentID = %q, want msg-parent-1", req.ParentID)
	}
	if req.State.ThreadID != "thread-1" {
		t.Fatalf("State.ThreadID = %q, want thread-1", req.State.ThreadID)
	}
	if got := req.CallSettings.Model; got != "gpt-4.1" {
		t.Fatalf("CallSettings.Model = %q, want gpt-4.1", got)
	}
	if got := req.CallSettings.Settings["reasoningEffort"]; got != "high" {
		t.Fatalf("CallSettings.Settings[reasoningEffort] = %#v, want high", got)
	}
	if got := req.Config["mode"]; got != "chat" {
		t.Fatalf("Config[mode] = %#v, want chat", got)
	}
	if len(req.Tools) != 2 {
		t.Fatalf("len(Tools) = %d, want 2", len(req.Tools))
	}
	if _, ok := req.Tools["web_search"]; !ok {
		t.Fatalf("Tools = %#v, want web_search key", req.Tools)
	}
	if len(req.Commands) != 8 {
		t.Fatalf("len(Commands) = %d, want 8", len(req.Commands))
	}

	addMessage, ok := req.Commands[0].(*assistantTransportAddMessageCommand)
	if !ok {
		t.Fatalf("command[0] type = %T, want *assistantTransportAddMessageCommand", req.Commands[0])
	}
	if addMessage.Type() != "add-message" {
		t.Fatalf("addMessage.Type() = %q, want add-message", addMessage.Type())
	}
	if !strings.Contains(string(addMessage.Message), `"hello"`) {
		t.Fatalf("addMessage.Message = %s, want hello payload", string(addMessage.Message))
	}
	transportCommand, err := assistantTransportCommandFromDecoded(addMessage, req, assistantTransportInitialState(req))
	if err != nil {
		t.Fatalf("assistantTransportCommandFromDecoded(addMessage) error = %v", err)
	}
	if transportCommand.AddMessage == nil || transportCommand.AddMessage.HostID != "host-prod-07" {
		t.Fatalf("AddMessage.HostID = %+v, want host-prod-07", transportCommand.AddMessage)
	}
	if got := transportCommand.AddMessage.Metadata["aiops.target.labelKey"]; got != "env" {
		t.Fatalf("AddMessage metadata labelKey = %q, want env", got)
	}

	retryCommand, ok := req.Commands[1].(*assistantTransportRetryCommand)
	if !ok {
		t.Fatalf("command[1] type = %T, want *assistantTransportRetryCommand", req.Commands[1])
	}
	if retryCommand.SessionID != "sess-1" || retryCommand.TurnID != "turn-retry-1" {
		t.Fatalf("retryCommand = %+v, want session/turn ids", retryCommand)
	}

	stopCommand, ok := req.Commands[2].(*assistantTransportStopCommand)
	if !ok {
		t.Fatalf("command[2] type = %T, want *assistantTransportStopCommand", req.Commands[2])
	}
	if stopCommand.SessionID != "sess-1" || stopCommand.TurnID != "turn-1" {
		t.Fatalf("stopCommand = %+v, want session/turn ids", stopCommand)
	}

	approvalCommand, ok := req.Commands[3].(*assistantTransportApprovalDecisionCommand)
	if !ok {
		t.Fatalf("command[3] type = %T, want *assistantTransportApprovalDecisionCommand", req.Commands[3])
	}
	if approvalCommand.ApprovalID != "approval-1" || approvalCommand.Decision != "accept" {
		t.Fatalf("approvalCommand = %+v, want approval-1/accept", approvalCommand)
	}

	choiceCommand, ok := req.Commands[4].(*assistantTransportChoiceAnswerCommand)
	if !ok {
		t.Fatalf("command[4] type = %T, want *assistantTransportChoiceAnswerCommand", req.Commands[4])
	}
	if choiceCommand.RequestID != "choice-1" || choiceCommand.Answer != "continue" {
		t.Fatalf("choiceCommand = %+v, want choice-1/continue", choiceCommand)
	}

	mcpAction, ok := req.Commands[5].(*assistantTransportMCPActionCommand)
	if !ok {
		t.Fatalf("command[5] type = %T, want *assistantTransportMCPActionCommand", req.Commands[5])
	}
	if mcpAction.SurfaceID != "surface-1" || mcpAction.Action != "refresh" || mcpAction.Target != "server-a" {
		t.Fatalf("mcpAction = %+v, want surface/action/target", mcpAction)
	}
	if got := mcpAction.Params["force"]; got != true {
		t.Fatalf("mcpAction.Params[force] = %#v, want true", got)
	}

	mcpRefresh, ok := req.Commands[6].(*assistantTransportMCPRefreshCommand)
	if !ok {
		t.Fatalf("command[6] type = %T, want *assistantTransportMCPRefreshCommand", req.Commands[6])
	}
	if mcpRefresh.SurfaceID != "surface-2" {
		t.Fatalf("mcpRefresh = %+v, want surface-2", mcpRefresh)
	}

	mcpPin, ok := req.Commands[7].(*assistantTransportMCPPinCommand)
	if !ok {
		t.Fatalf("command[7] type = %T, want *assistantTransportMCPPinCommand", req.Commands[7])
	}
	if mcpPin.SurfaceID != "surface-3" || !mcpPin.Pinned {
		t.Fatalf("mcpPin = %+v, want surface-3/pinned=true", mcpPin)
	}
}

func TestAssistantTransportRequestUsesDeprecatedTopLevelCallSettingsFallback(t *testing.T) {
	body := strings.NewReader(`{
		"threadId": "thread-legacy",
		"model": "gpt-4o-mini",
		"settings": {
			"temperature": 0.2
		},
		"commands": [
			{
				"type": "aiops.stop"
			}
		]
	}`)

	req, err := decodeAssistantTransportRequest(body)
	if err != nil {
		t.Fatalf("decodeAssistantTransportRequest() error = %v", err)
	}

	if got := req.CallSettings.Model; got != "gpt-4o-mini" {
		t.Fatalf("CallSettings.Model = %q, want gpt-4o-mini", got)
	}
	if got := req.CallSettings.Settings["temperature"]; got != 0.2 {
		t.Fatalf("CallSettings.Settings[temperature] = %#v, want 0.2", got)
	}
}

func TestAssistantTransportRequestRejectsUnknownCommand(t *testing.T) {
	body := strings.NewReader(`{
		"commands": [
			{
				"type": "aiops.unsupported"
			}
		]
	}`)

	_, err := decodeAssistantTransportRequest(body)
	if err == nil {
		t.Fatal("decodeAssistantTransportRequest() error = nil, want error")
	}

	var decodeErr *assistantTransportCommandDecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("error = %T %v, want *assistantTransportCommandDecodeError", err, err)
	}
	if decodeErr.CommandType != "aiops.unsupported" {
		t.Fatalf("decodeErr.CommandType = %q, want aiops.unsupported", decodeErr.CommandType)
	}
}
