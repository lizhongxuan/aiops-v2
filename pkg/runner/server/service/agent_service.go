package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
	"runner/logging"
	"runner/server/store/agentstore"
)

type AgentService struct {
	store        agentstore.Store
	offlineGrace time.Duration
	httpClient   *http.Client
}

func NewAgentService(store agentstore.Store, offlineGraceSec int) *AgentService {
	grace := time.Duration(offlineGraceSec) * time.Second
	if grace <= 0 {
		grace = 90 * time.Second
	}
	return &AgentService{
		store:        store,
		offlineGrace: grace,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *AgentService) List(ctx context.Context, filter AgentFilter) ([]*agentstore.AgentRecord, error) {
	items, err := s.store.List(ctx, agentstore.Filter{
		Status: filter.Status,
		Tag:    filter.Tag,
		Limit:  filter.Limit,
	})
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := make([]*agentstore.AgentRecord, 0, len(items))
	for _, item := range items {
		item.Status = s.effectiveStatus(item, now)
		cp := item
		out = append(out, &cp)
	}
	return out, nil
}

func (s *AgentService) Get(ctx context.Context, id string) (*agentstore.AgentRecord, error) {
	item, err := s.store.Get(ctx, id)
	if err != nil {
		if err == agentstore.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	item.Status = s.effectiveStatus(item, time.Now().UTC())
	return &item, nil
}

func (s *AgentService) Register(ctx context.Context, record *agentstore.AgentRecord) error {
	if record == nil {
		return fmt.Errorf("%w: empty agent record", ErrInvalid)
	}
	if strings.TrimSpace(record.ID) == "" {
		return fmt.Errorf("%w: id is required", ErrInvalid)
	}
	if strings.TrimSpace(record.Address) == "" {
		return fmt.Errorf("%w: address is required", ErrInvalid)
	}
	_, err := s.store.Create(ctx, *record)
	if err != nil {
		if err == agentstore.ErrExists {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (s *AgentService) Update(ctx context.Context, id string, record *agentstore.AgentRecord) error {
	if record == nil {
		return fmt.Errorf("%w: empty agent record", ErrInvalid)
	}
	_, err := s.store.Update(ctx, id, *record)
	if err != nil {
		if err == agentstore.ErrNotFound {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *AgentService) Delete(ctx context.Context, id string) error {
	err := s.store.Delete(ctx, id)
	if err == agentstore.ErrNotFound {
		return ErrNotFound
	}
	return err
}

func (s *AgentService) Heartbeat(ctx context.Context, id string, beat agentstore.Heartbeat) (*agentstore.AgentRecord, error) {
	item, err := s.store.Heartbeat(ctx, id, beat)
	if err != nil {
		if err == agentstore.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	item.Status = s.effectiveStatus(item, time.Now().UTC())
	return &item, nil
}

func (s *AgentService) Probe(ctx context.Context, id string) error {
	item, err := s.store.Get(ctx, id)
	if err != nil {
		if err == agentstore.ErrNotFound {
			return ErrNotFound
		}
		return err
	}
	url := strings.TrimRight(item.Address, "/") + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if token := strings.TrimSpace(item.Token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Runner-Token", token)
		req.Header.Set("X-Agent-Auth", token)
	}
	logging.L().Info("agent probe request auth",
		agentProbeAuthFields(item, req)...,
	)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		logging.L().Warn("agent probe transport failed",
			append(agentProbeAuthFields(item, req), zap.Error(err))...,
		)
		_, _ = s.store.Update(ctx, id, agentstore.AgentRecord{
			Status:    agentstore.StatusDegraded,
			LastError: err.Error(),
		})
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("probe failed: %s", resp.Status)
		logging.L().Warn("agent probe rejected",
			append(agentProbeAuthFields(item, req), zap.Int("status_code", resp.StatusCode), zap.String("status", resp.Status))...,
		)
		_, _ = s.store.Update(ctx, id, agentstore.AgentRecord{
			Status:    agentstore.StatusDegraded,
			LastError: err.Error(),
		})
		return err
	}
	_, err = s.store.Update(ctx, id, agentstore.AgentRecord{
		Status:    agentstore.StatusOnline,
		LastError: "",
	})
	return err
}

func (s *AgentService) Resolve(ctx context.Context, address string) (*agentstore.AgentRecord, error) {
	target := strings.TrimSpace(address)
	if target == "" {
		return nil, fmt.Errorf("%w: empty address", ErrInvalid)
	}
	lower := strings.ToLower(target)
	if lower == "local" || lower == "127.0.0.1" || strings.HasPrefix(lower, "localhost") {
		return &agentstore.AgentRecord{
			ID:      "local",
			Name:    "local",
			Address: "127.0.0.1",
			Status:  agentstore.StatusOnline,
		}, nil
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return &agentstore.AgentRecord{
			ID:      target,
			Name:    target,
			Address: target,
			Status:  agentstore.StatusOnline,
		}, nil
	}
	if !strings.HasPrefix(lower, "agent://") {
		return nil, fmt.Errorf("%w: unsupported address %q", ErrInvalid, address)
	}
	agentID := strings.TrimSpace(strings.TrimPrefix(target, "agent://"))
	item, err := s.store.Get(ctx, agentID)
	if err != nil {
		if err == agentstore.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	item.Status = s.effectiveStatus(item, time.Now().UTC())
	if item.Status != agentstore.StatusOnline {
		return nil, fmt.Errorf("%w: agent %s is %s", ErrUnavailable, item.ID, item.Status)
	}
	return &item, nil
}

func (s *AgentService) SupportsAction(agent *agentstore.AgentRecord, action string) bool {
	if agent == nil {
		return false
	}
	if len(agent.Capabilities) == 0 {
		return true
	}
	for _, capability := range agent.Capabilities {
		if strings.TrimSpace(capability) == strings.TrimSpace(action) {
			return true
		}
	}
	return false
}

func (s *AgentService) effectiveStatus(item agentstore.AgentRecord, now time.Time) string {
	status := strings.TrimSpace(item.Status)
	if status == "" {
		status = agentstore.StatusOnline
	}
	if item.LastBeatAt.IsZero() {
		return status
	}
	if now.Sub(item.LastBeatAt) > s.offlineGrace {
		return agentstore.StatusOffline
	}
	return status
}

func agentProbeAuthFields(item agentstore.AgentRecord, req *http.Request) []zap.Field {
	authHeader := ""
	runnerToken := ""
	agentAuth := ""
	url := ""
	if req != nil {
		authHeader = strings.TrimSpace(req.Header.Get("Authorization"))
		runnerToken = strings.TrimSpace(req.Header.Get("X-Runner-Token"))
		agentAuth = strings.TrimSpace(req.Header.Get("X-Agent-Auth"))
		if req.URL != nil {
			url = req.URL.String()
		}
	}
	token := strings.TrimSpace(item.Token)
	return []zap.Field{
		zap.String("agent_id", strings.TrimSpace(item.ID)),
		zap.String("agent_address", strings.TrimSpace(item.Address)),
		zap.String("url", url),
		zap.Bool("token_present", token != "" || authHeader != "" || runnerToken != "" || agentAuth != ""),
		zap.String("resolved_token", token),
		zap.String("authorization_header", authHeader),
		zap.String("authorization_token", trimBearerToken(authHeader)),
		zap.String("x_runner_token", runnerToken),
		zap.String("x_agent_auth", agentAuth),
	}
}

func trimBearerToken(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= len("Bearer ") && strings.EqualFold(value[:len("Bearer ")], "Bearer ") {
		return strings.TrimSpace(value[len("Bearer "):])
	}
	return value
}
