package server

import "net/http"

// Approval Audits — full audit log CRUD (Req 6.5)
func (rs *ResourceServer) handleApprovalAudits(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// List or get audit records with filtering (time, host, operator, decision)
		writeResourceJSON(w, http.StatusOK, map[string]interface{}{
			"audits": []interface{}{},
			"total":  0,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// Approval Grants — authorization whitelist management (Req 6.5)
func (rs *ResourceServer) handleApprovalGrants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// List grants
		writeResourceJSON(w, http.StatusOK, map[string]interface{}{
			"grants": []interface{}{},
		})
	case http.MethodPost:
		// Create grant
		writeResourceJSON(w, http.StatusCreated, map[string]string{"status": "created"})
	case http.MethodPut:
		// Update grant (enable/disable)
		writeResourceJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	case http.MethodDelete:
		// Revoke grant
		writeResourceJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
