package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/integrations/coroot"
	"aiops-v2/internal/plugins"
	"aiops-v2/internal/store"
)

type corootConfigRepository interface {
	GetCorootConfig() (*store.CorootConfig, error)
}

func builtinPluginSpecs(repo corootConfigRepository) ([]plugins.Spec, error) {
	spec, err := coroot.BuiltinPluginSpec(storeCorootClientProvider{repo: repo}, "")
	if err != nil {
		return nil, err
	}
	return []plugins.Spec{spec}, nil
}

func registerBuiltinPlugins(registrar *plugins.Registrar, repo corootConfigRepository) ([]plugins.Spec, error) {
	if registrar == nil {
		return nil, nil
	}
	specs, err := builtinPluginSpecs(repo)
	if err != nil {
		return nil, err
	}
	for _, spec := range specs {
		if err := registrar.Register(spec); err != nil {
			return nil, err
		}
	}
	return specs, nil
}

type storeCorootClientProvider struct {
	repo corootConfigRepository
}

func (p storeCorootClientProvider) CorootClient(context.Context) (*coroot.Client, error) {
	if p.repo == nil {
		return nil, &coroot.CorootError{Kind: "not_configured", Message: "Coroot is not configured from the Coroot observability page"}
	}
	cfg, err := p.repo.GetCorootConfig()
	if err != nil || cfg == nil || strings.TrimSpace(cfg.BaseURL) == "" {
		message := "Coroot is not configured from the Coroot observability page"
		if err != nil {
			message = fmt.Sprintf("%s: %v", message, err)
		}
		return nil, &coroot.CorootError{Kind: "not_configured", Message: message}
	}

	timeout := 30 * time.Second
	if raw := strings.TrimSpace(cfg.Timeout); raw != "" {
		parsed, parseErr := time.ParseDuration(raw)
		if parseErr != nil || parsed <= 0 {
			return nil, &coroot.CorootError{Kind: "bad_config", Message: "invalid Coroot timeout: " + raw}
		}
		timeout = parsed
	}
	client, err := coroot.NewClient(coroot.ClientConfig{
		BaseURL: cfg.BaseURL,
		Token:   cfg.Token,
		Project: cfg.Project,
		Timeout: timeout,
	})
	if err != nil {
		return nil, &coroot.CorootError{Kind: "bad_config", Message: err.Error()}
	}
	return client, nil
}
