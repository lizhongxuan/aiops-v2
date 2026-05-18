package k8s

import (
	"context"
	"errors"
	"testing"
)

type fakeReadOnlyClient struct{}

func (fakeReadOnlyClient) GetWorkload(context.Context, WorkloadQuery) (Workload, error) {
	return Workload{Name: "checkout", Namespace: "prod"}, nil
}

func (fakeReadOnlyClient) GetEvents(context.Context, WorkloadQuery) ([]Event, error) {
	return []Event{{Reason: "Healthy"}}, nil
}

func (fakeReadOnlyClient) GetLogs(context.Context, LogQuery) ([]LogLine, error) {
	return []LogLine{{Message: "ok"}}, nil
}

func (fakeReadOnlyClient) RolloutStatus(context.Context, WorkloadQuery) (RolloutStatus, error) {
	return RolloutStatus{Status: "healthy"}, nil
}

func TestReadOnlyClientInterfaceAndUnavailableEvidence(t *testing.T) {
	var client ReadOnlyClient = fakeReadOnlyClient{}
	workload, err := client.GetWorkload(context.Background(), WorkloadQuery{Name: "checkout", Namespace: "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if workload.Name != "checkout" || workload.Namespace != "prod" {
		t.Fatalf("workload = %#v, want checkout/prod", workload)
	}

	evidence := UnavailableEvidence("kubernetes", errors.New("api unavailable"))
	if evidence.Status != StatusUnavailable || evidence.Source != "kubernetes" || evidence.Error == "" {
		t.Fatalf("unavailable evidence = %#v, want unavailable k8s evidence", evidence)
	}
}
