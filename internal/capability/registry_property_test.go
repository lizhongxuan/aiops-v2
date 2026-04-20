package capability

// Feature: aiops-codex-eino-rewrite, Property 5: 六类能力注册完整性

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cloudwego/eino/schema"

	"pgregory.net/rapid"
)

// **Validates: Requirements 2.1, 2.2**

// --- Generators ---

// genKind generates one of the six valid capability kinds.
func genKind() *rapid.Generator[Kind] {
	return rapid.SampledFrom(allKinds)
}

// genNonToolKind generates a valid kind that is NOT KindTool (no ToolRuntime required).
func genNonToolKind() *rapid.Generator[Kind] {
	nonToolKinds := []Kind{KindSkill, KindMCPTool, KindUISurface, KindModeRule, KindWorkspace}
	return rapid.SampledFrom(nonToolKinds)
}

// genID generates a non-empty string suitable for entry IDs.
func genID() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-z][a-z0-9_]{2,20}`)
}

// genName generates a non-empty string suitable for entry names.
func genName() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-z][a-z0-9_]{2,20}`)
}

// propMockTool is a minimal ToolRuntime for property tests.
type propMockTool struct{}

func (m *propMockTool) Description() string                                          { return "prop test tool" }
func (m *propMockTool) CheckPermissions(_ context.Context) error                     { return nil }
func (m *propMockTool) IsReadOnly() bool                                             { return false }
func (m *propMockTool) IsDestructive() bool                                          { return false }
func (m *propMockTool) IsConcurrencySafe() bool                                      { return true }
func (m *propMockTool) Display() ToolDisplayPayload                                  { return ToolDisplayPayload{Type: "text"} }
func (m *propMockTool) InputSchema() json.RawMessage                                 { return json.RawMessage(`{"type":"object"}`) }
func (m *propMockTool) Execute(_ context.Context, _ json.RawMessage) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}

// TestProperty5_RegistrationSucceedsForAllSixKinds verifies that for any valid
// capability entry (one of six Kinds), registering to Capability Registry succeeds
// and Kind is correctly preserved after retrieval.
func TestProperty5_RegistrationSucceedsForAllSixKinds(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		registry := NewRegistry()

		kind := genKind().Draw(t, "kind")
		id := genID().Draw(t, "id")
		name := genName().Draw(t, "name")

		entry := Entry{
			ID:   id,
			Name: name,
			Kind: kind,
		}

		// tool:* kind requires a ToolRuntime
		if kind == KindTool {
			entry.Tool = &propMockTool{}
		}

		err := registry.Register(entry)
		if err != nil {
			t.Fatalf("Register should succeed for valid entry with kind %q: %v", kind, err)
		}

		// Verify the entry can be retrieved and Kind is preserved
		got, ok := registry.Get(id)
		if !ok {
			t.Fatalf("Get(%q) returned false after successful registration", id)
		}
		if got.Kind != kind {
			t.Fatalf("Kind not preserved: registered %q, got %q", kind, got.Kind)
		}
		if got.ID != id {
			t.Fatalf("ID not preserved: registered %q, got %q", id, got.ID)
		}
		if got.Name != name {
			t.Fatalf("Name not preserved: registered %q, got %q", name, got.Name)
		}
	})
}

// TestProperty5_ToolKindWithoutToolRuntimeIsRejected verifies that for tool:* type,
// registration without a complete UnifiedTool contract (nil ToolRuntime) is rejected.
func TestProperty5_ToolKindWithoutToolRuntimeIsRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		registry := NewRegistry()

		id := genID().Draw(t, "id")
		name := genName().Draw(t, "name")

		entry := Entry{
			ID:   id,
			Name: name,
			Kind: KindTool,
			Tool: nil, // deliberately nil — violates UnifiedTool contract
		}

		err := registry.Register(entry)
		if err == nil {
			t.Fatalf("Register should reject tool kind entry %q without ToolRuntime", id)
		}

		// Verify the entry was NOT stored
		_, ok := registry.Get(id)
		if ok {
			t.Fatalf("Entry %q should not be in registry after rejected registration", id)
		}
	})
}

