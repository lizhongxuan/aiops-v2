package runtimekernel

import (
	"context"
	"testing"

	"aiops-v2/internal/promptcompiler"
)

func TestRunTurnHostMentionsEnableHostOpsManagerPromptContext(t *testing.T) {
	compiler := &captureCompileContextCompiler{}
	kernel := newTestKernel(compiler)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-hostops-manager",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		TurnID:      "turn-hostops-manager",
		Input:       "@1.1.1.1和@1.1.1.2作为pg节点，搭建主从集群",
		Metadata: map[string]string{
			"aiops.hostops.mentions":                `[{"raw":"@1.1.1.1","value":"1.1.1.1"},{"raw":"@1.1.1.2","value":"1.1.1.2"}]`,
			"aiops.hostops.clientDetectedMultiHost": "true",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	if !compiler.last.HostOpsManager {
		t.Fatalf("HostOpsManager = false, want true")
	}
	if !compiler.last.HostOpsPlanRequired {
		t.Fatalf("HostOpsPlanRequired = false, want true")
	}
	if compiler.last.AgentKind != promptcompiler.AgentKindPlanner {
		t.Fatalf("AgentKind = %q, want planner", compiler.last.AgentKind)
	}
}
