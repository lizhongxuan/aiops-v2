package appui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

const hostAgentHeartbeatStaleAfter = time.Minute

type defaultHostService struct {
	writer           SessionStore
	repo             HostRepository
	builder          *SnapshotBuilder
	bootstrap        *HostBootstrapService
	sshPasswordStore HostSSHPasswordStore
	agentTokenStore  HostAgentTokenStore
	nodeHealthChecker HostNodeHealthChecker
}

type HostServiceOption func(*defaultHostService)

type HostNodeHealth struct {
	Status        string
	LastHeartbeat string
	AgentVersion  string
	Capabilities  []string
}

type HostNodeHealthChecker interface {
	CheckHostNodeHealth(ctx context.Context, host store.HostRecord) (HostNodeHealth, error)
}

func NewHostService(writer SessionStore, repo HostRepository, builder *SnapshotBuilder, bootstrap ...*HostBootstrapService) HostService {
	var bootstrapSvc *HostBootstrapService
	if len(bootstrap) > 0 {
		bootstrapSvc = bootstrap[0]
	}
	return NewHostServiceWithOptions(writer, repo, builder, bootstrapSvc)
}

func NewHostServiceWithOptions(writer SessionStore, repo HostRepository, builder *SnapshotBuilder, bootstrap *HostBootstrapService, opts ...HostServiceOption) HostService {
	service := &defaultHostService{
		writer:            writer,
		repo:              repo,
		builder:           builder,
		bootstrap:         bootstrap,
		nodeHealthChecker: newHTTPHostNodeHealthChecker(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func WithHostServiceSSHPasswordStore(store HostSSHPasswordStore) HostServiceOption {
	return func(service *defaultHostService) {
		service.sshPasswordStore = store
	}
}

func WithHostServiceHostAgentTokenStore(store HostAgentTokenStore) HostServiceOption {
	return func(service *defaultHostService) {
		service.agentTokenStore = store
	}
}

func WithHostServiceNodeHealthChecker(checker HostNodeHealthChecker) HostServiceOption {
	return func(service *defaultHostService) {
		service.nodeHealthChecker = checker
	}
}

func (s *defaultHostService) ListHosts(ctx context.Context) ([]HostSummary, error) {
	if s.builder == nil {
		return defaultStateSnapshot().Hosts, nil
	}
	s.refreshAIOPSPullHostHealth(ctx)
	return s.builder.buildHostSummaries(serverLocalHostID), nil
}

func (s *defaultHostService) CreateHost(ctx context.Context, payload HostUpsert) (HostMutationResponse, error) {
	if s.repo == nil {
		return HostMutationResponse{}, fmt.Errorf("host repository is not configured")
	}
	id, err := s.resolveCreateHostID(payload)
	if err != nil {
		return HostMutationResponse{}, err
	}
	if existing, _ := s.repo.GetHost(id); existing != nil {
		return HostMutationResponse{}, fmt.Errorf("host ID already exists: %s", id)
	}
	payload.ID = id
	record, err := buildNewHostRecord(payload)
	if err != nil {
		return HostMutationResponse{}, err
	}
	if err := s.ensureHostNameUnique(record.Name, record.ID); err != nil {
		return HostMutationResponse{}, err
	}
	if err := s.applySSHPassword(ctx, record.ID, payload.SSHPassword, &record.SSHCredentialRef); err != nil {
		return HostMutationResponse{}, err
	}
	if err := s.repo.SaveHost(record); err != nil {
		return HostMutationResponse{}, err
	}
	items, _ := s.ListHosts(context.Background())
	return HostMutationResponse{
		Host:              mapHostRecord(*record),
		Items:             items,
		InstallRunID:      record.InstallRunID,
		InstallWorkflowID: record.InstallWorkflowID,
	}, nil
}

func (s *defaultHostService) UpdateHost(ctx context.Context, hostID string, payload HostUpsert) (HostMutationResponse, error) {
	if s.repo == nil {
		return HostMutationResponse{}, fmt.Errorf("host repository is not configured")
	}
	targetID := strings.TrimSpace(firstNonEmpty(hostID, payload.ID))
	if targetID == "" {
		return HostMutationResponse{}, fmt.Errorf("host id is required")
	}
	if targetID == serverLocalHostID {
		return HostMutationResponse{}, fmt.Errorf("server-local cannot be edited")
	}
	current, err := s.repo.GetHost(targetID)
	if err != nil {
		return HostMutationResponse{}, err
	}
	updated := cloneHostRecord(*current)
	updated.Name = strings.TrimSpace(payload.Name)
	updated.Address = strings.TrimSpace(payload.Address)
	updated.SSHUser = strings.TrimSpace(payload.SSHUser)
	if agentURL := strings.TrimRight(strings.TrimSpace(payload.AgentURL), "/"); agentURL != "" {
		updated.AgentURL = agentURL
	}
	updated.AgentServerURL = strings.TrimSpace(payload.AgentServerURL)
	if mode := strings.TrimSpace(payload.ConnectionMode); mode != "" {
		updated.ConnectionMode = NormalizeHostConnectionMode(mode)
	} else {
		updated.ConnectionMode = NormalizeHostConnectionMode(updated.ConnectionMode)
	}
	if ref := strings.TrimSpace(payload.SSHCredentialRef); ref != "" {
		updated.SSHCredentialRef = ref
	}
	if err := s.ensureHostNameUnique(updated.Name, updated.ID); err != nil {
		return HostMutationResponse{}, err
	}
	if err := s.applySSHPassword(ctx, updated.ID, payload.SSHPassword, &updated.SSHCredentialRef); err != nil {
		return HostMutationResponse{}, err
	}
	if trimmed := strings.TrimSpace(payload.AgentVersion); trimmed != "" {
		updated.AgentVersion = trimmed
	}
	if payload.SSHPort > 0 {
		updated.SSHPort = payload.SSHPort
	}
	updated.Labels = cloneStringMap(payload.Labels)
	if err := s.repo.SaveHost(&updated); err != nil {
		return HostMutationResponse{}, err
	}
	items, _ := s.ListHosts(context.Background())
	return HostMutationResponse{
		Host:              mapHostRecord(updated),
		Items:             items,
		InstallRunID:      updated.InstallRunID,
		InstallWorkflowID: updated.InstallWorkflowID,
	}, nil
}

func (s *defaultHostService) InstallHost(ctx context.Context, hostID string, payload HostInstallRequest) (HostMutationResponse, error) {
	if s.repo == nil {
		return HostMutationResponse{}, fmt.Errorf("host repository is not configured")
	}
	targetID := strings.TrimSpace(hostID)
	if targetID == "" {
		return HostMutationResponse{}, fmt.Errorf("host id is required")
	}
	current, err := s.repo.GetHost(targetID)
	if err != nil {
		return HostMutationResponse{}, err
	}
	updated := cloneHostRecord(*current)
	if ref := strings.TrimSpace(payload.SSHCredentialRef); ref != "" {
		updated.SSHCredentialRef = ref
	}
	if err := s.applySSHPassword(ctx, updated.ID, payload.SSHPassword, &updated.SSHCredentialRef); err != nil {
		return HostMutationResponse{}, err
	}
	if version := strings.TrimSpace(payload.AgentVersion); version != "" {
		updated.AgentVersion = version
	}
	updated.ConnectionMode = NormalizeHostConnectionMode(firstNonEmpty(payload.ConnectionMode, updated.ConnectionMode))
	installAgentServerURL := ""
	if hostConnectionModeRequiresCallback(updated.ConnectionMode) {
		if serverURL := strings.TrimSpace(payload.AgentServerURL); serverURL != "" {
			updated.AgentServerURL = serverURL
		}
		installAgentServerURL = updated.AgentServerURL
	}
	if updated.AgentVersion == "" {
		updated.AgentVersion = "v0.1.0"
	}
	updated.Transport = "ssh_bootstrap"
	updated.Status = "installing"
	updated.InstallState = "pending_install"
	updated.InstallWorkflowID = BuiltinHostAgentInstallWorkflowID
	updated.InstallStep = "validate-inputs"
	updated.ControlMode = "managed"
	updated.LastError = ""
	if err := s.repo.SaveHost(&updated); err != nil {
		return HostMutationResponse{}, err
	}
	if s.bootstrap != nil {
		run, err := s.bootstrap.Install(ctx, updated.ID, HostInstallRequest{
			AgentVersion:     updated.AgentVersion,
			ConnectionMode:   updated.ConnectionMode,
			SSHCredentialRef: updated.SSHCredentialRef,
			AgentServerURL:   installAgentServerURL,
			Force:            payload.Force,
		})
		if err != nil {
			return HostMutationResponse{}, err
		}
		updated.InstallRunID = run.RunID
		updated.InstallWorkflowID = run.WorkflowID
	}
	items, _ := s.ListHosts(context.Background())
	return HostMutationResponse{
		Host:              mapHostRecord(updated),
		Items:             items,
		InstallRunID:      updated.InstallRunID,
		InstallWorkflowID: updated.InstallWorkflowID,
	}, nil
}

func (s *defaultHostService) TestHostSSH(ctx context.Context, hostID string, payload HostSSHTestRequest) (HostSSHTestResponse, error) {
	if s.repo == nil {
		return HostSSHTestResponse{}, fmt.Errorf("host repository is not configured")
	}
	targetID := strings.TrimSpace(hostID)
	if targetID == "" {
		return HostSSHTestResponse{}, fmt.Errorf("host id is required")
	}
	host, err := s.repo.GetHost(targetID)
	if err != nil {
		return HostSSHTestResponse{}, err
	}
	if strings.TrimSpace(host.Address) == "" {
		return HostSSHTestResponse{}, fmt.Errorf("host address is required")
	}
	if strings.TrimSpace(host.SSHUser) == "" {
		return HostSSHTestResponse{}, fmt.Errorf("ssh user is required")
	}
	nextReq := payload
	if err := s.applySSHPassword(ctx, host.ID, payload.SSHPassword, &nextReq.SSHCredentialRef); err != nil {
		return HostSSHTestResponse{}, err
	}
	nextReq.SSHPassword = ""
	var resp HostSSHTestResponse
	if s.bootstrap != nil {
		resp, err = s.bootstrap.TestSSH(ctx, targetID, nextReq)
	} else {
		resp = HostSSHTestResponse{
			Status:  "ok",
			OS:      host.OS,
			Arch:    host.Arch,
			Message: "SSH preflight input accepted",
		}
	}
	if err != nil {
		s.saveHostSSHTestState(host, nextReq.SSHCredentialRef, "failed", err.Error())
		return HostSSHTestResponse{}, err
	}
	s.saveHostSSHTestState(host, nextReq.SSHCredentialRef, firstNonEmpty(resp.Status, "ok"), "")
	return resp, nil
}

func (s *defaultHostService) DiagnoseHostNode(ctx context.Context, hostID string) (HostNodeDiagnosticsResponse, error) {
	if s.repo == nil {
		return HostNodeDiagnosticsResponse{}, fmt.Errorf("host repository is not configured")
	}
	targetID := strings.TrimSpace(hostID)
	if targetID == "" {
		return HostNodeDiagnosticsResponse{}, fmt.Errorf("host id is required")
	}
	host, err := s.repo.GetHost(targetID)
	if err != nil {
		return HostNodeDiagnosticsResponse{}, err
	}
	agentURL := strings.TrimRight(strings.TrimSpace(host.AgentURL), "/")
	if agentURL == "" {
		return HostNodeDiagnosticsResponse{}, fmt.Errorf("host %s has no Node agent URL", targetID)
	}
	if s.agentTokenStore == nil {
		return HostNodeDiagnosticsResponse{}, fmt.Errorf("host-agent token store is not configured")
	}
	ref := strings.TrimSpace(host.AgentTokenSecretRef)
	if ref == "" {
		return HostNodeDiagnosticsResponse{}, fmt.Errorf("host %s has no Node token secret", targetID)
	}
	token, err := s.agentTokenStore.ResolveHostAgentToken(ctx, ref)
	if err != nil {
		return HostNodeDiagnosticsResponse{}, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, agentURL+"/diagnostics", nil)
	if err != nil {
		return HostNodeDiagnosticsResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return HostNodeDiagnosticsResponse{}, fmt.Errorf("request Node diagnostics: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return HostNodeDiagnosticsResponse{}, fmt.Errorf("Node diagnostics returned status %d", resp.StatusCode)
	}
	var diagnostics map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&diagnostics); err != nil {
		return HostNodeDiagnosticsResponse{}, fmt.Errorf("decode Node diagnostics: %w", err)
	}
	return HostNodeDiagnosticsResponse{
		Status:      "ok",
		HostID:      host.ID,
		AgentURL:    agentURL,
		Diagnostics: diagnostics,
	}, nil
}

func (s *defaultHostService) DeleteHost(_ context.Context, hostID string) error {
	if s.repo == nil {
		return fmt.Errorf("host repository is not configured")
	}
	targetID := strings.TrimSpace(hostID)
	if targetID == "" {
		return fmt.Errorf("host id is required")
	}
	if targetID == serverLocalHostID {
		return fmt.Errorf("server-local cannot be deleted")
	}
	return s.repo.DeleteHost(targetID)
}

func (s *defaultHostService) SelectHost(_ context.Context, hostID string) (StateSnapshot, error) {
	targetID := strings.TrimSpace(firstNonEmpty(hostID, serverLocalHostID))
	if targetID != serverLocalHostID && s.repo != nil {
		if _, err := s.repo.GetHost(targetID); err != nil {
			return StateSnapshot{}, err
		}
	}
	if s.writer == nil {
		return s.builder.BuildStateSnapshot(nil), nil
	}
	active := s.writer.GetLatest()
	if active == nil {
		active = s.writer.GetOrCreate("", runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
	}
	active.HostID = targetID
	s.writer.Update(active)
	return s.builder.BuildStateSnapshot(active), nil
}

func (s *defaultHostService) applySSHPassword(ctx context.Context, hostID, password string, credentialRef *string) error {
	if strings.TrimSpace(password) == "" {
		return nil
	}
	if s.sshPasswordStore == nil {
		return fmt.Errorf("ssh password store is not configured")
	}
	ref, err := s.sshPasswordStore.StoreHostSSHPassword(ctx, hostID, password)
	if err != nil {
		return err
	}
	*credentialRef = ref
	return nil
}

func (s *defaultHostService) saveHostSSHTestState(host *store.HostRecord, credentialRef, status, lastError string) {
	if s == nil || s.repo == nil || host == nil {
		return
	}
	next := cloneHostRecord(*host)
	if ref := strings.TrimSpace(credentialRef); ref != "" {
		next.SSHCredentialRef = ref
	}
	next.AgentStatus = hostAgentStatus(next)
	next.SSHStatus = normalizeHostSSHStatus(status)
	next.RuntimeReachability = deriveHostRuntimeReachability(next.AgentStatus, next.SSHStatus, next)
	if next.SSHStatus == "ok" {
		next.LastError = ""
	} else if trimmed := strings.TrimSpace(lastError); trimmed != "" {
		next.LastError = trimmed
	}
	_ = s.repo.SaveHost(&next)
}

func (s *defaultHostService) resolveCreateHostID(payload HostUpsert) (string, error) {
	if id := strings.TrimSpace(payload.ID); id != "" {
		return id, nil
	}
	if s.repo == nil {
		return "", fmt.Errorf("host repository is not configured")
	}
	items, err := s.repo.ListHosts()
	if err != nil {
		return "", err
	}
	existing := make(map[string]bool, len(items)+1)
	existing[serverLocalHostID] = true
	for _, item := range items {
		if id := strings.TrimSpace(item.ID); id != "" {
			existing[id] = true
		}
	}
	base := safeSecretPathSegment(firstNonEmpty(payload.Name, payload.Address, payload.SSHUser, "host"))
	if base == "" {
		base = "host"
	}
	seed := fmt.Sprintf("%s|%s|%s|%d", strings.TrimSpace(payload.Name), strings.TrimSpace(payload.Address), strings.TrimSpace(payload.SSHUser), payload.SSHPort)
	sum := sha256.Sum256([]byte(seed))
	prefix := "host-" + base + "-" + hex.EncodeToString(sum[:4])
	candidate := prefix
	for index := 2; existing[candidate]; index++ {
		candidate = fmt.Sprintf("%s-%d", prefix, index)
	}
	return candidate, nil
}

func (s *defaultHostService) ensureHostNameUnique(name, excludeID string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil
	}
	if s.repo == nil {
		return fmt.Errorf("host repository is not configured")
	}
	items, err := s.repo.ListHosts()
	if err != nil {
		return err
	}
	excludeID = strings.TrimSpace(excludeID)
	for _, item := range items {
		if strings.TrimSpace(item.ID) == excludeID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.Name), trimmed) {
			return fmt.Errorf("host name must be unique: %s", trimmed)
		}
	}
	return nil
}

func buildNewHostRecord(payload HostUpsert) (*store.HostRecord, error) {
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		return nil, fmt.Errorf("host id is required")
	}
	if id == serverLocalHostID {
		return nil, fmt.Errorf("server-local is reserved")
	}
	record := &store.HostRecord{
		ID:               id,
		Name:             strings.TrimSpace(payload.Name),
		Kind:             "inventory",
		Address:          strings.TrimSpace(payload.Address),
		Status:           "offline",
		AgentStatus:      "offline",
		SSHStatus:        "unknown",
		Transport:        "manual",
		Labels:           cloneStringMap(payload.Labels),
		SSHUser:          strings.TrimSpace(payload.SSHUser),
		SSHPort:          payload.SSHPort,
		SSHCredentialRef: strings.TrimSpace(payload.SSHCredentialRef),
		AgentVersion:     strings.TrimSpace(payload.AgentVersion),
		ConnectionMode:   NormalizeHostConnectionMode(payload.ConnectionMode),
		AgentURL:         strings.TrimRight(strings.TrimSpace(payload.AgentURL), "/"),
		AgentServerURL:   strings.TrimSpace(payload.AgentServerURL),
		InstallState:     "inventory",
		ControlMode:      "inventory",
		LastHeartbeat:    "offline",
	}
	if record.SSHPort == 0 {
		record.SSHPort = 22
	}
	record.RuntimeReachability = deriveHostRuntimeReachability(record.AgentStatus, record.SSHStatus, *record)
	return record, nil
}

