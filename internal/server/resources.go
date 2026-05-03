package server

import (
	"net/http"
	"os"
	"strings"
	"time"
)

// ResourceServer provides CRUD endpoints for resource management and audit APIs.
type ResourceServer struct {
	mux    *http.ServeMux
	coroot corootProxyConfig
}

// NewResourceServer creates a ResourceServer and registers all resource routes.
func NewResourceServer() *ResourceServer {
	rs := &ResourceServer{
		mux:    http.NewServeMux(),
		coroot: corootProxyConfigFromEnv(),
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

func corootProxyConfigFromEnv() corootProxyConfig {
	timeout := 30 * time.Second
	if raw := strings.TrimSpace(os.Getenv("AIOPS_COROOT_TIMEOUT")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			timeout = parsed
		}
	}

	return corootProxyConfig{
		BaseURL:   firstNonEmptyEnv("AIOPS_COROOT_BASE_URL", "COROOT_BASE_URL"),
		Token:     firstNonEmptyEnv("AIOPS_COROOT_TOKEN", "COROOT_TOKEN"),
		IframeURL: firstNonEmptyEnv("AIOPS_COROOT_IFRAME_URL"),
		Timeout:   timeout,
	}
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
