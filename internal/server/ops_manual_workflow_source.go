package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/opsmanual"
)

type workflowSourceError struct {
	status  int
	message string
}

func (e workflowSourceError) Error() string { return e.message }

func (s *HTTPServer) opsManualGenerationRequestFromRunner(ctx context.Context, req appui.OpsManualGenerateFromWorkflowRequest) (opsmanual.WorkflowManualGenerationRequest, error) {
	workflowID := strings.TrimSpace(req.WorkflowID)
	if workflowID == "" {
		return opsmanual.WorkflowManualGenerationRequest{}, workflowSourceError{status: http.StatusBadRequest, message: "workflow_id is required"}
	}
	if s.runnerStudioHandler == nil {
		return opsmanual.WorkflowManualGenerationRequest{}, workflowSourceError{status: http.StatusServiceUnavailable, message: "embedded runner is not available"}
	}
	rawYAML, storageURI, err := s.fetchRunnerWorkflowYAML(ctx, workflowID)
	if err != nil {
		return opsmanual.WorkflowManualGenerationRequest{}, err
	}
	actionSpecs, err := s.fetchRunnerActionSpecs(ctx)
	if err != nil {
		return opsmanual.WorkflowManualGenerationRequest{}, err
	}
	out := opsmanual.WorkflowManualGenerationRequest{
		WorkflowID:      workflowID,
		WorkflowVersion: strings.TrimSpace(req.WorkflowVersion),
		StorageURI:      storageURI,
		RawYAML:         rawYAML,
		ActionSpecs:     actionSpecs,
		Options: opsmanual.WorkflowManualGenerationOptions{
			IncludeRecentRunRecords: req.Options.IncludeRecentRunRecords,
			UseLLMSummary:           req.Options.UseLLMSummary,
		},
	}
	if req.Options.IncludeRecentRunRecords {
		out.RecentRuns = s.fetchRunnerRunRecords(ctx, workflowID)
	}
	return out, nil
}

func (s *HTTPServer) fetchRunnerWorkflowYAML(ctx context.Context, workflowID string) ([]byte, string, error) {
	path := "/api/v1/workflows/" + url.PathEscape(workflowID) + "/graph"
	status, body, err := s.runnerStudioRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, "", err
	}
	if status == http.StatusNotFound {
		return nil, "", workflowSourceError{status: http.StatusNotFound, message: "workflow not found"}
	}
	if status < 200 || status >= 300 {
		return nil, "", workflowSourceError{status: http.StatusBadGateway, message: fmt.Sprintf("runner workflow graph returned status %d", status)}
	}
	if raw := yamlFromRunnerResponse(body); len(raw) > 0 {
		return raw, "runner-studio://" + workflowID, nil
	}
	status, compiled, err := s.runnerStudioRequest(ctx, http.MethodPost, "/api/v1/workflows/graph/compile", body, "application/json")
	if err != nil {
		return nil, "", err
	}
	if status < 200 || status >= 300 {
		return nil, "", workflowSourceError{status: http.StatusBadGateway, message: fmt.Sprintf("runner graph compile returned status %d", status)}
	}
	raw := yamlFromRunnerResponse(compiled)
	if len(raw) == 0 {
		return nil, "", workflowSourceError{status: http.StatusBadGateway, message: "runner graph compile response did not include yaml"}
	}
	return raw, "runner-studio://" + workflowID, nil
}

func (s *HTTPServer) fetchRunnerActionSpecs(ctx context.Context) ([]opsmanual.ActionSpecSummary, error) {
	status, body, err := s.runnerStudioRequest(ctx, http.MethodGet, "/api/v1/actions/catalog", nil, "")
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, workflowSourceError{status: http.StatusBadGateway, message: fmt.Sprintf("runner action catalog returned status %d", status)}
	}
	return actionSpecsFromRunnerCatalog(body), nil
}