func mapHostRecord(record store.HostRecord) HostSummary {
	agentStatus := hostAgentStatus(record)
	sshStatus := hostSSHStatus(record)
	runtimeReachability := hostRuntimeReachability(record)
	status := firstNonEmpty(record.Status, agentStatus, "offline")
	if agentStatus == "stale" && isOnlineHostStatus(status) {
		status = "stale"
	}
	return HostSummary{
		ID:                  record.ID,
		Name:                firstNonEmpty(record.Name, record.ID),
		Status:              status,
		AgentStatus:         agentStatus,
		SSHStatus:           sshStatus,
		RuntimeReachability: runtimeReachability,
		Kind:                record.Kind,
		Address:             record.Address,
		Transport:           record.Transport,
		ConnectionMode:      NormalizeHostConnectionMode(record.ConnectionMode),
		Executable:          record.Executable,
		TerminalCapable:     record.TerminalCapable,
		OS:                  record.OS,
		Arch:                record.Arch,
		OSRelease:           record.OSRelease,
		KernelVersion:       record.KernelVersion,
		CPUCores:            record.CPUCores,
		MemoryBytes:         record.MemoryBytes,
		AgentVersion:        record.AgentVersion,
		LastHeartbeat:       record.LastHeartbeat,
		Labels:              cloneStringMap(record.Labels),
		LastError:           record.LastError,
		SSHUser:             record.SSHUser,
		SSHPort:             record.SSHPort,
		SSHCredentialRef:    record.SSHCredentialRef,
		AgentURL:            record.AgentURL,
		AgentServerURL:      record.AgentServerURL,
		AgentTokenRef:       record.AgentTokenRef,
		InstallState:        record.InstallState,
		InstallRunID:        record.InstallRunID,
		InstallWorkflowID:   record.InstallWorkflowID,
		InstallStep:         record.InstallStep,
		ControlMode:         record.ControlMode,
	}
}

