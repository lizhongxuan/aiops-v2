package runtimekernel

import (
	"strings"
	"testing"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/promptcompiler"
)

func TestMCPInstructionContextRendersDeltaAndSparseReminder(t *testing.T) {
	reg := mcp.NewRegistry()
	if err := reg.RegisterServer(mcp.ServerConfig{ID: "synthetic-docs"}); err != nil {
		t.Fatalf("RegisterServer() error = %v", err)
	}
	reg.SetServerInstructions("synthetic-docs", "Use bounded resource reads only.")
	session := &SessionState{}

	ctx := appendMCPInstructionContext(promptcompiler.CompileContext{}, session)
	if len(ctx.ExtraSections) != 1 || !strings.Contains(ctx.ExtraSections[0].Content, "action=added") {
		t.Fatalf("first context = %+v", ctx.ExtraSections)
	}
	if len(session.MCPInstructions.Announced) != 1 {
		t.Fatalf("announced state = %+v", session.MCPInstructions)
	}

	ctx = appendMCPInstructionContext(promptcompiler.CompileContext{}, session)
	if len(ctx.ExtraSections) != 1 || !strings.Contains(ctx.ExtraSections[0].Content, "sparse reminder") {
		t.Fatalf("sparse context = %+v", ctx.ExtraSections)
	}
	if strings.Contains(ctx.ExtraSections[0].Content, "Use bounded resource reads only.") {
		t.Fatalf("sparse reminder leaked full instruction: %s", ctx.ExtraSections[0].Content)
	}
}
