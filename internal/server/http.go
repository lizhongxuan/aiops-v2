// Package server provides transport handlers for the first-party Web API.
package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/terminal"
)

// ---------------------------------------------------------------------------
// ChatMessageRequest is the JSON body of POST /api/v1/chat/message.
// It accepts both the first-party "content" field and the Web data-plane
// "message" alias while routing both through appui.ChatService.
// ---------------------------------------------------------------------------

// ChatMessageRequest represents the incoming chat message from the frontend.
type ChatMessageRequest struct {
	SessionID       string            `json:"sessionId,omitempty"`
	SessionType     string            `json:"sessionType,omitempty"` // host, workspace
	Mode            string            `json:"mode,omitempty"`        // chat, inspect, plan, execute
	Content         string            `json:"content"`
	Message         string            `json:"message,omitempty"`
	Role            string            `json:"role,omitempty"` // defaults to "user"
	HostID          string            `json:"hostId,omitempty"`
	ClientMessageID string            `json:"clientMessageId,omitempty"`
	ClientTurnID    string            `json:"clientTurnId,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// ChatMessageResponse is the JSON response for POST /api/v1/chat/message.
type ChatMessageResponse struct {
	Accepted        bool   `json:"accepted"`
	SessionID       string `json:"sessionId"`
	TurnID          string `json:"turnId"`
	ClientTurnID    string `json:"clientTurnId,omitempty"`
	ClientMessageID string `json:"clientMessageId,omitempty"`
	Status          string `json:"status"`
	Output          string `json:"output,omitempty"`
	Error           string `json:"error,omitempty"`
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
		Content:  chatRequestContent(req),
		Metadata: req.Metadata,
	}
}

func chatRequestContent(req ChatMessageRequest) string {
	if req.Content != "" {
		return req.Content
	}
	return req.Message
}

// ---------------------------------------------------------------------------
// HTTPServer holds the HTTP handler dependencies.
// ---------------------------------------------------------------------------

// HTTPServer provides the first-party HTTP REST API and WebSocket entrypoints.
type HTTPServer struct {
	ui                      appui.HTTPServices
	mux                     *http.ServeMux
	web                     http.Handler
	runnerStudioHandler     http.Handler
	runnerStudioUpstreamURL string
	terminalManager         *terminal.Manager
	appWSHeartbeatTick      time.Duration
	appSnapshots            *AppSnapshotBroadcaster
	agentEvents             appui.AgentEventService
	promptTraces            appui.PromptTraceService
}

// HTTPServerOption customizes transport-only HTTP server behavior.
type HTTPServerOption func(*HTTPServer)

// WithWebAssets mounts a static/SPA handler on "/" after API routes are
// registered. More specific API and websocket routes continue to win.
func WithWebAssets(handler http.Handler) HTTPServerOption {
	return func(s *HTTPServer) {
		s.web = handler
	}
}

// WithAppWebSocketHeartbeat customizes the heartbeat interval for the main /ws
// channel. It is primarily used by tests.
func WithAppWebSocketHeartbeat(interval time.Duration) HTTPServerOption {
	return func(s *HTTPServer) {
		s.appWSHeartbeatTick = interval
	}
}

// WithTerminalManager overrides the terminal session manager used by the
// dedicated terminal domain endpoints.
func WithTerminalManager(manager *terminal.Manager) HTTPServerOption {
	return func(s *HTTPServer) {
		s.terminalManager = manager
	}
}

func WithPromptTraceService(service appui.PromptTraceService) HTTPServerOption {
	return func(s *HTTPServer) {
		s.promptTraces = service
	}
}

// WithRunnerStudioUpstreamURL configures the server-side runner API upstream
// used by same-origin /api/runner-studio/* aggregation routes.
func WithRunnerStudioUpstreamURL(rawURL string) HTTPServerOption {
	return func(s *HTTPServer) {
		s.runnerStudioUpstreamURL = strings.TrimSpace(rawURL)
	}
}

// WithRunnerStudioHandler mounts an embedded Runner API handler for same-origin
// /api/runner-studio/* routes. It takes precedence over the legacy upstream URL.
func WithRunnerStudioHandler(handler http.Handler) HTTPServerOption {
	return func(s *HTTPServer) {
		s.runnerStudioHandler = handler
	}
}

// NewHTTPServer creates a new HTTPServer wired to the given application services.
func NewHTTPServer(ui appui.HTTPServices, opts ...HTTPServerOption) *HTTPServer {
	agentEvents := appui.NewAgentEventService(nil)
	if provider, ok := ui.(interface {
		AgentEventService() appui.AgentEventService
	}); ok {
		if provided := provider.AgentEventService(); provided != nil {
			agentEvents = provided
		}
	}
	s := &HTTPServer{
		ui:                 ui,
		mux:                http.NewServeMux(),
		terminalManager:    terminal.NewManager(),
		appWSHeartbeatTick: 15 * time.Second,
		agentEvents:        agentEvents,
		promptTraces:       appui.NewPromptTraceService(""),
		appSnapshots:       NewAppSnapshotBroadcaster(ui.StateService(), agentEvents),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
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
	s.mux.HandleFunc("/api/v1/chat/stop", s.handleChatStop)
	s.mux.HandleFunc("/api/v1/assistant/transport", s.handleAssistantTransport)
	s.mux.HandleFunc("/api/v1/assistant/resume", s.handleAssistantTransportResume)

	// State endpoint
	s.mux.HandleFunc("/api/v1/state", s.handleGetState)
	s.mux.HandleFunc("/api/v1/host/select", s.handleSelectHost)
	s.mux.HandleFunc("/api/v1/sessions", s.handleSessions)
	s.mux.HandleFunc("/api/v1/sessions/", s.handleSessions)
	s.mux.HandleFunc("/api/v1/settings", s.handleSettings)
	s.mux.HandleFunc("/api/v1/llm-config", s.handleLLMConfig)
	s.mux.HandleFunc("/api/v1/debug/model-input-traces", s.handlePromptTraces)
	s.mux.HandleFunc("/api/v1/debug/model-input-traces/file", s.handlePromptTraceFile)
	s.mux.HandleFunc("/api/v1/hosts", s.handleHosts)
	s.mux.HandleFunc("/api/v1/hosts/", s.handleHosts)
	s.mux.HandleFunc("/api/v1/mcp/servers", s.handleMCPServers)
	s.mux.HandleFunc("/api/v1/mcp/servers/", s.handleMCPServers)
	s.mux.HandleFunc("/api/v1/mcp/servers/refresh", s.handleMCPServersRefresh)
	s.registerAgentProfileRoutes()
	s.mux.Handle("/api/v1/auth/", buildAuthRouter(s.ui, http.NotFoundHandler()))
	s.mux.HandleFunc("/api/v1/terminal/sessions", s.handleTerminalSessions)
	s.mux.Handle("/api/v1/terminal/ws", s.handleTerminalWebSocket())
	s.mux.HandleFunc("/api/v1/incidents", s.handleIncidents)
	s.mux.HandleFunc("/api/v1/incidents/", s.handleIncidents)
	s.mux.HandleFunc("/api/v1/experience-packs", s.handleExperiencePacks)
	s.mux.HandleFunc("/api/v1/experience-packs/", s.handleExperiencePacks)
	s.mux.HandleFunc("/api/v1/coroot/webhook", s.handleCorootWebhook)
	s.mux.HandleFunc("/api/v1/opsgraph/lookup", s.handleOpsGraphLookup)
	s.mux.HandleFunc("/api/v1/opsgraph/entities/", s.handleOpsGraphEntity)
	s.mux.HandleFunc("/api/v1/runbooks", s.handleRunbooks)
	s.mux.HandleFunc("/api/v1/runbooks/", s.handleRunbooks)
	s.mux.HandleFunc("/api/v1/erp/", s.handleERPContext)
	s.mux.HandleFunc("/api/v1/changes/", s.handleChanges)
	s.mux.HandleFunc("/api/runner-studio/ai/", s.handleRunnerStudioAI)
	s.mux.HandleFunc("/api/runner-studio/", s.handleRunnerStudio)

	NewResourceServer().RegisterOnMux(s.mux)

	// Approval endpoints
	s.mux.HandleFunc("/api/v1/approvals", s.handleApprovals)
	s.mux.HandleFunc("/api/v1/approvals/", s.handleApprovals)
	s.mux.HandleFunc("/api/v1/choices/", s.handleChoices)

	// Turn management
	s.mux.HandleFunc("/api/v1/turn/resume", s.handleResumeTurn)
	s.mux.HandleFunc("/api/v1/turn/cancel", s.handleCancelTurn)
	s.mux.Handle("/ws", s.appWebSocketHandler())

	if s.web != nil {
		s.mux.Handle("/", s.web)
	}
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

	result, err := s.ui.ChatService().SendMessage(r.Context(), appui.ChatCommand{
		SessionID:       req.SessionID,
		SessionType:     req.SessionType,
		Mode:            req.Mode,
		Content:         chatRequestContent(req),
		Role:            req.Role,
		HostID:          req.HostID,
		ClientMessageID: req.ClientMessageID,
		ClientTurnID:    req.ClientTurnID,
		Metadata:        req.Metadata,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	resp := ChatMessageResponse{
		Accepted:        result.Error == "",
		SessionID:       result.SessionID,
		TurnID:          result.TurnID,
		ClientTurnID:    result.ClientTurnID,
		ClientMessageID: result.ClientMessageID,
		Status:          result.Status,
		Output:          result.Output,
		Error:           result.Error,
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
	stateSvc := s.ui.StateService()
	if stateSvc == nil {
		writeJSON(w, http.StatusOK, appui.NewSnapshotBuilder().BuildStateSnapshot(nil))
		return
	}
	state, err := stateSvc.GetState(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s.withAgentEventProjection(r.Context(), state))
}

func (s *HTTPServer) withAgentEventProjection(ctx context.Context, state appui.StateSnapshot) appui.StateSnapshot {
	if s == nil || s.agentEvents == nil || strings.TrimSpace(state.SessionID) == "" {
		return state
	}
	projection, err := s.agentEvents.Projection(ctx, state.SessionID)
	if err != nil {
		return state
	}
	projection = appui.SanitizeAgentEventProjectionForSnapshot(projection, state)
	state.AgentEventProjection = &projection
	return state
}

// ---------------------------------------------------------------------------
// Handler: /api/v1/approvals/{id}
// ---------------------------------------------------------------------------

func (s *HTTPServer) handleApprovals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		approvals, err := s.ui.ApprovalService().List(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"approvals": approvals})
	case http.MethodPost:
		var req struct {
			Decision string `json:"decision"`
		}
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
		}
		approvalID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/approvals/"), "/")
		approvalID = strings.TrimSuffix(approvalID, "/decision")
		approvalID = strings.Trim(approvalID, "/")
		result, err := s.ui.ApprovalService().Decide(r.Context(), appui.ApprovalDecision{
			ID:       approvalID,
			Decision: req.Decision,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
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

	var req appui.ResumeCommand
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	result, err := s.ui.ChatService().ResumeTurn(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ChatMessageResponse{
		Accepted:        result.Error == "",
		SessionID:       result.SessionID,
		TurnID:          result.TurnID,
		ClientTurnID:    result.ClientTurnID,
		ClientMessageID: result.ClientMessageID,
		Status:          result.Status,
		Output:          result.Output,
		Error:           result.Error,
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

	var req appui.CancelCommand
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	result, err := s.ui.ChatService().CancelTurn(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ChatMessageResponse{
		Accepted:        result.Error == "",
		SessionID:       result.SessionID,
		TurnID:          result.TurnID,
		ClientTurnID:    result.ClientTurnID,
		ClientMessageID: result.ClientMessageID,
		Status:          result.Status,
		Output:          result.Output,
		Error:           result.Error,
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
