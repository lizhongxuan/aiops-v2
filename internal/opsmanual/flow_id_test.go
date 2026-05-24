package opsmanual

import (
	"strings"
	"testing"
)

func TestOpsManualFlowIDStableAndScoped(t *testing.T) {
	frame := OperationFrame{
		ObjectType: "redis",
		Operation:  OperationProfile{Action: "rca_or_repair"},
		Target:     OperationTarget{Name: "aiops-redis-a"},
		TargetScope: TargetScope{
			Hosts:     []string{"server-local"},
			Namespace: "prod-pay",
		},
	}
	req := OpsManualFlowIDInput{
		SessionID:      "sess-1",
		TurnID:         "turn-2",
		ManualID:       "manual-redis-memory",
		WorkflowID:     "workflow-redis-memory",
		OperationFrame: frame,
	}

	first := BuildOpsManualFlowID(req)
	second := BuildOpsManualFlowID(req)
	if first == "" {
		t.Fatal("flow id is empty")
	}
	if first != second {
		t.Fatalf("flow id is not stable: first=%q second=%q", first, second)
	}

	req.ManualID = "manual-redis-slowlog"
	otherManual := BuildOpsManualFlowID(req)
	if otherManual == first {
		t.Fatalf("flow id did not change for a different manual: %q", first)
	}

	req.ManualID = "manual-redis-memory"
	req.OperationFrame.Target.Name = "aiops-redis-b"
	otherTarget := BuildOpsManualFlowID(req)
	if otherTarget == first {
		t.Fatalf("flow id did not change for a different target scope: %q", first)
	}
}

func TestOpsManualFlowIDDoesNotLeakSensitiveInput(t *testing.T) {
	flowID := BuildOpsManualFlowID(OpsManualFlowIDInput{
		SessionID:  "session-with-password",
		TurnID:     "turn-token",
		ManualID:   "manual-pg-backup",
		WorkflowID: "workflow-pg-backup",
		OperationFrame: OperationFrame{
			Target: OperationTarget{Name: "pg-primary token=raw-secret"},
			Metadata: map[string]any{
				"password": "raw-password",
				"token":    "raw-token",
			},
			RawText: "backup postgres with password raw-password and token raw-token",
		},
	})
	if flowID == "" {
		t.Fatal("flow id is empty")
	}
	for _, leaked := range []string{"raw-password", "raw-token", "raw-secret", "password", "token"} {
		if strings.Contains(strings.ToLower(flowID), strings.ToLower(leaked)) {
			t.Fatalf("flow id %q leaked sensitive fragment %q", flowID, leaked)
		}
	}
}
