package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/hostops"
	"aiops-v2/internal/runtimekernel"
)

const hostOpsChildAgentsPrefix = "/api/v1/host-ops/child-agents/"
const hostOpsMissionsPath = "/api/v1/host-ops/missions"
const hostOpsMissionsPrefix = "/api/v1/host-ops/missions/"

type hostOpsChildMessageRequest struct {
	Content string `json:"content"`
}

type hostOpsPlanReviseRequest struct {
	Instruction string `json:"instruction"`
}

func (s *HTTPServer) handleHostOpsMissions(w http.ResponseWriter, r *http.Request) {
	service := s.hostOpsService()
	if service == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "host ops service is not available"})
		return
	}
	if r.URL.Path == hostOpsMissionsPath {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var req appui.HostMissionCreateCommand
		if err := decodeHostOpsJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		view, err := service.CreateMission(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, view)
		return
	}

	path := strings.Trim(strings.TrimPrefix(r.URL.Path, hostOpsMissionsPrefix), "/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missionId is required"})
		return
	}
	parts := strings.Split(path, "/")
	missionID := strings.TrimSpace(parts[0])
	if missionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missionId is required"})
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		view, err := service.GetMission(r.Context(), missionID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, view)
		return
	}
	if len(parts) == 3 && parts[1] == "plans" && parts[2] == "revise" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var req hostOpsPlanReviseRequest
		if err := decodeHostOpsJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		view, err := service.RevisePlan(r.Context(), missionID, req.Instruction)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, view)
		return
	}
	if len(parts) == 4 && parts[1] == "plans" && parts[3] == "accept" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		planID := strings.TrimSpace(parts[2])
		if planID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "planId is required"})
			return
		}
		view, err := service.AcceptPlan(r.Context(), missionID, planID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, view)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "host ops endpoint not found"})
}

func (s *HTTPServer) handleHostOpsChildAgents(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, hostOpsChildAgentsPrefix), "/")
	parts := strings.Split(path, "/")
	childAgentID := ""
	if len(parts) > 0 {
		childAgentID = strings.TrimSpace(parts[0])
	}
	if childAgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "childAgentId is required"})
		return
	}
	if len(parts) < 2 {
		switch childAgentID {
		case "transcript", "messages", "stop":
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "childAgentId is required"})
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "host ops endpoint not found"})
		}
		return
	}
	service := s.hostOpsService()
	if service == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "host ops service is not available"})
		return
	}
	if len(parts) > 2 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "host ops endpoint not found"})
		return
	}
	switch parts[1] {
	case "transcript":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		transcript, err := service.ChildTranscript(r.Context(), childAgentID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		if transcript.ChildAgentID == "" {
			transcript.ChildAgentID = childAgentID
		}
		transcript = enrichHostOpsTranscriptWithRuntimeSession(transcript, childAgentID, s.assistantTransportSessionSource())
		writeJSON(w, http.StatusOK, transcript)
	case "messages":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var req hostOpsChildMessageRequest
		if err := decodeHostOpsJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		view, err := service.SendChildMessage(r.Context(), childAgentID, req.Content)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, view)
	case "stop":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		view, err := service.StopChildAgent(r.Context(), childAgentID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, view)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "host ops endpoint not found"})
	}
}

func (s *HTTPServer) hostOpsService() appui.HostOpsService {
	if s == nil || s.ui == nil {
		return nil
	}
	provider, ok := s.ui.(interface {
		HostOpsService() appui.HostOpsService
	})
	if !ok {
		return nil
	}
	return provider.HostOpsService()
}

func decodeHostOpsJSON(r *http.Request, target any) error {
	if r == nil || r.Body == nil {
		return io.EOF
	}
	err := json.NewDecoder(r.Body).Decode(target)
	if err == io.EOF {
		return nil
	}
	return err
}

