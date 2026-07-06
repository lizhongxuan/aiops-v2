package tooling

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestEinoToolAdapterDoesNotOwnVisibilityOrPermissionPolicy(t *testing.T) {
	t.Parallel()

	tool := &StaticTool{
		Meta: ToolMetadata{Name: "boundary_check", Description: "Boundary check."},
		EnabledFunc: func(ToolContext) bool {
			t.Fatal("Eino adapter must not evaluate visibility policy")
			return false
		},
		CheckPermissionsFunc: func(context.Context, json.RawMessage) PermissionDecision {
			t.Fatal("Eino adapter must not evaluate permission policy")
			return PermissionDecision{Action: PermissionActionDeny}
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "executed"}, nil
		},
	}

	adapter := NewEinoToolAdapter(tool)
	if _, err := adapter.Info(context.Background()); err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	got, err := adapter.InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}
	if got != "executed" {
		t.Fatalf("InvokableRun() = %q, want executed", got)
	}
}

func TestEinoAdapterSourceDoesNotContainBusinessRuntimeBranches(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("eino_adapter.go")
	if err != nil {
		t.Fatalf("read eino_adapter.go: %v", err)
	}
	source := string(data)
	for _, forbidden := range []string{
		"HostID",
		"ResourceBinding",
		"SessionTarget",
		"RoleBinding",
		"ApprovalScope",
		"PolicyEngine",
		"PromptCompiler",
		"CompileContext",
		"pg_primary",
		"pg_standby",
		"bound_host_id",
		"bound_role",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("eino adapter source contains business runtime branch marker %q", forbidden)
		}
	}
}
