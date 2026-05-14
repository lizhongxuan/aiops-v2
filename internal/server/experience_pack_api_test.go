package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aiops-v2/internal/appui"
)

func TestExperiencePackAPIListCandidatesDoesNot404(t *testing.T) {
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/api/v1/experience-packs/candidates?limit=100")
	if err != nil {
		t.Fatalf("get candidates: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want=%d", resp.StatusCode, http.StatusOK)
	}

	var payload struct {
		Items []any `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Items == nil {
		t.Fatal("items should be an empty array when no persisted packs exist")
	}
}

func TestExperiencePackAPIListReuseRecordsDoesNot404(t *testing.T) {
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/api/v1/experience-packs/pack-pg/reuse-records?limit=20")
	if err != nil {
		t.Fatalf("get reuse records: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want=%d", resp.StatusCode, http.StatusOK)
	}

	var payload struct {
		Items []any `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Items == nil {
		t.Fatal("items should be an empty array when no reuse records exist")
	}
}

func TestExperiencePackAPICandidateReviewAndRetrieveGates(t *testing.T) {
	fake := newExperiencePackAPIFake()
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil, appui.WithExperiencePackService(fake)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	retrieveResp, err := ts.Client().Get(ts.URL + "/api/v1/experience-packs/candidates/candidate-1/retrieve")
	if err != nil {
		t.Fatalf("retrieve unreviewed candidate: %v", err)
	}
	defer retrieveResp.Body.Close()
	if retrieveResp.StatusCode != http.StatusForbidden {
		t.Fatalf("retrieve unreviewed status=%d want=%d", retrieveResp.StatusCode, http.StatusForbidden)
	}

	approveResp, err := ts.Client().Post(ts.URL+"/api/v1/experience-packs/candidates/candidate-1/approve", "application/json", strings.NewReader(`{"reviewer":"sre"}`))
	if err != nil {
		t.Fatalf("approve candidate: %v", err)
	}
	defer approveResp.Body.Close()
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("approve status=%d want=%d", approveResp.StatusCode, http.StatusOK)
	}

	retrieveResp, err = ts.Client().Get(ts.URL + "/api/v1/experience-packs/candidates/candidate-1/retrieve")
	if err != nil {
		t.Fatalf("retrieve approved candidate: %v", err)
	}
	defer retrieveResp.Body.Close()
	if retrieveResp.StatusCode != http.StatusOK {
		t.Fatalf("retrieve approved status=%d want=%d", retrieveResp.StatusCode, http.StatusOK)
	}
}

func TestExperiencePackAPIValidationGateBlocksEnableButAllowsPauseAndAlias(t *testing.T) {
	fake := newExperiencePackAPIFake()
	fake.pack.ValidationGate.Status = "blocked"
	fake.pack.ValidationGate.Reasons = []string{"retrieval evaluation failed"}
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil, appui.WithExperiencePackService(fake)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	enableResp, err := ts.Client().Post(ts.URL+"/api/v1/experience-packs/pack-1/review/enable", "application/json", strings.NewReader(`{"reviewer":"sre"}`))
	if err != nil {
		t.Fatalf("enable blocked pack: %v", err)
	}
	defer enableResp.Body.Close()
	if enableResp.StatusCode != http.StatusConflict {
		t.Fatalf("enable blocked status=%d want=%d", enableResp.StatusCode, http.StatusConflict)
	}

	pauseReq, err := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/experience-packs/pack-1/enabled", strings.NewReader(`{"enabled":false}`))
	if err != nil {
		t.Fatalf("new pause request: %v", err)
	}
	pauseReq.Header.Set("Content-Type", "application/json")
	pauseResp, err := ts.Client().Do(pauseReq)
	if err != nil {
		t.Fatalf("pause through enabled alias: %v", err)
	}
	defer pauseResp.Body.Close()
	if pauseResp.StatusCode != http.StatusOK {
		t.Fatalf("pause status=%d want=%d", pauseResp.StatusCode, http.StatusOK)
	}
}

