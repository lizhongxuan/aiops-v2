package agents

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentIndexOmitsPromptBody(t *testing.T) {
	defs := []Definition{
		{
			Kind:        "explorer",
			Name:        "synthetic-explorer",
			Description: "collects read-only evidence",
			Prompt:      "SECRET_PROMPT_BODY_SHOULD_NOT_APPEAR",
			Discovery: AgentDiscoveryMetadata{
				WhenToUse:       strings.Repeat("a", 400),
				CapabilityKinds: []string{"evidence"},
				ResourceTypes:   []string{"logs"},
				OperationKinds:  []string{"read"},
				Modes:           []string{"explore"},
				ModelInvocable:  true,
			},
			Budget: AgentBudgetMetadata{MaxConcurrent: 2, CostClass: "low"},
		},
	}

	result := BuildAgentIndex(defs, AgentIndexOptions{MaxChars: 2048})
	if len(result.Entries) != 1 {
		t.Fatalf("Entries len = %d, want 1", len(result.Entries))
	}
	entry := result.Entries[0]
	listing, err := json.Marshal(result.Entries)
	if err != nil {
		t.Fatalf("Marshal(entries) error = %v", err)
	}
	if strings.Contains(string(listing), "Prompt") || strings.Contains(string(listing), "prompt") {
		t.Fatalf("index listing should not expose prompt fields: %s", listing)
	}
	if strings.Contains(string(listing), "SECRET_PROMPT_BODY_SHOULD_NOT_APPEAR") {
		t.Fatal("index listing must not contain full prompt body")
	}
	if len(entry.WhenToUse) != 360 {
		t.Fatalf("WhenToUse len = %d, want 360", len(entry.WhenToUse))
	}
	if result.Hash == "" {
		t.Fatal("Hash is empty")
	}
	if result.Bytes <= 0 {
		t.Fatalf("Bytes = %d, want > 0", result.Bytes)
	}
}

func TestAgentIndexRanksByQueryResourceOperationAndMode(t *testing.T) {
	defs := []Definition{
		{
			Kind:        "planner",
			Name:        "synthetic-planner",
			Description: "plans follow-up work",
			Discovery: AgentDiscoveryMetadata{
				CapabilityKinds: []string{"planning"},
				ResourceTypes:   []string{"config"},
				OperationKinds:  []string{"plan"},
				Modes:           []string{"plan"},
			},
			Budget: AgentBudgetMetadata{CostClass: "medium"},
		},
		{
			Kind:        "explorer",
			Name:        "synthetic-log-explorer",
			Description: "collects log evidence",
			Discovery: AgentDiscoveryMetadata{
				WhenToUse:       "Use for independent log evidence collection.",
				CapabilityKinds: []string{"evidence"},
				ResourceTypes:   []string{"logs"},
				OperationKinds:  []string{"read"},
				Modes:           []string{"explore"},
			},
			Budget: AgentBudgetMetadata{CostClass: "low"},
		},
	}

	result := BuildAgentIndex(defs, AgentIndexOptions{
		Query:         "need log evidence",
		ResourceType:  "logs",
		OperationKind: "read",
		Mode:          "explore",
		MaxChars:      2048,
	})
	if len(result.Entries) != 2 {
		t.Fatalf("Entries len = %d, want 2", len(result.Entries))
	}
	if result.Entries[0].Name != "synthetic-log-explorer" {
		t.Fatalf("top-ranked agent = %q, want synthetic-log-explorer", result.Entries[0].Name)
	}
}

func TestAgentIndexDropsLowestScoredEntriesWhenBudgetExceeded(t *testing.T) {
	defs := []Definition{
		{
			Kind:        "planner",
			Name:        "synthetic-planner",
			Description: "plans work",
			Discovery: AgentDiscoveryMetadata{
				CapabilityKinds: []string{"planning"},
				ResourceTypes:   []string{"config"},
				OperationKinds:  []string{"plan"},
				Modes:           []string{"plan"},
			},
		},
		{
			Kind:        "explorer",
			Name:        "synthetic-log-explorer",
			Description: "collects log evidence",
			Discovery: AgentDiscoveryMetadata{
				CapabilityKinds: []string{"evidence"},
				ResourceTypes:   []string{"logs"},
				OperationKinds:  []string{"read"},
				Modes:           []string{"explore"},
			},
		},
	}

	result := BuildAgentIndex(defs, AgentIndexOptions{
		Query:         "log evidence",
		ResourceType:  "logs",
		OperationKind: "read",
		Mode:          "explore",
		MaxChars:      220,
	})
	if len(result.Entries) != 1 {
		t.Fatalf("Entries len = %d, want 1: %#v", len(result.Entries), result.Entries)
	}
	if result.Entries[0].Name != "synthetic-log-explorer" {
		t.Fatalf("kept agent = %q, want synthetic-log-explorer", result.Entries[0].Name)
	}
	if len(result.Dropped) != 1 {
		t.Fatalf("Dropped len = %d, want 1", len(result.Dropped))
	}
	if result.Dropped[0].Name != "synthetic-planner" || result.Dropped[0].Reason != "budget_exceeded" {
		t.Fatalf("Dropped = %#v, want synthetic-planner budget_exceeded", result.Dropped[0])
	}
}

func TestAgentIndexHashChangesWhenVisibleListingChanges(t *testing.T) {
	defs := []Definition{
		{Kind: "worker", Name: "synthetic-worker", Description: "first"},
	}
	first := BuildAgentIndex(defs, AgentIndexOptions{MaxChars: 2048})
	defs[0].Description = "second"
	second := BuildAgentIndex(defs, AgentIndexOptions{MaxChars: 2048})

	if first.Hash == "" || second.Hash == "" {
		t.Fatalf("hashes must be non-empty: first=%q second=%q", first.Hash, second.Hash)
	}
	if first.Hash == second.Hash {
		t.Fatalf("hash did not change after visible listing changed: %q", first.Hash)
	}
}
