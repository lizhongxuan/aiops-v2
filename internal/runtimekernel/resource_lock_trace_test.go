package runtimekernel

import (
	"testing"

	"aiops-v2/internal/promptinput"
)

func TestBuildModelInputToolTraceFieldsIncludesResourceLocksFromSnapshot(t *testing.T) {
	snapshot := &TurnSnapshot{
		Iterations: []IterationState{{
			ResourceLocks: []promptinput.ResourceLockTrace{{
				AgentID: "sess:turn:call",
				Action:  "acquired",
				Key: promptinput.ResourceLockKeyTrace{
					ResourceType:  "file",
					ResourceID:    "config://service-a",
					OperationKind: "write",
				},
			}},
		}},
	}

	fields := buildModelInputToolTraceFields(nil, snapshot, "", "")
	if len(fields.ResourceLocks) != 1 {
		t.Fatalf("resource locks = %#v, want one trace", fields.ResourceLocks)
	}
	if fields.ResourceLocks[0].Action != "acquired" || fields.ResourceLocks[0].Key.ResourceType != "file" {
		t.Fatalf("resource lock trace = %#v, want acquired file lock", fields.ResourceLocks[0])
	}
}

func TestBuildModelInputToolTraceFieldsIncludesPublicWebBudget(t *testing.T) {
	fields := buildModelInputToolTraceFields(nil, nil, "", "")
	if fields.PublicWebBudget == nil {
		t.Fatal("expected public web budget trace")
	}
	budget := DefaultPublicWebBudget()
	if fields.PublicWebBudget.MaxSearchCalls != budget.MaxSearchCalls ||
		fields.PublicWebBudget.MaxQueries != budget.MaxQueries ||
		fields.PublicWebBudget.MaxResults != budget.MaxResults ||
		fields.PublicWebBudget.MaxCallsPerTurn != budget.MaxCallsPerTurn ||
		fields.PublicWebBudget.MaxQueriesPerCall != budget.MaxQueriesPerCall ||
		fields.PublicWebBudget.MaxResultsPerDomain != budget.MaxResultsPerDomain ||
		fields.PublicWebBudget.ExplicitUserRequested != budget.ExplicitUserRequested {
		t.Fatalf("public web budget = %#v, want %#v", fields.PublicWebBudget, budget)
	}
}
