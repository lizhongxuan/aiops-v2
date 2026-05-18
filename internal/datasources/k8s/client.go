package k8s

import (
	"context"
	"strings"
	"time"
)

type ReadOnlyClient interface {
	GetWorkload(ctx context.Context, req WorkloadQuery) (Workload, error)
	GetEvents(ctx context.Context, req WorkloadQuery) ([]Event, error)
	GetLogs(ctx context.Context, req LogQuery) ([]LogLine, error)
	RolloutStatus(ctx context.Context, req WorkloadQuery) (RolloutStatus, error)
}

type WorkloadQuery struct {
	Cluster   string
	Namespace string
	Kind      string
	Name      string
}

type LogQuery struct {
	WorkloadQuery
	Container string
	Since     time.Duration
	Limit     int
}

type Workload struct {
	Cluster   string
	Namespace string
	Kind      string
	Name      string
	Status    string
}

type Event struct {
	Type      string
	Reason    string
	Message   string
	Timestamp time.Time
}

type LogLine struct {
	Timestamp time.Time
	Message   string
}

type RolloutStatus struct {
	Status  string
	Message string
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
