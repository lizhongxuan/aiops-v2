package policyengine

import (
	"encoding/json"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestChatModeAllowsReadOnlyTerminalCommand(t *testing.T) {
	policy := &ChatModePolicy{}
	decision := policy.CheckTool(PolicyInput{
		ToolName: "exec_command",
		Tool:     tooling.ToolMetadata{Name: "exec_command"},
		Mode:     ModeChat,
		Arguments: json.RawMessage(`{
			"command": "date",
			"args": ["+%F"]
		}`),
	})
	if decision.Action != PolicyActionAllow {
		t.Fatalf("CheckTool() = %#v, want allow", decision)
	}
}

func TestChatModeAllowsReadOnlyTerminalCommandDespiteHighRiskMetadata(t *testing.T) {
	policy := &ChatModePolicy{}
	decision := policy.CheckTool(PolicyInput{
		ToolName: "exec_command",
		Tool: tooling.ToolMetadata{
			Name:      "exec_command",
			RiskLevel: tooling.ToolRiskHigh,
		},
		Mode: ModeChat,
		Arguments: json.RawMessage(`{
			"command": "nproc",
			"args": []
		}`),
	})
	if decision.Action != PolicyActionAllow {
		t.Fatalf("CheckTool() = %#v, want allow", decision)
	}
}

func TestChatModeAllowsMacOSVersionReadOnlyTerminalCommand(t *testing.T) {
	policy := &ChatModePolicy{}
	decision := policy.CheckTool(PolicyInput{
		ToolName: "exec_command",
		Tool:     tooling.ToolMetadata{Name: "exec_command"},
		Mode:     ModeChat,
		Arguments: json.RawMessage(`{
			"command": "sw_vers",
			"args": []
		}`),
	})
	if decision.Action != PolicyActionAllow {
		t.Fatalf("CheckTool() = %#v, want allow", decision)
	}
}

func TestChatModeAllowsReadOnlyNetworkInfoTerminalCommand(t *testing.T) {
	policy := &ChatModePolicy{}
	decision := policy.CheckTool(PolicyInput{
		ToolName: "exec_command",
		Tool:     tooling.ToolMetadata{Name: "exec_command"},
		Mode:     ModeChat,
		Arguments: json.RawMessage(`{
			"command": "ifconfig",
			"args": []
		}`),
	})
	if decision.Action != PolicyActionAllow {
		t.Fatalf("CheckTool() = %#v, want allow", decision)
	}
}

func TestChatModeAllowsSafeCurlGetTerminalCommand(t *testing.T) {
	policy := &ChatModePolicy{}
	decision := policy.CheckTool(PolicyInput{
		ToolName: "exec_command",
		Tool:     tooling.ToolMetadata{Name: "exec_command"},
		Mode:     ModeChat,
		Arguments: json.RawMessage(`{
			"command": "curl",
			"args": ["-sS", "--max-time", "5", "https://example.com/data.json"]
		}`),
	})
	if decision.Action != PolicyActionAllow {
		t.Fatalf("CheckTool() = %#v, want allow", decision)
	}
}

func TestChatModeAllowsSafeCurlGetCommandLineInCommandField(t *testing.T) {
	policy := &ChatModePolicy{}
	decision := policy.CheckTool(PolicyInput{
		ToolName: "exec_command",
		Tool:     tooling.ToolMetadata{Name: "exec_command"},
		Mode:     ModeChat,
		Arguments: json.RawMessage(`{
			"command": "curl -sS --max-time 5 https://example.com/data.json"
		}`),
	})
	if decision.Action != PolicyActionAllow {
		t.Fatalf("CheckTool() = %#v, want allow", decision)
	}
}

func TestChatModeRequiresApprovalForUnsafeCurlTerminalCommand(t *testing.T) {
	policy := &ChatModePolicy{}
	decision := policy.CheckTool(PolicyInput{
		ToolName:  "exec_command",
		Tool:      tooling.ToolMetadata{Name: "exec_command"},
		Mode:      ModeChat,
		Arguments: json.RawMessage(`{"command":"curl","args":["-sS","-X","POST","https://example.com/api"]}`),
	})
	if decision.Action != PolicyActionNeedApproval {
		t.Fatalf("CheckTool() = %#v, want need approval", decision)
	}
}

func TestChatModeRequiresApprovalForUnsafeNetworkInfoTerminalCommand(t *testing.T) {
	policy := &ChatModePolicy{}
	decision := policy.CheckTool(PolicyInput{
		ToolName:  "exec_command",
		Tool:      tooling.ToolMetadata{Name: "exec_command"},
		Mode:      ModeChat,
		Arguments: json.RawMessage(`{"command":"ifconfig","args":["en0","down"]}`),
	})
	if decision.Action != PolicyActionNeedApproval {
		t.Fatalf("CheckTool() = %#v, want need approval", decision)
	}
}

func TestChatModeRequiresApprovalForUnsafeTerminalCommand(t *testing.T) {
	policy := &ChatModePolicy{}
	decision := policy.CheckTool(PolicyInput{
		ToolName:  "exec_command",
		Tool:      tooling.ToolMetadata{Name: "exec_command"},
		Mode:      ModeChat,
		Arguments: json.RawMessage(`{"command":"touch","args":["marker"]}`),
	})
	if decision.Action != PolicyActionNeedApproval {
		t.Fatalf("CheckTool() = %#v, want need approval", decision)
	}
}

func TestChatModeDeniesTerminalShellOperators(t *testing.T) {
	policy := &ChatModePolicy{}
	decision := policy.CheckTool(PolicyInput{
		ToolName:  "exec_command",
		Tool:      tooling.ToolMetadata{Name: "exec_command"},
		Mode:      ModeChat,
		Arguments: json.RawMessage(`{"cmd":"echo ok && rm -rf /tmp/nope"}`),
	})
	if decision.Action != PolicyActionDeny {
		t.Fatalf("CheckTool() = %#v, want deny", decision)
	}
}
