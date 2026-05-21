package coroot

import (
	"fmt"
	"strings"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/plugins"
)

func BuiltinPluginSpec(provider ClientProvider, displayEndpoint string) (plugins.Spec, error) {
	if provider == nil {
		return plugins.Spec{}, fmt.Errorf("coroot: client provider is required")
	}
	command := strings.TrimSpace(displayEndpoint)
	var argv []string
	if command != "" {
		argv = []string{command}
	}
	cfg := mcp.ServerConfig{
		ID:        "coroot",
		Name:      "coroot",
		Transport: "builtin",
		Command:   argv,
		Source:    "builtin",
	}
	return plugins.Spec{
		Name: "coroot",
		Manifest: plugins.Manifest{
			Name:       "coroot",
			MCPServers: []mcp.ServerConfig{cfg},
			AIOps: plugins.AIOpsManifest{
				AgentUIRenderers: []plugins.AgentUIRendererManifest{{
					ID:            "coroot.chart.v1",
					ArtifactTypes: []string{"coroot_chart", "observability.chart"},
					SchemaVersion: "coroot.chart.v1",
					Component:     "CorootChartArtifact",
					Fallback:      "json_summary",
					Display: plugins.AgentUIRendererDisplayManifest{
						TitleField: "title",
						Icon:       "activity",
						HideFooter: true,
					},
				}},
			},
		},
		MCPServers: []plugins.MCPServerSpec{{
			Config: cfg,
			Tools:  corootToolsWithClientProvider(provider),
		}},
	}, nil
}
