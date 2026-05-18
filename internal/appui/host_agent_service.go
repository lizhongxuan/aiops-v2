package appui

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"aiops-v2/internal/store"
)

var ErrHostAgentUnauthorized = errors.New("host-agent token rejected")

type defaultHostAgentService struct {
	repo HostRepository
}

func NewHostAgentService(repo HostRepository) HostAgentService {
	return &defaultHostAgentService{repo: repo}
}

func (s *defaultHostAgentService) Register(ctx context.Context, req HostAgentRegisterRequest, token string) (HostAgentRegisterResponse, error) {
	if s.repo == nil {
		return HostAgentRegisterResponse{}, fmt.Errorf("host repository is not configured")
	}
	hostID := strings.TrimSpace(req.HostID)
	if hostID == "" {
		return HostAgentRegisterResponse{}, fmt.Errorf("host id is required")
	}
	host, err := s.repo.GetHost(hostID)
	if err != nil {
		return HostAgentRegisterResponse{}, err
	}
	bindToken := shouldBindHostAgentToken(*host, token)
	if !bindToken {
		if err := verifyHostAgentToken(host.AgentTokenRef, token); err != nil {
			return HostAgentRegisterResponse{}, err
		}
	}
	if bindToken && strings.TrimSpace(token) == "" {
		return HostAgentRegisterResponse{}, ErrHostAgentUnauthorized
	}
	next := cloneHostRecord(*host)
	if bindToken {
		next.AgentTokenRef = hostAgentTokenHashRef(token)
	}
	if name := strings.TrimSpace(req.Hostname); name != "" && strings.TrimSpace(next.Name) == "" {
		next.Name = name
	}
	next.Status = "online"
	next.InstallState = "installed"
	next.ControlMode = "managed"
	next.Transport = "agent_http"
	next.TerminalCapable = true
	next.Executable = true
	next.OS = strings.TrimSpace(req.OS)
	next.Arch = strings.TrimSpace(req.Arch)
	if version := strings.TrimSpace(req.AgentVersion); version != "" {
		next.AgentVersion = version
	}
	if agentURL := hostAgentURL(next.Address, req.ListenAddress); agentURL != "" {
		next.AgentURL = agentURL
	}
	next.LastHeartbeat = isoStamp(time.Now().UTC())
	next.LastError = ""
	next.Labels = mergeLabels(next.Labels, req.Labels)
	if err := s.repo.SaveHost(&next); err != nil {
		return HostAgentRegisterResponse{}, err
	}
	summary := mapHostRecord(next)
	return HostAgentRegisterResponse{
		Status:        summary.Status,
		HostID:        summary.ID,
		AgentURL:      summary.AgentURL,
		AgentVersion:  summary.AgentVersion,
		LastHeartbeat: summary.LastHeartbeat,
		Host:          summary,
	}, nil
}

func (s *defaultHostAgentService) Heartbeat(ctx context.Context, req HostAgentHeartbeatRequest, token string) (HostAgentHeartbeatResponse, error) {
	if s.repo == nil {
		return HostAgentHeartbeatResponse{}, fmt.Errorf("host repository is not configured")
	}
	hostID := strings.TrimSpace(req.HostID)
	if hostID == "" {
		return HostAgentHeartbeatResponse{}, fmt.Errorf("host id is required")
	}
	host, err := s.repo.GetHost(hostID)
	if err != nil {
		return HostAgentHeartbeatResponse{}, err
	}
	bindToken := shouldBindHostAgentToken(*host, token)
	if !bindToken {
		if err := verifyHostAgentToken(host.AgentTokenRef, token); err != nil {
			return HostAgentHeartbeatResponse{}, err
		}
	}
	if bindToken && strings.TrimSpace(token) == "" {
		return HostAgentHeartbeatResponse{}, ErrHostAgentUnauthorized
	}
	next := cloneHostRecord(*host)
	if bindToken {
		next.AgentTokenRef = hostAgentTokenHashRef(token)
	}
	next.Status = "online"
	next.LastHeartbeat = isoStamp(time.Now().UTC())
	next.LastError = ""
	if version := strings.TrimSpace(req.AgentVersion); version != "" {
		next.AgentVersion = version
	}
	if strings.TrimSpace(next.InstallState) == "" || next.InstallState == "running" || next.InstallState == "pending_install" {
		next.InstallState = "installed"
	}
	if strings.TrimSpace(next.ControlMode) == "" {
		next.ControlMode = "managed"
	}
	if err := s.repo.SaveHost(&next); err != nil {
		return HostAgentHeartbeatResponse{}, err
	}
	summary := mapHostRecord(next)
	return HostAgentHeartbeatResponse{
		Status:        summary.Status,
		HostID:        summary.ID,
		LastHeartbeat: summary.LastHeartbeat,
		Host:          summary,
	}, nil
}

func hostAgentTokenHashRef(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return fmt.Sprintf("sha256:%x", sum[:])
}

func verifyHostAgentToken(ref, token string) error {
	ref = strings.TrimSpace(ref)
	token = strings.TrimSpace(token)
	if ref == "" || token == "" {
		return ErrHostAgentUnauthorized
	}
	if strings.HasPrefix(strings.ToLower(ref), "sha256:") {
		want := strings.TrimPrefix(strings.ToLower(ref), "sha256:")
		got := strings.TrimPrefix(hostAgentTokenHashRef(token), "sha256:")
		if len(want) == len(got) && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1 {
			return nil
		}
		return ErrHostAgentUnauthorized
	}
	return fmt.Errorf("unsupported host-agent token ref")
}

func shouldBindHostAgentToken(host store.HostRecord, token string) bool {
	if strings.TrimSpace(host.AgentTokenRef) != "" || strings.TrimSpace(token) == "" {
		return false
	}
	status := strings.TrimSpace(host.Status)
	installState := strings.TrimSpace(host.InstallState)
	return status == "installing" || installState == "pending_install" || installState == "running"
}

func hostAgentURL(hostAddress, listenAddress string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(listenAddress))
	if err != nil || strings.TrimSpace(port) == "" {
		return ""
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" || host == "127.0.0.1" || strings.EqualFold(host, "localhost") {
		host = strings.TrimSpace(hostAddress)
	}
	if host == "" {
		return ""
	}
	return "http://" + net.JoinHostPort(host, port)
}

func mergeLabels(base, updates map[string]string) map[string]string {
	out := cloneStringMap(base)
	if len(updates) == 0 {
		return out
	}
	if out == nil {
		out = map[string]string{}
	}
	for key, value := range updates {
		if trimmed := strings.TrimSpace(key); trimmed != "" {
			out[trimmed] = strings.TrimSpace(value)
		}
	}
	return out
}
