package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"aiops-v2/internal/appui"
)

func (s *HTTPServer) handleExperiencePacks(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/experience-packs"), "/")
	service := s.experiencePackService()

	switch {
	case r.Method == http.MethodGet && path == "":
		result, err := service.ListPacks(appui.ListExperiencePacksRequest{
			Status:           r.URL.Query().Get("status"),
			Category:         r.URL.Query().Get("category"),
			UsageShape:       firstNonEmptyString(r.URL.Query().Get("usage_shape"), r.URL.Query().Get("usageShape")),
			Middleware:       r.URL.Query().Get("middleware"),
			Tag:              r.URL.Query().Get("tag"),
			HasRunnerBinding: r.URL.Query().Get("has_runner_binding"),
			Limit:            queryInt(r, "limit"),
			Cursor:           r.URL.Query().Get("cursor"),
		})
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodGet && path == "candidates":
		result, err := service.ListCandidates(appui.ListExperiencePackCandidatesRequest{
			CaseID:      r.URL.Query().Get("case_id"),
			Service:     r.URL.Query().Get("service"),
			Environment: r.URL.Query().Get("environment"),
			Limit:       queryInt(r, "limit"),
			Cursor:      r.URL.Query().Get("cursor"),
		})
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodPost && path == "retrieve":
		var req appui.ExperiencePackRetrieveRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.Retrieve(req)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodPost && path == "suggestions/evaluate":
		var req appui.ExperiencePackSuggestionEvaluateRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.EvaluateSuggestions(req)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodPost && path == "candidates/prepare":
		var req appui.ExperiencePackPrepareCandidateRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.PrepareCandidate(req)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodPost && path == "candidates/confirm":
		var req appui.ExperiencePackReviewRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.ConfirmCandidate("", req)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "candidates/") && strings.HasSuffix(path, "/retrieve"):
		candidateID, ok := pathSegmentBetween(path, "candidates/", "/retrieve")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack candidate not found"})
			return
		}
		result, err := service.RetrieveCandidate(candidateID)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodPost && strings.HasPrefix(path, "candidates/") && strings.HasSuffix(path, "/confirm"):
		candidateID, ok := pathSegmentBetween(path, "candidates/", "/confirm")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack candidate not found"})
			return
		}
		var req appui.ExperiencePackReviewRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.ConfirmCandidate(candidateID, req)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodPost && strings.HasPrefix(path, "candidates/") && strings.HasSuffix(path, "/approve"):
		candidateID, ok := pathSegmentBetween(path, "candidates/", "/approve")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack candidate not found"})
			return
		}
		var req appui.ExperiencePackReviewRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		req.Decision = firstNonEmptyString(req.Decision, "approve")
		result, err := service.ConfirmCandidate(candidateID, req)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/reuse-records"):
		packID, ok := pathSegmentSuffix(path, "/reuse-records")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		result, err := service.ListReuseRecords(packID, appui.ListExperiencePackReuseRecordsRequest{
			CaseID: r.URL.Query().Get("case_id"),
			Limit:  queryInt(r, "limit"),
			Cursor: r.URL.Query().Get("cursor"),
		})
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/retrieve"):
		packID, ok := pathSegmentSuffix(path, "/retrieve")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		result, err := service.GetPack(packID)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/validation-gate"):
		packID, ok := pathSegmentSuffix(path, "/validation-gate")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		gate, err := service.GetValidationGate(packID)
		writeExperiencePackResult(w, map[string]any{
			"validationGate":  gate,
			"validation_gate": gate,
		}, err)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/validation-gate/check"):
		packID, ok := pathSegmentSuffix(path, "/validation-gate/check")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		var req map[string]any
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		gate, err := service.GetValidationGate(packID)
		writeExperiencePackResult(w, map[string]any{
			"status":         gate.Status,
			"passed":         gate.Status != "blocked",
			"blockedReasons": gate.Reasons,
			"checks":         gate.Checks,
		}, err)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/review") && !strings.Contains(path, "/runner-bindings/"):
		packID, ok := pathSegmentSuffix(path, "/review")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		var req appui.ExperiencePackReviewRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.ConfirmCandidate(packID, req)
		if err != nil {
			pack, getErr := service.GetPack(packID)
			if getErr == nil {
				pack.ReviewStatus = firstNonEmptyString(req.Decision, "approved")
				if pack.ReviewStatus == "approve" {
					pack.ReviewStatus = "approved"
				}
				result = pack
				err = nil
			}
		}
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/enable") && !strings.HasSuffix(path, "/review/enable"):
		packID, ok := pathSegmentSuffix(path, "/enable")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		var req appui.ExperiencePackReviewRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.EnablePack(packID, req)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/pause"):
		packID, ok := pathSegmentSuffix(path, "/pause")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		var req appui.ExperiencePackReviewRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.PausePack(packID, req)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/review/enable"):
		packID, ok := pathSegmentSuffix(path, "/review/enable")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		var req appui.ExperiencePackReviewRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.EnablePack(packID, req)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/review/pause"):
		packID, ok := pathSegmentSuffix(path, "/review/pause")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		var req appui.ExperiencePackReviewRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.PausePack(packID, req)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodPost && path == "runner-candidates/prepare":
		var req appui.ExperiencePackRunnerCandidateRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.PrepareRunnerCandidate(req)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodPost && path == "runner-candidates/confirm":
		var req appui.ExperiencePackRunnerCandidateRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.ConfirmRunnerCandidate(req)
		writeExperiencePackResult(w, result, err)
	case (r.Method == http.MethodPatch || r.Method == http.MethodPut) && strings.HasSuffix(path, "/enabled"):
		packID, ok := pathSegmentSuffix(path, "/enabled")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		var req struct {
			Enabled  bool   `json:"enabled"`
			Reviewer string `json:"reviewer,omitempty"`
			Comment  string `json:"comment,omitempty"`
		}
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.SetPackEnabled(packID, req.Enabled, appui.ExperiencePackReviewRequest{
			Reviewer: req.Reviewer,
			Comment:  req.Comment,
		})
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/authorization-scopes"):
		packID, ok := pathSegmentSuffix(path, "/authorization-scopes")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		scopes, err := service.GetAuthorizationScopes(packID)
		writeExperiencePackResult(w, map[string]any{
			"items":  scopes,
			"scopes": scopes,
		}, err)
	case r.Method == http.MethodPut && strings.HasSuffix(path, "/authorization-scopes"):
		packID, ok := pathSegmentSuffix(path, "/authorization-scopes")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		var req struct {
			Scopes []appui.ExperiencePackAuthorizationScope `json:"scopes"`
		}
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.SaveAuthorizationScopes(packID, req.Scopes)
		writeExperiencePackResult(w, result, err)
	case (r.Method == http.MethodPut || r.Method == http.MethodPatch) && strings.HasSuffix(path, "/runner-bindings"):
		packID, ok := pathSegmentSuffix(path, "/runner-bindings")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		var req struct {
			Bindings []appui.ExperiencePackRunnerBinding `json:"bindings"`
		}
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.SaveRunnerBindings(packID, req.Bindings)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/runner-bindings"):
		packID, ok := pathSegmentSuffix(path, "/runner-bindings")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		pack, err := service.GetPack(packID)
		writeExperiencePackResult(w, map[string]any{"items": pack.RunnerBindings, "bindings": pack.RunnerBindings}, err)
	case r.Method == http.MethodPost && strings.Contains(path, "/runner-bindings/") && strings.HasSuffix(path, "/review") && !strings.HasSuffix(path, "/runner-bindings/review"):
		packID, bindingID, ok := pathBindingReview(path)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner binding not found"})
			return
		}
		var req appui.ExperiencePackRunnerBindingReviewRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		req.BindingIDs = append(req.BindingIDs, bindingID)
		pack, err := service.ReviewRunnerBindings(packID, req)
		var result any = pack
		if err == nil {
			for _, binding := range pack.RunnerBindings {
				if binding.ID == bindingID || binding.WorkflowID == bindingID {
					result = binding
					break
				}
			}
		}
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/runner-bindings/review"):
		packID, ok := pathSegmentSuffix(path, "/runner-bindings/review")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		var req appui.ExperiencePackRunnerBindingReviewRequest
		if !decodeExperiencePackJSON(w, r, &req) {
			return
		}
		result, err := service.ReviewRunnerBindings(packID, req)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodGet && !strings.Contains(path, "/"):
		result, err := service.GetPack(path)
		writeExperiencePackResult(w, result, err)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/skill"):
		packID, ok := pathSegmentSuffix(path, "/skill")
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
			return
		}
		pack, err := service.GetPack(packID)
		writeExperiencePackResult(w, pack.Skill, err)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/files"):
		writeExperiencePackResult(w, map[string]any{"items": []any{}}, nil)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/capsules"):
		writeExperiencePackResult(w, map[string]any{"items": []any{}}, nil)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/events"):
		writeExperiencePackResult(w, map[string]any{"items": []any{}}, nil)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/memory-events"):
		writeExperiencePackResult(w, map[string]any{"items": []any{}}, nil)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/avoid-cues"):
		writeExperiencePackResult(w, map[string]any{"items": []any{}}, nil)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) experiencePackService() appui.ExperiencePackService {
	if provider, ok := s.ui.(appui.ExperiencePackServiceProvider); ok {
		if service := provider.ExperiencePackService(); service != nil {
			return service
		}
	}
	return appui.NewExperiencePackService(nil)
}

func decodeExperiencePackJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	if r.Body == nil {
		return true
	}
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return false
	}
	return true
}

func writeExperiencePackResult(w http.ResponseWriter, result any, err error) {
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, appui.ErrExperiencePackNotFound), errors.Is(err, appui.ErrExperiencePackCandidateNotFound):
			status = http.StatusNotFound
		case errors.Is(err, appui.ErrExperiencePackCandidateNotApproved):
			status = http.StatusForbidden
		case errors.Is(err, appui.ErrExperiencePackValidationBlocked):
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func queryInt(r *http.Request, key string) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func pathSegmentBetween(path, prefix, suffix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	segment := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if segment == "" || strings.Contains(segment, "/") {
		return "", false
	}
	return unescapePathSegment(segment)
}

func pathSegmentSuffix(path, suffix string) (string, bool) {
	if !strings.HasSuffix(path, suffix) {
		return "", false
	}
	segment := strings.TrimSuffix(path, suffix)
	if segment == "" || strings.Contains(segment, "/") {
		return "", false
	}
	return unescapePathSegment(segment)
}

func pathBindingReview(path string) (string, string, bool) {
	prefix, suffix := "/runner-bindings/", "/review"
	if !strings.Contains(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", "", false
	}
	left, right, _ := strings.Cut(path, prefix)
	binding := strings.TrimSuffix(right, suffix)
	if left == "" || binding == "" || strings.Contains(binding, "/") {
		return "", "", false
	}
	packID, ok := unescapePathSegment(left)
	if !ok {
		return "", "", false
	}
	bindingID, ok := unescapePathSegment(binding)
	if !ok {
		return "", "", false
	}
	return packID, bindingID, true
}

func unescapePathSegment(segment string) (string, bool) {
	value, err := url.PathUnescape(segment)
	if err != nil || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
