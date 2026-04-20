package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/server"
)

// ---------------------------------------------------------------------------
// API Compatibility Integration Tests
// Validates: Requirements 6.1, 6.4
// ---------------------------------------------------------------------------

// mockKernel implements server.RuntimeKernelAPI for integration testing.
type mockKernel struct {
	runTurnFn    func(ctx context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error)
	resumeTurnFn func(ctx context.Context, req runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error)
	cancelTurnFn func(ctx context.Context, req runtimekernel.CancelRequest) (runtimekernel.TurnResult, error)
}

func (m *mockKernel) RunTurn(ctx context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	if m.runTurnFn != nil {
		return m.runTurnFn(ctx, req)
	}
	return runtimekernel.TurnResult{
		SessionType: req.SessionType,
		Mode:        req.Mode,
		SessionID:   "sess-001",
		TurnID:      req.TurnID,
		Status:      "completed",
		Output:      "mock response",
	}, nil
}

func (m *mockKernel) ResumeTurn(ctx context.Context, req runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	if m.resumeTurnFn != nil {
		return m.resumeTurnFn(ctx, req)
	}
	return runtimekernel.TurnResult{
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		Status:      "completed",
		Output:      "resumed",
	}, nil
}

func (m *mockKernel) CancelTurn(ctx context.Context, req runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	if m.cancelTurnFn != nil {
		return m.cancelTurnFn(ctx, req)
	}
	return runtimekernel.TurnResult{
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		Status:      "cancelled",
	}, nil
}

// TestChatMessageEndpoint verifies POST /api/v1/chat/message JSON structure.
func TestChatMessageEndpoint(t *testing.T) {
	kernel := &mockKernel{}
	srv := server.NewHTTPServer(kernel)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	tests := []struct {
		name           string
		body           server.ChatMessageRequest
		wantStatus     int
		wantSessionID  bool
		wantTurnID     bool
		wantOutputKey  bool
	}{
		{
			name: "host chat message",
			body: server.ChatMessageRequest{
				SessionType: "host",
				Mode:        "chat",
				Content:     "hello",
				HostID:      "host-1",
			},
			wantStatus:    http.StatusOK,
			wantSessionID: true,
			wantTurnID:    true,
			wantOutputKey: true,
		},
		{
			name: "workspace execute message",
			body: server.ChatMessageRequest{
				SessionType: "workspace",
				Mode:        "execute",
				Content:     "check all servers",
			},
			wantStatus:    http.StatusOK,
			wantSessionID: true,
			wantTurnID:    true,
			wantOutputKey: true,
		},
		{
			name: "message with metadata",
			body: server.ChatMessageRequest{
				SessionType: "host",
				Mode:        "inspect",
				Content:     "show disk usage",
				HostID:      "host-2",
				Metadata:    map[string]string{"source": "ui"},
			},
			wantStatus:    http.StatusOK,
			wantSessionID: true,
			wantTurnID:    true,
			wantOutputKey: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			resp, err := http.Post(ts.URL+"/api/v1/chat/message", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			// Verify Content-Type
			ct := resp.Header.Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			// Decode response and verify JSON structure
			var result server.ChatMessageResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if tt.wantSessionID && result.SessionID == "" {
				t.Error("expected non-empty sessionId in response")
			}
			if tt.wantTurnID && result.TurnID == "" {
				t.Error("expected non-empty turnId in response")
			}
			if result.Status == "" {
				t.Error("expected non-empty status in response")
			}
		})
	}
}

// TestChatMessageMethodNotAllowed verifies only POST is accepted.
func TestChatMessageMethodNotAllowed(t *testing.T) {
	kernel := &mockKernel{}
	srv := server.NewHTTPServer(kernel)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req, _ := http.NewRequest(method, ts.URL+"/api/v1/chat/message", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
			}
		})
	}
}

// TestChatMessageInvalidBody verifies error response for malformed JSON.
func TestChatMessageInvalidBody(t *testing.T) {
	kernel := &mockKernel{}
	srv := server.NewHTTPServer(kernel)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/chat/message", "application/json", bytes.NewReader([]byte("not json")))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var errResp map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp["error"] == "" {
		t.Error("expected error field in response")
	}
}

// TestStateEndpoint verifies GET /api/v1/state JSON structure.
func TestStateEndpoint(t *testing.T) {
	kernel := &mockKernel{}
	srv := server.NewHTTPServer(kernel)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/state")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var state map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("failed to decode state: %v", err)
	}

	// Verify expected keys exist
	if _, ok := state["sessions"]; !ok {
		t.Error("expected 'sessions' key in state response")
	}
	if _, ok := state["status"]; !ok {
		t.Error("expected 'status' key in state response")
	}
}

