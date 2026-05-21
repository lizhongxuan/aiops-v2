package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/opsmanual"
)

func TestOpsManualGenerateFromWorkflowUsesRunnerStudioGraphAndCatalog(t *testing.T) {
	workflowYAML := readOpsManualWorkflowFixture(t, "pg_restore.yaml")
	runner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workflows/pg-restore/graph":
			writeJSON(w, http.StatusOK, map[string]any{"graph": map[string]any{"nodes": []any{}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/workflows/graph/compile":
			writeJSON(w, http.StatusOK, map[string]any{"yaml": string(workflowYAML)})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/actions/catalog":
			writeJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{{
				"action":        "script.shell",
				"title":         "Shell",
				"risk":          "high",
				"required_args": []string{"script"},
				"outputs":       []string{"stdout"},
			}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs":
			writeJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{{
				"id":                "run-ok",
				"workflow_id":       "pg-restore",
				"execution_status":  "success",
				"validation_status": "success",
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer runner.Close()

	repo := opsmanual.NewMemoryStore()
	service := appui.NewOpsManualService(opsmanual.NewService(repo))
	server := NewHTTPServer(&opsManualAPITestServices{service: service}, WithRunnerStudioUpstreamURL(runner.URL), WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	body, _ := json.Marshal(appui.OpsManualGenerateFromWorkflowRequest{WorkflowID: "pg-restore"})
	resp, err := http.Post(ts.URL+"/api/v1/ops-manuals/candidates/generate-from-workflow", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST generate-from-workflow error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST generate-from-workflow status = %d, want 200", resp.StatusCode)
	}
	var payload appui.OpsManualGenerateFromWorkflowResult
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Candidate.SourceType != "workflow_reverse_generated" || payload.ValidationReport.Status == "" || len(payload.UserSummary.Understood) == 0 {
		t.Fatalf("payload = %#v, want generated candidate, validation report and user summary", payload)
	}
	if payload.Candidate.ProposedManual.Operation.TargetType != "postgresql" || payload.Candidate.ProposedManual.Operation.Action != "restore" {
		t.Fatalf("operation = %#v, want postgresql restore", payload.Candidate.ProposedManual.Operation)
	}
	candidates, err := repo.ListCandidates()
	if err != nil {
		t.Fatalf("ListCandidates() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != payload.Candidate.ID {
		t.Fatalf("candidates = %#v, want generated candidate persisted", candidates)
	}
}

func TestOpsManualGenerateFromWorkflowRejectsMissingWorkflowID(t *testing.T) {
	repo := opsmanual.NewMemoryStore()
	service := appui.NewOpsManualService(opsmanual.NewService(repo))
	server := NewHTTPServer(&opsManualAPITestServices{service: service}, WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/ops-manuals/candidates/generate-from-workflow", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("POST generate-from-workflow error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var payload map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if payload["error_code"] != opsManualErrInvalidGenerationReq {
		t.Fatalf("payload = %#v, want invalid generation request error", payload)
	}
}

func TestOpsManualGenerateFromWorkflowHandlesRunnerUnavailable(t *testing.T) {
	repo := opsmanual.NewMemoryStore()
	service := appui.NewOpsManualService(opsmanual.NewService(repo))
	server := NewHTTPServer(&opsManualAPITestServices{service: service}, WithWebAssets(http.NotFoundHandler()))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	body, _ := json.Marshal(appui.OpsManualGenerateFromWorkflowRequest{WorkflowID: "pg-restore"})
	resp, err := http.Post(ts.URL+"/api/v1/ops-manuals/candidates/generate-from-workflow", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST generate-from-workflow error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	var payload map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if payload["error"] != "runner studio upstream is not configured" {
		t.Fatalf("payload = %#v, want runner unavailable error", payload)
	}
}

func readOpsManualWorkflowFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "opsmanual", "testdata", "workflow_reverse", "real", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}
