package featureflag

import (
	"os"
	"reflect"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestDefaultAndClone(t *testing.T) {
	f := Default()
	if f.UnifiedToolModel || f.ToolSearch || f.MCPServerRegistry || f.SkillRegistry || f.AgentRegistry || f.HooksV2 {
		t.Fatalf("default flags should be false: %#v", f)
	}
	if !f.DiagnosticProtocol {
		t.Fatalf("diagnostic protocol should default on: %#v", f)
	}
	if f.DisabledTools != nil || f.DeferredTools != nil || f.ExperimentalMetaTools != nil {
		t.Fatalf("default slices should be nil: %#v", f)
	}

	f.DisabledTools = []string{"a"}
	f.DeferredTools = []string{"b"}
	f.ExperimentalMetaTools = []string{"c"}

	clone := f.Clone()
	if !reflect.DeepEqual(f, clone) {
		t.Fatalf("clone should match original: %#v vs %#v", f, clone)
	}

	clone.DisabledTools[0] = "x"
	clone.DeferredTools[0] = "y"
	clone.ExperimentalMetaTools[0] = "z"
	if f.DisabledTools[0] != "a" || f.DeferredTools[0] != "b" || f.ExperimentalMetaTools[0] != "c" {
		t.Fatalf("clone must deep copy slices: %#v", f)
	}
}

func TestFromEnvParsing(t *testing.T) {
	sep := string(os.PathListSeparator)
	lookup := func(key string) string {
		switch key {
		case envUnifiedToolModel:
			return "YES"
		case envToolSearch:
			return "on"
		case envMCPServerRegistry:
			return "1"
		case envSkillRegistry:
			return "true"
		case envAgentRegistry:
			return "No"
		case envHooksV2:
			return "false"
		case envDiagnosticProtocol:
			return "0"
		case envDisabledTools:
			return " alpha, beta\nalpha " + sep + "gamma "
		case envDeferredTools:
			return "delta" + sep + "delta, epsilon\n"
		case envExperimentalMetaTools:
			return "zeta\n eta" + sep + "zeta"
		default:
			return ""
		}
	}

	f := FromEnv(lookup)
	if !f.UnifiedToolModel || !f.ToolSearch || !f.MCPServerRegistry || !f.SkillRegistry {
		t.Fatalf("bool env parsing failed: %#v", f)
	}
	if f.AgentRegistry || f.HooksV2 {
		t.Fatalf("false values should remain false: %#v", f)
	}
	if f.DiagnosticProtocol {
		t.Fatalf("diagnostic protocol should parse explicit 0 as disabled: %#v", f)
	}

	if want := []string{"alpha", "beta", "gamma"}; !reflect.DeepEqual(f.DisabledTools, want) {
		t.Fatalf("disabled tools mismatch: got %#v want %#v", f.DisabledTools, want)
	}
	if want := []string{"delta", "epsilon"}; !reflect.DeepEqual(f.DeferredTools, want) {
		t.Fatalf("deferred tools mismatch: got %#v want %#v", f.DeferredTools, want)
	}
	if want := []string{"zeta", "eta"}; !reflect.DeepEqual(f.ExperimentalMetaTools, want) {
		t.Fatalf("experimental tools mismatch: got %#v want %#v", f.ExperimentalMetaTools, want)
	}
}

func TestFromEnvDiagnosticProtocolDefaultsOn(t *testing.T) {
	f := FromEnv(func(string) string { return "" })
	if !f.DiagnosticProtocol {
		t.Fatalf("diagnostic protocol should default on when env is unset: %#v", f)
	}
}

func TestIsToolVisibleDisabledTool(t *testing.T) {
	f := Flags{DisabledTools: []string{"blocked"}}
	meta := tooling.ToolMetadata{Name: "blocked", Origin: tooling.ToolOriginBuiltin}
	if f.IsToolVisible(meta) {
		t.Fatalf("disabled tool should not be visible")
	}
}

func TestIsToolVisibleExperimentalMetaToolRespectsToolSearch(t *testing.T) {
	meta := tooling.ToolMetadata{Name: "exp", Origin: tooling.ToolOriginMeta}

	f := Flags{ExperimentalMetaTools: []string{"exp"}}
	if f.IsToolVisible(meta) {
		t.Fatalf("experimental meta tool should be hidden when tool search is off")
	}

	f.ToolSearch = true
	if !f.IsToolVisible(meta) {
		t.Fatalf("experimental meta tool should be visible when tool search is on")
	}
}

func TestIsToolVisibleExperimentalMetaToolByNameWithoutOrigin(t *testing.T) {
	meta := tooling.ToolMetadata{Name: "exp"}

	f := Flags{ExperimentalMetaTools: []string{"exp"}}
	if f.IsToolVisible(meta) {
		t.Fatalf("experimental meta tool should be hidden by explicit name even without origin")
	}

	f.ToolSearch = true
	if !f.IsToolVisible(meta) {
		t.Fatalf("experimental meta tool should be visible when tool search is on")
	}
}

func TestApplyToolMetadataDeferredTool(t *testing.T) {
	original := tooling.ToolMetadata{
		Name:        "defer-me",
		Aliases:     []string{"alias"},
		Description: "desc",
		Origin:      tooling.ToolOriginBuiltin,
		MCPInfo:     tooling.MCPInfo{Raw: []byte(`{"a":1}`)},
	}

	f := Flags{DeferredTools: []string{"defer-me"}}
	out := f.ApplyToolMetadata(original)

	if !out.ShouldDefer {
		t.Fatalf("deferred tool should set ShouldDefer")
	}
	if original.ShouldDefer {
		t.Fatalf("original metadata must not be modified")
	}
	if !reflect.DeepEqual(out.Aliases, original.Aliases) || out.Description != original.Description || out.Origin != original.Origin {
		t.Fatalf("metadata fields should remain unchanged: %#v", out)
	}

	out.Aliases[0] = "changed"
	out.MCPInfo.Raw[0] = '{'
	if original.Aliases[0] != "alias" {
		t.Fatalf("aliases must be deep copied")
	}
	if string(original.MCPInfo.Raw) != `{"a":1}` {
		t.Fatalf("raw MCP payload must be deep copied")
	}
}
