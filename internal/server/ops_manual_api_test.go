package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/opsmanual"
)

type opsManualAPITestServices struct {
	service appui.OpsManualService
}

func (s *opsManualAPITestServices) ChatService() appui.ChatService         { return nil }
func (s *opsManualAPITestServices) StateService() appui.StateService       { return nil }
func (s *opsManualAPITestServices) SessionService() appui.SessionService   { return nil }
func (s *opsManualAPITestServices) ApprovalService() appui.ApprovalService { return nil }
func (s *opsManualAPITestServices) ChoiceService() appui.ChoiceService     { return nil }
func (s *opsManualAPITestServices) SettingsService() appui.SettingsService { return nil }
func (s *opsManualAPITestServices) HostService() appui.HostService         { return nil }
func (s *opsManualAPITestServices) MCPService() appui.MCPService           { return nil }
func (s *opsManualAPITestServices) AgentProfileService() appui.AgentProfileService {
	return nil
}
func (s *opsManualAPITestServices) AuthService() appui.AuthService         { return nil }
func (s *opsManualAPITestServices) TerminalService() appui.TerminalService { return nil }
func (s *opsManualAPITestServices) OpsManualService() appui.OpsManualService {
	return s.service
}

func TestOpsManualAPIRetrievePrepareAndConfirm(t *testing.T) {
	repo := opsmanual.NewMemoryStore()
	if err := repo.SaveManual(serverRedisManual()); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	if err := repo.SaveRunRecord(opsmanual.RunRecord{
		ID:               "record-redis-1",
		ManualID:         "manual-redis-memory",
		WorkflowID:       "workflow-redis-memory",
		ValidationStatus: "passed",
		CompletedAt:      "2026-05-14T08:00:00Z",
	}); err != nil {
		t.Fatalf("SaveRunRecord() error = %v", err)
	}
	service := appui.NewOpsManualService(opsmanual.NewService(repo))
	server := NewHTTPServer(&opsManualAPITestServices{service: service}, WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	retrieveResp, err := http.Post(ts.URL+"/api/v1/ops-manuals/retrieve", "application/json", bytes.NewReader([]byte(`{"text":"排查 Redis"}`)))
	if err != nil {
		t.Fatalf("POST /retrieve error = %v", err)
	}
	defer retrieveResp.Body.Close()
	if retrieveResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /retrieve status = %d, want 200", retrieveResp.StatusCode)
	}
	var retrievePayload struct {
		Matches []opsmanual.ManualMatch `json:"matches"`
	}
	if err := json.NewDecoder(retrieveResp.Body).Decode(&retrievePayload); err != nil {
		t.Fatalf("decode retrieve payload: %v", err)
	}
	if len(retrievePayload.Matches) != 1 || retrievePayload.Matches[0].State != opsmanual.DecisionNeedMoreInfo {
		t.Fatalf("retrieve payload = %#v, want need_more_info", retrievePayload)
	}

	recordsResp, err := http.Get(ts.URL + "/api/v1/ops-manuals/manual-redis-memory/run-records")
	if err != nil {
		t.Fatalf("GET /run-records error = %v", err)
	}
	defer recordsResp.Body.Close()
	var recordsPayload struct {
		Items []opsmanual.RunRecord `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.NewDecoder(recordsResp.Body).Decode(&recordsPayload); err != nil {
		t.Fatalf("decode run records payload: %v", err)
	}
	if recordsPayload.Total != 1 || recordsPayload.Items[0].ID != "record-redis-1" {
		t.Fatalf("run records payload = %#v, want seeded record", recordsPayload)
	}

	prepareBody, _ := json.Marshal(appui.OpsManualPrepareCandidateRequest{
		SourceType: "workflow",
		SourceRefs: []string{"workflow-pg-backup"},
		Manual: opsmanual.OpsManual{
			ID:               "manual-pg-backup",
			Title:            "PostgreSQL backup",
			WorkflowRef:      opsmanual.WorkflowRef{WorkflowID: "workflow-pg-backup"},
			Operation:        opsmanual.OperationProfile{TargetType: "postgresql", Action: "backup"},
			Validation:       []string{"backup file exists"},
			CannotUseWhen:    []string{"目标实例未知"},
			DocumentMarkdown: "PostgreSQL backup manual",
		},
	})
	prepareResp, err := http.Post(ts.URL+"/api/v1/ops-manuals/candidates/prepare", "application/json", bytes.NewReader(prepareBody))
	if err != nil {
		t.Fatalf("POST /candidates/prepare error = %v", err)
	}
	defer prepareResp.Body.Close()
	if prepareResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /candidates/prepare status = %d, want 200", prepareResp.StatusCode)
	}
	var preparePayload struct {
		Candidate opsmanual.ManualCandidate `json:"candidate"`
	}
	if err := json.NewDecoder(prepareResp.Body).Decode(&preparePayload); err != nil {
		t.Fatalf("decode prepare payload: %v", err)
	}
	if preparePayload.Candidate.ID != "manual-pg-backup" {
		t.Fatalf("candidate = %#v, want manual-pg-backup", preparePayload.Candidate)
	}

	candidatesResp, err := http.Get(ts.URL + "/api/v1/ops-manuals/candidates")
	if err != nil {
		t.Fatalf("GET /candidates error = %v", err)
	}
	defer candidatesResp.Body.Close()
	if candidatesResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /candidates status = %d, want 200", candidatesResp.StatusCode)
	}
	var candidatesPayload struct {
		Items []opsmanual.ManualCandidate `json:"items"`
		Total int                         `json:"total"`
	}
	if err := json.NewDecoder(candidatesResp.Body).Decode(&candidatesPayload); err != nil {
		t.Fatalf("decode candidates payload: %v", err)
	}
	if candidatesPayload.Total != 1 || candidatesPayload.Items[0].ID != "manual-pg-backup" {
		t.Fatalf("candidates payload = %#v, want pending manual-pg-backup", candidatesPayload)
	}

	allRecordsResp, err := http.Get(ts.URL + "/api/v1/ops-manuals/run-records")
	if err != nil {
		t.Fatalf("GET global /run-records error = %v", err)
	}
	defer allRecordsResp.Body.Close()
	if allRecordsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET global /run-records status = %d, want 200", allRecordsResp.StatusCode)
	}
	var allRecordsPayload struct {
		Items []opsmanual.RunRecord `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.NewDecoder(allRecordsResp.Body).Decode(&allRecordsPayload); err != nil {
		t.Fatalf("decode global run records payload: %v", err)
	}
	if allRecordsPayload.Total != 1 || allRecordsPayload.Items[0].ID != "record-redis-1" {
		t.Fatalf("global run records payload = %#v, want seeded record", allRecordsPayload)
	}

	listBeforeResp, err := http.Get(ts.URL + "/api/v1/ops-manuals")
	if err != nil {
		t.Fatalf("GET /ops-manuals error = %v", err)
	}
	defer listBeforeResp.Body.Close()
	var listBefore struct {
		Items []opsmanual.OpsManual `json:"items"`
	}
	if err := json.NewDecoder(listBeforeResp.Body).Decode(&listBefore); err != nil {
		t.Fatalf("decode list before: %v", err)
	}
	if len(listBefore.Items) != 1 {
		t.Fatalf("list before confirm = %#v, want only seeded manual", listBefore.Items)
	}

	confirmBody, _ := json.Marshal(appui.OpsManualReviewRequest{Reviewer: "sre"})
	confirmResp, err := http.Post(ts.URL+"/api/v1/ops-manuals/candidates/manual-pg-backup/confirm", "application/json", bytes.NewReader(confirmBody))
	if err != nil {
		t.Fatalf("POST /candidates/{id}/confirm error = %v", err)
	}
	defer confirmResp.Body.Close()
	if confirmResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /candidates/{id}/confirm status = %d, want 200", confirmResp.StatusCode)
	}
	listAfterResp, err := http.Get(ts.URL + "/api/v1/ops-manuals")
	if err != nil {
		t.Fatalf("GET /ops-manuals after confirm error = %v", err)
	}
	defer listAfterResp.Body.Close()
	var listAfter struct {
		Items []opsmanual.OpsManual `json:"items"`
	}
	if err := json.NewDecoder(listAfterResp.Body).Decode(&listAfter); err != nil {
		t.Fatalf("decode list after: %v", err)
	}
	if len(listAfter.Items) != 2 {
		t.Fatalf("list after confirm = %#v, want two manuals", listAfter.Items)
	}
}

