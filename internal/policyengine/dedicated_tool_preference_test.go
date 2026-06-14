package policyengine

import (
	"encoding/json"
	"testing"

	"aiops-v2/internal/tooling"
)

type syntheticDiscoveryMetadata struct {
	CapabilityKind string
	ResourceTypes  []string
	OperationKinds []string
}

type syntheticVisibleToolMetadata struct {
	tooling.ToolMetadata
	Discovery syntheticDiscoveryMetadata
}

func TestDedicatedToolPreferenceRequiresReasonForEquivalentReadOnlyShellFallback(t *testing.T) {
	decision := EvaluateDedicatedToolPreference("exec_command", json.RawMessage(`{
		"command": "curl",
		"args": ["-sS", "https://synthetic.example/resource"]
	}`), []syntheticVisibleToolMetadata{
		{
			ToolMetadata: tooling.ToolMetadata{Name: "synthetic.fetch_resource"},
			Discovery: syntheticDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"network"},
				OperationKinds: []string{"read"},
			},
		},
	}, "")

	if decision.Action != DedicatedToolPreferenceRequireReason {
		t.Fatalf("Action = %q, want %q (decision: %#v)", decision.Action, DedicatedToolPreferenceRequireReason, decision)
	}
	if len(decision.PreferredTools) != 1 || decision.PreferredTools[0] != "synthetic.fetch_resource" {
		t.Fatalf("PreferredTools = %#v, want synthetic.fetch_resource", decision.PreferredTools)
	}
}

func TestDedicatedToolPreferenceAllowsReadOnlyShellFallbackWithReason(t *testing.T) {
	decision := EvaluateDedicatedToolPreference("exec_command", json.RawMessage(`{
		"command": "curl",
		"args": ["-sS", "https://synthetic.example/resource"]
	}`), []syntheticVisibleToolMetadata{
		{
			ToolMetadata: tooling.ToolMetadata{Name: "synthetic.fetch_resource"},
			Discovery: syntheticDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"network"},
				OperationKinds: []string{"read"},
			},
		},
	}, "need response headers that the visible tool does not expose")

	if decision.Action != DedicatedToolPreferenceAllow {
		t.Fatalf("Action = %q, want %q (decision: %#v)", decision.Action, DedicatedToolPreferenceAllow, decision)
	}
}

func TestDedicatedToolPreferenceRejectsMutatingShellFallbackWhenDedicatedToolMatches(t *testing.T) {
	decision := EvaluateDedicatedToolPreference("exec_command", json.RawMessage(`{
		"command": "touch",
		"args": ["synthetic-output.txt"]
	}`), []syntheticVisibleToolMetadata{
		{
			ToolMetadata: tooling.ToolMetadata{Name: "synthetic.write_resource", Mutating: true},
			Discovery: syntheticDiscoveryMetadata{
				CapabilityKind: "write",
				ResourceTypes:  []string{"file"},
				OperationKinds: []string{"write"},
			},
		},
	}, "manual fallback")

	if decision.Action != DedicatedToolPreferenceRejectPreferDedicatedTool {
		t.Fatalf("Action = %q, want %q (decision: %#v)", decision.Action, DedicatedToolPreferenceRejectPreferDedicatedTool, decision)
	}
}

func TestDedicatedToolPreferenceAllowsShellFallbackWithoutEquivalentVisibleTool(t *testing.T) {
	decision := EvaluateDedicatedToolPreference("exec_command", json.RawMessage(`{
		"command": "curl",
		"args": ["-sS", "https://synthetic.example/resource"]
	}`), []syntheticVisibleToolMetadata{
		{
			ToolMetadata: tooling.ToolMetadata{Name: "synthetic.search_records"},
			Discovery: syntheticDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"record"},
				OperationKinds: []string{"search"},
			},
		},
	}, "")

	if decision.Action != DedicatedToolPreferenceAllow {
		t.Fatalf("Action = %q, want %q (decision: %#v)", decision.Action, DedicatedToolPreferenceAllow, decision)
	}
}

func TestDedicatedToolPreferenceDoesNotPreferCurrentShellTool(t *testing.T) {
	decision := EvaluateDedicatedToolPreference("exec_command", json.RawMessage(`{
		"command": "customdiag"
	}`), []syntheticVisibleToolMetadata{
		{
			ToolMetadata: tooling.ToolMetadata{Name: "exec_command"},
			Discovery: syntheticDiscoveryMetadata{
				CapabilityKind: "execute",
				ResourceTypes:  []string{"host", "system"},
				OperationKinds: []string{"inspect", "read", "execute"},
			},
		},
	}, "")

	if decision.Action != DedicatedToolPreferenceAllow {
		t.Fatalf("Action = %q, want %q (decision: %#v)", decision.Action, DedicatedToolPreferenceAllow, decision)
	}
}
