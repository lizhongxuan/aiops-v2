package tooling

import (
	"reflect"
	"testing"
)

func TestMetadataTriggersEnableSyntheticPack(t *testing.T) {
	tools := []Tool{
		&StaticTool{Meta: ToolMetadata{
			Name:           "synthetic.metrics.read",
			Layer:          ToolLayerDeferred,
			Pack:           "synthetic_metrics",
			DeferByDefault: true,
			Triggers:       []string{"latency", "metric"},
			Discovery: ToolDiscoveryMetadata{
				DiscoveryTags:  []string{"timeseries"},
				CapabilityKind: "read",
				ResourceTypes:  []string{"metric"},
				OperationKinds: []string{"read"},
			},
		}},
		&StaticTool{Meta: ToolMetadata{
			Name:           "synthetic.logs.search",
			Layer:          ToolLayerDeferred,
			Pack:           "synthetic_logs",
			DeferByDefault: true,
			Triggers:       []string{"log"},
			Discovery: ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"log"},
				OperationKinds: []string{"search"},
			},
		}},
	}

	matches := MatchToolPacksByMetadata(tools, "read metric latency timeseries")
	if len(matches) == 0 {
		t.Fatal("MatchToolPacksByMetadata returned no matches")
	}
	if matches[0].Pack != "synthetic_metrics" {
		t.Fatalf("top match = %#v, want synthetic_metrics", matches[0])
	}
	if !reflect.DeepEqual(matches[0].ToolNames, []string{"synthetic.metrics.read"}) {
		t.Fatalf("ToolNames = %#v", matches[0].ToolNames)
	}
}

func TestMetadataTriggersIgnoreHiddenAndCoreTools(t *testing.T) {
	tools := []Tool{
		&StaticTool{Meta: ToolMetadata{
			Name:  "synthetic.core",
			Layer: ToolLayerCore,
			Pack:  "synthetic_core",
			Discovery: ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"metric"},
				OperationKinds: []string{"read"},
			},
		}},
		&StaticTool{Meta: ToolMetadata{
			Name:           "synthetic.hidden",
			Layer:          ToolLayerDeferred,
			Pack:           "synthetic_hidden",
			DeferByDefault: true,
			Discovery: ToolDiscoveryMetadata{
				HiddenFromDiscovery: true,
				CapabilityKind:      "read",
				ResourceTypes:       []string{"metric"},
				OperationKinds:      []string{"read"},
			},
		}},
	}

	if matches := MatchToolPacksByMetadata(tools, "metric read"); len(matches) != 0 {
		t.Fatalf("matches = %#v, want none", matches)
	}
}
