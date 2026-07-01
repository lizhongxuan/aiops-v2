package tooling

import "testing"

func TestNormalizeToolSearchRequestTrimsAndClonesMCPHealth(t *testing.T) {
	req := NormalizeToolSearchRequest(ToolSearchRequest{
		Mode:        " search ",
		Query:       " service metrics ",
		Intent:      " RCA ",
		SessionType: " host ",
		RuntimeMode: " chat ",
		Limit:       5,
		MCPHealth: map[string]string{
			" coroot ": " Unavailable ",
			"":         "healthy",
		},
		Ranker: " bm25 ",
	})

	if req.Mode != "search" || req.Query != "service metrics" || req.Intent != "RCA" || req.Ranker != "bm25" {
		t.Fatalf("normalized request = %#v", req)
	}
	if got := req.MCPHealth["coroot"]; got != "unavailable" {
		t.Fatalf("MCPHealth[coroot] = %q, want unavailable", got)
	}
	req.MCPHealth["coroot"] = "mutated"
	if got := NormalizeToolSearchRequest(ToolSearchRequest{MCPHealth: map[string]string{"coroot": "Unavailable"}}).MCPHealth["coroot"]; got != "unavailable" {
		t.Fatalf("NormalizeToolSearchRequest should clone health map, got %q", got)
	}
}

func TestToolSearchV3CandidateIncludesLoadableSpecAndSelectablePack(t *testing.T) {
	candidate := ToolCandidateFromMetadata(ToolMetadata{
		Name:        "coroot.postgres.rca",
		Description: "Read Coroot PostgreSQL RCA evidence",
		Layer:       ToolLayerDeferred,
		Pack:        "coroot_postgres",
		RiskLevel:   ToolRiskLow,
		Discovery: ToolDiscoveryMetadata{
			CapabilityKind: "rca",
			ResourceTypes:  []string{"postgres", "service"},
			OperationKinds: []string{"read"},
			RequiresSelect: true,
		},
	})

	if candidate.Name != "coroot.postgres.rca" || candidate.Capability != "rca" {
		t.Fatalf("candidate = %#v, want normalized name/capability", candidate)
	}
	if candidate.LoadableToolSpec == nil {
		t.Fatalf("LoadableToolSpec = nil, want loadable spec: %#v", candidate)
	}
	if candidate.LoadableToolSpec.Name != "coroot.postgres.rca" ||
		candidate.LoadableToolSpec.Pack != "coroot_postgres" ||
		!candidate.LoadableToolSpec.RequiresSelect {
		t.Fatalf("LoadableToolSpec = %#v, want coroot postgres selectable tool", candidate.LoadableToolSpec)
	}
	if candidate.SelectablePack == nil {
		t.Fatalf("SelectablePack = nil, want selectable pack: %#v", candidate)
	}
	if candidate.SelectablePack.Pack != "coroot_postgres" ||
		len(candidate.SelectablePack.Tools) != 1 ||
		candidate.SelectablePack.Tools[0] != "coroot.postgres.rca" {
		t.Fatalf("SelectablePack = %#v, want coroot_postgres pack with tool", candidate.SelectablePack)
	}
}
