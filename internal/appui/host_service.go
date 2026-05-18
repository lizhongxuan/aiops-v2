package appui

import (
	"context"
	"fmt"
	"strings"

	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

type defaultHostService struct {
	writer  SessionStore
	repo    HostRepository
	builder *SnapshotBuilder
}

func NewHostService(writer SessionStore, repo HostRepository, builder *SnapshotBuilder) HostService {
	return &defaultHostService{
		writer:  writer,
		repo:    repo,
		builder: builder,
	}
}

func (s *defaultHostService) ListHosts(context.Context) ([]HostSummary, error) {
	if s.builder == nil {
		return defaultStateSnapshot().Hosts, nil
	}
	return s.builder.buildHostSummaries(serverLocalHostID), nil
}

func (s *defaultHostService) CreateHost(_ context.Context, payload HostUpsert) (HostMutationResponse, error) {
	if s.repo == nil {
		return HostMutationResponse{}, fmt.Errorf("host repository is not configured")
	}
	record, err := buildNewHostRecord(payload)
	if err != nil {
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

func (s *defaultHostService) UpdateHost(_ context.Context, hostID string, payload HostUpsert) (HostMutationResponse, error) {
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
	if trimmed := strings.TrimSpace(payload.Name); trimmed != "" {
		updated.Name = trimmed
	}
	updated.Address = strings.TrimSpace(payload.Address)
	updated.SSHUser = strings.TrimSpace(payload.SSHUser)
	updated.SSHCredentialRef = strings.TrimSpace(payload.SSHCredentialRef)
	if trimmed := strings.TrimSpace(payload.AgentVersion); trimmed != "" {
		updated.AgentVersion = trimmed
	}
	if payload.SSHPort > 0 {
		updated.SSHPort = payload.SSHPort
	}
	updated.Labels = cloneStringMap(payload.Labels)
	if payload.InstallViaSSH {
		if updated.AgentVersion == "" {
			updated.AgentVersion = "v0.1.0"
		}
		updated.Transport = "ssh_bootstrap"
		updated.Status = "installing"
		updated.InstallState = "pending_install"
		updated.ControlMode = "managed"
	}
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
		Name:             strings.TrimSpace(firstNonEmpty(payload.Name, id)),
		Kind:             "inventory",
		Address:          strings.TrimSpace(payload.Address),
		Status:           "offline",
		Transport:        "inventory",
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
	if payload.InstallViaSSH {
		if record.AgentVersion == "" {
			record.AgentVersion = "v0.1.0"
		}
		record.Transport = "ssh_bootstrap"
		record.Status = "installing"
		record.InstallState = "pending_install"
		record.ControlMode = "managed"
		record.LastHeartbeat = ""
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