func (s *HTTPServer) fetchRunnerRunRecords(ctx context.Context, workflowID string) []opsmanual.RunRecord {
	status, body, err := s.runnerStudioRequest(ctx, http.MethodGet, "/api/v1/runs", nil, "workflow="+url.QueryEscape(workflowID)+"&limit=20")
	if err != nil || status < 200 || status >= 300 {
		return nil
	}
	var decoded struct {
		Items []struct {
			ID               string `json:"id"`
			WorkflowID       string `json:"workflow_id"`
			WorkflowVersion  string `json:"workflow_version"`
			WorkflowDigest   string `json:"workflow_digest"`
			ExecutionStatus  string `json:"execution_status"`
			ValidationStatus string `json:"validation_status"`
			StartedAt        string `json:"started_at"`
			CompletedAt      string `json:"completed_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	out := make([]opsmanual.RunRecord, 0, len(decoded.Items))
	for _, item := range decoded.Items {
		out = append(out, opsmanual.RunRecord{
			ID:               item.ID,
			WorkflowID:       firstNonEmptyServerString(item.WorkflowID, workflowID),
			WorkflowVersion:  item.WorkflowVersion,
			WorkflowDigest:   item.WorkflowDigest,
			ExecutionStatus:  item.ExecutionStatus,
			ValidationStatus: item.ValidationStatus,
			StartedAt:        item.StartedAt,
			CompletedAt:      item.CompletedAt,
		})
	}
	return out
}

func (s *HTTPServer) runnerStudioRequest(ctx context.Context, method string, path string, body []byte, rawQuery string) (int, []byte, error) {
	if s.runnerStudioHandler != nil {
		target := path
		if rawQuery != "" {
			target += "?" + rawQuery
		}
		req := httptest.NewRequest(method, target, bytes.NewReader(body)).WithContext(ctx)
		if len(body) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		s.runnerStudioHandler.ServeHTTP(rec, req)
		return rec.Code, rec.Body.Bytes(), nil
	}
	return 0, nil, workflowSourceError{status: http.StatusServiceUnavailable, message: "embedded runner is not available"}
}

func yamlFromRunnerResponse(raw []byte) []byte {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		if bytes.Contains(raw, []byte("version:")) {
			return bytes.TrimSpace(raw)
		}
		return nil
	}
	if yaml := yamlStringFromAny(decoded); strings.TrimSpace(yaml) != "" {
		return []byte(strings.TrimSpace(yaml))
	}
	return nil
}

func yamlStringFromAny(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"yaml", "workflow_yaml", "raw_yaml"} {
			if text, ok := typed[key].(string); ok && strings.TrimSpace(text) != "" {
				return text
			}
		}
		for _, key := range []string{"workflow", "graph", "data"} {
			if nested, ok := typed[key]; ok {
				if text := yamlStringFromAny(nested); text != "" {
					return text
				}
			}
		}
	case string:
		if strings.Contains(typed, "version:") {
			return typed
		}
	}
	return ""
}

func actionSpecsFromRunnerCatalog(raw []byte) []opsmanual.ActionSpecSummary {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	items := runnerCatalogItems(decoded)
	out := make([]opsmanual.ActionSpecSummary, 0, len(items))
	for _, item := range items {
		spec := opsmanual.ActionSpecSummary{
			Action:       firstJSONText(item, "action", "name", "id"),
			Title:        firstJSONText(item, "title", "label"),
			Category:     firstJSONText(item, "category"),
			Risk:         firstJSONText(item, "risk", "risk_level"),
			RequiredArgs: jsonStringSlice(item["required_args"], item["requiredArgs"]),
			Outputs:      jsonStringSlice(item["outputs"]),
		}
		if deprecated, ok := item["deprecated"].(bool); ok {
			spec.Deprecated = deprecated
		}
		if spec.Action != "" {
			out = append(out, spec)
		}
	}
	return out
}

func runnerCatalogItems(decoded any) []map[string]any {
	switch typed := decoded.(type) {
	case []any:
		return mapsFromAnySlice(typed)
	case map[string]any:
		for _, key := range []string{"items", "actions", "catalog"} {
			if values, ok := typed[key].([]any); ok {
				return mapsFromAnySlice(values)
			}
		}
	}
	return nil
}

func mapsFromAnySlice(values []any) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if item, ok := value.(map[string]any); ok {
			out = append(out, item)
		}
	}
	return out
}

func firstJSONText(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key]; ok && strings.TrimSpace(fmt.Sprint(value)) != "" {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func jsonStringSlice(values ...any) []string {
	out := []string{}
	for _, value := range values {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				out = append(out, strings.TrimSpace(fmt.Sprint(item)))
			}
		case []string:
			out = append(out, typed...)
		case string:
			if strings.TrimSpace(typed) != "" {
				out = append(out, strings.TrimSpace(typed))
			}
		}
	}
	return out
}

func firstNonEmptyServerString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
