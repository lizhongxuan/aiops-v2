package server

import (
	"net/http"
	"sync"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/store"
)

// ResourceServer provides CRUD endpoints for resource management and audit APIs.
type ResourceServer struct {
	mux            *http.ServeMux
	corootConfig   appui.CorootConfigRepository
	uiCards        appui.UICardService
	agentArtifacts appui.AgentUIArtifactService
}

// ResourceServerOption customizes resource-only API dependencies.
type ResourceServerOption func(*ResourceServer)

func WithCorootConfigRepository(repo appui.CorootConfigRepository) ResourceServerOption {
	return func(rs *ResourceServer) {
		if repo != nil {
			rs.corootConfig = repo
		}
	}
}

func WithUICardService(service appui.UICardService) ResourceServerOption {
	return func(rs *ResourceServer) {
		if service != nil {
			rs.uiCards = service
		}
	}
}

// NewResourceServer creates a ResourceServer and registers all resource routes.
func NewResourceServer(opts ...ResourceServerOption) *ResourceServer {
	rs := &ResourceServer{
		mux:            http.NewServeMux(),
		corootConfig:   &resourceCorootConfigRepository{},
		uiCards:        appui.NewUICardService(&resourceUICardRepository{}),
		agentArtifacts: appui.NewAgentUIArtifactService(nil),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(rs)
		}
	}
	rs.registerRoutes()
	return rs
}

type resourceCorootConfigRepository struct {
	mu     sync.RWMutex
	config *store.CorootConfig
}

func (r *resourceCorootConfigRepository) GetCorootConfig() (*store.CorootConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.config == nil {
		return nil, errCorootConfigNotFound
	}
	cp := *r.config
	return &cp, nil
}

func (r *resourceCorootConfigRepository) SaveCorootConfig(config *store.CorootConfig) error {
	if config == nil {
		return errCorootConfigNil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *config
	r.config = &cp
	return nil
}

type resourceUICardRepository struct {
	mu    sync.RWMutex
	cards []store.UICard
}

func (r *resourceUICardRepository) GetUICards() ([]store.UICard, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]store.UICard(nil), r.cards...), nil
}

func (r *resourceUICardRepository) SaveUICards(cards []store.UICard) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cards = append([]store.UICard(nil), cards...)
	return nil
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
