package opsgraph

import (
	"path/filepath"
	"testing"
)

func TestLoadSeedCoversCoreERPCapabilities(t *testing.T) {
	store, err := LoadSeedFile(filepath.Join("..", "..", "data", "opsgraph", "erp.seed.yaml"))
	if err != nil {
		t.Fatalf("LoadSeedFile() error = %v", err)
	}
	for _, query := range []string{"订单提交", "库存扣减", "报表任务"} {
		matches := store.Lookup(LookupRequest{Query: query})
		if len(matches) == 0 {
			t.Fatalf("Lookup(%q) returned no matches", query)
		}
	}
}

func TestNeighborhoodImpactAndRunbooks(t *testing.T) {
	store, err := LoadSeedFile(filepath.Join("..", "..", "data", "opsgraph", "erp.seed.yaml"))
	if err != nil {
		t.Fatalf("LoadSeedFile() error = %v", err)
	}
	neighbors := store.Neighborhood("capability.order.submit", 2)
	if len(neighbors.Entities) < 4 || len(neighbors.Relationships) == 0 {
		t.Fatalf("Neighborhood() = %#v, want connected entities and relationships", neighbors)
	}
	impact := store.BusinessImpact("service.order-api")
	if len(impact.Modules) == 0 || len(impact.Capabilities) == 0 || len(impact.Tenants) == 0 {
		t.Fatalf("BusinessImpact(service.order-api) = %#v, want modules/capabilities/tenants", impact)
	}
	runbooks := store.RelatedRunbooks("capability.order.submit")
	if len(runbooks) == 0 {
		t.Fatal("RelatedRunbooks(capability.order.submit) returned no runbooks")
	}
	if runbooks[0].Runbook.ID == "" || runbooks[0].Reason == "" {
		t.Fatalf("runbook match = %#v, want id and reason", runbooks[0])
	}
}
