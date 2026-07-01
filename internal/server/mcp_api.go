package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"aiops-v2/internal/appui"
)

func (s *HTTPServer) handleMCPServers(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/mcp/servers" && !strings.HasPrefix(r.URL.Path, "/api/v1/mcp/servers/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		payload, err := s.ui.MCPService().List(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodPost:
		if strings.TrimSuffix(r.URL.Path, "/") == "/api/v1/mcp/servers" {
			var req appui.MCPServerUpsert
			if err := decodeJSONBody(r, &req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
			payload, err := s.ui.MCPService().Create(r.Context(), req)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, payload)
			return
		}

		name, action, ok := parseMCPServerAction(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		payload, err := s.ui.MCPService().Act(r.Context(), name, action)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodPut:
		name := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/mcp/servers/"))
		name = strings.Trim(name, "/")
		if name == "" || strings.Contains(name, "/") {
			http.NotFound(w, r)
			return
		}
		var req appui.MCPServerUpsert
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		payload, err := s.ui.MCPService().Update(r.Context(), name, req)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodDelete:
		name := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/mcp/servers/"))
		name = strings.Trim(name, "/")
		if name == "" || strings.Contains(name, "/") {
			http.NotFound(w, r)
			return
		}
		payload, err := s.ui.MCPService().Delete(r.Context(), name)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleMCPServersRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload, err := s.ui.MCPService().Refresh(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *HTTPServer) handleRuntimeMCPHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	base := "/api/v2/runtime/mcp-health"
	if r.URL.Path == base || r.URL.Path == base+"/" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		payload, err := s.ui.MCPService().Health(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
		return
	}
	trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, base+"/"), "/")
	if r.Method == http.MethodPost {
		parts := strings.Split(trimmed, "/")
		if len(parts) != 2 || parts[1] != "refresh" || strings.TrimSpace(parts[0]) == "" {
			http.NotFound(w, r)
			return
		}
		serverID := strings.TrimSpace(parts[0])
		if _, err := s.ui.MCPService().Act(r.Context(), serverID, "refresh"); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		payload, err := s.ui.MCPService().HealthOne(r.Context(), serverID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
		return
	}
	serverID := trimmed
	if serverID == "" || strings.Contains(serverID, "/") {
		http.NotFound(w, r)
		return
	}
	payload, err := s.ui.MCPService().HealthOne(r.Context(), serverID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func decodeJSONBody(r *http.Request, target any) error {
	if r.Body == nil {
		return io.EOF
	}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(target)
}

func parseMCPServerAction(path string) (string, string, bool) {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v1/mcp/servers/"), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	name := strings.TrimSpace(parts[0])
	action := strings.TrimSpace(parts[1])
	switch action {
	case "open", "close", "refresh":
		return name, action, name != ""
	default:
		return "", "", false
	}
}
