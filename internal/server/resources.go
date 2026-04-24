package server

import "net/http"

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
	registerResourceRoutes(mux, rs)
}

func (rs *ResourceServer) registerRoutes() {
	rs.RegisterOnMux(rs.mux)
}