func TestExperiencePackAPINewEndpointsDispatchToAppService(t *testing.T) {
	fake := newExperiencePackAPIFake()
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil, appui.WithExperiencePackService(fake)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/experience-packs/suggestions/evaluate", `{"caseId":"case-1"}`},
		{http.MethodPost, "/api/v1/experience-packs/candidates/prepare", `{"caseId":"case-1"}`},
		{http.MethodPost, "/api/v1/experience-packs/candidates/candidate-1/confirm", `{"reviewer":"sre"}`},
		{http.MethodPost, "/api/v1/experience-packs/runner-candidates/prepare", `{"packId":"pack-1"}`},
		{http.MethodPost, "/api/v1/experience-packs/runner-candidates/confirm", `{"packId":"pack-1"}`},
		{http.MethodGet, "/api/v1/experience-packs/pack-1/validation-gate", ``},
		{http.MethodPut, "/api/v1/experience-packs/pack-1/authorization-scopes", `{"scopes":[{"type":"service","value":"checkout","searchable":true}]}`},
		{http.MethodGet, "/api/v1/experience-packs/pack-1/authorization-scopes", ``},
		{http.MethodPut, "/api/v1/experience-packs/pack-1/runner-bindings", `{"bindings":[{"workflowId":"wf-1","status":"bound"}]}`},
		{http.MethodPost, "/api/v1/experience-packs/pack-1/runner-bindings/review", `{"reviewer":"sre","decision":"approve"}`},
		{http.MethodGet, "/api/v1/experience-packs/pack-1/retrieve", ``},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status=%d want=%d", resp.StatusCode, http.StatusOK)
			}
		})
	}
}

