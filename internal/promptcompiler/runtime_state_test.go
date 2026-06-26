package promptcompiler

import (
	"strings"
	"testing"
)

func TestRuntimeStateRendersCompactStructuredFields(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		Mode:                   "execute",
		Profile:                PromptProfileHostWorker,
		HostContext:            "host-a",
		WebState:               "available",
		OpsGraphState:          "not_requested",
		CorootState:            "available",
		OpsManusState:          "not_requested",
		PendingApprovals:       2,
		PendingEvidence:        1,
		VisibleToolFingerprint: "tools:abc",
		UserConstraints:        []string{"read_only_until_approval", "no_restart"},
		TimeoutRecoveryState:   "retry_read_only_once",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	state := compiledPromptSectionForTest(t, compiled, "runtime.state").Content
	for _, want := range []string{
		"# Runtime State",
		"profile: host_worker",
		"mode: execute",
		"host_scope: bound",
		"mutation: approval_required",
		"web: available",
		"ops_graph: not_requested",
		"coroot: available",
		"ops_manus: not_requested",
		"pending_approvals: 2",
		"pending_evidence: 1",
		"visible_tool_fingerprint: tools:abc",
		"user_constraints: read_only_until_approval; no_restart",
		"timeout_recovery_state: retry_read_only_once",
	} {
		if !strings.Contains(state, want) {
			t.Fatalf("runtime.state missing %q:\n%s", want, state)
		}
	}
	for _, forbidden := range []string{
		"completion gate",
		"approval philosophy",
		"tool failure interpretation",
		"RCA method",
		"final answer template",
		"All operations are permitted",
		"strictly forbidden",
	} {
		if strings.Contains(strings.ToLower(state), strings.ToLower(forbidden)) {
			t.Fatalf("runtime.state leaked policy prose %q:\n%s", forbidden, state)
		}
	}
	if len([]byte(state)) > 1024 {
		t.Fatalf("runtime.state bytes = %d, want <= 1024:\n%s", len([]byte(state)), state)
	}
}
