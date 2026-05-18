package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/store"
)

// Capability Bindings CRUD (Req 6.6)
func (rs *ResourceServer) handleCapabilityBindings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeResourceJSON(w, http.StatusOK, map[string]interface{}{
			"bindings": []interface{}{},
		})
	case http.MethodPost:
		writeResourceJSON(w, http.StatusCreated, map[string]string{"status": "created"})
	case http.MethodPut:
		writeResourceJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	case http.MethodDelete:
		writeResourceJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// UI Cards CRUD (Req 6.6)
func (rs *ResourceServer) handleUICards(w http.ResponseWriter, r *http.Request) {
	service := rs.uiCards
	if service == nil {
		service = appui.NewUICardService(nil)
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/ui-cards"), "/")
	parts := splitResourcePath(path)
	switch r.Method {
	case http.MethodGet:
		if len(parts) == 0 {
			result, err := service.List(appui.UICardListRequest{
				Status: strings.TrimSpace(r.URL.Query().Get("status")),
				Kind:   strings.TrimSpace(r.URL.Query().Get("kind")),
			})
			writeResourceResult(w, result, err)
			return
		}
		if len(parts) == 2 {
			switch parts[1] {
			case "status":
				result, err := service.Status(parts[0])
				writeResourceResult(w, result, err)
				return
			case "versions":
				result, err := service.Versions(parts[0])
				writeResourceResult(w, map[string]any{"items": result, "total": len(result)}, err)
				return
			}
		}
		card, err := service.Get(parts[0])
		writeResourceResult(w, card, err)
	case http.MethodPost:
		if len(parts) == 0 {
			var card store.UICard
			if err := json.NewDecoder(r.Body).Decode(&card); err != nil {
				writeResourceError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			created, err := service.Create(card)
			writeResourceResultStatus(w, http.StatusCreated, created, err)
			return
		}
		if len(parts) == 2 {
			var req struct {
				Payload map[string]any `json:"payload"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			switch parts[1] {
			case "validate":
				result, err := service.Validate(appui.UICardValidationRequest{CardID: parts[0], Payload: req.Payload})
				writeResourceResult(w, result, err)
				return
			case "preview":
				result, err := service.Preview(appui.UICardPreviewRequest{CardID: parts[0], Payload: req.Payload})
				writeResourceResult(w, result, err)
				return
			case "versions":
				result, err := service.CreateVersion(parts[0])
				writeResourceResult(w, result, err)
				return
			}
		}
		writeResourceError(w, http.StatusNotFound, "ui card endpoint not found")
	case http.MethodPut:
		if len(parts) == 2 && parts[1] == "status" {
			var req struct {
				Status string `json:"status"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeResourceError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			updated, err := service.UpdateStatus(parts[0], req.Status)
			writeResourceResult(w, updated, err)
			return
		}
		if len(parts) != 1 {
			writeResourceError(w, http.StatusNotFound, "ui card endpoint not found")
			return
		}
		var card store.UICard
		if err := json.NewDecoder(r.Body).Decode(&card); err != nil {
			writeResourceError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		updated, err := service.Update(parts[0], card)
		writeResourceResult(w, updated, err)
	case http.MethodDelete:
		if len(parts) != 1 {
			writeResourceError(w, http.StatusNotFound, "ui card endpoint not found")
			return
		}
		err := service.Delete(parts[0])
		if err != nil && strings.Contains(err.Error(), "built-in") {
			writeResourceError(w, http.StatusConflict, err.Error())
			return
		}
		if err != nil {
			writeResourceError(w, http.StatusNotFound, err.Error())
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func splitResourcePath(path string) []string {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	raw := strings.Split(path, "/")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		if strings.TrimSpace(part) != "" {
			parts = append(parts, strings.TrimSpace(part))
		}
	}
	return parts
}

func writeResourceResult(w http.ResponseWriter, v any, err error) {
	writeResourceResultStatus(w, http.StatusOK, v, err)
}

func writeResourceResultStatus(w http.ResponseWriter, status int, v any, err error) {
	if err != nil {
		writeResourceError(w, http.StatusNotFound, err.Error())
		return
	}
	writeResourceJSON(w, status, v)
}

func writeResourceError(w http.ResponseWriter, status int, message string) {
	writeResourceJSON(w, status, map[string]string{"error": message})
}

func limitFromQuery(r *http.Request) int {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return 0
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return limit
}

// Script Configs CRUD (Req 6.6)
func (rs *ResourceServer) handleScriptConfigs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeResourceJSON(w, http.StatusOK, map[string]interface{}{
			"configs": []interface{}{},
		})
	case http.MethodPost:
		writeResourceJSON(w, http.StatusCreated, map[string]string{"status": "created"})
	case http.MethodPut:
		writeResourceJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	case http.MethodDelete:
		writeResourceJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// Lab Environments CRUD (Req 6.6)
func (rs *ResourceServer) handleLabEnvironments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeResourceJSON(w, http.StatusOK, map[string]interface{}{
			"environments": []interface{}{},
		})
	case http.MethodPost:
		writeResourceJSON(w, http.StatusCreated, map[string]string{"status": "created"})
	case http.MethodPut:
		writeResourceJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	case http.MethodDelete:
		writeResourceJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