func enrichHostOpsTranscriptWithRuntimeSession(
	transcript appui.HostChildTranscriptView,
	childAgentID string,
	source appui.SessionSource,
) appui.HostChildTranscriptView {
	if source == nil {
		return transcript
	}
	session := findHostOpsRuntimeSessionForChild(source, firstNonEmptyHostOpsString(childAgentID, transcript.ChildAgentID))
	if session == nil {
		return transcript
	}
	seen := make(map[string]struct{}, len(transcript.Items))
	for _, item := range transcript.Items {
		if item.ID != "" {
			seen[item.ID] = struct{}{}
		}
	}
	for _, turn := range assistantTransportSessionTurns(session) {
		for _, item := range hostOpsRuntimeTranscriptItems(turn) {
			if item.ID != "" {
				if _, ok := seen[item.ID]; ok {
					continue
				}
				seen[item.ID] = struct{}{}
			}
			transcript.Items = append(transcript.Items, item)
		}
	}
	return transcript
}

func findHostOpsRuntimeSessionForChild(source appui.SessionSource, childAgentID string) *runtimekernel.SessionState {
	childAgentID = strings.TrimSpace(childAgentID)
	if childAgentID == "" || source == nil {
		return nil
	}
	sanitizedTarget := strings.TrimPrefix(childAgentID, "host-child-")
	for _, session := range source.List() {
		if session == nil {
			continue
		}
		sessionID := strings.TrimSpace(session.ID)
		if !strings.HasPrefix(sessionID, "host-child:") {
			continue
		}
		if strings.TrimPrefix(hostOpsSanitizeIDPart(sessionID), "host-child-") == sanitizedTarget {
			return session
		}
	}
	return nil
}

func hostOpsRuntimeTranscriptItems(turn runtimekernel.TurnSnapshot) []hostops.TranscriptItem {
	items := make([]hostops.TranscriptItem, 0, len(turn.AgentItems))
	for _, item := range turn.AgentItems {
		transcriptItem, ok := hostOpsRuntimeTranscriptItem(turn, item)
		if !ok {
			continue
		}
		items = append(items, transcriptItem)
	}
	return items
}

func hostOpsRuntimeTranscriptItem(turn runtimekernel.TurnSnapshot, item agentstate.TurnItem) (hostops.TranscriptItem, bool) {
	payload := hostOpsAgentItemPayload(item)
	transcript := hostops.TranscriptItem{
		ID:        hostOpsRuntimeTranscriptItemID(turn, item),
		Status:    string(item.Status),
		Payload:   payload,
		CreatedAt: hostOpsAgentItemTime(turn, item),
	}
	switch item.Type {
	case agentstate.TurnItemTypeModelCall:
		transcript.Type = hostops.TranscriptItemType("llm_request")
		transcript.Content = hostOpsModelCallContent(item, payload)
	case agentstate.TurnItemTypeAssistantMessage:
		transcript.Type = hostops.TranscriptItemType("llm_response")
		transcript.Content = hostOpsFirstNonEmptyString(
			item.Payload.Summary,
			hostOpsPayloadString(payload, "content"),
			hostOpsPayloadString(payload, "text"),
			hostOpsPayloadString(payload, "output"),
		)
	case agentstate.TurnItemTypeToolCall:
		transcript.Type = hostops.TranscriptItemToolCall
		transcript.ToolName = hostOpsFirstNonEmptyString(hostOpsPayloadString(payload, "toolName"), item.Payload.Summary, item.Payload.Kind)
		transcript.Content = hostOpsFirstNonEmptyString(
			hostOpsPayloadString(payload, "inputSummary"),
			hostOpsPayloadString(payload, "command"),
			hostOpsPayloadString(payload, "arguments"),
			item.Payload.Summary,
		)
	case agentstate.TurnItemTypeToolResult:
		transcript.Type = hostops.TranscriptItemToolResult
		transcript.ToolName = hostOpsFirstNonEmptyString(hostOpsPayloadString(payload, "toolName"), item.Payload.Summary, item.Payload.Kind)
		transcript.Content = hostOpsToolResultContent(item, payload)
	case agentstate.TurnItemTypeApproval:
		transcript.Type = hostops.TranscriptItemApproval
		transcript.ApprovalID = hostOpsPayloadString(payload, "approvalId")
		transcript.Content = hostOpsFirstNonEmptyString(item.Payload.Summary, hostOpsPayloadString(payload, "summary"))
	case agentstate.TurnItemTypeError:
		transcript.Type = hostops.TranscriptItemError
		transcript.Content = hostOpsFirstNonEmptyString(item.Payload.Summary, hostOpsPayloadString(payload, "error"))
	default:
		return hostops.TranscriptItem{}, false
	}
	if strings.TrimSpace(transcript.Content) == "" && strings.TrimSpace(item.Payload.Summary) == "" {
		return hostops.TranscriptItem{}, false
	}
	return transcript, true
}

