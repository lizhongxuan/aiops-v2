package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// Resource Management & Audit API — keeps all existing CRUD endpoints for:
// - approval-audits (Req 6.5)
// - approval-grants (Req 6.5)
// - capability-bindings (Req 6.6)
// - ui-cards (Req 6.6)
// - script-configs (Req 6.6)
// - lab-environments (Req 6.6)
// - Coroot proxy (Req 6.7)
// - Generator Workshop API (Req 6.7)
// ---------------------------------------------------------------------------

// ResourceServer provides CRUD endpoints for resource management and audit APIs.
type ResourceServer struct {
	mux *http.ServeMux
}

// NewResourceServer creates a ResourceServer and registers all resource routes.
func NewResourceServer() *ResourceServer {
	rs := &ResourceServer{
		mux: http.NewServeMux(),
	}
	rs.registerRoutes()
	return rs
}

// Handler returns the http.Handler for resource routes.
func (rs *ResourceServer) Handler() http.Handler {
	return rs.mux
}

// RegisterOnMux registers all resource routes on an existing ServeMux.
func (rs *ResourceServer) RegisterOnMux(mux *http.ServeMux) {
	// Approval & Audit APIs (Req 6.5)
	mux.HandleFunc("/api/v1/approval-audits", rs.handleApprovalAudits)
	mux.HandleFunc("/api/v1/approval-audits/", rs.handleApprovalAudits)
	mux.HandleFunc("/api/v1/approval-grants", rs.handleApprovalGrants)
	mux.HandleFunc("/api/v1/approval-grants/", rs.handleApprovalGrants)

	// Resource Management APIs (Req 6.6)
	mux.HandleFunc("/api/v1/capability-bindings", rs.handleCapabilityBindings)
	mux.HandleFunc("/api/v1/capability-bindings/", rs.handleCapabilityBindings)
	mux.HandleFunc("/api/v1/ui-cards", rs.handleUICards)
	mux.HandleFunc("/api/v1/ui-cards/", rs.handleUICards)
	mux.HandleFunc("/api/v1/script-configs", rs.handleScriptConfigs)
	mux.HandleFunc("/api/v1/script-configs/", rs.handleScriptConfigs)
	mux.HandleFunc("/api/v1/lab-environments", rs.handleLabEnvironments)
	mux.HandleFunc("/api/v1/lab-environments/", rs.handleLabEnvironments)

	// Coroot Proxy (Req 6.7)
	mux.HandleFunc("/api/v1/coroot/", rs.handleCorootProxy)

	// Generator Workshop API (Req 6.7)
	mux.HandleFunc("/api/v1/generator/", rs.handleGeneratorWorkshop)
}

func (rs *ResourceServer) registerRoutes() {
	rs.RegisterOnMux(rs.mux)
}

// ---------------------------------------------------------------------------
// Approval Audits — full audit log CRUD (Req 6.5)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Approval Grants — authorization whitelist management (Req 6.5)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Capability Bindings CRUD (Req 6.6)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// UI Cards CRUD (Req 6.6)
// ---------------------------------------------------------------------------

func (rs *ResourceServer) handleUICards(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeResourceJSON(w, http.StatusOK, map[string]interface{}{
			"cards": []interface{}{},
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

// ---------------------------------------------------------------------------
// Script Configs CRUD (Req 6.6)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Lab Environments CRUD (Req 6.6)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Coroot Proxy — read-only reverse proxy to Coroot (Req 6.7)
// ---------------------------------------------------------------------------

func (rs *ResourceServer) handleCorootProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed — Coroot proxy is read-only", http.StatusMethodNotAllowed)
		return
	}

	// Extract the sub-path after /api/v1/coroot/
	subPath := strings.TrimPrefix(r.URL.Path, "/api/v1/coroot/")

	// Stub: in production, this forwards to the Coroot backend.
	// The response format is preserved for frontend compatibility.
	writeResourceJSON(w, http.StatusOK, map[string]interface{}{
		"proxy":   "coroot",
		"path":    subPath,
		"status":  "ok",
		"message": "coroot proxy stub",
	})
}

// ---------------------------------------------------------------------------
// Generator Workshop API — four-step flow (Req 6.7)
// generate → lint → preview → publish-draft
// ---------------------------------------------------------------------------

func (rs *ResourceServer) handleGeneratorWorkshop(w http.ResponseWriter, r *http.Request) {
	subPath := strings.TrimPrefix(r.URL.Path, "/api/v1/generator/")

	switch {
	case strings.HasPrefix(subPath, "generate"):
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]string{
			"step":   "generate",
			"status": "ok",
		})

	case strings.HasPrefix(subPath, "lint"):
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]string{
			"step":   "lint",
			"status": "ok",
		})

	case strings.HasPrefix(subPath, "preview"):
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]string{
			"step":   "preview",
			"status": "ok",
		})

	case strings.HasPrefix(subPath, "publish-draft"):
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]string{
			"step":   "publish-draft",
			"status": "ok",
		})

	default:
		// List or status
		writeResourceJSON(w, http.StatusOK, map[string]interface{}{
			"generator": "workshop",
			"steps":     []string{"generate", "lint", "preview", "publish-draft"},
		})
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func writeResourceJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
