package appui

import (
	"context"
	"fmt"
	"strings"

	"aiops-v2/internal/store"
	"runner/workflow/visual"
)

type HostBootstrapRunner interface {
	SubmitHostInstallGraph(ctx context.Context, graph visual.Graph, vars map[string]any, idempotencyKey string) (HostInstallRun, error)
	GetHostInstallRun(ctx context.Context, runID string) (HostInstallRun, error)
}

type HostBootstrapService struct {
	repo   HostRepository
	runner HostBootstrapRunner
}

func NewHostBootstrapService(repo HostRepository, runner HostBootstrapRunner) *HostBootstrapService {
	return &HostBootstrapService{repo: repo, runner: runner}
}

func (s *HostBootstrapService) Install(ctx context.Context, hostID string, req HostInstallRequest) (HostInstallRun, error) {
	if s == nil || s.repo == nil {
		return HostInstallRun{}, fmt.Errorf("host bootstrap repository is not configured")
	}
	if s.runner == nil {
		return HostInstallRun{}, fmt.Errorf("runner runtime is not configured")
	}
	host, err := s.repo.GetHost(strings.TrimSpace(hostID))
	if err != nil {
		return HostInstallRun{}, err
	}
	if host == nil {
		return HostInstallRun{}, fmt.Errorf("host not found: %s", hostID)
	}
	next := cloneHostRecord(*host)
	if ref := strings.TrimSpace(req.SSHCredentialRef); ref != "" {
		next.SSHCredentialRef = ref
	}
	if version := strings.TrimSpace(req.AgentVersion); version != "" {
		next.AgentVersion = version
	}
	if next.AgentVersion == "" {
		next.AgentVersion = "v0.1.0"
	}
	if err := validateBootstrapHost(next); err != nil {
		next.Status = "install_failed"
		next.InstallState = "failed"
		next.LastError = err.Error()
		_ = s.repo.SaveHost(&next)
		return HostInstallRun{}, err
	}
	graph := BuiltinHostAgentInstallGraph()
	if err := ValidateHostAgentInstallGraph(graph); err != nil {
		next.Status = "install_failed"
		next.InstallState = "failed"
		next.LastError = err.Error()
		_ = s.repo.SaveHost(&next)
		return HostInstallRun{}, err
	}
	next.Transport = "ssh_bootstrap"
	next.Status = "installing"
	next.InstallState = "pending_install"
	next.InstallWorkflowID = BuiltinHostAgentInstallWorkflowID
	next.InstallStep = "validate-inputs"
	next.ControlMode = "managed"
	next.LastError = ""
	if err := s.repo.SaveHost(&next); err != nil {
		return HostInstallRun{}, err
	}

	run, err := s.runner.SubmitHostInstallGraph(ctx, graph, hostInstallVars(next), hostInstallIdempotencyKey(next.ID, next.AgentVersion))
	if err != nil {
		next.Status = "install_failed"
		next.InstallState = "failed"
		next.LastError = err.Error()
		next.InstallStep = firstNonEmpty(next.InstallStep, "submit-workflow")
		_ = s.repo.SaveHost(&next)
		return HostInstallRun{}, err
	}
	next.InstallRunID = run.RunID
	next.InstallWorkflowID = firstNonEmpty(run.WorkflowID, BuiltinHostAgentInstallWorkflowID)
	next.InstallState = "running"
	next.Status = "installing"
	next.InstallStep = firstNonEmpty(run.CurrentStep, next.InstallStep)
	if err := s.repo.SaveHost(&next); err != nil {
		return HostInstallRun{}, err
	}
	run.HostID = next.ID
	run.WorkflowID = next.InstallWorkflowID
	run.AgentVersion = next.AgentVersion
	return run, nil
}

func validateBootstrapHost(host store.HostRecord) error {
	if strings.TrimSpace(host.ID) == "" {
		return fmt.Errorf("host id is required")
	}
	if strings.TrimSpace(host.Address) == "" {
		return fmt.Errorf("host address is required")
	}
	if strings.TrimSpace(host.SSHUser) == "" {
		return fmt.Errorf("ssh user is required")
	}
	if strings.TrimSpace(host.SSHCredentialRef) == "" {
		return fmt.Errorf("ssh credential ref is required")
	}
	if host.SSHPort <= 0 {
		return fmt.Errorf("ssh port is required")
	}
	return nil
}

func hostInstallVars(host store.HostRecord) map[string]any {
	return map[string]any{
		"host_id":            host.ID,
		"ssh_host":           host.Address,
		"ssh_user":           host.SSHUser,
		"ssh_port":           host.SSHPort,
		"ssh_credential_ref": host.SSHCredentialRef,
		"agent_version":      host.AgentVersion,
		"agent_server_url":   firstNonEmpty(host.AgentURL, "http://127.0.0.1:18080"),
		"agent_listen_port":  7072,
		"labels":             cloneStringMap(host.Labels),
	}
}

func hostInstallIdempotencyKey(hostID, agentVersion string) string {
	return "host-agent-install:" + strings.TrimSpace(hostID) + ":" + strings.TrimSpace(firstNonEmpty(agentVersion, "v0.1.0"))
}
