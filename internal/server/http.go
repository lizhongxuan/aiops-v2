// Package server provides HTTP/WebSocket/gRPC API compatibility layer.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"aiops-v2/internal/runtimekernel"
)

// ---------------------------------------------------------------------------
// ChatMessageRequest is the JSON body of POST /api/v1/chat/message.
// It mirrors the existing frontend protocol.
// ---------------------------------------------------------------------------

// ChatMessageRequest represents the incoming chat message from the frontend.
type ChatMessageRequest struct {
	SessionID   string            `json:"sessionId,omitempty"`
	SessionType string            `json:"sessionType,omitempty"` // host, workspace
	Mode        string            `json:"mode,omitempty"`        // chat, inspect, plan, execute
	Content     string            `json:"content"`
	Role        string            `json:"role,omitempty"` // defaults to "user"
	HostID      string            `json:"hostId,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ChatMessageResponse is the JSON response for POST /api/v1/chat/message.
type ChatMessageResponse struct {
	SessionID string `json:"sessionId"`
	TurnID    string `json:"turnId"`
	Status    string `json:"status"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// EinoMessage represents the Eino-compatible message format used internally.
// ---------------------------------------------------------------------------

// EinoMessage is the internal Eino-compatible message representation.
type EinoMessage struct {
	Role     string            `json:"role"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ---------------------------------------------------------------------------
// ConvertChatToEinoMessage converts a ChatMessageRequest to an EinoMessage.
// This implements the /api/v1/chat/message → Eino Message conversion (Req 6.4).
// ---------------------------------------------------------------------------

// ConvertChatToEinoMessage converts a frontend ChatMessageRequest into the
// internal Eino message format, preserving role, content, and metadata.
func ConvertChatToEinoMessage(req ChatMessageRequest) EinoMessage {
	role := req.Role
	if role == "" {
		role = "user"
	}
	return EinoMessage{
		Role:     role,
		Content:  req.Content,
		Metadata: req.Metadata,
	}
}

// ---------------------------------------------------------------------------
// RuntimeKernelAPI is the interface the HTTP handler uses to invoke the kernel.
// ---------------------------------------------------------------------------

// RuntimeKernelAPI abstracts the RuntimeKernel for the HTTP layer.
type RuntimeKernelAPI interface {
	RunTurn(ctx context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error)
	ResumeTurn(ctx context.Context, req runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error)
	CancelTurn(ctx context.Context, req runtimekernel.CancelRequest) (runtimekernel.TurnResult, error)
}

// ---------------------------------------------------------------------------
// HTTPServer holds the HTTP handler dependencies.
// ---------------------------------------------------------------------------

// HTTPServer provides the HTTP REST API compatibility layer.
type HTTPServer struct {
	kernel RuntimeKernelAPI
	mux    *http.ServeMux
}

// NewHTTPServer creates a new HTTPServer wired to the given RuntimeKernel.
func NewHTTPServer(kernel RuntimeKernelAPI) *HTTPServer {
	s := &HTTPServer{
		kernel: kernel,
		mux:    http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Handler returns the http.Handler for this server.
func (s *HTTPServer) Handler() http.Handler {
	return s.mux
}

// ---------------------------------------------------------------------------
// Route registration — keeps all endpoint paths/methods/JSON unchanged.
// ---------------------------------------------------------------------------

func (s *HTTPServer) registerRoutes() {
	// Core chat endpoint
	s.mux.HandleFunc("/api/v1/chat/message", s.handleChatMessage)

	// State endpoint
	s.mux.HandleFunc("/api/v1/state", s.handleGetState)

	// Approval endpoints
	s.mux.HandleFunc("/api/v1/approvals/", s.handleApprovals)

	// Turn management
	s.mux.HandleFunc("/api/v1/turn/resume", s.handleResumeTurn)
	s.mux.HandleFunc("/api/v1/turn/cancel", s.handleCancelTurn)
}

// ---------------------------------------------------------------------------
// Handler: POST /api/v1/chat/message
// Converts frontend message to Eino format and invokes RuntimeKernel.
// ---------------------------------------------------------------------------

func (s *HTTPServer) handleChatMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Convert to Eino message format (Req 6.4)
	_ = ConvertChatToEinoMessage(req)

	// Map to TurnRequest for RuntimeKernel
	sessionType := runtimekernel.SessionTypeHost
	if req.SessionType == "workspace" {
		sessionType = runtimekernel.SessionTypeWorkspace
	}

	mode := runtimekernel.ModeChat
	switch req.Mode {
	case "inspect":
		mode = runtimekernel.ModeInspect
	case "plan":
		mode = runtimekernel.ModePlan
	case "execute":
		mode = runtimekernel.ModeExecute
	}

	turnReq := runtimekernel.TurnRequest{
		SessionType: sessionType,
		Mode:        mode,
		SessionID:   req.SessionID,
		TurnID:      fmt.Sprintf("turn-%d", time.Now().UnixNano()),
		Input:       req.Content,
		HostID:      req.HostID,
		Metadata:    req.Metadata,
	}

	result, err := s.kernel.RunTurn(r.Context(), turnReq)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	resp := ChatMessageResponse{
		SessionID: result.SessionID,
		TurnID:    result.TurnID,
		Status:    result.Status,
		Output:    result.Output,
		Error:     result.Error,
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Handler: GET /api/v1/state
// ---------------------------------------------------------------------------

func (s *HTTPServer) handleGetState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Stub: returns empty state object (real implementation reads from store)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessions": []interface{}{},
		"status":   "ok",
	})
}

// ---------------------------------------------------------------------------
// Handler: /api/v1/approvals/{id}
// ---------------------------------------------------------------------------

func (s *HTTPServer) handleApprovals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Stub: list or get approval
		writeJSON(w, http.StatusOK, map[string]interface{}{"approvals": []interface{}{}})
	case http.MethodPost:
		// Stub: create/decide approval
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// Handler: POST /api/v1/turn/resume
// ---------------------------------------------------------------------------

func (s *HTTPServer) handleResumeTurn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req runtimekernel.ResumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	result, err := s.kernel.ResumeTurn(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ChatMessageResponse{
		SessionID: result.SessionID,
		TurnID:    result.TurnID,
		Status:    result.Status,
		Output:    result.Output,
		Error:     result.Error,
	})
}

// ---------------------------------------------------------------------------
// Handler: POST /api/v1/turn/cancel
// ---------------------------------------------------------------------------

func (s *HTTPServer) handleCancelTurn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req runtimekernel.CancelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	result, err := s.kernel.CancelTurn(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ChatMessageResponse{
		SessionID: result.SessionID,
		TurnID:    result.TurnID,
		Status:    result.Status,
		Output:    result.Output,
		Error:     result.Error,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
