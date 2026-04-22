package permissions

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestEngineDefaultAllow(t *testing.T) {
	engine := NewEngine(nil)

	dec := engine.Decide(context.Background(), Request{
		Tool: tooling.ToolMetadata{Name: "read_file"},
	})

	if dec.Action != ActionAllow {
		t.Fatalf("expected allow, got %q", dec.Action)
	}
}

func TestEngineFirstMatchWins(t *testing.T) {
	engine := NewEngine([]Rule{
		{
			Name:    "deny-all",
			Matcher: Matcher{ToolNames: []string{"read_file"}},
			Action:  ActionDeny,
			Reason:  "deny",
		},
		{
			Name:    "allow-all",
			Matcher: Matcher{ToolNames: []string{"read_file"}},
			Action:  ActionAllow,
			Reason:  "allow",
		},
	})

	dec := engine.Decide(context.Background(), Request{
		Tool: tooling.ToolMetadata{Name: "read_file"},
	})

	if dec.Action != ActionDeny {
		t.Fatalf("expected deny, got %q", dec.Action)
	}
	if dec.Reason != "deny" {
		t.Fatalf("expected first rule reason, got %q", dec.Reason)
	}
}

func TestMatcherMatchesAlias(t *testing.T) {
	m := Matcher{ToolNames: []string{"ls"}}

	if !m.Matches(Request{Tool: tooling.ToolMetadata{Name: "list_files", Aliases: []string{"ls"}}}) {
		t.Fatal("expected alias match")
	}
}

func TestMatcherMatchesSourceSessionMode(t *testing.T) {
	m := Matcher{
		Sources:      []tooling.ToolSource{tooling.ToolSourceMCP},
		SessionTypes: []string{"workspace"},
		Modes:        []string{"chat"},
	}

	if !m.Matches(Request{
		Tool:        tooling.ToolMetadata{Name: "read_file", IsMCP: true},
		SessionType: "workspace",
		Mode:        "chat",
	}) {
		t.Fatal("expected matcher to satisfy source/session/mode")
	}
}

func TestMatcherMatchesInputContains(t *testing.T) {
	m := Matcher{InputContains: []string{`"path":"foo.txt"`}}

	if !m.Matches(Request{
		Tool:      tooling.ToolMetadata{Name: "read_file"},
		Arguments: json.RawMessage(`{"path":"foo.txt","mode":"r"}`),
	}) {
		t.Fatal("expected input contains match")
	}
}

func TestMatcherMatchesSourceTraitsAndDestinationWithoutOrigin(t *testing.T) {
	m := Matcher{
		Sources:      []tooling.ToolSource{tooling.ToolSourceMCP},
		Destinations: []string{"/var/log/*"},
	}

	if !m.Matches(Request{
		Tool:      tooling.ToolMetadata{Name: "tail_logs", IsMCP: true},
		Arguments: json.RawMessage(`{"path":"/var/log/system.log"}`),
	}) {
		t.Fatal("expected matcher to satisfy traits-based source and destination")
	}
}

func TestMatcherMatchesAdditionalDirectoriesAsDestinationScope(t *testing.T) {
	m := Matcher{
		Destinations: []string{"/repo/.codex/*"},
	}

	if !m.Matches(Request{
		Tool: tooling.ToolMetadata{Name: "read_file"},
		Arguments: json.RawMessage(`{
			"path":"/repo/main.go",
			"additionalDirectories":["/repo/.codex/skills","/tmp"]
		}`),
	}) {
		t.Fatal("expected additionalDirectories to participate in destination matching")
	}
}

func TestEngineSourcePrecedencePolicyOverridesUser(t *testing.T) {
	engine := NewEngine([]Rule{
		{
			Name:   "user-allow",
			Source: RuleSourceUserSettings,
			Matcher: Matcher{
				ToolNames: []string{"disk_usage"},
			},
			Action: ActionAllow,
		},
		{
			Name:   "policy-deny",
			Source: RuleSourcePolicySettings,
			Matcher: Matcher{
				ToolNames: []string{"disk_usage"},
			},
			Action: ActionDeny,
			Reason: "managed deny",
		},
	})

	dec := engine.Decide(context.Background(), Request{
		Tool: tooling.ToolMetadata{Name: "disk_usage"},
	})
	if dec.Action != ActionDeny || dec.Reason != "managed deny" {
		t.Fatalf("expected policy deny to win, got %+v", dec)
	}
}

