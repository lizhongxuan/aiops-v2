package appui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/store"
)

const mcpConfigFileName = "mcp-servers.json"

type defaultMCPService struct {
	repo     MCPRepository
	registry *mcp.Registry
	runtime  MCPRuntime
}

func NewMCPService(repo MCPRepository, registry *mcp.Registry) MCPService {
	return &defaultMCPService{
		repo:     repo,
		registry: registry,
	}
}

type MCPRuntime interface {
	Connect(ctx context.Context, serverID string) error
	Disconnect(ctx context.Context, serverID string) error
	RefreshTools(ctx context.Context, serverID string) error
}

func NewMCPServiceWithRuntime(repo MCPRepository, registry *mcp.Registry, runtime MCPRuntime) MCPService {
	return &defaultMCPService{
		repo:     repo,
		registry: registry,
		runtime:  runtime,
	}
}

func (s *defaultMCPService) List(context.Context) (MCPServersPayload, error) {
	return s.buildPayload(), nil
}

func (s *defaultMCPService) Health(context.Context) (MCPHealthPayload, error) {
	items := s.collectItems()
	displayNames := s.registryDisplayNames()
	out := make([]MCPHealthView, 0, len(items))
	for _, item := range items {
		out = append(out, mcpHealthViewFromServer(item, displayNames))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ServerID < out[j].ServerID })
	return MCPHealthPayload{Items: out}, nil
}

func (s *defaultMCPService) HealthOne(_ context.Context, serverID string) (MCPHealthView, error) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return MCPHealthView{}, fmt.Errorf("mcp server id is required")
	}
	displayNames := s.registryDisplayNames()
	for _, item := range s.collectItems() {
		if strings.TrimSpace(item.Name) == serverID {
			return mcpHealthViewFromServer(item, displayNames), nil
		}
	}
	return MCPHealthView{}, fmt.Errorf("mcp server %q not found", serverID)
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
			record.ResourceCount = len(registry.ListResources(name))
			records[name] = record
		}
	}

	registry := s.resolveRegistry()
	items := make([]MCPServerView, 0, len(records))
	for _, record := range records {
		view := mapMCPServerRecord(record)
		if registry != nil {
			view.Health = healthForRegistry(registry, view.Name, view.Disabled)
		}
		items = append(items, view)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (s *defaultMCPService) applyRuntime(ctx context.Context, name string) error {
	record, err := s.getRecord(name)
	if err != nil {
		return err
	}
	if record.Disabled {
		if usesExternalMCPRuntime(record.Transport) {
			_ = s.disconnectRuntime(ctx, record.Name)
		}
		s.setRegistryDisabled(record.Name, true)
		record.Status = "disconnected"
		record.Error = ""
		s.setRegistryHealth(record.Name, mcp.HealthDisabled, nil, nil)
		record.ToolCount = s.registryToolCount(record.Name)
		record.ResourceCount = s.registryResourceCount(record.Name)
		return s.persist(record)
	}
	if err := s.register(record); err != nil {
		record.Status = "error"
		record.Error = redactedMCPError(err)
		s.setRegistryHealth(record.Name, mcp.HealthUnavailable, err, nil)
		_ = s.persist(record)
		return err
	}
	if !usesExternalMCPRuntime(record.Transport) {
		s.markRegistryRecordConnected(&record)
		return s.persist(record)
	}
	s.setRegistryDisabled(record.Name, false)
	if err := s.connectRuntime(ctx, record.Name); err != nil {
		record.Status = "error"
		record.Error = redactedMCPError(err)
		s.setRegistryHealth(record.Name, mcp.HealthUnavailable, err, nil)
		record.ToolCount = s.registryToolCount(record.Name)
		record.ResourceCount = s.registryResourceCount(record.Name)
		_ = s.persist(record)
		return err
	}
	s.markRegistryRecordConnected(&record)
	return s.persist(record)
}

func (s *defaultMCPService) refreshAll(ctx context.Context) error {
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
			if usesExternalMCPRuntime(record.Transport) {
				_ = s.disconnectRuntime(ctx, record.Name)
			}
			s.setRegistryDisabled(record.Name, true)
			record.Status = "disconnected"
			record.Error = ""
			s.setRegistryHealth(record.Name, mcp.HealthDisabled, nil, nil)
		} else if err := s.register(record); err != nil {
			record.Status = "error"
			record.Error = redactedMCPError(err)
			s.setRegistryHealth(record.Name, mcp.HealthUnavailable, err, nil)
		} else if !usesExternalMCPRuntime(record.Transport) {
			s.markRegistryRecordConnected(&record)
		} else if err := s.refreshRuntime(ctx, record.Name); err != nil {
			record.Status = "error"
			record.Error = redactedMCPError(err)
			s.setRegistryHealth(record.Name, mcp.HealthUnavailable, err, nil)
		} else {
			s.markRegistryRecordConnected(&record)
		}
		record.ToolCount = s.registryToolCount(record.Name)
		record.ResourceCount = s.registryResourceCount(record.Name)
		if err := s.persist(record); err != nil {
			return err
		}
	}
	return nil
}

