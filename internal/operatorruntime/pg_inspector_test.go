package operatorruntime

import (
	"context"
	"errors"
	"testing"
)

type fakePGQueryExecutor struct {
	primaryRows []map[string]any
	replicaRows map[string]map[string]any
	replicaErrs map[string]error
}

func (f fakePGQueryExecutor) QueryPrimary(context.Context, PGCluster, string) ([]map[string]any, error) {
	return f.primaryRows, nil
}

func (f fakePGQueryExecutor) QueryReplica(_ context.Context, _ PGCluster, replica PGInstance, _ string) (map[string]any, error) {
	if err := f.replicaErrs[replica.ID]; err != nil {
		return nil, err
	}
	return f.replicaRows[replica.ID], nil
}

func TestPGInspectorNormalizesReplicaStreaming(t *testing.T) {
	inspector := NewPGInspector(fakePGQueryExecutor{
		replicaRows: map[string]map[string]any{
			"pg-order-replica-a": {
				"in_recovery":        true,
				"receiver_running":   true,
				"replay_lag_seconds": 3.0,
				"replay_lag_bytes":   128.0,
				"receive_lsn":        "0/300",
				"replay_lsn":         "0/280",
			},
		},
	})
	results, err := inspector.Inspect(context.Background(), validPGCluster(), validInspectionTemplate())
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	got := results[0]
	if !got.Fields[FieldReplicaReachable].Bool || !got.Fields[FieldReplicaReceiverRunning].Bool {
		t.Fatalf("expected reachable streaming replica, got %#v", got.Fields)
	}
	if got.Fields[FieldReplicaReplayLagSeconds].Number != 3 {
		t.Fatalf("unexpected replay lag: %#v", got.Fields[FieldReplicaReplayLagSeconds])
	}
}

func TestPGInspectorMarksReplayLagUnknownWhenTimestampNull(t *testing.T) {
	inspector := NewPGInspector(fakePGQueryExecutor{
		replicaRows: map[string]map[string]any{
			"pg-order-replica-a": {
				"in_recovery":        true,
				"receiver_running":   true,
				"replay_lag_seconds": nil,
			},
		},
	})
	results, err := inspector.Inspect(context.Background(), validPGCluster(), validInspectionTemplate())
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if results[0].Fields[FieldReplicaReplayLagSeconds].Known {
		t.Fatalf("expected unknown replay lag when source timestamp is null")
	}
}

func TestPGInspectorMarksReplicaUnreachable(t *testing.T) {
	inspector := NewPGInspector(fakePGQueryExecutor{
		replicaRows: map[string]map[string]any{},
		replicaErrs: map[string]error{"pg-order-replica-a": errors.New("connection refused")},
	})
	results, err := inspector.Inspect(context.Background(), validPGCluster(), validInspectionTemplate())
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if results[0].Fields[FieldReplicaReachable].Bool {
		t.Fatalf("expected replica unreachable")
	}
	if len(results[0].Errors) == 0 {
		t.Fatalf("expected connection error to be recorded")
	}
}

func TestPGInspectorDoesNotExecuteWriteSQL(t *testing.T) {
	template := validInspectionTemplate()
	template.PrimarySQL = "delete from pg_stat_replication"
	inspector := NewPGInspector(fakePGQueryExecutor{})
	if _, err := inspector.Inspect(context.Background(), validPGCluster(), template); err == nil {
		t.Fatalf("expected write SQL to be rejected")
	}
}
