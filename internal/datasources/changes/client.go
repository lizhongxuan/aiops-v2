package changes

import (
	"context"
	"strings"
	"time"
)

type Client interface {
	RecentDeployments(ctx context.Context, req Query) ([]Deployment, error)
	RecentConfigChanges(ctx context.Context, req Query) ([]ConfigChange, error)
}

type Query struct {
	Service     string
	Environment string
	TimeRange   string
	Limit       int
}

type Deployment struct {
	ID          string
	Service     string
	Environment string
	Version     string
	Actor       string
	CreatedAt   time.Time
}

type ConfigChange struct {
	ID          string
	Service     string
	Environment string
	Key         string
	Actor       string
	CreatedAt   time.Time
}

type Status string

const StatusUnavailable Status = "unavailable"

type Evidence struct {
	Status Status
	Source string
	Error  string
}

func UnavailableEvidence(source string, err error) Evidence {
	message := ""
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	return Evidence{
		Status: StatusUnavailable,
		Source: strings.TrimSpace(source),
		Error:  message,
	}
}
