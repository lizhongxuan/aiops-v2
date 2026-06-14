package terminalpolicy

import "testing"

func TestEngineEvaluatesUserPolicyWithHardDenyFirst(t *testing.T) {
	engine := NewEngine(Config{
		SchemaVersion: "aiops.terminal_policy/v1",
		Rules: []Rule{
			{ID: "allow-rm", Effect: RuleEffectAllow, Command: "rm"},
			{ID: "allow-ss-listen", Effect: RuleEffectAllow, Command: "ss", ArgsPrefix: []string{"-ltnp"}, Reason: "bounded socket inspection"},
			{ID: "deny-lsof", Effect: RuleEffectDeny, Command: "lsof", ArgsPrefix: []string{"-i", ":1234"}, Reason: "tenant policy disabled lsof"},
			{ID: "approve-journalctl", Effect: RuleEffectApprovalRequired, Command: "journalctl", ArgsPrefix: []string{"-u"}, Reason: "service logs require approval"},
		},
	})

	if decision := engine.Evaluate(CommandRequest{Command: "rm", Args: []string{"-rf", "/tmp/nope"}}); decision.Action != PolicyActionDeny || decision.Source != PolicySourceHardDeny {
		t.Fatalf("rm decision = %#v, want hard deny", decision)
	}
	if decision := engine.Evaluate(CommandRequest{Command: "ss", Args: []string{"-ltnp"}}); decision.Action != PolicyActionAllow || decision.RuleID != "allow-ss-listen" {
		t.Fatalf("ss decision = %#v, want user allow", decision)
	}
	if decision := engine.Evaluate(CommandRequest{Command: "lsof", Args: []string{"-i", ":1234"}}); decision.Action != PolicyActionDeny || decision.RuleID != "deny-lsof" {
		t.Fatalf("lsof decision = %#v, want user deny", decision)
	}
	if decision := engine.Evaluate(CommandRequest{Command: "journalctl", Args: []string{"-u", "nginx"}}); decision.Action != PolicyActionNeedApproval || decision.RuleID != "approve-journalctl" {
		t.Fatalf("journalctl decision = %#v, want user approval", decision)
	}
	if decision := engine.Evaluate(CommandRequest{Command: "lsof", Args: []string{"-i", ":4321"}}); decision.Action != PolicyActionAllow || decision.Source != PolicySourceBuiltin {
		t.Fatalf("builtin lsof decision = %#v, want builtin allow", decision)
	}
}

func TestValidateConfigRejectsUnsafeRules(t *testing.T) {
	err := ValidateConfig(Config{
		SchemaVersion: "aiops.terminal_policy/v1",
		Rules:         []Rule{{ID: "bad", Effect: RuleEffectAllow, Command: "sh", ArgsPrefix: []string{"-c", "rm -rf /"}}},
	})
	if err == nil {
		t.Fatal("ValidateConfig() error = nil, want unsafe shell rule rejected")
	}
}
