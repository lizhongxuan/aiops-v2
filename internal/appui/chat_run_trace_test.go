package appui

import (
	"testing"

	"aiops-v2/internal/runtimekernel"
)

func TestCorootRCARequiresExplicitMentionAndHealthyMCP(t *testing.T) {
	healthy := runtimekernel.TurnRequest{
		Input:    "@Coroot 分析 checkout 服务异常",
		Metadata: map[string]string{"mcpHealth.coroot": "healthy"},
	}
	ensureCorootRCAMetadata(&healthy)
	if healthy.Metadata[metadataCorootExplicitRCA] != "true" || healthy.Metadata[metadataCorootRCADisplayAllowed] != "true" {
		t.Fatalf("healthy metadata = %#v, want explicit/display RCA", healthy.Metadata)
	}
	if healthy.Metadata["aiops.coroot.mcpHealthStatus"] != "healthy" {
		t.Fatalf("healthy metadata = %#v, want health status", healthy.Metadata)
	}
	if healthy.Metadata["aiops.coroot.skipReason"] != "" {
		t.Fatalf("healthy metadata = %#v, want no skip reason", healthy.Metadata)
	}

	unhealthy := runtimekernel.TurnRequest{
		Input:    "@Coroot 分析 checkout 服务异常",
		Metadata: map[string]string{"mcpHealth.coroot": "unavailable"},
	}
	ensureCorootRCAMetadata(&unhealthy)
	if unhealthy.Metadata[metadataCorootExplicitRCA] != "true" {
		t.Fatalf("unhealthy metadata = %#v, want explicit mention recorded", unhealthy.Metadata)
	}
	if unhealthy.Metadata[metadataCorootRCADisplayAllowed] == "true" {
		t.Fatalf("unhealthy metadata = %#v, must not display RCA", unhealthy.Metadata)
	}
	if unhealthy.Metadata["aiops.coroot.mcpHealthStatus"] != "unavailable" {
		t.Fatalf("unhealthy metadata = %#v, want unavailable health status", unhealthy.Metadata)
	}
	if unhealthy.Metadata["aiops.coroot.skipReason"] != "mcp_unavailable" {
		t.Fatalf("unhealthy metadata = %#v, want mcp_unavailable skip reason", unhealthy.Metadata)
	}

	withoutMention := runtimekernel.TurnRequest{
		Input:    "请结合 Coroot 指标证据排查 checkout 服务异常",
		Metadata: map[string]string{"mcpHealth.coroot": "healthy"},
	}
	ensureCorootRCAMetadata(&withoutMention)
	if withoutMention.Metadata[metadataCorootExplicitRCA] != "" || withoutMention.Metadata[metadataCorootRCADisplayAllowed] != "" {
		t.Fatalf("withoutMention metadata = %#v, want no RCA markers", withoutMention.Metadata)
	}
}
