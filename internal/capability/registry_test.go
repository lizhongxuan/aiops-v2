package capability

import (
	"context"
	"encoding/json"
	"testing"
)

// mockTool implements ToolRuntime for testing.
type mockTool struct {
	name        string
	readOnly    bool
	destructive bool
	concurrent  bool
}

func (m *mockTool) Description() string                      { return m.name + " description" }
func (m *mockTool) CheckPermissions(_ context.Context) error { return nil }
func (m *mockTool) IsReadOnly() bool                         { return m.readOnly }
func (m *mockTool) IsDestructive() bool                      { return m.destructive }
func (m *mockTool) IsConcurrencySafe() bool                  { return m.concurrent }
func (m *mockTool) Display() ToolDisplayPayload {
	return ToolDisplayPayload{Type: "text", Title: m.name}
}
func (m *mockTool) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (m *mockTool) Execute(_ context.Context, _ json.RawMessage) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}

func TestKindIsValid(t *testing.T) {
	for _, k := range AllKinds() {
		if !k.IsValid() {
			t.Errorf("AllKinds() returned invalid kind %q", k)
		}
	}
	if Kind("unknown").IsValid() {
		t.Error("expected 'unknown' kind to be invalid")
	}
}

func TestEntryValidate(t *testing.T) {
	tests := []struct {
		name    string
		entry   Entry
		wantErr bool
	}{
		{
			name:    "empty id",
			entry:   Entry{Name: "test", Kind: KindSkill},
			wantErr: true,
		},
		{
			name:    "empty name",
			entry:   Entry{ID: "test", Kind: KindSkill},
			wantErr: true,
		},
		{
			name:    "invalid kind",
			entry:   Entry{ID: "test", Name: "test", Kind: "bad"},
			wantErr: true,
		},
		{
			name:    "tool kind without ToolRuntime",
			entry:   Entry{ID: "test", Name: "test", Kind: KindTool},
			wantErr: true,
		},
		{
			name:    "tool kind with ToolRuntime",
			entry:   Entry{ID: "test", Name: "test", Kind: KindTool, Tool: &mockTool{name: "test"}},
			wantErr: false,
		},
		{
			name:    "skill kind without tool is ok",
			entry:   Entry{ID: "test", Name: "test", Kind: KindSkill},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.entry.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()

	entry := Entry{
		ID:   "tool:disk_usage",
		Name: "disk_usage",
		Kind: KindTool,
		Tool: &mockTool{name: "disk_usage", readOnly: true},
	}

	if err := r.Register(entry); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, ok := r.Get("tool:disk_usage")
	if !ok {
		t.Fatal("Get() returned false for registered entry")
	}
	if got.Name != "disk_usage" {
		t.Errorf("Get().Name = %q, want %q", got.Name, "disk_usage")
	}
}

func TestRegistryUnregister(t *testing.T) {
	r := NewRegistry()

	entry := Entry{ID: "s1", Name: "skill1", Kind: KindSkill}
	_ = r.Register(entry)

	r.Unregister("s1")

	if _, ok := r.Get("s1"); ok {
		t.Error("Get() returned true after Unregister")
	}
}

func TestRegistryRegisterBatch(t *testing.T) {
	r := NewRegistry()

	entries := []Entry{
		{ID: "s1", Name: "skill1", Kind: KindSkill},
		{ID: "s2", Name: "skill2", Kind: KindSkill},
	}

	if err := r.RegisterBatch(entries); err != nil {
		t.Fatalf("RegisterBatch() error = %v", err)
	}

	if _, ok := r.Get("s1"); !ok {
		t.Error("s1 not found after batch register")
	}
	if _, ok := r.Get("s2"); !ok {
		t.Error("s2 not found after batch register")
	}
}

func TestRegistryRegisterBatchAtomicity(t *testing.T) {
	r := NewRegistry()

	// Second entry is invalid (tool kind without ToolRuntime)
	entries := []Entry{
		{ID: "s1", Name: "skill1", Kind: KindSkill},
		{ID: "bad", Name: "bad", Kind: KindTool}, // missing Tool
	}

	if err := r.RegisterBatch(entries); err == nil {
		t.Fatal("RegisterBatch() should fail with invalid entry")
	}

	// Neither should be registered
	if _, ok := r.Get("s1"); ok {
		t.Error("s1 should not be registered after failed batch")
	}
}

func TestVisibleCapabilities(t *testing.T) {
	r := NewRegistry()

	// Register entries with different visibility
	_ = r.Register(Entry{
		ID: "t1", Name: "tool1", Kind: KindTool,
		Tool:       &mockTool{name: "tool1"},
		Visibility: Visibility{SessionTypes: []string{"host"}},
	})
	_ = r.Register(Entry{
		ID: "w1", Name: "ws_tool", Kind: KindWorkspace,
		Visibility: Visibility{SessionTypes: []string{"workspace"}},
	})
	_ = r.Register(Entry{
		ID: "s1", Name: "skill1", Kind: KindSkill,
		// No visibility constraints = visible everywhere
	})

	// Host session should see tool1 and skill1, not ws_tool
	hostVisible := r.VisibleCapabilities("host", "chat")
	if len(hostVisible) != 2 {
		t.Errorf("host visible count = %d, want 2", len(hostVisible))
	}

	// Workspace session should see ws_tool and skill1, not tool1
	wsVisible := r.VisibleCapabilities("workspace", "chat")
	if len(wsVisible) != 2 {
		t.Errorf("workspace visible count = %d, want 2", len(wsVisible))
	}
}

func TestVisibleCapabilitiesModeFiltering(t *testing.T) {
	r := NewRegistry()

	// Tool visible only in execute mode
	_ = r.Register(Entry{
		ID: "t1", Name: "mutation_tool", Kind: KindTool,
		Tool:       &mockTool{name: "mutation_tool"},
		Visibility: Visibility{Modes: []string{"execute"}},
	})
	// Tool visible in all modes (no constraint)
	_ = r.Register(Entry{
		ID: "t2", Name: "read_tool", Kind: KindTool,
		Tool: &mockTool{name: "read_tool"},
	})
	// Tool visible only in inspect and execute modes
	_ = r.Register(Entry{
		ID: "t3", Name: "inspect_tool", Kind: KindTool,
		Tool:       &mockTool{name: "inspect_tool"},
		Visibility: Visibility{Modes: []string{"inspect", "execute"}},
	})

	// Chat mode should only see read_tool (no mode constraint)
	chatVisible := r.VisibleCapabilities("host", "chat")
	if len(chatVisible) != 1 {
		t.Errorf("chat mode visible count = %d, want 1", len(chatVisible))
	}
	if len(chatVisible) > 0 && chatVisible[0].Name != "read_tool" {
		t.Errorf("chat mode visible[0].Name = %q, want %q", chatVisible[0].Name, "read_tool")
	}

	// Execute mode should see all three
	execVisible := r.VisibleCapabilities("host", "execute")
	if len(execVisible) != 3 {
		t.Errorf("execute mode visible count = %d, want 3", len(execVisible))
	}

	// Inspect mode should see read_tool and inspect_tool
	inspectVisible := r.VisibleCapabilities("host", "inspect")
	if len(inspectVisible) != 2 {
		t.Errorf("inspect mode visible count = %d, want 2", len(inspectVisible))
	}
}

func TestVisibleCapabilitiesSessionAndModeCombo(t *testing.T) {
	r := NewRegistry()

	// Workspace-only tool, execute-only mode
	_ = r.Register(Entry{
		ID: "w1", Name: "ws_exec_tool", Kind: KindWorkspace,
		Visibility: Visibility{
			SessionTypes: []string{"workspace"},
			Modes:        []string{"execute"},
		},
	})

	// Should NOT be visible in workspace+chat
	wsChat := r.VisibleCapabilities("workspace", "chat")
	for _, e := range wsChat {
		if e.ID == "w1" {
			t.Error("w1 should not be visible in workspace+chat")
		}
	}

	// Should NOT be visible in host+execute
	hostExec := r.VisibleCapabilities("host", "execute")
	for _, e := range hostExec {
		if e.ID == "w1" {
			t.Error("w1 should not be visible in host+execute")
		}
	}

	// Should be visible in workspace+execute
	wsExec := r.VisibleCapabilities("workspace", "execute")
	found := false
	for _, e := range wsExec {
		if e.ID == "w1" {
			found = true
		}
	}
	if !found {
		t.Error("w1 should be visible in workspace+execute")
	}
}

func TestRegisterDuplicateID(t *testing.T) {
	r := NewRegistry()

	entry1 := Entry{ID: "t1", Name: "tool1", Kind: KindSkill}
	entry2 := Entry{ID: "t1", Name: "tool1_updated", Kind: KindSkill}

	_ = r.Register(entry1)
	_ = r.Register(entry2)

	got, ok := r.Get("t1")
	if !ok {
		t.Fatal("Get() returned false for registered entry")
	}
	// Last registration wins
	if got.Name != "tool1_updated" {
		t.Errorf("Get().Name = %q, want %q (last registration should win)", got.Name, "tool1_updated")
	}
}

func TestRegisterAllSixKinds(t *testing.T) {
	r := NewRegistry()

	entries := []Entry{
		{ID: "tool1", Name: "tool1", Kind: KindTool, Tool: &mockTool{name: "tool1"}},
		{ID: "skill1", Name: "skill1", Kind: KindSkill},
		{ID: "mcp1", Name: "mcp1", Kind: KindMCPTool},
		{ID: "ui1", Name: "ui1", Kind: KindUISurface},
		{ID: "mode1", Name: "mode1", Kind: KindModeRule},
		{ID: "ws1", Name: "ws1", Kind: KindWorkspace},
	}

	if err := r.RegisterBatch(entries); err != nil {
		t.Fatalf("RegisterBatch() error = %v", err)
	}

	for _, e := range entries {
		got, ok := r.Get(e.ID)
		if !ok {
			t.Errorf("Get(%q) returned false", e.ID)
			continue
		}
		if got.Kind != e.Kind {
			t.Errorf("Get(%q).Kind = %q, want %q", e.ID, got.Kind, e.Kind)
		}
	}
}

func TestEinoToolAdapter(t *testing.T) {
	tool := &mockTool{name: "disk_usage", readOnly: true}
	entry := Entry{ID: "t1", Name: "disk_usage", Kind: KindTool, Tool: tool}
	r := NewRegistry()

	adapter := NewEinoToolAdapter(tool, entry, r)
	def := adapter.ToEinoTool()

	if def.Name != "disk_usage" {
		t.Errorf("ToEinoTool().Name = %q, want %q", def.Name, "disk_usage")
	}
	if def.Desc != "disk_usage description" {
		t.Errorf("ToEinoTool().Desc = %q, want %q", def.Desc, "disk_usage description")
	}

	result, err := adapter.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "ok" {
		t.Errorf("Execute().Content = %q, want %q", result.Content, "ok")
	}
}

func TestAssembleToolPoolPriority(t *testing.T) {
	r := NewRegistry()

	// Built-in tool
	_ = r.Register(Entry{
		ID: "builtin:read_file", Name: "read_file", Kind: KindTool,
		Tool: &mockTool{name: "read_file_builtin"},
	})

	// MCP tool with same name
	_ = r.Register(Entry{
		ID: "mcp:read_file", Name: "read_file", Kind: KindMCPTool,
		Tool: &mockTool{name: "read_file_mcp"},
	})

	// MCP tool with unique name
	_ = r.Register(Entry{
		ID: "mcp:coroot_list", Name: "coroot_list", Kind: KindMCPTool,
		Tool: &mockTool{name: "coroot_list"},
	})

	pool := r.AssembleToolPool("host", "chat")

	// Should have 2 tools: built-in read_file (priority) + coroot_list
	if len(pool) != 2 {
		t.Fatalf("AssembleToolPool() len = %d, want 2", len(pool))
	}

	// The read_file entry should use built-in description
	for _, bt := range pool {
		info, err := bt.Info(context.Background())
		if err != nil {
			t.Fatalf("Info() error: %v", err)
		}
		if info.Name == "read_file" {
			if info.Desc != "read_file_builtin description" {
				t.Errorf("read_file should use built-in, got description %q", info.Desc)
			}
		}
	}
}

func TestAssembleToolsCompatibility(t *testing.T) {
	r := NewRegistry()

	_ = r.Register(Entry{
		ID: "builtin:read_file", Name: "read_file", Kind: KindTool,
		Tool: &mockTool{name: "read_file_builtin"},
	})
	_ = r.Register(Entry{
		ID: "mcp:read_file", Name: "read_file", Kind: KindMCPTool,
		Tool: &mockTool{name: "read_file_mcp"},
	})
	_ = r.Register(Entry{
		ID: "mcp:coroot_list", Name: "coroot_list", Kind: KindMCPTool,
		Tool: &mockTool{name: "coroot_list"},
	})

	tools := r.AssembleTools("host", "chat")
	if len(tools) != 2 {
		t.Fatalf("AssembleTools() len = %d, want 2", len(tools))
	}

	pool := r.AssembleToolPool("host", "chat")
	if len(pool) != len(tools) {
		t.Fatalf("AssembleToolPool() len = %d, want %d", len(pool), len(tools))
	}

	for i, ttool := range tools {
		meta := ttool.Metadata()
		info, err := pool[i].Info(context.Background())
		if err != nil {
			t.Fatalf("Info() error: %v", err)
		}
		if meta.Name == "read_file" && meta.Description != "read_file_builtin description" {
			t.Fatalf("AssembleTools() should pick built-in read_file, got %q", meta.Description)
		}
		if meta.Name != info.Name {
			t.Fatalf("tool name mismatch: AssembleTools=%q pool=%q", meta.Name, info.Name)
		}
	}
}

func TestSkillPromptAssets(t *testing.T) {
	r := NewRegistry()

	_ = r.Register(Entry{
		ID:          "skill:deploy",
		Name:        "deploy",
		Kind:        KindSkill,
		Description: "Review rollout plans before deploy.",
		Visibility: Visibility{
			SessionTypes: []string{"host"},
			Modes:        []string{"chat"},
		},
	})
	_ = r.Register(Entry{
		ID:          "skill:hidden",
		Name:        "hidden",
		Kind:        KindSkill,
		Description: "Should not be visible here.",
		Visibility: Visibility{
			SessionTypes: []string{"workspace"},
			Modes:        []string{"execute"},
		},
	})

	assets := r.SkillPromptAssets("host", "chat")
	if len(assets) != 1 {
		t.Fatalf("SkillPromptAssets() len = %d, want 1", len(assets))
	}
	if assets[0] != "Skill available: deploy - Review rollout plans before deploy." {
		t.Fatalf("SkillPromptAssets()[0] = %q", assets[0])
	}
}

func TestMCPPromptAssets(t *testing.T) {
	r := NewRegistry()

	_ = r.Register(Entry{
		ID:          "coroot/list_services",
		Name:        "coroot.list_services",
		Kind:        KindMCPTool,
		Description: "List Coroot services.",
		Visibility: Visibility{
			SessionTypes: []string{"host"},
			Modes:        []string{"inspect"},
		},
	})
	_ = r.Register(Entry{
		ID:          "coroot/hidden",
		Name:        "coroot.hidden",
		Kind:        KindMCPTool,
		Description: "Should not be visible here.",
		Visibility: Visibility{
			SessionTypes: []string{"workspace"},
			Modes:        []string{"execute"},
		},
	})

	assets := r.MCPPromptAssets("host", "inspect")
	if len(assets) != 1 {
		t.Fatalf("MCPPromptAssets() len = %d, want 1", len(assets))
	}
	if assets[0] != "MCP tool available: coroot.list_services - List Coroot services." {
		t.Fatalf("MCPPromptAssets()[0] = %q", assets[0])
	}
}
