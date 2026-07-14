package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"aiops-v2/internal/appui"
)

func TestPromptTraceAPIListsAndReadsFiles(t *testing.T) {
	root := t.TempDir()
	traceDir := filepath.Join(root, "sess-api", "turn-api")
	if err := os.MkdirAll(traceDir, 0o755); err != nil {
		t.Fatalf("mkdir trace dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(traceDir, "iteration-000-20260502T000000.000000000Z.json"), []byte(`{
  "createdAt": "2026-05-02T00:00:00Z",
  "sessionId": "sess-api",
  "turnId": "turn-api",
  "caseId": "case-api",
  "modelInput": [{"providerRole": "user"}]
}`), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(traceDir, "iteration-000-20260502T000000.000000000Z.md"), []byte("prompt body"), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}
	server := NewHTTPServer(promptTraceAPIServices{}, WithPromptTraceService(appui.NewPromptTraceService(root)))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/debug/model-input-traces?limit=5&caseId=case-api&trace=sess-api%2Fturn-api%2Fiteration-000-20260502T000000.000000000Z.json")
	if err != nil {
		t.Fatalf("GET list error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET list status = %d, want 200", resp.StatusCode)
	}
	var list appui.PromptTraceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Traces) != 1 || list.Traces[0].SessionID != "sess-api" || list.Traces[0].CaseID != "case-api" || list.SelectedID == "" {
		t.Fatalf("list = %#v", list)
	}

	fileResp, err := http.Get(ts.URL + "/api/v1/debug/model-input-traces/file?path=" + list.Traces[0].MarkdownPath)
	if err != nil {
		t.Fatalf("GET file error = %v", err)
	}
	defer fileResp.Body.Close()
	if fileResp.StatusCode != http.StatusOK {
		t.Fatalf("GET file status = %d, want 200", fileResp.StatusCode)
	}
	var file appui.PromptTraceFileResponse
	if err := json.NewDecoder(fileResp.Body).Decode(&file); err != nil {
		t.Fatalf("decode file: %v", err)
	}
	if file.Content != "prompt body" {
		t.Fatalf("file content = %q", file.Content)
	}
}

func TestPromptTraceAPIForwardsCanonicalControlChainQuery(t *testing.T) {
	service := &recordingPromptTraceService{}
	server := NewHTTPServer(promptTraceAPIServices{}, WithPromptTraceService(service))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/debug/model-input-traces?sessionId=session-api&turnId=turn-api&includeControlChain=true", nil)

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if service.listRequest.SessionID != "session-api" || service.listRequest.TurnID != "turn-api" || !service.listRequest.IncludeControlChain {
		t.Fatalf("forwarded request = %#v", service.listRequest)
	}
}

func TestPromptTraceAPIRejectsInvalidIncludeControlChain(t *testing.T) {
	server := NewHTTPServer(promptTraceAPIServices{}, WithPromptTraceService(&recordingPromptTraceService{}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/debug/model-input-traces?includeControlChain=definitely", nil)

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
}

type recordingPromptTraceService struct {
	listRequest appui.PromptTraceListRequest
}

func (s *recordingPromptTraceService) ListModelInputTraces(_ context.Context, req appui.PromptTraceListRequest) (appui.PromptTraceListResponse, error) {
	s.listRequest = req
	return appui.PromptTraceListResponse{}, nil
}

func (*recordingPromptTraceService) GetModelInputTraceFile(context.Context, appui.PromptTraceFileRequest) (appui.PromptTraceFileResponse, error) {
	return appui.PromptTraceFileResponse{}, nil
}

type promptTraceAPIServices struct{}

func (promptTraceAPIServices) ChatService() appui.ChatService                 { return nil }
func (promptTraceAPIServices) StateService() appui.StateService               { return promptTraceStateService{} }
func (promptTraceAPIServices) SessionService() appui.SessionService           { return nil }
func (promptTraceAPIServices) ApprovalService() appui.ApprovalService         { return nil }
func (promptTraceAPIServices) ChoiceService() appui.ChoiceService             { return nil }
func (promptTraceAPIServices) SettingsService() appui.SettingsService         { return nil }
func (promptTraceAPIServices) HostService() appui.HostService                 { return nil }
func (promptTraceAPIServices) MCPService() appui.MCPService                   { return nil }
func (promptTraceAPIServices) AgentProfileService() appui.AgentProfileService { return nil }
func (promptTraceAPIServices) AuthService() appui.AuthService                 { return nil }
func (promptTraceAPIServices) TerminalService() appui.TerminalService         { return nil }

type promptTraceStateService struct{}

func (promptTraceStateService) GetState(context.Context) (appui.StateSnapshot, error) {
	return appui.StateSnapshot{}, nil
}
