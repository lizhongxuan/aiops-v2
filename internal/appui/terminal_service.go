package appui

import (
	"context"
	"fmt"
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
	if manager == nil {
		manager = terminal.NewManager()
	}
	var hostRepo HostRepository
	if len(hosts) > 0 {
		hostRepo = hosts[0]
	}
	return &defaultTerminalService{manager: manager, hosts: hostRepo}
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
	return targetID, nil
}