func hostOpsModelCallContent(item agentstate.TurnItem, payload map[string]any) string {
	iteration := hostOpsPayloadString(payload, "iteration")
	lines := make([]string, 0, 4)
	if iteration != "" {
		lines = append(lines, fmt.Sprintf("第 %s 轮调用 LLM", iteration))
	} else {
		lines = append(lines, hostOpsFirstNonEmptyString(item.Payload.Summary, "调用 LLM"))
	}
	if traceFile := hostOpsPayloadString(payload, "traceFile"); traceFile != "" {
		lines = append(lines, "trace: "+traceFile)
	}
	if visibleTools := hostOpsPayloadString(payload, "visibleTools"); visibleTools != "" {
		lines = append(lines, "visibleTools: "+visibleTools)
	}
	if summary := strings.TrimSpace(item.Payload.Summary); summary != "" && summary != "calling model" {
		lines = append(lines, summary)
	}
	return strings.Join(lines, "\n")
}

func hostOpsToolResultContent(item agentstate.TurnItem, payload map[string]any) string {
	content := hostOpsFirstNonEmptyString(
		hostOpsPayloadString(payload, "outputSummary"),
		hostOpsPayloadString(payload, "outputPreview"),
		hostOpsPayloadString(payload, "content"),
		item.Payload.Summary,
	)
	if refs := hostOpsPayloadString(payload, "evidenceRefs"); refs != "" {
		if strings.TrimSpace(content) != "" {
			content += "\n"
		}
		content += "evidenceRefs: " + refs
	}
	return content
}

func hostOpsAgentItemPayload(item agentstate.TurnItem) map[string]any {
	payload := map[string]any{}
	if item.Payload.Kind != "" {
		payload["kind"] = item.Payload.Kind
	}
	if item.Payload.Summary != "" {
		payload["summary"] = item.Payload.Summary
	}
	if len(item.Payload.Data) == 0 {
		return payload
	}
	var data map[string]any
	if err := json.Unmarshal(item.Payload.Data, &data); err != nil {
		payload["raw"] = string(item.Payload.Data)
		return payload
	}
	for key, value := range data {
		payload[key] = value
	}
	return payload
}

func hostOpsPayloadString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%g", typed)
	case bool:
		return fmt.Sprintf("%t", typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, part := range typed {
			text := strings.TrimSpace(fmt.Sprint(part))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func hostOpsRuntimeTranscriptItemID(turn runtimekernel.TurnSnapshot, item agentstate.TurnItem) string {
	itemID := strings.TrimSpace(item.ID)
	if itemID == "" {
		itemID = fmt.Sprintf("%s-%d", string(item.Type), len(turn.AgentItems))
	}
	turnID := strings.TrimSpace(turn.ID)
	if turnID == "" {
		turnID = strings.TrimSpace(turn.SessionID)
	}
	return "runtime-" + hostOpsSanitizeIDPart(turnID) + "-" + hostOpsSanitizeIDPart(itemID)
}

func hostOpsAgentItemTime(turn runtimekernel.TurnSnapshot, item agentstate.TurnItem) time.Time {
	switch {
	case !item.CreatedAt.IsZero():
		return item.CreatedAt
	case !item.UpdatedAt.IsZero():
		return item.UpdatedAt
	case !turn.StartedAt.IsZero():
		return turn.StartedAt
	case !turn.UpdatedAt.IsZero():
		return turn.UpdatedAt
	default:
		return time.Time{}
	}
}

func hostOpsSanitizeIDPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "item"
	}
	return out
}

func firstNonEmptyHostOpsString(values ...string) string {
	return hostOpsFirstNonEmptyString(values...)
}

func hostOpsFirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
