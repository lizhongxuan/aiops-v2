package appui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/store"
)

const mcpConfigFileName = "mcp-servers.json"

type defaultMCPService struct {
	repo     MCPRepository
	registry *mcp.Registry
}

func NewMCPService(repo MCPRepository, registry *mcp.Registry) MCPService {
	return &defaultMCPService{
		repo:     repo,
		registry: registry,
	}
}

func (s *defaultMCPService) List(context.Context) (MCPServersPayload, error) {
	return s.buildPayload(), nil
}

func (s *defaultMCPService) Create(ctx context.Context, payload MCPServerUpsert) (MCPServersPayload, error) {
	record, err := normalizeMCPServerRecord("", payload)
	if err != nil {
		return MCPServersPayload{}, err
	}
	if err := s.upsertRecord(record); err != nil {
		return MCPServersPayload{}, err
	}
	if err := s.applyRuntime(ctx, record.Name); err != nil {
		return s.buildPayload(), err
	}
	return s.buildPayload(), nil
}

func (s *defaultMCPService) Update(ctx context.Context, name string, payload MCPServerUpsert) (MCPServersPayload, error) {
	target := strings.TrimSpace(name)
	if target == "" {
		return MCPServersPayload{}, fmt.Errorf("mcp server name is required")
	}
	record, err := normalizeMCPServerRecord(target, payload)
	if err != nil {
		return MCPServersPayload{}, err
	}
	if target != record.Name {
		if err := s.deleteRecord(target); err != nil {
			return MCPServersPayload{}, err
		}
	}
	if err := s.upsertRecord(record); err != nil {
		return MCPServersPayload{}, err
	}
	if err := s.applyRuntime(ctx, record.Name); err != nil {
		return s.buildPayload(), err
	}
	return s.buildPayload(), nil
}

func (s *defaultMCPService) Delete(_ context.Context, name string) (MCPServersPayload, error) {
	target := strings.TrimSpace(name)
	if target == "" {
		return MCPServersPayload{}, fmt.Errorf("mcp server name is required")
	}
	if err := s.deleteRecord(target); err != nil {
		return MCPServersPayload{}, err
	}
	s.unregister(target)
	return s.buildPayload(), nil
}

func (s *defaultMCPService) Act(ctx context.Context, name, action string) (MCPServersPayload, error) {
	target := strings.TrimSpace(name)
	if target == "" {
		return MCPServersPayload{}, fmt.Errorf("mcp server name is required")
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "open":
		if err := s.setDisabled(ctx, target, false); err != nil {
			return s.buildPayload(), err
		}
	case "close":
		if err := s.setDisabled(ctx, target, true); err != nil {
			return s.buildPayload(), err
		}
	case "refresh":
		if err := s.refreshOne(ctx, target); err != nil {
			return s.buildPayload(), err
		}
	default:
		return MCPServersPayload{}, fmt.Errorf("unsupported mcp action %q", action)
	}
	return s.buildPayload(), nil
}

func (s *defaultMCPService) Refresh(ctx context.Context) (MCPServersPayload, error) {
	if err := s.refreshAll(ctx); err != nil {
		return s.buildPayload(), err
	}
	return s.buildPayload(), nil
}

func (s *defaultMCPService) buildPayload() MCPServersPayload {
	return MCPServersPayload{
		ConfigPath: mcpConfigFileName,
		Items:      s.collectItems(),
	}
}

