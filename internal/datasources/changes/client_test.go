package changes

import (
	"context"
	"errors"
	"testing"
)

type fakeChangesClient struct{}

func (fakeChangesClient) RecentDeployments(context.Context, Query) ([]Deployment, error) {
	return []Deployment{{ID: "deploy-1", Service: "checkout"}}, nil
}

func (fakeChangesClient) RecentConfigChanges(context.Context, Query) ([]ConfigChange, error) {
	return []ConfigChange{{ID: "cfg-1", Service: "checkout"}}, nil
}

func TestClientInterfaceAndUnavailableEvidence(t *testing.T) {
	var client Client = fakeChangesClient{}
	deployments, err := client.RecentDeployments(context.Background(), Query{Service: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	if len(deployments) != 1 || deployments[0].Service != "checkout" {
		t.Fatalf("deployments = %#v, want checkout deployment", deployments)
	}

	evidence := UnavailableEvidence("changes", errors.New("cmdb unavailable"))
	if evidence.Status != StatusUnavailable || evidence.Source != "changes" || evidence.Error == "" {
		t.Fatalf("unavailable evidence = %#v, want unavailable changes evidence", evidence)
	}
}
