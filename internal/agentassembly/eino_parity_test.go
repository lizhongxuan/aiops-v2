package agentassembly

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestEinoToolPoolMatchesToolSurfaceSnapshotVisibleTools(t *testing.T) {
	tools := []tooling.Tool{
		&tooling.StaticTool{
			Meta:            tooling.ToolMetadata{Name: "host.exec", Description: "execute"},
			InputSchemaData: json.RawMessage(`{"type":"object"}`),
		},
		&tooling.StaticTool{
			Meta:            tooling.ToolMetadata{Name: "host.read", Description: "read"},
			InputSchemaData: json.RawMessage(`{"type":"object"}`),
		},
	}
	metas := make([]tooling.ToolMetadata, 0, len(tools))
	for _, tool := range tools {
		metas = append(metas, tool.Metadata())
	}
	snapshot := BuildToolSurfaceSnapshot(ToolSurfaceInput{
		ModelVisibleTools: metas,
		DispatchableTools: metas,
	})
	pool := tooling.AssembleEinoToolPool(tools)
	if len(pool) != len(snapshot.ModelVisibleTools) {
		t.Fatalf("eino pool len = %d, snapshot visible = %d", len(pool), len(snapshot.ModelVisibleTools))
	}
	seen := map[string]bool{}
	for _, item := range snapshot.ModelVisibleTools {
		seen[tooling.ProviderSafeToolName(item.Name)] = true
	}
	for _, tool := range pool {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}
		if !seen[info.Name] {
			t.Fatalf("eino tool %q not present in snapshot visible tools %#v", info.Name, snapshot.ModelVisibleTools)
		}
	}
}
