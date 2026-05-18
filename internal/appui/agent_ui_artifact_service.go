package appui

import (
	"fmt"
	"sort"
	"strings"
)

type AgentUIArtifactService interface {
	List(AgentUIArtifactListRequest) (AgentUIArtifactListResult, error)
	Get(id string) (AiopsTransportAgentUIArtifact, error)
	Validate(AgentUIArtifactValidationRequest) (AgentUIArtifactValidationResult, error)
}

type AgentUIArtifactListRequest struct {
	Source string
	Type   string
	CaseID string
	Limit  int
	Cursor string
}

type AgentUIArtifactListResult struct {
	Items      []AiopsTransportAgentUIArtifact `json:"items"`
	Total      int                             `json:"total"`
	NextCursor string                          `json:"nextCursor,omitempty"`
}

type AgentUIArtifactValidationRequest struct {
	ArtifactID string                        `json:"artifactId,omitempty"`
	Artifact   AiopsTransportAgentUIArtifact `json:"artifact,omitempty"`
}

type AgentUIArtifactValidationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

type defaultAgentUIArtifactService struct {
	items []AiopsTransportAgentUIArtifact
}

func NewAgentUIArtifactService(items []AiopsTransportAgentUIArtifact) AgentUIArtifactService {
	if len(items) == 0 {
		items = defaultAgentUIArtifacts()
	}
	cp := make([]AiopsTransportAgentUIArtifact, len(items))
	copy(cp, items)
	return &defaultAgentUIArtifactService{items: cp}
}

func (s *defaultAgentUIArtifactService) List(req AgentUIArtifactListRequest) (AgentUIArtifactListResult, error) {
	filtered := make([]AiopsTransportAgentUIArtifact, 0, len(s.items))
	for _, item := range s.items {
		if req.Source != "" && item.Source != req.Source {
			continue
		}
		if req.Type != "" && item.Type != req.Type {
			continue
		}
		if req.CaseID != "" && artifactCaseID(item) != req.CaseID {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].CreatedAt != filtered[j].CreatedAt {
			return filtered[i].CreatedAt < filtered[j].CreatedAt
		}
		return filtered[i].ID < filtered[j].ID
	})
	total := len(filtered)
	start := parseCursor(req.Cursor)
	if start > total {
		start = total
	}
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	end := start + limit
	if end > total {
		end = total
	}
	next := ""
	if end < total {
		next = fmt.Sprintf("%d", end)
	}
	return AgentUIArtifactListResult{Items: filtered[start:end], Total: total, NextCursor: next}, nil
}

func (s *defaultAgentUIArtifactService) Get(id string) (AiopsTransportAgentUIArtifact, error) {
	id = strings.TrimSpace(id)
	for _, item := range s.items {
		if item.ID == id {
			return item, nil
		}
	}
	return AiopsTransportAgentUIArtifact{}, fmt.Errorf("agent ui artifact %q not found", id)
}

func (s *defaultAgentUIArtifactService) Validate(req AgentUIArtifactValidationRequest) (AgentUIArtifactValidationResult, error) {
	artifact := req.Artifact
	if strings.TrimSpace(req.ArtifactID) != "" {
		found, err := s.Get(req.ArtifactID)
		if err != nil {
			return AgentUIArtifactValidationResult{}, err
		}
		artifact = found
	}
	var errors []string
	if strings.TrimSpace(artifact.ID) == "" {
		errors = append(errors, "id is required")
	}
	if strings.TrimSpace(artifact.Type) == "" {
		errors = append(errors, "type is required")
	}
	if strings.TrimSpace(artifact.Source) == "" {
		errors = append(errors, "source is required")
	}
	for _, key := range findDangerousKeys(artifact.Payload) {
		errors = append(errors, "dangerous key is not allowed: "+key)
	}
	for _, key := range findDangerousKeys(artifact.InlineData) {
		errors = append(errors, "dangerous key is not allowed: "+key)
	}
	return AgentUIArtifactValidationResult{Valid: len(errors) == 0, Errors: errors}, nil
}

func artifactCaseID(item AiopsTransportAgentUIArtifact) string {
	for _, source := range []map[string]any{item.Metadata, item.Payload, item.InlineData} {
		if source == nil {
			continue
		}
		for _, key := range []string{"caseId", "caseID", "case_id"} {
			if value, ok := source[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func defaultAgentUIArtifacts() []AiopsTransportAgentUIArtifact {
	return []AiopsTransportAgentUIArtifact{
		{
			ID:        "artifact-ops-manual-search-demo",
			Type:      "ops_manual_search_result",
			Title:     "Ops manual search result",
			Summary:   "Need target context before selecting a workflow.",
			Status:    "need_info",
			Source:    "tool:search_ops_manuals",
			Metadata:  map[string]any{"caseId": "case-demo"},
			Payload:   map[string]any{"decision": "need_info", "summary": "missing target instance"},
			CreatedAt: "2026-05-16T10:00:00Z",
			UpdatedAt: "2026-05-16T10:00:00Z",
		},
		{
			ID:        "artifact-ops-manual-preflight-demo",
			Type:      "ops_manual_preflight_result",
			Title:     "Ops manual preflight result",
			Summary:   "Preflight passed.",
			Status:    "passed",
			Source:    "tool:run_ops_manual_preflight",
			Metadata:  map[string]any{"caseId": "case-demo"},
			Payload:   map[string]any{"status": "passed", "ready": true},
			CreatedAt: "2026-05-16T10:01:00Z",
			UpdatedAt: "2026-05-16T10:01:00Z",
		},
	}
}
