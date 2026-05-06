package appui

import "context"

type ChangeQueryCommand struct {
	Service     string `json:"service,omitempty"`
	Environment string `json:"environment,omitempty"`
	Window      string `json:"window,omitempty"`
}

type DeploymentView struct {
	ID          string `json:"id"`
	Service     string `json:"service"`
	Environment string `json:"environment"`
	Version     string `json:"version"`
	Actor       string `json:"actor"`
	StartedAt   string `json:"startedAt"`
	Status      string `json:"status"`
}

type ConfigChangeView struct {
	ID          string `json:"id"`
	Service     string `json:"service"`
	Environment string `json:"environment"`
	Key         string `json:"key"`
	Actor       string `json:"actor"`
	ChangedAt   string `json:"changedAt"`
	Summary     string `json:"summary"`
}

type ChangeContextService interface {
	RecentDeployments(ctx context.Context, cmd ChangeQueryCommand) ([]DeploymentView, error)
	RecentConfigChanges(ctx context.Context, cmd ChangeQueryCommand) ([]ConfigChangeView, error)
}

type defaultChangeContextService struct{}

func NewChangeContextService() ChangeContextService {
	return &defaultChangeContextService{}
}

func (s *defaultChangeContextService) RecentDeployments(_ context.Context, cmd ChangeQueryCommand) ([]DeploymentView, error) {
	return []DeploymentView{{
		ID:          "deploy-20260504-1",
		Service:     firstNonEmptyString(cmd.Service, "order-api"),
		Environment: firstNonEmptyString(cmd.Environment, "prod"),
		Version:     "2026.05.04-1",
		Actor:       "ci",
		StartedAt:   "2026-05-04T09:12:00Z",
		Status:      "completed",
	}}, nil
}

func (s *defaultChangeContextService) RecentConfigChanges(_ context.Context, cmd ChangeQueryCommand) ([]ConfigChangeView, error) {
	return []ConfigChangeView{{
		ID:          "cfg-20260504-1",
		Service:     firstNonEmptyString(cmd.Service, "order-api"),
		Environment: firstNonEmptyString(cmd.Environment, "prod"),
		Key:         "db.maxConnections",
		Actor:       "ops",
		ChangedAt:   "2026-05-04T08:47:00Z",
		Summary:     "raised connection pool limit",
	}}, nil
}
