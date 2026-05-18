package appui

import "testing"

func TestAgentUIArtifactServiceListFiltersAndCursor(t *testing.T) {
	service := NewAgentUIArtifactService([]AiopsTransportAgentUIArtifact{
		{ID: "a1", Type: "ops_manual_search_result", Source: "tool:search", Metadata: map[string]any{"caseId": "case-1"}, CreatedAt: "2026-05-16T10:00:00Z"},
		{ID: "a2", Type: "ops_manual_preflight_result", Source: "tool:preflight", Metadata: map[string]any{"caseId": "case-1"}, CreatedAt: "2026-05-16T10:01:00Z"},
		{ID: "a3", Type: "evidence", Source: "coroot", Metadata: map[string]any{"caseId": "case-2"}, CreatedAt: "2026-05-16T10:02:00Z"},
	})

	first, err := service.List(AgentUIArtifactListRequest{CaseID: "case-1", Limit: 1})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(first.Items) != 1 || first.Total != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %#v, want one of two and cursor", first)
	}
	second, err := service.List(AgentUIArtifactListRequest{CaseID: "case-1", Cursor: first.NextCursor, Limit: 1})
	if err != nil {
		t.Fatalf("List() second page error = %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].ID == first.Items[0].ID || second.NextCursor != "" {
		t.Fatalf("second page = %#v, want remaining item", second)
	}

	filtered, err := service.List(AgentUIArtifactListRequest{Source: "coroot", Type: "evidence"})
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].ID != "a3" {
		t.Fatalf("filtered = %#v, %v", filtered, err)
	}
}

func TestAgentUIArtifactServiceGetAndValidate(t *testing.T) {
	service := NewAgentUIArtifactService([]AiopsTransportAgentUIArtifact{
		{ID: "artifact-ok", Type: "evidence", Source: "coroot", Payload: map[string]any{"title": "cpu"}, CreatedAt: "2026-05-16T10:00:00Z"},
	})

	artifact, err := service.Get("artifact-ok")
	if err != nil || artifact.Type != "evidence" {
		t.Fatalf("Get() = %#v, %v", artifact, err)
	}
	validation, err := service.Validate(AgentUIArtifactValidationRequest{ArtifactID: "artifact-ok"})
	if err != nil || !validation.Valid {
		t.Fatalf("Validate(existing) = %#v, %v", validation, err)
	}
	validation, err = service.Validate(AgentUIArtifactValidationRequest{Artifact: AiopsTransportAgentUIArtifact{ID: "bad", Source: "coroot"}})
	if err != nil {
		t.Fatalf("Validate(inline) error = %v", err)
	}
	if validation.Valid || len(validation.Errors) == 0 {
		t.Fatalf("Validate(inline) = %#v, want errors", validation)
	}
}
