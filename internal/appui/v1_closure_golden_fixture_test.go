package appui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	opsgraph "aiops-v2/internal/opsgraph"
)

func TestV1ClosurePGGoldenPathFixturesCoverOpsGraphAndReadonlyEvidence(t *testing.T) {
	graph, err := opsgraph.LoadSeedFile(filepath.Join("..", "..", "data", "opsgraph", "v1-closure-pg.seed.yaml"))
	if err != nil {
		t.Fatalf("LoadSeedFile() error = %v", err)
	}
	for _, id := range []string{"host.a", "host.b", "host.c", "pg.instance.a", "pg.instance.b", "monitor.pg_mon.c"} {
		if _, ok := graph.Entity(id); !ok {
			t.Fatalf("fixture opsgraph missing entity %q", id)
		}
	}
	neighborhood := graph.Neighborhood("cluster.pg.demo", 3)
	if len(neighborhood.Relationships) < 7 {
		t.Fatalf("opsgraph relationships = %#v, want PG instances and pg_mon observation relationships", neighborhood.Relationships)
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "v1-closure-pg-readonly-evidence.json"))
	if err != nil {
		t.Fatalf("ReadFile(evidence fixture) error = %v", err)
	}
	var fixture struct {
		Readonly     bool `json:"readonly"`
		EvidenceRefs []struct {
			ID      string         `json:"id"`
			Source  string         `json:"source"`
			HostID  string         `json:"hostId"`
			Covers  []string       `json:"covers"`
			Data    map[string]any `json:"data"`
			Summary string         `json:"summary"`
		} `json:"evidenceRefs"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("Unmarshal(evidence fixture) error = %v", err)
	}
	if !fixture.Readonly || len(fixture.EvidenceRefs) < 5 {
		t.Fatalf("fixture readonly/evidence count = %v/%d, want readonly with opsgraph + host A/B/C + network evidence", fixture.Readonly, len(fixture.EvidenceRefs))
	}
	for _, cover := range []string{"opsgraph", "role", "wal", "lsn", "replication_lag", "pg_mon", "network"} {
		if !fixtureCovers(fixture.EvidenceRefs, cover) {
			t.Fatalf("evidence fixture missing cover %q: %#v", cover, fixture.EvidenceRefs)
		}
	}
}

func fixtureCovers(items []struct {
	ID      string         `json:"id"`
	Source  string         `json:"source"`
	HostID  string         `json:"hostId"`
	Covers  []string       `json:"covers"`
	Data    map[string]any `json:"data"`
	Summary string         `json:"summary"`
}, cover string) bool {
	for _, item := range items {
		for _, candidate := range item.Covers {
			if candidate == cover {
				return true
			}
		}
	}
	return false
}