func TestEnginePolicySourcePrecedenceWithinPolicySettings(t *testing.T) {
	engine := NewEngine([]Rule{
		{
			Name:   "policy-remote-deny",
			Source: RuleSourcePolicyRemote,
			Matcher: Matcher{
				ToolNames: []string{"disk_usage"},
			},
			Action: ActionDeny,
			Reason: "remote deny",
		},
		{
			Name:   "policy-user-allow",
			Source: RuleSourcePolicyUser,
			Matcher: Matcher{
				ToolNames: []string{"disk_usage"},
			},
			Action: ActionAllow,
			Reason: "hkcu allow",
		},
	})

	dec := engine.Decide(context.Background(), Request{
		Tool: tooling.ToolMetadata{Name: "disk_usage"},
	})
	if dec.Action != ActionAllow || dec.Reason != "hkcu allow" {
		t.Fatalf("expected higher-priority policy layer to win, got %+v", dec)
	}
}

func TestRulesAreSortedByPolicySourcePrecedence(t *testing.T) {
	engine := NewEngine([]Rule{
		{Name: "managed", Source: RuleSourcePolicyManaged},
		{Name: "remote", Source: RuleSourcePolicyRemote},
		{Name: "machine", Source: RuleSourcePolicyMachine},
		{Name: "user", Source: RuleSourcePolicyUser},
	})

	got := engine.Rules()
	want := []RuleSource{
		RuleSourcePolicyUser,
		RuleSourcePolicyManaged,
		RuleSourcePolicyMachine,
		RuleSourcePolicyRemote,
	}

	if len(got) != len(want) {
		t.Fatalf("Rules() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Source != want[i] {
			t.Fatalf("Rules()[%d].Source = %q, want %q", i, got[i].Source, want[i])
		}
	}
}

func TestRulesAreSortedAcrossRuntimeAndSettingsSources(t *testing.T) {
	engine := NewEngine([]Rule{
		{Name: "project", Source: RuleSourceProjectSettings},
		{Name: "command", Source: RuleSourceCommand},
		{Name: "flag", Source: RuleSourceFlagSettings},
		{Name: "session", Source: RuleSourceSession},
		{Name: "cli", Source: RuleSourceCLIArg},
		{Name: "local", Source: RuleSourceLocalSettings},
		{Name: "policy-user", Source: RuleSourcePolicyUser},
		{Name: "user", Source: RuleSourceUserSettings},
	})

	got := engine.Rules()
	want := []RuleSource{
		RuleSourceSession,
		RuleSourceCommand,
		RuleSourceCLIArg,
		RuleSourcePolicyUser,
		RuleSourceFlagSettings,
		RuleSourceLocalSettings,
		RuleSourceProjectSettings,
		RuleSourceUserSettings,
	}

	if len(got) != len(want) {
		t.Fatalf("Rules() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Source != want[i] {
			t.Fatalf("Rules()[%d].Source = %q, want %q", i, got[i].Source, want[i])
		}
	}
}

func TestRulesReturnsCopy(t *testing.T) {
	original := []Rule{
		{
			Name: "rule",
			Matcher: Matcher{
				ToolNames:     []string{"read_file"},
				Sources:       []tooling.ToolSource{tooling.ToolSourceBuiltin},
				SessionTypes:  []string{"workspace"},
				Modes:         []string{"chat"},
				InputContains: []string{"foo"},
			},
			Action: ActionAsk,
			Reason: "ask",
		},
	}
	engine := NewEngine(original)

	rules := engine.Rules()
	rules[0].Name = "mutated"
	rules[0].Matcher.ToolNames[0] = "changed"
	rules[0].Matcher.Sources[0] = tooling.ToolSourceMCP
	rules[0].Matcher.SessionTypes[0] = "changed"
	rules[0].Matcher.Modes[0] = "changed"
	rules[0].Matcher.InputContains[0] = "changed"

	got := engine.Rules()
	if got[0].Name != "rule" {
		t.Fatalf("expected engine rule name to remain unchanged, got %q", got[0].Name)
	}
	if got[0].Matcher.ToolNames[0] != "read_file" {
		t.Fatalf("expected tool names to remain unchanged, got %q", got[0].Matcher.ToolNames[0])
	}
	if got[0].Matcher.Sources[0] != tooling.ToolSourceBuiltin {
		t.Fatalf("expected source to remain unchanged, got %q", got[0].Matcher.Sources[0])
	}
	if got[0].Matcher.SessionTypes[0] != "workspace" {
		t.Fatalf("expected session type to remain unchanged, got %q", got[0].Matcher.SessionTypes[0])
	}
	if got[0].Matcher.Modes[0] != "chat" {
		t.Fatalf("expected mode to remain unchanged, got %q", got[0].Matcher.Modes[0])
	}
	if got[0].Matcher.InputContains[0] != "foo" {
		t.Fatalf("expected input contains to remain unchanged, got %q", got[0].Matcher.InputContains[0])
	}
}