func TestOpsManualFlowTimelineAPIAssociatesEvents(t *testing.T) {
	repo := opsmanual.NewMemoryStore()
	if err := repo.SaveManual(serverRedisManual()); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	if err := repo.SaveParamResolutionEvent(opsmanual.ParamResolutionEvent{
		ID:              "param-event-1",
		SessionID:       "sess-1",
		TurnID:          "turn-1",
		OpsManualFlowID: "flow-redis-1",
		ManualID:        "manual-redis-memory",
		WorkflowID:      "workflow-redis-memory",
		Result: opsmanual.ParamResolutionResult{
			Status: opsmanual.ParamResolutionResolved,
			ResolvedParams: []opsmanual.ResolvedParam{{
				ID: "target_instance", Value: "redis-1", Source: "user_form",
			}},
		},
		CreatedAt: "2026-05-19T10:00:00Z",
	}); err != nil {
		t.Fatalf("SaveParamResolutionEvent() error = %v", err)
	}
	if err := repo.SaveRunRecord(opsmanual.RunRecord{
		ID:               "run-record-1",
		SessionID:        "sess-1",
		OpsManualFlowID:  "flow-redis-1",
		ManualID:         "manual-redis-memory",
		WorkflowID:       "workflow-redis-memory",
		WorkflowDigest:   "sha256:abc",
		PreflightStatus:  "passed",
		DryRunStatus:     "passed",
		ExecutionStatus:  "success",
		ValidationStatus: "passed",
		UserFeedback:     "applicable",
		CompletedAt:      "2026-05-19T10:05:00Z",
	}); err != nil {
		t.Fatalf("SaveRunRecord() error = %v", err)
	}
	if err := repo.SaveManualGuidedChatEvent(opsmanual.ManualGuidedChatEvent{
		ID:              "manual-guided-1",
		SessionID:       "sess-1",
		OpsManualFlowID: "flow-redis-1",
		ManualID:        "manual-redis-memory",
		WorkflowID:      "workflow-redis-memory",
		ReferenceMode:   "manual_guided_chat",
		StageSummary:    "只参考手册继续只读排查",
		WorkflowRunID:   "",
		RedactionStatus: "redacted",
		CreatedAt:       "2026-05-19T10:03:00Z",
	}); err != nil {
		t.Fatalf("SaveManualGuidedChatEvent() error = %v", err)
	}
	service := appui.NewOpsManualService(opsmanual.NewService(repo))
	server := NewHTTPServer(&opsManualAPITestServices{service: service}, WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/ops-manuals/flows/flow-redis-1/timeline")
	if err != nil {
		t.Fatalf("GET timeline error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET timeline status = %d, want 200", resp.StatusCode)
	}
	var payload struct {
		Items []opsmanual.FlowTimelineEvent `json:"items"`
		Total int                           `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode timeline payload: %v", err)
	}
	if payload.Total != 9 {
		t.Fatalf("timeline payload = %#v, want nine events", payload)
	}
	wantTypes := map[string]bool{
		"search":                  false,
		"param_resolution":        false,
		"user_form_submit":        false,
		"preflight":               false,
		"dry_run":                 false,
		"execution":               false,
		"verification":            false,
		"user_feedback":           false,
		"manual_guided_reference": false,
	}
	for _, event := range payload.Items {
		if event.OpsManualFlowID != "flow-redis-1" || event.ManualID != "manual-redis-memory" || event.WorkflowID != "workflow-redis-memory" || event.SessionID != "sess-1" {
			t.Fatalf("timeline event = %#v, want shared flow/manual/workflow/session", event)
		}
		if event.RedactionStatus != "redacted" {
			t.Fatalf("timeline event = %#v, want redacted status", event)
		}
		if _, ok := wantTypes[event.Type]; ok {
			wantTypes[event.Type] = true
		}
	}
	for eventType, seen := range wantTypes {
		if !seen {
			t.Fatalf("timeline payload = %#v, missing event type %s", payload, eventType)
		}
	}
}

func TestOpsManualSearchAPIReturnsAdaptForCentOSPostgresBackup(t *testing.T) {
	repo := opsmanual.NewMemoryStore()
	if err := repo.SaveManual(serverPGBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu")); err != nil {
		t.Fatal(err)
	}
	service := appui.NewOpsManualService(opsmanual.NewService(repo))
	server := NewHTTPServer(&opsManualAPITestServices{service: service}, WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	body := bytes.NewReader([]byte(`{"text":"在 CentOS 主机 pg-centos-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常","metadata":{"target_name":"pg-centos-01"}}`))
	resp, err := http.Post(ts.URL+"/api/v1/ops-manuals/search", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var payload opsmanual.SearchOpsManualsResult
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Decision != opsmanual.DecisionAdapt {
		t.Fatalf("decision = %q, want adapt", payload.Decision)
	}
}

func TestOpsManualPreflightAPIReturnsPassed(t *testing.T) {
	repo := opsmanual.NewMemoryStore()
	manual := serverPGBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu")
	manual.RunnableConditions.RequiredParams = []string{"target_instance", "backup_path"}
	manual.PreflightProbe = opsmanual.PreflightProbe{ID: "pg-backup-readonly", ReadOnly: true, RequiredOutputs: []string{"ssh_access", "pg_isready"}}
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := appui.NewOpsManualService(opsmanual.NewService(repo))
	server := NewHTTPServer(&opsManualAPITestServices{service: service}, WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	body, _ := json.Marshal(opsmanual.PreflightRequest{
		ManualID: manual.ID,
		OperationFrame: opsmanual.BuildOperationFrame(
			"在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
			map[string]any{"target_name": "pg-ubuntu-01"},
		),
		Parameters: map[string]any{"target_instance": "pg-ubuntu-01", "backup_path": "/data/backups"},
	})
	resp, err := http.Post(ts.URL+"/api/v1/ops-manuals/preflight", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var payload opsmanual.PreflightResult
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != opsmanual.PreflightStatusPassed || !payload.Ready || payload.NextAction != "start_dry_run" {
		t.Fatalf("payload = %#v, want passed ready start_dry_run", payload)
	}
}

func TestOpsManualResolveParamsAPIReturnsDynamicResult(t *testing.T) {
	repo := opsmanual.NewMemoryStore()
	manual := serverRedisManual()
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := appui.NewOpsManualService(opsmanual.NewService(repo))
	server := NewHTTPServer(&opsManualAPITestServices{service: service}, WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	body := bytes.NewReader([]byte(`{
		"request_text":"排查 Redis",
		"manual_id":"manual-redis-memory",
		"operation_frame":{"object_type":"redis","operation_type":"rca_or_repair"},
		"metadata":{
			"selected_host":"server-local",
			"resource_candidates":[{"id":"docker:aiops-redis","name":"aiops-redis","type":"redis","source":"docker","confidence":0.92}]
		}
	}`))
	resp, err := http.Post(ts.URL+"/api/v1/ops-manuals/resolve-params", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var payload opsmanual.ParamResolutionResult
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != opsmanual.ParamResolutionResolved {
		t.Fatalf("payload = %#v, want resolved", payload)
	}
	for _, field := range payload.Fields {
		if field.ID == "target_location" || field.ID == "execution_surface" || field.ID == "symptom" {
			t.Fatalf("payload returned fixed legacy field %#v", field)
		}
	}
}

func TestOpsManualPreflightAPIRejectsMissingManualID(t *testing.T) {
	service := appui.NewOpsManualService(opsmanual.NewService(opsmanual.NewMemoryStore()))
	server := NewHTTPServer(&opsManualAPITestServices{service: service}, WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/ops-manuals/preflight", "application/json", bytes.NewReader([]byte(`{"parameters":{"target_instance":"pg-01"}}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestOpsManualPreflightAPIReturnsBlockedNotFoundAndNoProbe(t *testing.T) {
	repo := opsmanual.NewMemoryStore()
	manual := serverPGBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu")
	manual.RunnableConditions.RequiredParams = []string{"target_instance", "backup_path"}
	manual.PreflightProbe = opsmanual.PreflightProbe{ID: "pg-backup-readonly", ReadOnly: true, RequiredOutputs: []string{"ssh_access"}}
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	noProbe := serverPGBackupManual("manual-pg-backup-no-probe", "ubuntu", "ssh", "workflow-pg-backup-no-probe")
	noProbe.RunnableConditions.RequiredParams = []string{"target_instance", "backup_path"}
	if err := repo.SaveManual(noProbe); err != nil {
		t.Fatal(err)
	}
	service := appui.NewOpsManualService(opsmanual.NewService(repo))
	server := NewHTTPServer(&opsManualAPITestServices{service: service}, WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()
	frame := opsmanual.BuildOperationFrame(
		"在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
		map[string]any{"target_name": "pg-ubuntu-01"},
	)
	frameMissingBackupPath := opsmanual.BuildOperationFrame(
		"在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，已确认 ssh_access 和 pg_isready 正常",
		map[string]any{"target_name": "pg-ubuntu-01"},
	)

	cases := []struct {
		name       string
		request    opsmanual.PreflightRequest
		wantHTTP   int
		wantStatus opsmanual.PreflightStatus
	}{
		{
			name:       "missing params blocked",
			request:    opsmanual.PreflightRequest{ManualID: manual.ID, OperationFrame: frameMissingBackupPath, Parameters: map[string]any{"target_instance": "pg-ubuntu-01"}},
			wantHTTP:   http.StatusOK,
			wantStatus: opsmanual.PreflightStatusBlocked,
		},
		{
			name:       "manual not found",
			request:    opsmanual.PreflightRequest{ManualID: "missing-manual", OperationFrame: frame, Parameters: map[string]any{"target_instance": "pg-ubuntu-01", "backup_path": "/data/backups"}},
			wantHTTP:   http.StatusBadRequest,
			wantStatus: "",
		},
		{
			name:       "no probe not applicable",
			request:    opsmanual.PreflightRequest{ManualID: noProbe.ID, OperationFrame: frame, Parameters: map[string]any{"target_instance": "pg-ubuntu-01", "backup_path": "/data/backups"}},
			wantHTTP:   http.StatusOK,
			wantStatus: opsmanual.PreflightStatusNotApplicable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.request)
			resp, err := http.Post(ts.URL+"/api/v1/ops-manuals/preflight", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantHTTP {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantHTTP)
			}
			if tc.wantStatus == "" {
				return
			}
			var payload opsmanual.PreflightResult
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Status != tc.wantStatus {
				t.Fatalf("payload = %#v, want status %s", payload, tc.wantStatus)
			}
		})
	}
}

func serverRedisManual() opsmanual.OpsManual {
	return opsmanual.OpsManual{
		ID:          "manual-redis-memory",
		Title:       "Redis memory pressure",
		Status:      opsmanual.ManualStatusVerified,
		WorkflowRef: opsmanual.WorkflowRef{WorkflowID: "workflow-redis-memory"},
		Operation:   opsmanual.OperationProfile{TargetType: "redis", Action: "rca_or_repair"},
		Applicability: opsmanual.ApplicabilityProfile{
			Middleware:       "redis",
			ExecutionSurface: []string{"ssh"},
		},
		RequiredContext: opsmanual.RequiredContext{
			RequiredInputs:   []string{"target_instance"},
			RequiredEvidence: []string{"used_memory_rss", "p95"},
		},
		DocumentMarkdown: "Redis manual",
	}
}

func serverPGBackupManual(id, osName, executionSurface, workflowID string) opsmanual.OpsManual {
	return opsmanual.OpsManual{
		ID:          id,
		Title:       "PostgreSQL 备份 Ubuntu 运维手册",
		Status:      opsmanual.ManualStatusVerified,
		WorkflowRef: opsmanual.WorkflowRef{WorkflowID: workflowID},
		Operation:   opsmanual.OperationProfile{TargetType: "postgresql", Action: "backup", Stateful: true},
		Applicability: opsmanual.ApplicabilityProfile{
			Middleware:       "postgresql",
			OS:               []string{osName},
			ExecutionSurface: []string{executionSurface},
		},
		RequiredContext: opsmanual.RequiredContext{
			RequiredInputs:   []string{"target_instance", "backup_path"},
			RequiredEvidence: []string{"ssh_access", "pg_isready"},
		},
		Preconditions:    []string{"ssh access"},
		Validation:       []string{"pg_isready", "backup file exists"},
		CannotUseWhen:    []string{"目标实例未知"},
		DocumentMarkdown: "PostgreSQL backup manual.",
	}
}
