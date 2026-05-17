package appui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"aiops-v2/internal/store"
)

type UICardRepository interface {
	GetUICards() ([]store.UICard, error)
	SaveUICards(cards []store.UICard) error
}

type UICardService interface {
	List(UICardListRequest) (UICardListResult, error)
	Get(id string) (store.UICard, error)
	Create(card store.UICard) (store.UICard, error)
	Update(id string, patch store.UICard) (store.UICard, error)
	Delete(id string) error
	Status(id string) (UICardStatus, error)
	UpdateStatus(id string, status string) (store.UICard, error)
	Versions(id string) ([]UICardVersion, error)
	CreateVersion(id string) (store.UICard, error)
	Validate(req UICardValidationRequest) (UICardValidationResult, error)
	Preview(req UICardPreviewRequest) (UICardPreviewResult, error)
}

type UICardListRequest struct {
	Status string
	Kind   string
}

type UICardListResult struct {
	Items []store.UICard `json:"items"`
	Cards []store.UICard `json:"cards,omitempty"`
	Stats map[string]int `json:"stats"`
	Total int            `json:"total"`
}

type UICardStatus struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	BuiltIn   bool   `json:"builtIn"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type UICardVersion struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Status    string `json:"status,omitempty"`
}

type UICardValidationRequest struct {
	CardID  string         `json:"cardId,omitempty"`
	Card    store.UICard   `json:"card,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

type UICardValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type UICardPreviewRequest struct {
	CardID  string         `json:"cardId,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

type UICardPreviewResult struct {
	Card       store.UICard           `json:"card"`
	Payload    map[string]any         `json:"payload"`
	Validation UICardValidationResult `json:"validation"`
}

type defaultUICardService struct {
	repo UICardRepository
}

func NewUICardService(repo UICardRepository) UICardService {
	return &defaultUICardService{repo: repo}
}

func (s *defaultUICardService) List(req UICardListRequest) (UICardListResult, error) {
	cards, err := s.allCards()
	if err != nil {
		return UICardListResult{}, err
	}
	filtered := make([]store.UICard, 0, len(cards))
	stats := map[string]int{"builtIn": 0, "custom": 0, "active": 0, "draft": 0, "deprecated": 0}
	for _, card := range cards {
		status := strings.TrimSpace(card.Status)
		if status == "" {
			status = "active"
		}
		if card.BuiltIn {
			stats["builtIn"]++
		} else {
			stats["custom"]++
		}
		stats[status]++
		if req.Status != "" && status != req.Status {
			continue
		}
		if req.Kind != "" && card.Kind != req.Kind {
			continue
		}
		filtered = append(filtered, card)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].BuiltIn != filtered[j].BuiltIn {
			return filtered[i].BuiltIn
		}
		return filtered[i].ID < filtered[j].ID
	})
	return UICardListResult{Items: filtered, Cards: filtered, Stats: stats, Total: len(filtered)}, nil
}

func (s *defaultUICardService) Get(id string) (store.UICard, error) {
	id = strings.TrimSpace(id)
	for _, card := range mustCards(s.allCards()) {
		if card.ID == id {
			return card, nil
		}
	}
	return store.UICard{}, fmt.Errorf("ui card %q not found", id)
}

func (s *defaultUICardService) Create(card store.UICard) (store.UICard, error) {
	card.ID = strings.TrimSpace(card.ID)
	if card.ID == "" {
		return store.UICard{}, fmt.Errorf("id is required")
	}
	existing, err := s.persistedCards()
	if err != nil {
		return store.UICard{}, err
	}
	for _, item := range append(defaultUICardDefinitions(), existing...) {
		if item.ID == card.ID {
			return store.UICard{}, fmt.Errorf("ui card %q already exists", card.ID)
		}
	}
	now := time.Now().UTC()
	card.BuiltIn = false
	card.Version = maxInt(card.Version, 1)
	if card.Status == "" {
		card.Status = "draft"
	}
	if card.CreatedAt.IsZero() {
		card.CreatedAt = now
	}
	card.UpdatedAt = now
	card = normalizeUICard(card)
	existing = append(existing, card)
	if err := s.savePersisted(existing); err != nil {
		return store.UICard{}, err
	}
	return card, nil
}

func (s *defaultUICardService) Update(id string, patch store.UICard) (store.UICard, error) {
	id = strings.TrimSpace(id)
	existing, err := s.persistedCards()
	if err != nil {
		return store.UICard{}, err
	}
	for idx, card := range existing {
		if card.ID != id {
			continue
		}
		merged := mergeUICard(card, patch)
		merged.ID = id
		merged.BuiltIn = false
		merged.Version = maxInt(card.Version+1, 1)
		merged.UpdatedAt = time.Now().UTC()
		merged = normalizeUICard(merged)
		existing[idx] = merged
		if err := s.savePersisted(existing); err != nil {
			return store.UICard{}, err
		}
		return merged, nil
	}
	for _, card := range defaultUICardDefinitions() {
		if card.ID == id {
			return store.UICard{}, fmt.Errorf("built-in ui card %q cannot be updated", id)
		}
	}
	return store.UICard{}, fmt.Errorf("ui card %q not found", id)
}

func (s *defaultUICardService) Delete(id string) error {
	id = strings.TrimSpace(id)
	for _, card := range defaultUICardDefinitions() {
		if card.ID == id {
			return fmt.Errorf("built-in ui card %q cannot be deleted", id)
		}
	}
	existing, err := s.persistedCards()
	if err != nil {
		return err
	}
	for idx, card := range existing {
		if card.ID == id {
			existing = append(existing[:idx], existing[idx+1:]...)
			return s.savePersisted(existing)
		}
	}
	return fmt.Errorf("ui card %q not found", id)
}

func (s *defaultUICardService) Status(id string) (UICardStatus, error) {
	card, err := s.Get(id)
	if err != nil {
		return UICardStatus{}, err
	}
	return UICardStatus{ID: card.ID, Status: card.Status, BuiltIn: card.BuiltIn, Version: card.Version, UpdatedAt: formatCardTime(card.UpdatedAt)}, nil
}

func (s *defaultUICardService) UpdateStatus(id string, status string) (store.UICard, error) {
	status = strings.TrimSpace(status)
	switch status {
	case "active", "draft", "deprecated", "disabled":
	default:
		return store.UICard{}, fmt.Errorf("unsupported ui card status %q", status)
	}
	return s.Update(id, store.UICard{Status: status})
}

func (s *defaultUICardService) Versions(id string) ([]UICardVersion, error) {
	card, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	return []UICardVersion{{ID: card.ID, Version: card.Version, UpdatedAt: formatCardTime(card.UpdatedAt), Status: card.Status}}, nil
}

func (s *defaultUICardService) CreateVersion(id string) (store.UICard, error) {
	card, err := s.Get(id)
	if err != nil {
		return store.UICard{}, err
	}
	card.Version++
	return s.Update(id, card)
}

func (s *defaultUICardService) Validate(req UICardValidationRequest) (UICardValidationResult, error) {
	var errors []string
	card := req.Card
	if strings.TrimSpace(req.CardID) != "" {
		found, err := s.Get(req.CardID)
		if err != nil {
			return UICardValidationResult{}, err
		}
		card = found
	}
	if strings.TrimSpace(card.ID) == "" {
		errors = append(errors, "id is required")
	}
	if strings.TrimSpace(card.Renderer) == "" {
		errors = append(errors, "renderer is required")
	}
	if strings.TrimSpace(card.SchemaVersion) == "" {
		errors = append(errors, "schemaVersion is required")
	}
	for _, key := range findDangerousKeys(req.Payload) {
		errors = append(errors, "dangerous key is not allowed: "+key)
	}
	return UICardValidationResult{Valid: len(errors) == 0, Errors: errors}, nil
}

func (s *defaultUICardService) Preview(req UICardPreviewRequest) (UICardPreviewResult, error) {
	card, err := s.Get(req.CardID)
	if err != nil {
		return UICardPreviewResult{}, err
	}
	validation, err := s.Validate(UICardValidationRequest{CardID: req.CardID, Payload: req.Payload})
	if err != nil {
		return UICardPreviewResult{}, err
	}
	return UICardPreviewResult{Card: card, Payload: cloneMap(req.Payload), Validation: validation}, nil
}

func (s *defaultUICardService) allCards() ([]store.UICard, error) {
	persisted, err := s.persistedCards()
	if err != nil {
		return nil, err
	}
	return append(defaultUICardDefinitions(), persisted...), nil
}

func (s *defaultUICardService) persistedCards() ([]store.UICard, error) {
	if s.repo == nil {
		return nil, nil
	}
	cards, err := s.repo.GetUICards()
	if err != nil {
		return nil, err
	}
	result := make([]store.UICard, 0, len(cards))
	for _, card := range cards {
		if card.BuiltIn {
			continue
		}
		result = append(result, normalizeUICard(card))
	}
	return result, nil
}

func (s *defaultUICardService) savePersisted(cards []store.UICard) error {
	if s.repo == nil {
		return fmt.Errorf("ui card repository is not configured")
	}
	return s.repo.SaveUICards(cards)
}

func defaultUICardDefinitions() []store.UICard {
	now := time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC)
	baseSchema := map[string]any{"type": "object", "additionalProperties": true}
	policy := map[string]any{"dangerousKeys": dangerousUICardKeys()}
	// Keep this built-in type set aligned with web/src/lib/agentUiCardDefinitions.ts and internal/appui/ui_card_service.go.
	specs := []struct {
		id      string
		kind    string
		name    string
		summary string
		actions []any
		sample  map[string]any
	}{
		{"coroot-chart", "coroot_chart", "Coroot Chart", "Shows Coroot metrics and chart evidence.", []any{"open_coroot", "open_evidence"}, map[string]any{"id": "sample-coroot-chart", "type": "coroot_chart", "payload": map[string]any{"chart": "p95"}}},
		{"trace-summary", "trace_summary", "Trace Summary", "Shows distributed trace summaries.", []any{"open_prompt_trace", "open_evidence"}, map[string]any{"id": "sample-trace-summary", "type": "trace_summary", "payload": map[string]any{"traceId": "trace-demo"}}},
		{"topology-slice", "topology_slice", "Topology Slice", "Shows service upstream and downstream dependencies.", []any{"open_coroot"}, map[string]any{"id": "sample-topology-slice", "type": "topology_slice", "payload": map[string]any{"service": "checkout"}}},
		{"rca-report", "rca_report", "RCA Report", "Shows Coroot MCP root-cause reports with evidence and limitations.", []any{"open_case", "open_evidence", "open_prompt_trace"}, map[string]any{"id": "sample-rca-report", "type": "rca_report", "payload": map[string]any{"status": "ok", "summary": "checkout-api p95 latency root cause"}}},
		{"workflow-result", "workflow_result", "Workflow Result", "Shows Runner workflow execution result.", []any{"open_workflow_run"}, map[string]any{"id": "sample-workflow-result", "type": "workflow_result", "payload": map[string]any{"status": "success"}}},
		{"verification-result", "verification_result", "Verification Result", "Shows recovery or validation proof.", []any{"open_evidence"}, map[string]any{"id": "sample-verification-result", "type": "verification_result", "payload": map[string]any{"status": "passed"}}},
		{"experience-match", "experience_match", "Experience Match", "Shows matched legacy experience-pack evidence.", []any{"open_case"}, map[string]any{"id": "sample-experience-match", "type": "experience_match", "payload": map[string]any{"match": "redis-memory"}}},
		{"ops-manual-match", "ops_manual_match", "Ops Manual Match", "Shows selected ops manual and workflow readiness.", []any{"open_ops_manual", "start_dry_run"}, map[string]any{"id": "sample-ops-manual-match", "type": "ops_manual_match", "payload": map[string]any{"manualId": "manual-demo"}}},
		{"ops-manual-search-result", "ops_manual_search_result", "Ops Manual Search Result", "Shows ops manual retrieval decisions.", []any{"run_preflight_probe", "start_dry_run", "review_manual"}, map[string]any{"id": "sample-ops-manual-search-result", "type": "ops_manual_search_result", "payload": map[string]any{"decision": "need_info", "summary": "missing target instance"}}},
		{"ops-manual-preflight-result", "ops_manual_preflight_result", "Ops Manual Preflight Result", "Shows ops manual preflight readiness.", []any{"start_dry_run", "request_permission", "collect_context"}, map[string]any{"id": "sample-ops-manual-preflight-result", "type": "ops_manual_preflight_result", "payload": map[string]any{"status": "passed", "ready": true}}},
		{"ops-manual-fallback-guide", "ops_manual_fallback_guide", "Ops Manual Fallback Guide", "Shows manual step-by-step fallback instructions.", []any{"open_ops_manual"}, map[string]any{"id": "sample-ops-manual-fallback-guide", "type": "ops_manual_fallback_guide", "payload": map[string]any{"step": "read-only probe"}}},
		{"runner-workflow-generation", "runner_workflow_generation", "Runner Workflow Generation", "Shows Runner workflow draft generation progress.", []any{"open_workflow_run"}, map[string]any{"id": "sample-runner-workflow-generation", "type": "runner_workflow_generation", "payload": map[string]any{"status": "running"}}},
	}
	cards := make([]store.UICard, 0, len(specs))
	for _, spec := range specs {
		cards = append(cards, store.UICard{
			ID:                spec.id,
			Name:              spec.name,
			Kind:              spec.kind,
			Renderer:          "agent-ui/" + spec.id,
			RendererVersion:   "1.0.0",
			SchemaVersion:     "2026-05-16",
			PayloadSchema:     cloneMap(baseSchema),
			MetadataSchema:    cloneMap(baseSchema),
			ActionPolicy:      map[string]any{"allowed": spec.actions},
			DisplayPolicy:     map[string]any{"density": "compact", "placement": "assistant_turn"},
			RedactionPolicy:   cloneMap(policy),
			SamplePayloads:    []map[string]any{{"id": spec.id + "-sample", "name": "sample", "artifact": spec.sample}},
			BundleSupport:     []string{"web"},
			PlacementDefaults: []string{"assistant_turn"},
			Summary:           spec.summary,
			Status:            "active",
			BuiltIn:           true,
			Version:           1,
			CreatedAt:         now,
			UpdatedAt:         now,
		})
	}
	return cards
}

func normalizeUICard(card store.UICard) store.UICard {
	card.ID = strings.TrimSpace(card.ID)
	card.Name = strings.TrimSpace(card.Name)
	card.Kind = strings.TrimSpace(card.Kind)
	card.Renderer = strings.TrimSpace(card.Renderer)
	if card.Status == "" {
		card.Status = "active"
	}
	if card.RendererVersion == "" {
		card.RendererVersion = "1.0.0"
	}
	if card.SchemaVersion == "" {
		card.SchemaVersion = "2026-05-16"
	}
	return card
}

func mergeUICard(base, patch store.UICard) store.UICard {
	if patch.Name != "" {
		base.Name = patch.Name
	}
	if patch.Kind != "" {
		base.Kind = patch.Kind
	}
	if patch.Renderer != "" {
		base.Renderer = patch.Renderer
	}
	if patch.RendererVersion != "" {
		base.RendererVersion = patch.RendererVersion
	}
	if patch.SchemaVersion != "" {
		base.SchemaVersion = patch.SchemaVersion
	}
	if patch.PayloadSchema != nil {
		base.PayloadSchema = patch.PayloadSchema
	}
	if patch.MetadataSchema != nil {
		base.MetadataSchema = patch.MetadataSchema
	}
	if patch.ActionPolicy != nil {
		base.ActionPolicy = patch.ActionPolicy
	}
	if patch.DisplayPolicy != nil {
		base.DisplayPolicy = patch.DisplayPolicy
	}
	if patch.RedactionPolicy != nil {
		base.RedactionPolicy = patch.RedactionPolicy
	}
	if patch.SamplePayloads != nil {
		base.SamplePayloads = patch.SamplePayloads
	}
	if patch.BundleSupport != nil {
		base.BundleSupport = patch.BundleSupport
	}
	if patch.PlacementDefaults != nil {
		base.PlacementDefaults = patch.PlacementDefaults
	}
	if patch.Summary != "" {
		base.Summary = patch.Summary
	}
	if patch.Capabilities != nil {
		base.Capabilities = patch.Capabilities
	}
	if patch.TriggerTypes != nil {
		base.TriggerTypes = patch.TriggerTypes
	}
	if patch.EditableFields != nil {
		base.EditableFields = patch.EditableFields
	}
	if patch.Status != "" {
		base.Status = patch.Status
	}
	return base
}

func findDangerousKeys(value any) []string {
	dangerous := map[string]bool{}
	for _, key := range dangerousUICardKeys() {
		dangerous[strings.ToLower(key)] = true
	}
	var found []string
	var walk func(any)
	walk = func(v any) {
		switch typed := v.(type) {
		case map[string]any:
			for key, child := range typed {
				if dangerous[strings.ToLower(strings.TrimSpace(key))] {
					found = append(found, key)
				}
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case []map[string]any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	sort.Strings(found)
	return found
}

func dangerousUICardKeys() []string {
	return []string{"html", "script", "iframe", "innerHTML", "outerHTML", "dangerouslySetInnerHTML", "onClick", "onLoad", "styleText"}
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func mustCards(cards []store.UICard, err error) []store.UICard {
	if err != nil {
		return nil
	}
	return cards
}

func formatCardTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func parseCursor(raw string) int {
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
