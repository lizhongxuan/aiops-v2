package appui

import (
	"sort"
	"strings"
	"testing"

	"aiops-v2/internal/plugins"
	"aiops-v2/internal/store"
)

func TestUICardServiceListsBuiltInsWithStats(t *testing.T) {
	service := NewUICardService(nil)

	result, err := service.List(UICardListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if result.Total == 0 || len(result.Items) == 0 || result.Stats["builtIn"] == 0 {
		t.Fatalf("List() = %#v, want built-in items and stats", result)
	}
	if result.Items[0].RendererVersion == "" || result.Items[0].SchemaVersion == "" || result.Items[0].PayloadSchema == nil {
		t.Fatalf("built-in card missing nested policy fields: %#v", result.Items[0])
	}
}

func TestUICardServiceBuiltInTypesMatchFrontendRegistryContract(t *testing.T) {
	service := NewUICardService(nil)
	result, err := service.List(UICardListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	var got []string
	for _, item := range result.Items {
		if item.BuiltIn {
			got = append(got, item.Kind)
		}
	}
	want := []string{
		"coroot_chart",
		"trace_summary",
		"topology_slice",
		"rca_report",
		"workflow_result",
		"verification_result",
		"experience_match",
		"ops_manual_match",
		"ops_manual_search_result",
		"ops_manual_preflight_result",
		"ops_manual_fallback_guide",
		"runner_workflow_generation",
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("built-in kinds = %#v, want %#v", got, want)
	}
}

func TestUICardServiceListsPluginRendererMetadata(t *testing.T) {
	service := NewUICardService(nil, WithUICardPluginSpecs([]plugins.Spec{{
		Name: "observability-plugin",
		Manifest: plugins.Manifest{
			Name: "observability-plugin",
			AIOps: plugins.AIOpsManifest{
				AgentUIRenderers: []plugins.AgentUIRendererManifest{{
					ID:            "observability.chart.v1",
					ArtifactTypes: []string{"observability.chart"},
					SchemaVersion: "observability.chart.v1",
					Component:     "CorootChartArtifact",
					Fallback:      "json_summary",
					Display: plugins.AgentUIRendererDisplayManifest{
						Icon:       "line-chart",
						HideFooter: true,
					},
				}},
			},
		},
	}}))

	renderers, err := service.ListRenderers()
	if err != nil {
		t.Fatalf("ListRenderers() error = %v", err)
	}
	if renderers.Total != 1 || renderers.Items[0].ID != "observability.chart.v1" {
		t.Fatalf("ListRenderers() = %#v, want plugin renderer metadata", renderers)
	}
	if renderers.Items[0].Display["hide_footer"] != true {
		t.Fatalf("renderer display = %#v, want hide_footer true", renderers.Items[0].Display)
	}

	cards, err := service.List(UICardListRequest{Kind: "observability.chart"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if cards.Total != 1 || cards.Items[0].Renderer != "observability.chart.v1" {
		t.Fatalf("plugin renderer card = %#v, want renderer-backed UI card", cards)
	}
}

func TestUICardServiceCreateUpdateValidatePreviewAndDelete(t *testing.T) {
	repo := &uiCardMemoryRepo{}
	service := NewUICardService(repo)
	card := store.UICard{
		ID:              "custom-timeline",
		Name:            "Timeline",
		Kind:            "timeline",
		Renderer:        "agent-ui/timeline",
		RendererVersion: "0.1.0",
		SchemaVersion:   "2026-05-16",
		PayloadSchema:   map[string]any{"type": "object"},
		ActionPolicy:    map[string]any{"allowed": []any{"inspect"}},
		DisplayPolicy:   map[string]any{"density": "comfortable"},
		RedactionPolicy: map[string]any{"mode": "default"},
		SamplePayloads:  []map[string]any{{"title": "sample"}},
		Status:          "draft",
	}

	created, err := service.Create(card)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Version != 1 || created.Status != "draft" {
		t.Fatalf("created = %#v, want version 1 draft", created)
	}
	updated, err := service.Update("custom-timeline", store.UICard{Name: "Timeline v2", Status: "active"})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Version != 2 || updated.Name != "Timeline v2" || updated.Renderer != "agent-ui/timeline" {
		t.Fatalf("updated = %#v, want merged version 2", updated)
	}
	status, err := service.Status("custom-timeline")
	if err != nil || status.Status != "active" {
		t.Fatalf("Status() = %#v, %v", status, err)
	}
	versions, err := service.Versions("custom-timeline")
	if err != nil || len(versions) != 1 || versions[0].Version != 2 {
		t.Fatalf("Versions() = %#v, %v", versions, err)
	}
	validation, err := service.Validate(UICardValidationRequest{CardID: "custom-timeline", Payload: map[string]any{"title": "ok"}})
	if err != nil || !validation.Valid {
		t.Fatalf("Validate() = %#v, %v", validation, err)
	}
	preview, err := service.Preview(UICardPreviewRequest{CardID: "custom-timeline", Payload: map[string]any{"title": "ok"}})
	if err != nil || preview.Card.ID != "custom-timeline" || preview.Payload["title"] != "ok" {
		t.Fatalf("Preview() = %#v, %v", preview, err)
	}
	if err := service.Delete("custom-timeline"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := service.Get("custom-timeline"); err == nil {
		t.Fatal("Get() after Delete() error = nil, want not found")
	}
}

func TestUICardServiceRejectsDangerousKeysAndBuiltInDelete(t *testing.T) {
	service := NewUICardService(nil)
	result, err := service.List(UICardListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	builtInID := result.Items[0].ID
	if err := service.Delete(builtInID); err == nil || !strings.Contains(err.Error(), "built-in") {
		t.Fatalf("Delete(%q) error = %v, want built-in rejection", builtInID, err)
	}
	validation, err := service.Validate(UICardValidationRequest{
		CardID:  builtInID,
		Payload: map[string]any{"safe": map[string]any{"dangerouslySetInnerHTML": "<script>alert(1)</script>"}},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if validation.Valid || len(validation.Errors) == 0 {
		t.Fatalf("Validate() = %#v, want dangerous key error", validation)
	}
}

type uiCardMemoryRepo struct {
	items []store.UICard
}

func (r *uiCardMemoryRepo) GetUICards() ([]store.UICard, error) {
	return append([]store.UICard(nil), r.items...), nil
}

func (r *uiCardMemoryRepo) SaveUICards(items []store.UICard) error {
	r.items = append([]store.UICard(nil), items...)
	return nil
}