func (s *defaultMCPService) collectItems() []MCPServerView {
	records := map[string]store.MCPServerRecord{}
	if s.repo != nil {
		if items, err := s.repo.GetMCPServers(); err == nil {
			for _, item := range items {
				records[strings.TrimSpace(item.Name)] = cloneMCPServerRecord(item)
			}
		}
	}

	if registry := s.resolveRegistry(); registry != nil {
		for _, cfg := range registry.ListServers() {
			name := strings.TrimSpace(firstNonEmpty(cfg.ID, cfg.Name))
			if name == "" {
				continue
			}
			record := records[name]
			if record.Name == "" {
				record = recordFromServerConfig(cfg)
			}
			record.Name = name
			record.Transport = firstNonEmpty(strings.TrimSpace(record.Transport), strings.TrimSpace(cfg.Transport))
			if len(record.Args) == 0 && len(cfg.Command) > 1 {
				record.Args = append([]string(nil), cfg.Command[1:]...)
			}
			if strings.TrimSpace(record.Command) == "" && len(cfg.Command) > 0 && record.Transport != "http" {
				record.Command = strings.TrimSpace(cfg.Command[0])
			}
			if strings.TrimSpace(record.URL) == "" && record.Transport == "http" && len(cfg.Command) > 0 {
				record.URL = strings.TrimSpace(cfg.Command[0])
			}
			record.Disabled = record.Disabled || cfg.Disabled || registry.IsServerDisabled(name)
			record.Status = statusForRegistry(registry, name, record.Status, record.Disabled)
			record.ToolCount = len(registry.ListServerTools(name))
			record.ResourceCount = 0
			records[name] = record
		}
	}

	items := make([]MCPServerView, 0, len(records))
	for _, record := range records {
		items = append(items, mapMCPServerRecord(record))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (s *defaultMCPService) applyRuntime(_ context.Context, name string) error {
	record, err := s.getRecord(name)
	if err != nil {
		return err
	}
	if record.Disabled {
		s.setRegistryDisabled(record.Name, true)
		record.Status = "disconnected"
		record.Error = ""
		record.ToolCount = s.registryToolCount(record.Name)
		record.ResourceCount = 0
		return s.persist(record)
	}
	if err := s.register(record); err != nil {
		record.Status = "error"
		record.Error = err.Error()
		_ = s.persist(record)
		return err
	}
	s.setRegistryDisabled(record.Name, false)
	record.Status = "connected"
	record.Error = ""
	record.ToolCount = s.registryToolCount(record.Name)
	record.ResourceCount = 0
	return s.persist(record)
}

func (s *defaultMCPService) refreshAll(_ context.Context) error {
	if s.repo == nil {
		return nil
	}
	items, err := s.repo.GetMCPServers()
	if err != nil {
		return err
	}
	for _, item := range items {
		record := cloneMCPServerRecord(item)
		if record.Disabled {
			s.setRegistryDisabled(record.Name, true)
			record.Status = "disconnected"
			record.Error = ""
		} else if err := s.register(record); err != nil {
			record.Status = "error"
			record.Error = err.Error()
		} else {
			s.setRegistryDisabled(record.Name, false)
			record.Status = "connected"
			record.Error = ""
		}
		record.ToolCount = s.registryToolCount(record.Name)
		record.ResourceCount = 0
		if err := s.persist(record); err != nil {
			return err
		}
	}
	return nil
}

func (s *defaultMCPService) refreshOne(_ context.Context, name string) error {
	record, err := s.getRecord(name)
	if err != nil {
		return err
	}
	if record.Disabled {
		s.setRegistryDisabled(record.Name, true)
		record.Status = "disconnected"
		record.Error = ""
		record.ToolCount = s.registryToolCount(record.Name)
		record.ResourceCount = 0
		return s.persist(record)
	}
	if err := s.register(record); err != nil {
		record.Status = "error"
		record.Error = err.Error()
	} else {
		s.setRegistryDisabled(record.Name, false)
		record.Status = "connected"
		record.Error = ""
		record.ToolCount = s.registryToolCount(record.Name)
		record.ResourceCount = 0
	}
	return s.persist(record)
}

func (s *defaultMCPService) setDisabled(_ context.Context, name string, disabled bool) error {
	record, err := s.getRecord(name)
	if err != nil {
		return err
	}
	record.Disabled = disabled
	if disabled {
		record.Status = "disconnected"
		record.Error = ""
		record.ToolCount = s.registryToolCount(record.Name)
		record.ResourceCount = 0
		s.setRegistryDisabled(record.Name, true)
		return s.persist(record)
	}
	if err := s.register(record); err != nil {
		record.Status = "error"
		record.Error = err.Error()
		_ = s.persist(record)
		return err
	}
	s.setRegistryDisabled(record.Name, false)
	record.Status = "connected"
	record.Error = ""
	record.ToolCount = s.registryToolCount(record.Name)
	record.ResourceCount = 0
	return s.persist(record)
}

func (s *defaultMCPService) getRecord(name string) (store.MCPServerRecord, error) {
	target := strings.TrimSpace(name)
	if target == "" {
		return store.MCPServerRecord{}, fmt.Errorf("mcp server name is required")
	}
	if s.repo != nil {
		items, err := s.repo.GetMCPServers()
		if err != nil {
			return store.MCPServerRecord{}, err
		}
		for _, item := range items {
			if strings.TrimSpace(item.Name) == target {
				return cloneMCPServerRecord(item), nil
			}
		}
	}
	if registry := s.resolveRegistry(); registry != nil {
		if cfg, ok := registry.GetServer(target); ok {
			return recordFromServerConfig(cfg), nil
		}
	}
	return store.MCPServerRecord{}, fmt.Errorf("mcp server %q not found", target)
}

func (s *defaultMCPService) upsertRecord(record store.MCPServerRecord) error {
	if s.repo == nil {
		return nil
	}
	items, err := s.repo.GetMCPServers()
	if err != nil {
		return err
	}
	updated := false
	for i := range items {
		if strings.TrimSpace(items[i].Name) == record.Name {
			items[i] = cloneMCPServerRecord(record)
			updated = true
			break
		}
	}
	if !updated {
		items = append(items, cloneMCPServerRecord(record))
	}
	return s.repo.SaveMCPServers(items)
}

func (s *defaultMCPService) deleteRecord(name string) error {
	if s.repo == nil {
		return nil
	}
	items, err := s.repo.GetMCPServers()
	if err != nil {
		return err
	}
	target := strings.TrimSpace(name)
	out := make([]store.MCPServerRecord, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Name) == target {
			continue
		}
		out = append(out, cloneMCPServerRecord(item))
	}
	return s.repo.SaveMCPServers(out)
}

func (s *defaultMCPService) persist(record store.MCPServerRecord) error {
	return s.upsertRecord(record)
}

func (s *defaultMCPService) register(record store.MCPServerRecord) error {
	if registry := s.resolveRegistry(); registry != nil {
		return registry.RegisterServer(mcp.ServerConfig{
			ID:        record.Name,
			Name:      record.Name,
			Transport: record.Transport,
			Command:   buildRegistryCommand(record),
			Source:    "userSettings",
			Disabled:  record.Disabled,
		})
	}
	return nil
}

func (s *defaultMCPService) setRegistryDisabled(name string, disabled bool) {
	if registry := s.resolveRegistry(); registry != nil {
		registry.SetServerDisabled(name, disabled)
	}
}

func (s *defaultMCPService) unregister(name string) {
	if registry := s.resolveRegistry(); registry != nil {
		registry.UnregisterServer(name)
	}
}

func (s *defaultMCPService) registryToolCount(name string) int {
	if registry := s.resolveRegistry(); registry != nil {
		return len(registry.ListServerTools(name))
	}
	return 0
}

func (s *defaultMCPService) resolveRegistry() *mcp.Registry {
	if s.registry != nil {
		return s.registry
	}
	return mcp.DefaultRegistry()
}

func normalizeMCPServerRecord(fallbackName string, payload MCPServerUpsert) (store.MCPServerRecord, error) {
	name := strings.TrimSpace(firstNonEmpty(payload.Name, fallbackName))
	if name == "" {
		return store.MCPServerRecord{}, fmt.Errorf("mcp server name is required")
	}
	transport := strings.ToLower(strings.TrimSpace(payload.Transport))
	if transport == "" {
		transport = "http"
	}

	record := store.MCPServerRecord{
		Name:      name,
		Transport: transport,
		Command:   strings.TrimSpace(payload.Command),
		Args:      append([]string(nil), payload.Args...),
		URL:       strings.TrimSpace(payload.URL),
		Env:       cloneStringMap(payload.Env),
		Disabled:  payload.Disabled,
	}
	for i := range record.Args {
		record.Args[i] = strings.TrimSpace(record.Args[i])
	}
	record.Args = compactStrings(record.Args)
	switch transport {
	case "stdio":
		if record.Command == "" {
			return store.MCPServerRecord{}, fmt.Errorf("mcp stdio server requires command")
		}
		record.URL = ""
	case "http":
		if record.URL == "" {
			return store.MCPServerRecord{}, fmt.Errorf("mcp http server requires url")
		}
		record.Command = ""
	default:
		return store.MCPServerRecord{}, fmt.Errorf("unsupported mcp transport %q", transport)
	}
	if len(record.Env) == 0 {
		record.Env = map[string]string{}
	}
	return record, nil
}

func mapMCPServerRecord(record store.MCPServerRecord) MCPServerView {
	return MCPServerView{
		Name:          strings.TrimSpace(record.Name),
		Transport:     strings.TrimSpace(record.Transport),
		Command:       strings.TrimSpace(record.Command),
		Args:          append([]string(nil), record.Args...),
		URL:           strings.TrimSpace(record.URL),
		Env:           cloneStringMap(record.Env),
		Disabled:      record.Disabled,
		Status:        firstNonEmpty(strings.TrimSpace(record.Status), statusFromRecord(record)),
		Error:         strings.TrimSpace(record.Error),
		ToolCount:     record.ToolCount,
		ResourceCount: record.ResourceCount,
	}
}

func recordFromServerConfig(cfg mcp.ServerConfig) store.MCPServerRecord {
	record := store.MCPServerRecord{
		Name:      strings.TrimSpace(firstNonEmpty(cfg.ID, cfg.Name)),
		Transport: strings.TrimSpace(cfg.Transport),
		Disabled:  cfg.Disabled,
		Env:       map[string]string{},
	}
	if record.Disabled {
		record.Status = "disconnected"
	} else {
		record.Status = "connected"
	}
	if len(cfg.Command) > 0 {
		if strings.EqualFold(record.Transport, "http") {
			record.URL = strings.TrimSpace(cfg.Command[0])
		} else {
			record.Command = strings.TrimSpace(cfg.Command[0])
			record.Args = append([]string(nil), cfg.Command[1:]...)
		}
	}
	return record
}

func buildRegistryCommand(record store.MCPServerRecord) []string {
	switch strings.ToLower(strings.TrimSpace(record.Transport)) {
	case "http":
		if strings.TrimSpace(record.URL) != "" {
			return []string{strings.TrimSpace(record.URL)}
		}
	case "stdio":
		command := strings.TrimSpace(record.Command)
		if command == "" {
			return nil
		}
		out := []string{command}
		out = append(out, compactStrings(record.Args)...)
		return out
	}
	return nil
}

func statusFromRecord(record store.MCPServerRecord) string {
	if record.Disabled {
		return "disconnected"
	}
	if strings.TrimSpace(record.Error) != "" {
		return "error"
	}
	if record.Status != "" {
		return record.Status
	}
	return "connected"
}

func statusForRegistry(registry *mcp.Registry, name, fallback string, disabled bool) string {
	if disabled {
		return "disconnected"
	}
	if registry == nil {
		return firstNonEmpty(fallback, "disconnected")
	}
	if _, ok := registry.GetServer(name); ok {
		return "connected"
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "disconnected"
}

func cloneMCPServerRecord(record store.MCPServerRecord) store.MCPServerRecord {
	record.Args = append([]string(nil), record.Args...)
	record.Env = cloneStringMap(record.Env)
	return record
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