// TestProperty5_NonToolKindWithoutToolRuntimeSucceeds verifies that non-tool kinds
// do NOT require a ToolRuntime and registration succeeds without one.
func TestProperty5_NonToolKindWithoutToolRuntimeSucceeds(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		registry := NewRegistry()

		kind := genNonToolKind().Draw(t, "kind")
		id := genID().Draw(t, "id")
		name := genName().Draw(t, "name")

		entry := Entry{
			ID:   id,
			Name: name,
			Kind: kind,
			Tool: nil, // non-tool kinds don't need ToolRuntime
		}

		err := registry.Register(entry)
		if err != nil {
			t.Fatalf("Register should succeed for non-tool kind %q without ToolRuntime: %v", kind, err)
		}

		got, ok := registry.Get(id)
		if !ok {
			t.Fatalf("Get(%q) returned false after successful registration", id)
		}
		if got.Kind != kind {
			t.Fatalf("Kind not preserved: registered %q, got %q", kind, got.Kind)
		}
	})
}

// TestProperty5_BatchRegistrationPreservesAllKinds verifies that batch registration
// of entries across all six kinds succeeds and preserves each entry's Kind.
func TestProperty5_BatchRegistrationPreservesAllKinds(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		registry := NewRegistry()

		// Generate a batch with one entry per kind
		var entries []Entry
		for i, kind := range allKinds {
			id := fmt.Sprintf("entry_%d_%s", i, genID().Draw(t, fmt.Sprintf("id_%d", i)))
			name := genName().Draw(t, fmt.Sprintf("name_%d", i))

			entry := Entry{
				ID:   id,
				Name: name,
				Kind: kind,
			}
			if kind == KindTool {
				entry.Tool = &propMockTool{}
			}
			entries = append(entries, entry)
		}

		err := registry.RegisterBatch(entries)
		if err != nil {
			t.Fatalf("RegisterBatch should succeed for valid entries across all kinds: %v", err)
		}

		// Verify all entries are retrievable with correct Kind
		for _, expected := range entries {
			got, ok := registry.Get(expected.ID)
			if !ok {
				t.Fatalf("Get(%q) returned false after batch registration", expected.ID)
			}
			if got.Kind != expected.Kind {
				t.Fatalf("Kind not preserved for %q: registered %q, got %q", expected.ID, expected.Kind, got.Kind)
			}
		}
	})
}

// Feature: aiops-codex-eino-rewrite, Property 6: UnifiedTool → Eino 适配器保真

// **Validates: Requirements 2.3**

// --- Property 6 Generators & Helpers ---

