package appui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

type defaultHostService struct {
	writer           SessionStore
	repo             HostRepository
	builder          *SnapshotBuilder
	bootstrap        *HostBootstrapService
	sshPasswordStore HostSSHPasswordStore
}

type HostServiceOption func(*defaultHostService)

func NewHostService(writer SessionStore, repo HostRepository, builder *SnapshotBuilder, bootstrap ...*HostBootstrapService) HostService {
	var bootstrapSvc *HostBootstrapService
	if len(bootstrap) > 0 {
		bootstrapSvc = bootstrap[0]
	}
	return NewHostServiceWithOptions(writer, repo, builder, bootstrapSvc)
}

func NewHostServiceWithOptions(writer SessionStore, repo HostRepository, builder *SnapshotBuilder, bootstrap *HostBootstrapService, opts ...HostServiceOption) HostService {
	service := &defaultHostService{
		writer:    writer,
		repo:      repo,
		builder:   builder,
		bootstrap: bootstrap,
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

func (s *defaultHostService) ListHosts(context.Context) ([]HostSummary, error) {
	if s.builder == nil {
		return defaultStateSnapshot().Hosts, nil
	}
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
			SSHCredentialRef: updated.SSHCredentialRef,
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
	if s.bootstrap != nil {
		return s.bootstrap.TestSSH(ctx, targetID, payload)
	}
	return HostSSHTestResponse{
		Status:  "ok",
		OS:      host.OS,
		Arch:    host.Arch,
		Message: "SSH preflight input accepted",
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
		Transport:        "manual",
		Labels:           cloneStringMap(payload.Labels),
		SSHUser:          strings.TrimSpace(payload.SSHUser),
		SSHPort:          payload.SSHPort,
		SSHCredentialRef: strings.TrimSpace(payload.SSHCredentialRef),
		AgentVersion:     strings.TrimSpace(payload.AgentVersion),
		InstallState:     "inventory",
		ControlMode:      "inventory",
		LastHeartbeat:    "offline",
	}
	if record.SSHPort == 0 {
		record.SSHPort = 22
	}
	return record, nil
}

func mapHostRecord(record store.HostRecord) HostSummary {
	return HostSummary{
		ID:                record.ID,
		Name:              firstNonEmpty(record.Name, record.ID),
		Status:            firstNonEmpty(record.Status, "offline"),
		Kind:              record.Kind,
		Address:           record.Address,
		Transport:         record.Transport,
		Executable:        record.Executable,
		TerminalCapable:   record.TerminalCapable,
		OS:                record.OS,
		Arch:              record.Arch,
		OSRelease:         record.OSRelease,
		KernelVersion:     record.KernelVersion,
		CPUCores:          record.CPUCores,
		MemoryBytes:       record.MemoryBytes,
		AgentVersion:      record.AgentVersion,
		LastHeartbeat:     record.LastHeartbeat,
		Labels:            cloneStringMap(record.Labels),
		LastError:         record.LastError,
		SSHUser:           record.SSHUser,
		SSHPort:           record.SSHPort,
		SSHCredentialRef:  record.SSHCredentialRef,
		AgentURL:          record.AgentURL,
		AgentTokenRef:     record.AgentTokenRef,
		InstallState:      record.InstallState,
		InstallRunID:      record.InstallRunID,
		InstallWorkflowID: record.InstallWorkflowID,
		InstallStep:       record.InstallStep,
		ControlMode:       record.ControlMode,
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