func (s *defaultMCPService) refreshOne(ctx context.Context, name string) error {
	record, err := s.getRecord(name)
	if err != nil {
		return err
	}
	if record.Disabled {
		if usesExternalMCPRuntime(record.Transport) {
			_ = s.disconnectRuntime(ctx, record.Name)
		}
		s.setRegistryDisabled(record.Name, true)
		record.Status = "disconnected"
		record.Error = ""
		s.setRegistryHealth(record.Name, mcp.HealthDisabled, nil, nil)
		record.ToolCount = s.registryToolCount(record.Name)
		record.ResourceCount = s.registryResourceCount(record.Name)
		return s.persist(record)
	}
	if err := s.register(record); err != nil {
		record.Status = "error"
		record.Error = redactedMCPError(err)
		s.setRegistryHealth(record.Name, mcp.HealthUnavailable, err, nil)
	} else if !usesExternalMCPRuntime(record.Transport) {
		s.markRegistryRecordConnected(&record)
	} else if err := s.refreshRuntime(ctx, record.Name); err != nil {
		record.Status = "error"
		record.Error = redactedMCPError(err)
		s.setRegistryHealth(record.Name, mcp.HealthUnavailable, err, nil)
	} else {
		s.markRegistryRecordConnected(&record)
	}
	return s.persist(record)
}