// genToolName generates a random tool name (alphanumeric with dots/underscores).
func genToolName() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-z][a-z0-9_.]{2,30}`)
}

// genDescription generates a random tool description string.
func genDescription() *rapid.Generator[string] {
	return rapid.StringMatching(`[A-Za-z][A-Za-z0-9 ,.\-]{5,80}`)
}

// genInputSchema generates a random but valid JSON Schema object for tool input.
func genInputSchema() *rapid.Generator[json.RawMessage] {
	return rapid.Custom[json.RawMessage](func(t *rapid.T) json.RawMessage {
		// Generate a random number of properties
		numProps := rapid.IntRange(0, 5).Draw(t, "numProps")
		props := make(map[string]interface{})
		required := []string{}

		for i := 0; i < numProps; i++ {
			propName := fmt.Sprintf("prop_%d", i)
			propType := rapid.SampledFrom([]string{"string", "number", "boolean", "integer"}).Draw(t, fmt.Sprintf("propType_%d", i))
			props[propName] = map[string]interface{}{
				"type":        propType,
				"description": fmt.Sprintf("Property %d description", i),
			}
			// Randomly mark some properties as required
			if rapid.Bool().Draw(t, fmt.Sprintf("required_%d", i)) {
				required = append(required, propName)
			}
		}

		schemaObj := map[string]interface{}{
			"type":       "object",
			"properties": props,
		}
		if len(required) > 0 {
			schemaObj["required"] = required
		}

		data, err := json.Marshal(schemaObj)
		if err != nil {
			t.Fatalf("failed to marshal schema: %v", err)
		}
		return json.RawMessage(data)
	})
}

// fidelityMockTool is a configurable ToolRuntime mock for Property 6 tests.
type fidelityMockTool struct {
	description string
	inputSchema json.RawMessage
}

func (m *fidelityMockTool) Description() string                                          { return m.description }
func (m *fidelityMockTool) CheckPermissions(_ context.Context) error                     { return nil }
func (m *fidelityMockTool) IsReadOnly() bool                                             { return false }
func (m *fidelityMockTool) IsDestructive() bool                                          { return false }
func (m *fidelityMockTool) IsConcurrencySafe() bool                                      { return true }
func (m *fidelityMockTool) Display() ToolDisplayPayload                                  { return ToolDisplayPayload{Type: "text"} }
func (m *fidelityMockTool) InputSchema() json.RawMessage                                 { return m.inputSchema }
func (m *fidelityMockTool) Execute(_ context.Context, _ json.RawMessage) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}

// TestProperty6_EinoAdapterPreservesNameDescriptionSchema verifies that for any
// UnifiedTool definition, through EinoToolAdapter conversion to Eino *schema.ToolInfo,
// the tool's name and description are completely preserved.
func TestProperty6_EinoAdapterPreservesNameDescriptionSchema(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random tool attributes
		toolName := genToolName().Draw(t, "toolName")
		description := genDescription().Draw(t, "description")
		inputSchema := genInputSchema().Draw(t, "inputSchema")

		// Create a mock ToolRuntime with the generated values
		mt := &fidelityMockTool{
			description: description,
			inputSchema: inputSchema,
		}

		// Create an Entry with the generated name
		entry := Entry{
			ID:   toolName,
			Name: toolName,
			Kind: KindTool,
			Tool: mt,
		}

		// Create the adapter and convert to Eino *schema.ToolInfo
		registry := NewRegistry()
		adapter := NewEinoToolAdapter(mt, entry, registry)
		einoDef := adapter.ToEinoTool()

		// Verify name is preserved
		if einoDef.Name != toolName {
			t.Fatalf("Name not preserved: expected %q, got %q", toolName, einoDef.Name)
		}

		// Verify description is preserved
		if einoDef.Desc != description {
			t.Fatalf("Description not preserved: expected %q, got %q", description, einoDef.Desc)
		}

		// Verify ParamsOneOf is non-nil (schema was provided)
		if einoDef.ParamsOneOf == nil {
			t.Fatal("ParamsOneOf should not be nil when inputSchema is provided")
		}
	})
}

// TestProperty6_EinoAdapterPreservesSchemaWithEmptyProperties verifies that
// the adapter correctly handles edge cases like empty schemas.
func TestProperty6_EinoAdapterPreservesSchemaWithEmptyProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := genToolName().Draw(t, "toolName")
		description := genDescription().Draw(t, "description")

		// Use a minimal schema with no properties
		minimalSchema := json.RawMessage(`{"type":"object"}`)

		mt := &fidelityMockTool{
			description: description,
			inputSchema: minimalSchema,
		}

		entry := Entry{
			ID:   toolName,
			Name: toolName,
			Kind: KindTool,
			Tool: mt,
		}

		registry := NewRegistry()
		adapter := NewEinoToolAdapter(mt, entry, registry)
		einoDef := adapter.ToEinoTool()

		if einoDef.Name != toolName {
			t.Fatalf("Name not preserved: expected %q, got %q", toolName, einoDef.Name)
		}
		if einoDef.Desc != description {
			t.Fatalf("Description not preserved: expected %q, got %q", description, einoDef.Desc)
		}
		if einoDef.ParamsOneOf == nil {
			t.Fatal("ParamsOneOf should not be nil for minimal schema")
		}
	})
}

// TestProperty6_EinoAdapterPreservesComplexNestedSchema verifies that deeply
// nested JSON schemas are preserved through the adapter conversion.
func TestProperty6_EinoAdapterPreservesComplexNestedSchema(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := genToolName().Draw(t, "toolName")
		description := genDescription().Draw(t, "description")

		// Generate a complex nested schema
		numNested := rapid.IntRange(1, 3).Draw(t, "numNested")
		nestedProps := make(map[string]interface{})
		for i := 0; i < numNested; i++ {
			nestedProps[fmt.Sprintf("nested_%d", i)] = map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"inner_field": map[string]interface{}{
						"type": "string",
					},
				},
			}
		}

		schemaObj := map[string]interface{}{
			"type":       "object",
			"properties": nestedProps,
		}
		schemaBytes, err := json.Marshal(schemaObj)
		if err != nil {
			t.Fatalf("failed to marshal schema: %v", err)
		}
		inputSchema := json.RawMessage(schemaBytes)

		mt := &fidelityMockTool{
			description: description,
			inputSchema: inputSchema,
		}

		entry := Entry{
			ID:   toolName,
			Name: toolName,
			Kind: KindTool,
			Tool: mt,
		}

		registry := NewRegistry()
		adapter := NewEinoToolAdapter(mt, entry, registry)
		einoDef := adapter.ToEinoTool()

		if einoDef.Name != toolName {
			t.Fatalf("Name not preserved: expected %q, got %q", toolName, einoDef.Name)
		}
		if einoDef.Desc != description {
			t.Fatalf("Description not preserved: expected %q, got %q", description, einoDef.Desc)
		}
		if einoDef.ParamsOneOf == nil {
			t.Fatal("ParamsOneOf should not be nil for complex schema")
		}
	})
}


// Feature: aiops-codex-eino-rewrite, Property 7: MCP 动态注册/注销一致性

// **Validates: Requirements 2.5**

// --- Property 7 Generators & Helpers ---

// genServerID generates a random MCP server identifier.
func genServerID() *rapid.Generator[string] {
	return rapid.StringMatching(`[a-z][a-z0-9\-]{2,15}`)
}

// genMCPToolEntry generates a random MCP tool entry.
func genMCPToolEntry() *rapid.Generator[Entry] {
	return rapid.Custom[Entry](func(t *rapid.T) Entry {
		return Entry{
			ID:   genID().Draw(t, "toolID"),
			Name: genName().Draw(t, "toolName"),
			Kind: KindMCPTool,
		}
	})
}

// genMCPToolEntries generates a slice of 1-5 MCP tool entries with unique IDs.
func genMCPToolEntries() *rapid.Generator[[]Entry] {
	return rapid.Custom[[]Entry](func(t *rapid.T) []Entry {
		count := rapid.IntRange(1, 5).Draw(t, "toolCount")
		seen := make(map[string]bool)
		var entries []Entry
		for len(entries) < count {
			e := genMCPToolEntry().Draw(t, fmt.Sprintf("tool_%d", len(entries)))
			if !seen[e.ID] {
				seen[e.ID] = true
				entries = append(entries, e)
			}
		}
		return entries
	})
}

// opKind represents an MCP operation type for property testing.
type opKind int

const (
	opConnect    opKind = 0
	opDisconnect opKind = 1
)

// mcpOp represents a single MCP operation in a random sequence.
type mcpOp struct {
	Kind     opKind
	ServerID string
	Tools    []Entry
}

// genMCPOpSequence generates a random sequence of connect/disconnect operations.
func genMCPOpSequence() *rapid.Generator[[]mcpOp] {
	return rapid.Custom[[]mcpOp](func(t *rapid.T) []mcpOp {
		numServers := rapid.IntRange(2, 4).Draw(t, "numServers")
		serverIDs := make([]string, numServers)
		for i := 0; i < numServers; i++ {
			serverIDs[i] = genServerID().Draw(t, fmt.Sprintf("serverID_%d", i))
		}

		numOps := rapid.IntRange(3, 15).Draw(t, "numOps")
		ops := make([]mcpOp, numOps)
		for i := 0; i < numOps; i++ {
			serverID := rapid.SampledFrom(serverIDs).Draw(t, fmt.Sprintf("opServer_%d", i))
			kind := rapid.SampledFrom([]opKind{opConnect, opDisconnect}).Draw(t, fmt.Sprintf("opKind_%d", i))
			op := mcpOp{
				Kind:     kind,
				ServerID: serverID,
			}
			if kind == opConnect {
				op.Tools = genMCPToolEntries().Draw(t, fmt.Sprintf("opTools_%d", i))
			}
			ops[i] = op
		}
		return ops
	})
}

// TestProperty7_MCPDynamicRegisterUnregisterConsistency verifies consistency.
func TestProperty7_MCPDynamicRegisterUnregisterConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		registry := NewRegistry()
		mgr := NewMCPServerManager(registry)

		ops := genMCPOpSequence().Draw(t, "ops")

		expectedConnected := make(map[string][]string)

		for _, op := range ops {
			switch op.Kind {
			case opConnect:
				err := mgr.OnServerConnected(op.ServerID, op.Tools)
				if err != nil {
					t.Fatalf("OnServerConnected(%q) failed: %v", op.ServerID, err)
				}
				var expectedIDs []string
				for _, tool := range op.Tools {
					expectedIDs = append(expectedIDs, op.ServerID+"/"+tool.ID)
				}
				expectedConnected[op.ServerID] = expectedIDs

			case opDisconnect:
				mgr.OnServerDisconnected(op.ServerID)
				delete(expectedConnected, op.ServerID)
			}

			// Invariant: connected server tools are visible
			for serverID, entryIDs := range expectedConnected {
				tools := mgr.ListServerTools(serverID)
				if len(tools) != len(entryIDs) {
					t.Fatalf("After ops, server %q should have %d tools visible, got %d",
						serverID, len(entryIDs), len(tools))
				}
				for _, id := range entryIDs {
					_, found := registry.Get(id)
					if !found {
						t.Fatalf("Connected server %q tool %q should be visible", serverID, id)
					}
				}
			}

			// Invariant: disconnected server tools are invisible
			seenServers := make(map[string]bool)
			for _, o := range ops {
				seenServers[o.ServerID] = true
			}
			for serverID := range seenServers {
				if _, connected := expectedConnected[serverID]; !connected {
					tools := mgr.ListServerTools(serverID)
					if tools != nil {
						t.Fatalf("Disconnected server %q should return nil, got %d tools",
							serverID, len(tools))
					}
				}
			}
		}
	})
}

// TestProperty7_ReconnectReplacesOldTools verifies reconnect replaces old tools.
func TestProperty7_ReconnectReplacesOldTools(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		registry := NewRegistry()
		mgr := NewMCPServerManager(registry)

		serverID := genServerID().Draw(t, "serverID")
		firstTools := genMCPToolEntries().Draw(t, "firstTools")
		secondTools := genMCPToolEntries().Draw(t, "secondTools")

		_ = mgr.OnServerConnected(serverID, firstTools)
		_ = mgr.OnServerConnected(serverID, secondTools)

		// Second tools are visible
		for _, tool := range secondTools {
			prefixedID := serverID + "/" + tool.ID
			_, found := registry.Get(prefixedID)
			if !found {
				t.Fatalf("After reconnect, new tool %q should be visible", prefixedID)
			}
		}

		// First tools are gone (unless same ID in second set)
		for _, tool := range firstTools {
			prefixedID := serverID + "/" + tool.ID
			isInSecond := false
			for _, st := range secondTools {
				if serverID+"/"+st.ID == prefixedID {
					isInSecond = true
					break
				}
			}
			if !isInSecond {
				_, found := registry.Get(prefixedID)
				if found {
					t.Fatalf("After reconnect, old tool %q should NOT be visible", prefixedID)
				}
			}
		}
	})
}

// TestProperty7_DisconnectNonExistentServerIsNoOp verifies no corruption.
func TestProperty7_DisconnectNonExistentServerIsNoOp(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		registry := NewRegistry()
		mgr := NewMCPServerManager(registry)

		connectedServer := genServerID().Draw(t, "connectedServer")
		tools := genMCPToolEntries().Draw(t, "tools")
		_ = mgr.OnServerConnected(connectedServer, tools)

		nonExistentServer := genServerID().Draw(t, "nonExistentServer")
		if nonExistentServer == connectedServer {
			nonExistentServer = nonExistentServer + "_other"
		}

		mgr.OnServerDisconnected(nonExistentServer)

		listedTools := mgr.ListServerTools(connectedServer)
		if len(listedTools) != len(tools) {
			t.Fatalf("Connected server should still have %d tools, got %d",
				len(tools), len(listedTools))
		}
	})
}

// Feature: aiops-codex-eino-rewrite, Property 8: AssembleToolPool 合并优先级

// **Validates: Requirements 2.7**

// poolMockTool is a configurable ToolRuntime mock for Property 8 tests.
type poolMockTool struct {
	description string
	schemaData  json.RawMessage
}

func (m *poolMockTool) Description() string                                          { return m.description }
func (m *poolMockTool) CheckPermissions(_ context.Context) error                     { return nil }
func (m *poolMockTool) IsReadOnly() bool                                             { return false }
func (m *poolMockTool) IsDestructive() bool                                          { return false }
func (m *poolMockTool) IsConcurrencySafe() bool                                      { return true }
func (m *poolMockTool) Display() ToolDisplayPayload                                  { return ToolDisplayPayload{Type: "text"} }
func (m *poolMockTool) InputSchema() json.RawMessage                                 { return m.schemaData }
func (m *poolMockTool) Execute(_ context.Context, _ json.RawMessage) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}

// genUniqueNames generates a slice of unique tool names of the given count.
func genUniqueNames(count int) *rapid.Generator[[]string] {
	return rapid.Custom[[]string](func(t *rapid.T) []string {
		seen := make(map[string]bool)
		var names []string
		for len(names) < count {
			name := genToolName().Draw(t, fmt.Sprintf("name_%d", len(names)))
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
		return names
	})
}

// TestProperty8_AssembleToolPoolBuiltInPriority verifies built-in tool priority.
func TestProperty8_AssembleToolPoolBuiltInPriority(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		registry := NewRegistry()

		numBuiltInOnly := rapid.IntRange(1, 5).Draw(t, "numBuiltInOnly")
		numMCPOnly := rapid.IntRange(1, 5).Draw(t, "numMCPOnly")
		numConflicting := rapid.IntRange(1, 4).Draw(t, "numConflicting")

		totalUniqueNames := numBuiltInOnly + numMCPOnly + numConflicting
		allNames := genUniqueNames(totalUniqueNames).Draw(t, "allNames")

		builtInOnlyNames := allNames[:numBuiltInOnly]
		mcpOnlyNames := allNames[numBuiltInOnly : numBuiltInOnly+numMCPOnly]
		conflictingNames := allNames[numBuiltInOnly+numMCPOnly:]

		expectedDesc := make(map[string]string)

		// Register built-in only tools
		for i, name := range builtInOnlyNames {
			desc := fmt.Sprintf("builtin_only_%d_%s", i, name)
			entry := Entry{
				ID:   fmt.Sprintf("builtin_%s", name),
				Name: name,
				Kind: KindTool,
				Tool: &poolMockTool{description: desc, schemaData: json.RawMessage(`{"type":"object"}`)},
			}
			_ = registry.Register(entry)
			expectedDesc[name] = desc
		}

		// Register MCP only tools
		for i, name := range mcpOnlyNames {
			desc := fmt.Sprintf("mcp_only_%d_%s", i, name)
			entry := Entry{
				ID:   fmt.Sprintf("mcp_%s", name),
				Name: name,
				Kind: KindMCPTool,
				Tool: &poolMockTool{description: desc, schemaData: json.RawMessage(`{"type":"object"}`)},
			}
			_ = registry.Register(entry)
			expectedDesc[name] = desc
		}

		// Register conflicting tools (both built-in AND MCP)
		for i, name := range conflictingNames {
			builtInDesc := fmt.Sprintf("builtin_conflict_%d_%s", i, name)
			mcpDesc := fmt.Sprintf("mcp_conflict_%d_%s", i, name)

			builtInEntry := Entry{
				ID:   fmt.Sprintf("builtin_%s", name),
				Name: name,
				Kind: KindTool,
				Tool: &poolMockTool{description: builtInDesc, schemaData: json.RawMessage(`{"type":"object"}`)},
			}
			_ = registry.Register(builtInEntry)

			mcpEntry := Entry{
				ID:   fmt.Sprintf("mcp_%s", name),
				Name: name,
				Kind: KindMCPTool,
				Tool: &poolMockTool{description: mcpDesc, schemaData: json.RawMessage(`{"type":"object"}`)},
			}
			_ = registry.Register(mcpEntry)

			// Expected: built-in wins on conflict
			expectedDesc[name] = builtInDesc
		}

		pool := registry.AssembleToolPool("host", "execute")

		// Verify total pool size equals unique names
		if len(pool) != totalUniqueNames {
			t.Fatalf("pool size should be %d, got %d", totalUniqueNames, len(pool))
		}

		// Build map for verification — extract ToolInfo from each BaseTool
		poolByName := make(map[string]*schema.ToolInfo)
		for _, bt := range pool {
			info, err := bt.Info(context.Background())
			if err != nil {
				t.Fatalf("Info() error: %v", err)
			}
			if _, dup := poolByName[info.Name]; dup {
				t.Fatalf("duplicate name %q in pool", info.Name)
			}
			poolByName[info.Name] = info
		}

		// Verify conflicting names use built-in description
		for _, name := range conflictingNames {
			def, found := poolByName[name]
			if !found {
				t.Fatalf("conflicting tool %q should be in pool", name)
			}
			if def.Desc != expectedDesc[name] {
				t.Fatalf("conflicting tool %q should use built-in desc %q, got %q",
					name, expectedDesc[name], def.Desc)
			}
		}

		// Verify non-conflicting MCP tools are included
		for _, name := range mcpOnlyNames {
			def, found := poolByName[name]
			if !found {
				t.Fatalf("MCP-only tool %q should be in pool", name)
			}
			if def.Desc != expectedDesc[name] {
				t.Fatalf("MCP-only tool %q desc mismatch: expected %q, got %q",
					name, expectedDesc[name], def.Desc)
			}
		}

		// Verify built-in only tools are included
		for _, name := range builtInOnlyNames {
			def, found := poolByName[name]
			if !found {
				t.Fatalf("built-in only tool %q should be in pool", name)
			}
			if def.Desc != expectedDesc[name] {
				t.Fatalf("built-in only tool %q desc mismatch: expected %q, got %q",
					name, expectedDesc[name], def.Desc)
			}
		}
	})
}
