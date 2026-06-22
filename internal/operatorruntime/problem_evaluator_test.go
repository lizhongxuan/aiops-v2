package operatorruntime

import (
	"testing"
	"time"
)

func lagInspectionResult(lag FieldValue) InspectionResult {
	return InspectionResult{
		SnapshotID: "snap-1",
		ClusterID:  "pg-order",
		ReplicaID:  "pg-order-replica-a",
		Fields: map[string]FieldValue{
			FieldReplicaReachable:        KnownBool(true),
			FieldReplicaReceiverRunning:  KnownBool(true),
			FieldReplicaReplayLagSeconds: lag,
		},
		ObservedAt: time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC),
	}
}

func TestProblemEvaluatorRequiresSustainedLag(t *testing.T) {
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	evaluator := NewProblemEvaluator(func() time.Time { return now })
	result := lagInspectionResult(KnownNumber(120))
	problem := validLagProblem()

	first := evaluator.Evaluate(problem, result)
	if len(first.Matches) != 0 {
		t.Fatalf("first hit should not match until sustained window passes")
	}
	now = now.Add(179 * time.Second)
	stillWaiting := evaluator.Evaluate(problem, result)
	if len(stillWaiting.Matches) != 0 {
		t.Fatalf("179 seconds should not satisfy 180 second window")
	}
	now = now.Add(2 * time.Second)
	matched := evaluator.Evaluate(problem, result)
	if len(matched.Matches) != 1 {
		t.Fatalf("expected sustained lag match, got %#v", matched)
	}
}

func TestProblemEvaluatorMatchesReceiverStopped(t *testing.T) {
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	evaluator := NewProblemEvaluator(func() time.Time { return now })
	result := InspectionResult{
		SnapshotID: "snap-1",
		ClusterID:  "pg-order",
		ReplicaID:  "pg-order-replica-a",
		Fields: map[string]FieldValue{
			FieldReplicaReachable:       KnownBool(true),
			FieldReplicaReceiverRunning: KnownBool(false),
		},
	}
	problem := validReceiverStoppedProblem()
	_ = evaluator.Evaluate(problem, result)
	now = now.Add(61 * time.Second)
	got := evaluator.Evaluate(problem, result)
	if len(got.Matches) != 1 || got.Matches[0].ProblemTypeID != problem.ID {
		t.Fatalf("expected receiver stopped match, got %#v", got)
	}
}

func TestProblemEvaluatorDoesNotAutoRepairUnreachableReplica(t *testing.T) {
	problem := ProblemType{
		ID:                "pg.replica.unreachable",
		DisplayName:       "PG 从库不可达",
		ForSeconds:        60,
		AutoRepairAllowed: false,
		Conditions: []ProblemCondition{
			{Field: FieldReplicaReachable, Operator: OperatorEqual, Value: KnownBool(false)},
		},
	}
	result := InspectionResult{
		SnapshotID: "snap-1",
		ClusterID:  "pg-order",
		ReplicaID:  "pg-order-replica-a",
		Fields: map[string]FieldValue{
			FieldReplicaReachable: KnownBool(false),
		},
	}
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	evaluator := NewProblemEvaluator(func() time.Time { return now })
	_ = evaluator.Evaluate(problem, result)
	now = now.Add(61 * time.Second)
	got := evaluator.Evaluate(problem, result)
	if len(got.Matches) != 1 || got.Matches[0].AutoRepairAllowed {
		t.Fatalf("unreachable replica should match but not auto repair: %#v", got)
	}
}

func TestProblemEvaluatorUnknownLagProducesMissingEvidence(t *testing.T) {
	evaluator := NewProblemEvaluator(func() time.Time { return time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC) })
	got := evaluator.Evaluate(validLagProblem(), lagInspectionResult(Unknown(FieldTypeNumber)))
	if len(got.Matches) != 0 {
		t.Fatalf("unknown lag should not match: %#v", got)
	}
	if len(got.MissingEvidence) == 0 {
		t.Fatalf("unknown lag should produce missing evidence")
	}
}
