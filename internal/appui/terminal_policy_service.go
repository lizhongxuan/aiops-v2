package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"aiops-v2/internal/terminalpolicy"
)

type TerminalPolicyService interface {
	GetConfig(ctx context.Context) (terminalpolicy.Config, error)
	UpdateConfig(ctx context.Context, config terminalpolicy.Config) (terminalpolicy.Config, error)
	Evaluate(req terminalpolicy.CommandRequest) terminalpolicy.Decision
}

type localTerminalPolicyService struct {
	mu     sync.RWMutex
	path   string
	config terminalpolicy.Config
	engine *terminalpolicy.Engine
}

func NewTerminalPolicyService(path string) TerminalPolicyService {
	service := &localTerminalPolicyService{
		path: strings.TrimSpace(path),
	}
	service.config = terminalpolicy.DefaultConfig()
	service.engine = terminalpolicy.NewEngine(service.config)
	_ = service.load()
	return service
}

func (s *localTerminalPolicyService) GetConfig(context.Context) (terminalpolicy.Config, error) {
	if err := s.load(); err != nil {
		return terminalpolicy.Config{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config, nil
}

func (s *localTerminalPolicyService) UpdateConfig(_ context.Context, config terminalpolicy.Config) (terminalpolicy.Config, error) {
	config = normalizeTerminalPolicyConfig(config)
	if err := terminalpolicy.ValidateConfig(config); err != nil {
		return terminalpolicy.Config{}, err
	}
	if err := s.write(config); err != nil {
		return terminalpolicy.Config{}, err
	}
	s.mu.Lock()
	s.config = config
	s.engine = terminalpolicy.NewEngine(config)
	s.mu.Unlock()
	return config, nil
}

func (s *localTerminalPolicyService) Evaluate(req terminalpolicy.CommandRequest) terminalpolicy.Decision {
	s.mu.RLock()
	engine := s.engine
	s.mu.RUnlock()
	if engine == nil {
		engine = terminalpolicy.NewEngine(terminalpolicy.DefaultConfig())
	}
	return engine.Evaluate(req)
}

func (s *localTerminalPolicyService) load() error {
	if s.path == "" {
		return nil
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read terminal policy config: %w", err)
	}
	var config terminalpolicy.Config
	if err := json.Unmarshal(raw, &config); err != nil {
		return fmt.Errorf("decode terminal policy config: %w", err)
	}
	config = normalizeTerminalPolicyConfig(config)
	if err := terminalpolicy.ValidateConfig(config); err != nil {
		return fmt.Errorf("validate terminal policy config: %w", err)
	}
	s.mu.Lock()
	s.config = config
	s.engine = terminalpolicy.NewEngine(config)
	s.mu.Unlock()
	return nil
}

func (s *localTerminalPolicyService) write(config terminalpolicy.Config) error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create terminal policy directory: %w", err)
	}
	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode terminal policy config: %w", err)
	}
	content = append(content, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return fmt.Errorf("write terminal policy temp file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace terminal policy config: %w", err)
	}
	return nil
}

func normalizeTerminalPolicyConfig(config terminalpolicy.Config) terminalpolicy.Config {
	if config.SchemaVersion == "" {
		config.SchemaVersion = terminalpolicy.SchemaVersion
	}
	if config.Rules == nil {
		config.Rules = []terminalpolicy.Rule{}
	}
	return config
}
