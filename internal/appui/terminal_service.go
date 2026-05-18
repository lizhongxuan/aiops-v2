package appui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"aiops-v2/internal/terminal"
)

type TerminalService interface {
	CreateSession(context.Context, TerminalCreateSessionCommand) (terminal.SessionMetadata, error)
	ListSessions(context.Context) (TerminalSessionListResponse, error)
}

type TerminalCreateSessionCommand struct {
	HostID string `json:"hostId,omitempty"`
	Cwd    string `json:"cwd,omitempty"`
	Shell  string `json:"shell,omitempty"`
	Cols   int    `json:"cols,omitempty"`
	Rows   int    `json:"rows,omitempty"`
}

type TerminalSessionListResponse struct {
	Sessions []terminal.SessionMetadata `json:"sessions"`
}

type defaultTerminalService struct {
	manager *terminal.Manager
	hosts   HostRepository
}

func NewTerminalService(manager *terminal.Manager, hosts ...HostRepository) TerminalService {
	return newTerminalService(manager, nil, hosts...)
}

func NewTerminalServiceWithCredentialResolver(manager *terminal.Manager, resolver CredentialResolver, hosts ...HostRepository) TerminalService {
	return newTerminalService(manager, resolver, hosts...)
}

func newTerminalService(manager *terminal.Manager, resolver CredentialResolver, hosts ...HostRepository) TerminalService {
	if manager == nil {
		manager = terminal.NewManager()
	}
	var hostRepo HostRepository
	if len(hosts) > 0 {
		hostRepo = hosts[0]
	}
	if hostRepo != nil && resolver != nil {
		manager.SetCommandFactory(NewHostSSHCommandFactory(hostRepo, resolver))
	}
	return &defaultTerminalService{manager: manager, hosts: hostRepo}
}

func NewHostSSHCommandFactory(hosts HostRepository, resolver CredentialResolver) terminal.CommandFactory {
	return func(req terminal.CreateSessionRequest) (*exec.Cmd, error) {
		hostID := strings.TrimSpace(firstNonEmpty(req.HostID, serverLocalHostID))
		if hostID == serverLocalHostID {
			return nil, nil
		}
		if hosts == nil {
			return nil, fmt.Errorf("host repository is not configured")
		}
		if resolver == nil {
			return nil, fmt.Errorf("ssh credential resolver is not configured")
		}
		host, err := hosts.GetHost(hostID)
		if err != nil {
			return nil, err
		}
		if host == nil {
			return nil, fmt.Errorf("host not found: %s", hostID)
		}
		ref := strings.TrimSpace(host.SSHCredentialRef)
		if ref == "" {
			return nil, fmt.Errorf("ssh credential ref is required")
		}
		credential, err := resolver.ResolveSSHCredential(context.Background(), ref)
		if err != nil {
			return nil, err
		}
		cmd, err := terminal.BuildSSHCommand(terminal.SSHCommandRequest{
			HostID:  host.ID,
			Address: host.Address,
			User:    host.SSHUser,
			Port:    host.SSHPort,
			Credential: terminal.SSHCredential{
				PrivateKeyPath: credential.PrivateKeyPath,
				Password:       credential.Password,
				Cleanup:        credential.Cleanup,
			},
		})
		if err != nil {
			if credential.Cleanup != nil {
				_ = credential.Cleanup()
			}
			return nil, err
		}
		return cmd, nil
	}
}

func (s *defaultTerminalService) CreateSession(ctx context.Context, req TerminalCreateSessionCommand) (terminal.SessionMetadata, error) {
	if s == nil || s.manager == nil {
		return terminal.SessionMetadata{}, fmt.Errorf("terminal manager is not configured")
	}
	hostID, err := s.validateTerminalHost(req.HostID)
	if err != nil {
		return terminal.SessionMetadata{}, err
	}
	return s.manager.CreateSession(ctx, terminal.CreateSessionRequest{
		HostID: hostID,
		Cwd:    req.Cwd,
		Shell:  req.Shell,
		Cols:   req.Cols,
		Rows:   req.Rows,
	})
}

func (s *defaultTerminalService) ListSessions(context.Context) (TerminalSessionListResponse, error) {
	if s == nil || s.manager == nil {
		return TerminalSessionListResponse{Sessions: []terminal.SessionMetadata{}}, nil
	}
	return TerminalSessionListResponse{Sessions: s.manager.ListSessions()}, nil
}

func (s *defaultTerminalService) validateTerminalHost(hostID string) (string, error) {
	targetID := strings.TrimSpace(firstNonEmpty(hostID, serverLocalHostID))
	if targetID == serverLocalHostID {
		return serverLocalHostID, nil
	}
	if s.hosts == nil {
		return targetID, nil
	}
	host, err := s.hosts.GetHost(targetID)
	if err != nil {
		return "", fmt.Errorf("host not found: %s", targetID)
	}
	if host == nil {
		return "", fmt.Errorf("host not found: %s", targetID)
	}
	if !strings.EqualFold(strings.TrimSpace(host.Status), "online") {
		return "", fmt.Errorf("host %s is %s", targetID, firstNonEmpty(host.Status, "offline"))
	}
	if !host.TerminalCapable && !host.Executable {
		return "", fmt.Errorf("terminal is not enabled for host %s", targetID)
	}
	if strings.TrimSpace(host.SSHCredentialRef) == "" {
		return "", fmt.Errorf("ssh credential ref is required for host %s", targetID)
	}
	return targetID, nil
}