// TestResumeTurnEndpoint verifies POST /api/v1/turn/resume JSON structure.
func TestResumeTurnEndpoint(t *testing.T) {
	kernel := &mockKernel{}
	srv := server.NewHTTPServer(kernel)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := runtimekernel.ResumeRequest{
		SessionID:  "sess-001",
		TurnID:     "turn-001",
		ApprovalID: "appr-001",
		Decision:   "approved",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(ts.URL+"/api/v1/turn/resume", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result server.ChatMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.SessionID != "sess-001" {
		t.Errorf("sessionId = %q, want %q", result.SessionID, "sess-001")
	}
	if result.TurnID != "turn-001" {
		t.Errorf("turnId = %q, want %q", result.TurnID, "turn-001")
	}
}

// TestCancelTurnEndpoint verifies POST /api/v1/turn/cancel JSON structure.
func TestCancelTurnEndpoint(t *testing.T) {
	kernel := &mockKernel{}
	srv := server.NewHTTPServer(kernel)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := runtimekernel.CancelRequest{
		SessionID: "sess-001",
		TurnID:    "turn-001",
		Reason:    "user cancelled",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(ts.URL+"/api/v1/turn/cancel", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result server.ChatMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Status != "cancelled" {
		t.Errorf("status = %q, want %q", result.Status, "cancelled")
	}
}

// TestChatToEinoMessageConversion verifies the ChatMessageRequest → EinoMessage conversion (Req 6.4).
func TestChatToEinoMessageConversion(t *testing.T) {
	tests := []struct {
		name     string
		input    server.ChatMessageRequest
		wantRole string
	}{
		{
			name: "default role is user",
			input: server.ChatMessageRequest{
				Content: "hello",
			},
			wantRole: "user",
		},
		{
			name: "explicit role preserved",
			input: server.ChatMessageRequest{
				Role:    "system",
				Content: "you are helpful",
			},
			wantRole: "system",
		},
		{
			name: "metadata preserved",
			input: server.ChatMessageRequest{
				Content:  "test",
				Metadata: map[string]string{"key": "value"},
			},
			wantRole: "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := server.ConvertChatToEinoMessage(tt.input)

			if msg.Role != tt.wantRole {
				t.Errorf("role = %q, want %q", msg.Role, tt.wantRole)
			}
			if msg.Content != tt.input.Content {
				t.Errorf("content = %q, want %q", msg.Content, tt.input.Content)
			}
			if tt.input.Metadata != nil {
				if msg.Metadata == nil {
					t.Error("expected metadata to be preserved")
				} else {
					for k, v := range tt.input.Metadata {
						if msg.Metadata[k] != v {
							t.Errorf("metadata[%q] = %q, want %q", k, msg.Metadata[k], v)
						}
					}
				}
			}
		})
	}
}

// TestResourceEndpoints verifies resource management API endpoints (Req 6.5, 6.6, 6.7).
func TestResourceEndpoints(t *testing.T) {
	rs := server.NewResourceServer()
	ts := httptest.NewServer(rs.Handler())
	defer ts.Close()

	endpoints := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"list audits", http.MethodGet, "/api/v1/approval-audits", http.StatusOK},
		{"list grants", http.MethodGet, "/api/v1/approval-grants", http.StatusOK},
		{"create grant", http.MethodPost, "/api/v1/approval-grants", http.StatusCreated},
		{"list bindings", http.MethodGet, "/api/v1/capability-bindings", http.StatusOK},
		{"list cards", http.MethodGet, "/api/v1/ui-cards", http.StatusOK},
		{"list configs", http.MethodGet, "/api/v1/script-configs", http.StatusOK},
		{"list environments", http.MethodGet, "/api/v1/lab-environments", http.StatusOK},
		{"coroot proxy", http.MethodGet, "/api/v1/coroot/services", http.StatusOK},
		{"generator status", http.MethodGet, "/api/v1/generator/", http.StatusOK},
		{"generator generate", http.MethodPost, "/api/v1/generator/generate", http.StatusOK},
		{"generator lint", http.MethodPost, "/api/v1/generator/lint", http.StatusOK},
		{"generator preview", http.MethodPost, "/api/v1/generator/preview", http.StatusOK},
		{"generator publish", http.MethodPost, "/api/v1/generator/publish-draft", http.StatusOK},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ts.URL+ep.path, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != ep.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, ep.want)
			}

			// All responses should be JSON
			ct := resp.Header.Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
		})
	}
}