func TestExperiencePackAPIRunnerCandidateConfirmReturnsRunnerDraft(t *testing.T) {
	fake := newExperiencePackAPIFake()
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil, appui.WithExperiencePackService(fake)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Post(ts.URL+"/api/v1/experience-packs/runner-candidates/confirm", "application/json", strings.NewReader(`{"packId":"pack-1","title":"Checkout RCA"}`))
	if err != nil {
		t.Fatalf("confirm runner candidate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want=%d", resp.StatusCode, http.StatusOK)
	}
	var payload struct {
		ID              string         `json:"id"`
		WorkflowID      string         `json:"workflow_id"`
		StudioDraftLink string         `json:"studio_draft_link"`
		Workflow        map[string]any `json:"workflow"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ID == "" || payload.WorkflowID == "" {
		t.Fatalf("runner candidate response missing ids: %+v", payload)
	}
	if !strings.HasPrefix(payload.StudioDraftLink, "/runner/") {
		t.Fatalf("studio draft link=%q want /runner/ prefix", payload.StudioDraftLink)
	}
	if payload.Workflow["graph"] == nil {
		t.Fatalf("workflow draft should include graph: %+v", payload.Workflow)
	}
}

type experiencePackAPIFake struct {
	candidate appui.ExperiencePackCandidate
	pack      appui.ExperiencePack
}

func newExperiencePackAPIFake() *experiencePackAPIFake {
	pack := appui.ExperiencePack{
		ID:           "pack-1",
		Title:        "Checkout lock wait",
		ReviewStatus: "pending",
		Enabled:      false,
		Status:       "disabled",
		ValidationGate: appui.ExperiencePackValidationGate{
			Status: "passed",
		},
		AuthorizationScopes: []appui.ExperiencePackAuthorizationScope{
			{ID: "service:checkout", Type: "service", Value: "checkout", Searchable: true},
		},
		RunnerBindings: []appui.ExperiencePackRunnerBinding{
			{WorkflowID: "wf-1", WorkflowName: "Checkout RCA", Status: "bound"},
		},
	}
	return &experiencePackAPIFake{
		pack: pack,
		candidate: appui.ExperiencePackCandidate{
			ID:             "candidate-1",
			PackID:         pack.ID,
			Title:          pack.Title,
			Status:         "candidate",
			ExperiencePack: &pack,
		},
	}
}

func (f *experiencePackAPIFake) ListCandidates(_ appui.ListExperiencePackCandidatesRequest) (appui.ExperiencePackCandidateList, error) {
	return appui.ExperiencePackCandidateList{Items: []appui.ExperiencePackCandidate{f.candidate}, Total: 1}, nil
}

func (f *experiencePackAPIFake) ListPacks(_ appui.ListExperiencePacksRequest) (appui.ExperiencePackLibraryList, error) {
	return appui.ExperiencePackLibraryList{Items: []appui.ExperiencePack{f.pack}, Total: 1}, nil
}

func (f *experiencePackAPIFake) ListExperiencePacks(req appui.ListExperiencePacksRequest) (appui.ExperiencePackLibraryList, error) {
	return f.ListPacks(req)
}

func (f *experiencePackAPIFake) Retrieve(_ appui.ExperiencePackRetrieveRequest) (appui.ExperiencePackMatchList, error) {
	return appui.ExperiencePackMatchList{Items: []appui.ExperiencePackMatch{{
		PackID:      f.pack.ID,
		Skill:       appui.ExperiencePackSkill{Name: f.pack.Title, Summary: f.pack.Summary, Path: "skills/SKILL.md"},
		NextActions: []string{"view_skill", "check_preconditions", "view_history", "mark_not_applicable"},
	}}, Total: 1}, nil
}

func (f *experiencePackAPIFake) RetrieveCandidate(id string) (appui.ExperiencePack, error) {
	if f.candidate.ID != id {
		return appui.ExperiencePack{}, appui.ErrExperiencePackCandidateNotFound
	}
	if f.candidate.Status != "approved" {
		return appui.ExperiencePack{}, appui.ErrExperiencePackCandidateNotApproved
	}
	return f.pack, nil
}

func (f *experiencePackAPIFake) EvaluateSuggestions(appui.ExperiencePackSuggestionEvaluateRequest) (appui.ExperiencePackSuggestionEvaluateResult, error) {
	item := appui.ExperiencePackSuggestion{ID: "generate_experience_pack_candidate", Type: "generate_experience_pack_candidate", Label: "生成经验包"}
	return appui.ExperiencePackSuggestionEvaluateResult{Items: []appui.ExperiencePackSuggestion{item}, Suggestions: []appui.ExperiencePackSuggestion{item}, Total: 1}, nil
}

func (f *experiencePackAPIFake) PrepareCandidate(appui.ExperiencePackPrepareCandidateRequest) (appui.ExperiencePackCandidate, error) {
	return f.candidate, nil
}

func (f *experiencePackAPIFake) ConfirmCandidate(id string, _ appui.ExperiencePackReviewRequest) (appui.ExperiencePack, error) {
	if f.candidate.ID != id {
		return appui.ExperiencePack{}, appui.ErrExperiencePackCandidateNotFound
	}
	f.candidate.Status = "approved"
	f.pack.ReviewStatus = "approved"
	return f.pack, nil
}

func (f *experiencePackAPIFake) PrepareRunnerCandidate(req appui.ExperiencePackRunnerCandidateRequest) (appui.ExperiencePackRunnerCandidate, error) {
	return f.runnerCandidate(req), nil
}

func (f *experiencePackAPIFake) ConfirmRunnerCandidate(req appui.ExperiencePackRunnerCandidateRequest) (appui.ExperiencePackRunnerCandidate, error) {
	candidate := f.runnerCandidate(req)
	f.pack.RunnerBindings = append(f.pack.RunnerBindings, candidate.RunnerBinding)
	return candidate, nil
}

func (f *experiencePackAPIFake) runnerCandidate(req appui.ExperiencePackRunnerCandidateRequest) appui.ExperiencePackRunnerCandidate {
	workflowID := "runner-candidate-1"
	graph := map[string]any{"workflow": map[string]any{"name": workflowID}, "nodes": []any{}, "edges": []any{}}
	return appui.ExperiencePackRunnerCandidate{
		ID:              workflowID,
		PackID:          firstNonEmptyString(req.PackID, f.pack.ID),
		WorkflowID:      workflowID,
		WorkflowName:    firstNonEmptyString(req.Title, "Checkout RCA Workflow"),
		Status:          "draft",
		StudioDraftLink: "/runner/" + workflowID,
		Graph:           graph,
		Workflow:        map[string]any{"id": workflowID, "name": workflowID, "status": "draft", "local_draft": true, "graph": graph},
		RunnerBinding: appui.ExperiencePackRunnerBinding{
			ID:           "binding-" + workflowID,
			WorkflowID:   workflowID,
			WorkflowName: "Checkout RCA Workflow",
			Status:       "draft",
			ReviewStatus: "pending",
		},
	}
}

func (f *experiencePackAPIFake) GetPack(id string) (appui.ExperiencePack, error) {
	if f.pack.ID != id {
		return appui.ExperiencePack{}, appui.ErrExperiencePackNotFound
	}
	return f.pack, nil
}

func (f *experiencePackAPIFake) GetValidationGate(id string) (appui.ExperiencePackValidationGate, error) {
	if f.pack.ID != id {
		return appui.ExperiencePackValidationGate{}, appui.ErrExperiencePackNotFound
	}
	return f.pack.ValidationGate, nil
}

func (f *experiencePackAPIFake) EnablePack(id string, _ appui.ExperiencePackReviewRequest) (appui.ExperiencePack, error) {
	if f.pack.ID != id {
		return appui.ExperiencePack{}, appui.ErrExperiencePackNotFound
	}
	if f.pack.ValidationGate.Status == "blocked" {
		return appui.ExperiencePack{}, appui.ErrExperiencePackValidationBlocked
	}
	f.pack.Enabled = true
	f.pack.Status = "enabled"
	return f.pack, nil
}

func (f *experiencePackAPIFake) PausePack(id string, _ appui.ExperiencePackReviewRequest) (appui.ExperiencePack, error) {
	if f.pack.ID != id {
		return appui.ExperiencePack{}, appui.ErrExperiencePackNotFound
	}
	f.pack.Enabled = false
	f.pack.Status = "disabled"
	return f.pack, nil
}

func (f *experiencePackAPIFake) SetPackEnabled(id string, enabled bool, req appui.ExperiencePackReviewRequest) (appui.ExperiencePack, error) {
	if enabled {
		return f.EnablePack(id, req)
	}
	return f.PausePack(id, req)
}

func (f *experiencePackAPIFake) SaveAuthorizationScopes(id string, scopes []appui.ExperiencePackAuthorizationScope) (appui.ExperiencePack, error) {
	if f.pack.ID != id {
		return appui.ExperiencePack{}, appui.ErrExperiencePackNotFound
	}
	f.pack.AuthorizationScopes = scopes
	return f.pack, nil
}

func (f *experiencePackAPIFake) GetAuthorizationScopes(id string) ([]appui.ExperiencePackAuthorizationScope, error) {
	if f.pack.ID != id {
		return nil, appui.ErrExperiencePackNotFound
	}
	return f.pack.AuthorizationScopes, nil
}

func (f *experiencePackAPIFake) SaveRunnerBindings(id string, bindings []appui.ExperiencePackRunnerBinding) (appui.ExperiencePack, error) {
	if f.pack.ID != id {
		return appui.ExperiencePack{}, appui.ErrExperiencePackNotFound
	}
	f.pack.RunnerBindings = bindings
	return f.pack, nil
}

func (f *experiencePackAPIFake) ReviewRunnerBindings(id string, req appui.ExperiencePackRunnerBindingReviewRequest) (appui.ExperiencePack, error) {
	if f.pack.ID != id {
		return appui.ExperiencePack{}, appui.ErrExperiencePackNotFound
	}
	for i := range f.pack.RunnerBindings {
		f.pack.RunnerBindings[i].ReviewStatus = req.Decision
	}
	return f.pack, nil
}

func (f *experiencePackAPIFake) ListReuseRecords(string, appui.ListExperiencePackReuseRecordsRequest) (appui.ExperiencePackReuseRecordList, error) {
	return appui.ExperiencePackReuseRecordList{Items: []appui.ExperiencePackReuseRecord{}, Total: 0}, nil
}
