package promptcompiler

import (
	"strings"
	"testing"
)

func TestCompileIncludesPromptFingerprint(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		SessionType: "host",
		Mode:        "execute",
		HostContext: "server-local",
		ProtocolState: ProtocolPromptState{Items: []ProtocolPromptItem{
			{Kind: "approval", ID: "approval-1", Status: "pending", Text: "exec_command needs approval"},
		}},
	})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	fp := compiled.Fingerprint
	if fp.Version == "" {
		t.Fatal("fingerprint version is empty")
	}
	for name, value := range map[string]string{
		"stable":         fp.StableHash,
		"system":         fp.SystemHash,
		"developer":      fp.DeveloperHash,
		"tool_registry":  fp.ToolRegistryHash,
		"runtime_policy": fp.RuntimePolicyHash,
		"protocol_state": fp.ProtocolStateHash,
	} {
		if value == "" {
			t.Fatalf("%s hash is empty: %#v", name, fp)
		}
	}
}

func TestPromptFingerprintChangesOnlyForChangedLayer(t *testing.T) {
	base, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute", HostContext: "server-a"})
	if err != nil {
		t.Fatalf("Compile base failed: %v", err)
	}
	changedSystem, err := NewCompiler().Compile(CompileContext{SessionType: "host", Mode: "execute", HostContext: "server-b"})
	if err != nil {
		t.Fatalf("Compile changed system failed: %v", err)
	}
	if base.Fingerprint.SystemHash != changedSystem.Fingerprint.SystemHash {
		t.Fatal("base contract hash should not change when only host context changes")
	}
	if base.Fingerprint.DeveloperHash == changedSystem.Fingerprint.DeveloperHash {
		t.Fatal("profile hash should change when host context changes")
	}
	if base.Fingerprint.StableHash == changedSystem.Fingerprint.StableHash {
		t.Fatal("stable envelope hash should change when host profile context changes")
	}

	changedProtocol, err := NewCompiler().Compile(CompileContext{
		SessionType: "host",
		Mode:        "execute",
		HostContext: "server-a",
		ProtocolState: ProtocolPromptState{Items: []ProtocolPromptItem{
			{Kind: "approval", ID: "approval-1", Status: "pending", Text: "approval required"},
		}},
	})
	if err != nil {
		t.Fatalf("Compile changed protocol failed: %v", err)
	}
	if base.Fingerprint.ProtocolStateHash == changedProtocol.Fingerprint.ProtocolStateHash {
		t.Fatal("protocol state hash should change when protocol state changes")
	}
	if base.Fingerprint.StableHash != changedProtocol.Fingerprint.StableHash {
		t.Fatal("stable hash should not change when only protocol state changes")
	}
}

func TestProtocolStateRendersFailureSwitchPathReason(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		SessionType: "host",
		Mode:        "inspect",
		ProtocolState: ProtocolPromptState{
			FailureSwitchPath: &FailureSwitchPathPromptState{
				Signature:        "failure:abc123",
				SeenCount:        3,
				Action:           "do_not_repeat_same_path",
				SwitchPathReason: "same normalized failure repeated; use an independent read-only evidence source",
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	content := compiled.Dynamic.Content
	for _, want := range []string{
		"## Failure Switch-path State",
		"action: do_not_repeat_same_path",
		"signature: failure:abc123",
		"seen_count: 3",
		"switch_path_reason: same normalized failure repeated; use an independent read-only evidence source",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("protocol content missing %q:\n%s", want, content)
		}
	}
}
