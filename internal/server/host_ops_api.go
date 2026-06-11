package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"aiops-v2/internal/appui"
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