func (s *defaultMCPService) setDisabled(ctx context.Context, name string, disabled bool) error {
	record, err := s.getRecord(name)
	if err != nil {
		return err
	}
	record.Disabled = disabled
	if disabled {
		if usesExternalMCPRuntime(record.Transport) {
			_ = s.disconnectRuntime(ctx, record.Name)
		}
		record.Status = "disconnected"
		record.Error = ""
		s.setRegistryHealth(record.Name, mcp.HealthDisabled, nil, nil)
		record.ToolCount = s.registryToolCount(record.Name)
		record.ResourceCount = s.registryResourceCount(record.Name)
		s.setRegistryDisabled(record.Name, true)
		return s.persist(record)
	}
	if err := s.register(record); err != nil {
		record.Status = "error"
		record.Error = redactedMCPError(err)
		s.setRegistryHealth(record.Name, mcp.HealthUnavailable, err, nil)
		_ = s.persist(record)
		return err
	}
	if !usesExternalMCPRuntime(record.Transport) {
		s.markRegistryRecordConnected(&record)
		return s.persist(record)
	}
	s.setRegistryDisabled(record.Name, false)
	if err := s.connectRuntime(ctx, record.Name); err != nil {
		record.Status = "error"
		record.Error = redactedMCPError(err)
		s.setRegistryHealth(record.Name, mcp.HealthUnavailable, err, nil)
		record.ToolCount = s.registryToolCount(record.Name)
		record.ResourceCount = s.registryResourceCount(record.Name)
		_ = s.persist(record)
		return err
	}
	s.markRegistryRecordConnected(&record)
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
	_ = s.disconnectRuntime(context.Background(), name)
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

func (s *defaultMCPService) registryResourceCount(name string) int {
	if registry := s.resolveRegistry(); registry != nil {
		return len(registry.ListResources(name))
	}
	return 0
}

func (s *defaultMCPService) setRegistryHealth(name string, status mcp.HealthStatus, err error, capabilities []string) {
	if registry := s.resolveRegistry(); registry != nil {
		now := time.Now()
		snapshot := mcp.HealthSnapshot{
			ServerID:      strings.TrimSpace(name),
			Status:        status,
			LastCheckedAt: now,
			TTLSeconds:    int(mcp.DefaultHealthTTL.Seconds()),
			Capabilities:  append([]string(nil), capabilities...),
		}
		if status == mcp.HealthHealthy || status == mcp.HealthDegraded {
			snapshot.LastSuccessAt = now
		}
		if err != nil {
			snapshot.LastError = redactedMCPError(err)
		}
		registry.SetServerHealthSnapshot(snapshot)
	}
}

func (s *defaultMCPService) markRegistryRecordConnected(record *store.MCPServerRecord) {
	if record == nil {
		return
	}
	s.setRegistryDisabled(record.Name, false)
	record.Status = "connected"
	record.Error = ""
	record.ToolCount = s.registryToolCount(record.Name)
	record.ResourceCount = s.registryResourceCount(record.Name)
	s.setRegistryHealth(record.Name, mcp.HealthHealthy, nil, mcpCapabilitiesForCounts(record.ToolCount, record.ResourceCount))
}

func (s *defaultMCPService) connectRuntime(ctx context.Context, name string) error {
	if s.runtime == nil {
		return nil
	}
	return s.runtime.Connect(ctx, name)
}

func (s *defaultMCPService) refreshRuntime(ctx context.Context, name string) error {
	if s.runtime == nil {
		return nil
	}
	return s.runtime.RefreshTools(ctx, name)
}

func (s *defaultMCPService) disconnectRuntime(ctx context.Context, name string) error {
	if s.runtime == nil {
		return nil
	}
	return s.runtime.Disconnect(ctx, name)
}

func (s *defaultMCPService) resolveRegistry() *mcp.Registry {
	if s.registry != nil {
		return s.registry
	}
	return mcp.DefaultRegistry()
}

func (s *defaultMCPService) registryDisplayNames() map[string]string {
	registry := s.resolveRegistry()
	if registry == nil {
		return nil
	}
	out := map[string]string{}
	for _, cfg := range registry.ListServers() {
		id := strings.TrimSpace(firstNonEmpty(cfg.ID, cfg.Name))
		displayName := strings.TrimSpace(firstNonEmpty(cfg.Name, cfg.ID))
		if id != "" && displayName != "" {
			out[id] = displayName
		}
	}
	return out
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
		Error:         mcp.RedactHealthError(record.Error),
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
	if status, ok := registry.GetServerStatus(name); ok && status.State != "" {
		switch status.State {
		case mcp.ServerStateFailed:
			return "error"
		case mcp.ServerStateDisconnected:
			if strings.TrimSpace(fallback) != "" {
				return fallback
			}
		default:
			return string(status.State)
		}
	}
	if _, ok := registry.GetServer(name); ok {
		return "connected"
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "disconnected"
}

func healthForRegistry(registry *mcp.Registry, name string, disabled bool) mcp.HealthSnapshot {
	name = strings.TrimSpace(name)
	if disabled {
		return mcp.HealthSnapshot{
			ServerID:      name,
			Status:        mcp.HealthDisabled,
			LastCheckedAt: time.Now(),
			TTLSeconds:    int(mcp.DefaultHealthTTL.Seconds()),
		}
	}
	if registry == nil {
		return mcp.HealthSnapshot{ServerID: name, Status: mcp.HealthUnknown}
	}
	if snapshot, ok := registry.GetServerHealthSnapshot(name); ok {
		if shouldTreatBuiltinRuntimeTransportErrorAsRegistered(registry, name, snapshot) {
			if registered, ok := registeredBuiltinHealth(registry, name); ok {
				return registered
			}
		}
		return snapshot
	}
	if registered, ok := registeredBuiltinHealth(registry, name); ok {
		return registered
	}
	return mcp.HealthSnapshot{ServerID: name, Status: mcp.HealthUnknown, TTLSeconds: int(mcp.DefaultHealthTTL.Seconds())}
}

func mcpHealthViewFromServer(server MCPServerView, displayNames map[string]string) MCPHealthView {
	health := server.Health
	serverID := firstNonEmpty(strings.TrimSpace(health.ServerID), strings.TrimSpace(server.Name))
	lastError := strings.TrimSpace(health.LastError)
	if lastError == "" && healthAllowsRecordErrorFallback(health.Status) {
		lastError = strings.TrimSpace(server.Error)
	}
	view := MCPHealthView{
		ServerID:           serverID,
		DisplayName:        firstNonEmpty(strings.TrimSpace(displayNames[serverID]), strings.TrimSpace(server.Name)),
		Status:             runtimeHealthStatus(health.Status, server.Disabled),
		LastCheckedAt:      isoStamp(health.LastCheckedAt),
		LastError:          mcp.RedactHealthError(lastError),
		AvailableToolCount: server.ToolCount,
		RetryAfterSeconds:  health.TTLSeconds,
	}
	if server.Disabled || health.Status == mcp.HealthDisabled {
		view.DisabledReason = "server_disabled"
	}
	return view
}

func healthAllowsRecordErrorFallback(status mcp.HealthStatus) bool {
	switch status {
	case mcp.HealthHealthy, mcp.HealthDegraded:
		return false
	default:
		return true
	}
}

func shouldTreatBuiltinRuntimeTransportErrorAsRegistered(registry *mcp.Registry, name string, snapshot mcp.HealthSnapshot) bool {
	if snapshot.Status != mcp.HealthUnavailable {
		return false
	}
	if !strings.Contains(strings.ToLower(snapshot.LastError), "unsupported transport") {
		return false
	}
	cfg, ok := registry.GetServer(name)
	return ok && !usesExternalMCPRuntime(cfg.Transport)
}

func registeredBuiltinHealth(registry *mcp.Registry, name string) (mcp.HealthSnapshot, bool) {
	if registry == nil {
		return mcp.HealthSnapshot{}, false
	}
	cfg, ok := registry.GetServer(name)
	if !ok || usesExternalMCPRuntime(cfg.Transport) {
		return mcp.HealthSnapshot{}, false
	}
	toolCount := len(registry.ListServerTools(name))
	resourceCount := len(registry.ListResources(name))
	if toolCount == 0 && resourceCount == 0 {
		return mcp.HealthSnapshot{}, false
	}
	now := time.Now()
	return mcp.HealthSnapshot{
		ServerID:      name,
		Status:        mcp.HealthHealthy,
		LastCheckedAt: now,
		LastSuccessAt: now,
		TTLSeconds:    int(mcp.DefaultHealthTTL.Seconds()),
		Capabilities:  mcpCapabilitiesForCounts(toolCount, resourceCount),
	}, true
}

func usesExternalMCPRuntime(transport string) bool {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "http", "stdio":
		return true
	default:
		return false
	}
}

func mcpCapabilitiesForCounts(toolCount, resourceCount int) []string {
	capabilities := make([]string, 0, 2)
	if toolCount > 0 {
		capabilities = append(capabilities, "tools")
	}
	if resourceCount > 0 {
		capabilities = append(capabilities, "resources")
	}
	if len(capabilities) == 0 {
		capabilities = append(capabilities, "builtin")
	}
	return capabilities
}

func runtimeHealthStatus(status mcp.HealthStatus, disabled bool) string {
	if disabled {
		return "unhealthy"
	}
	switch status {
	case mcp.HealthHealthy:
		return "healthy"
	case mcp.HealthDegraded:
		return "degraded"
	case mcp.HealthUnavailable, mcp.HealthDisabled:
		return "unhealthy"
	case mcp.HealthUnknown, "":
		return "unknown"
	default:
		return "unknown"
	}
}

func redactedMCPError(err error) string {
	if err == nil {
		return ""
	}
	return mcp.RedactHealthError(err.Error())
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
