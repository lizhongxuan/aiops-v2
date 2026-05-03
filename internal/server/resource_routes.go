package server

import "net/http"

func registerResourceRoutes(mux *http.ServeMux, rs *ResourceServer) {
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
	mux.HandleFunc("/api/v1/coroot", rs.handleCorootProxy)
	mux.HandleFunc("/api/v1/coroot/", rs.handleCorootProxy)

	// Generator Workshop API (Req 6.7)
	mux.HandleFunc("/api/v1/generator/", rs.handleGeneratorWorkshop)
}