func hostAgentStatus(record store.HostRecord) string {
	status := firstNonEmpty(record.AgentStatus, record.Status, "offline")
	if hostAgentHeartbeatIsStale(record, status) {
		return "stale"
	}
	return status
}

func hostSSHStatus(record store.HostRecord) string {
	if status := normalizeHostSSHStatus(record.SSHStatus); status != "" {
		return status
	}
	if strings.TrimSpace(record.Address) == "" || strings.TrimSpace(record.SSHUser) == "" {
		return "not_configured"
	}
	return "unknown"
}

func hostRuntimeReachability(record store.HostRecord) string {
	agentStatus := hostAgentStatus(record)
	stored := strings.ToLower(strings.TrimSpace(record.RuntimeReachability))
	if agentStatus == "stale" && (stored == "" || stored == "agent_online") {
		return "agent_stale"
	}
	return firstNonEmpty(record.RuntimeReachability, deriveHostRuntimeReachability(agentStatus, hostSSHStatus(record), record))
}

func normalizeHostSSHStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ok", "ready", "available", "connected", "success":
		return "ok"
	case "failed", "error", "unavailable", "denied", "timeout":
		return "failed"
	case "not_configured":
		return "not_configured"
	case "unknown", "pending", "untested":
		return "unknown"
	case "":
		return ""
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func deriveHostRuntimeReachability(agentStatus, sshStatus string, record store.HostRecord) string {
	switch strings.ToLower(strings.TrimSpace(agentStatus)) {
	case "online", "ready", "healthy":
		return "agent_online"
	case "stale", "timeout":
		return "agent_stale"
	case "installing", "pending_install":
		return "installing"
	}
	switch strings.ToLower(strings.TrimSpace(sshStatus)) {
	case "ok":
		return "ssh_available"
	case "failed":
		return "ssh_failed"
	case "not_configured":
		return "inventory_only"
	}
	if strings.TrimSpace(record.Address) != "" && strings.TrimSpace(record.SSHUser) != "" {
		return "ssh_unverified"
	}
	return "inventory_only"
}

func hostAgentHeartbeatIsStale(record store.HostRecord, status string) bool {
	if !isOnlineHostStatus(status) {
		return false
	}
	timestamp, err := time.Parse(time.RFC3339, strings.TrimSpace(record.LastHeartbeat))
	if err != nil || timestamp.IsZero() {
		return false
	}
	return time.Since(timestamp.UTC()) > hostAgentHeartbeatStaleAfter
}

func isOnlineHostStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "online", "ready", "healthy":
		return true
	default:
		return false
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		if trimmed := strings.TrimSpace(key); trimmed != "" {
			out[trimmed] = strings.TrimSpace(value)
		}
	}
	return out
}

func cloneHostRecord(record store.HostRecord) store.HostRecord {
	record.Labels = cloneStringMap(record.Labels)
	return record
}
