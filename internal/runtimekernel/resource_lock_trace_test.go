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
