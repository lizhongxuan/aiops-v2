package appui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	s.startInstallMonitor(next.ID, next.InstallRunID)
	run.HostID = next.ID
	run.WorkflowID = next.InstallWorkflowID
	run.AgentVersion = next.AgentVersion
	return run, nil
}

func (s *HostBootstrapService) startInstallMonitor(hostID, runID string) {
	if s == nil || s.runner == nil || s.repo == nil || strings.TrimSpace(runID) == "" {
		return
	}
	go s.monitorInstallRun(context.Background(), hostID, runID, 300, 2*time.Second)
}

func (s *HostBootstrapService) monitorInstallRun(ctx context.Context, hostID, runID string, attempts int, delay time.Duration) {
	if attempts <= 0 {
		attempts = 1
	}
	if delay <= 0 {
		delay = time.Second
	}
	for i := 0; i < attempts; i++ {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		run, err := s.runner.GetHostInstallRun(ctx, runID)
		if err != nil {
			continue
		}
		if !isTerminalInstallRunStatus(run.Status) {
			s.updateHostInstallProgress(hostID, run)
			continue
		}
		s.updateHostInstallProgress(hostID, run)
		return
	}
}

func (s *HostBootstrapService) updateHostInstallProgress(hostID string, run HostInstallRun) {
	host, err := s.repo.GetHost(strings.TrimSpace(hostID))
	if err != nil || host == nil {
		return
	}
	next := cloneHostRecord(*host)
	if step := strings.TrimSpace(run.CurrentStep); step != "" {
		next.InstallStep = step
	}
	if runID := strings.TrimSpace(run.RunID); runID != "" {
		next.InstallRunID = runID
	}
	if workflowID := strings.TrimSpace(run.WorkflowID); workflowID != "" {
		next.InstallWorkflowID = workflowID
	}
	switch strings.TrimSpace(run.Status) {
	case "success", "succeeded", "completed":
		if next.Status != "online" {
			next.Status = "online"
		}
		next.InstallState = "installed"
		next.ControlMode = "managed"
		next.LastError = ""
	case "failed", "error", "canceled", "cancelled":
		next.Status = "install_failed"
		next.InstallState = installFailureState(run)
		next.LastError = redactInstallError(firstNonEmpty(run.LastError, "host-agent install failed"))
	default:
		if next.Status != "online" {
			next.Status = "installing"
		}
		next.InstallState = "running"
	}
	_ = s.repo.SaveHost(&next)
}

func isTerminalInstallRunStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "success", "succeeded", "completed", "failed", "error", "canceled", "cancelled":
		return true
	default:
		return false
	}
}

func installFailureState(run HostInstallRun) string {
	if strings.TrimSpace(run.CurrentStep) == "detect-platform" {
		return "unsupported_platform"
	}
	return "failed"
}

func redactInstallError(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "host-agent install failed"
	}
	for _, marker := range []string{"BEGIN OPENSSH PRIVATE KEY", "BEGIN RSA PRIVATE KEY", "password=", "Authorization: Bearer "} {
		if strings.Contains(message, marker) {
			return "host-agent install failed; sensitive detail redacted"
		}
	}
	return message
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
		"agent_server_url":   firstNonEmpty(os.Getenv("AIOPS_AGENT_SERVER_URL"), host.AgentURL, "http://127.0.0.1:18080"),
		"aiops_api_url":      firstNonEmpty(os.Getenv("AIOPS_API_URL"), os.Getenv("AIOPS_AGENT_SERVER_URL"), "http://127.0.0.1:18080"),
		"agent_listen_port":  7072,
		"secret_dir":         defaultHostInstallSecretDir(),
		"repo_root":          defaultHostInstallRepoRoot(),
		"labels":             cloneStringMap(host.Labels),
	}
}

func defaultHostInstallSecretDir() string {
	if value := strings.TrimSpace(os.Getenv("AIOPS_SECRET_DIR")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("AIOPS_DATA_DIR")); value != "" {
		return filepath.Join(value, "secrets")
	}
	return filepath.Join(".data", "secrets")
}

func defaultHostInstallRepoRoot() string {
	if value := strings.TrimSpace(os.Getenv("AIOPS_REPO_ROOT")); value != "" {
		return value
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func hostInstallIdempotencyKey(hostID, agentVersion string) string {
	return "host-agent-install:" + strings.TrimSpace(hostID) + ":" + strings.TrimSpace(firstNonEmpty(agentVersion, "v0.1.0"))
}
