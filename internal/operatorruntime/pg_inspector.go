package operatorruntime

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

type PGQueryExecutor interface {
	QueryPrimary(context.Context, PGCluster, string) ([]map[string]any, error)
	QueryReplica(context.Context, PGCluster, PGInstance, string) (map[string]any, error)
}

type InspectionRunner interface {
	Inspect(context.Context, PGCluster, InspectionTemplate) ([]InspectionResult, error)
}

type PGInspector struct {
	executor PGQueryExecutor
}

func NewPGInspector(executor PGQueryExecutor) *PGInspector {
	return &PGInspector{executor: executor}
}

func (i *PGInspector) Inspect(ctx context.Context, cluster PGCluster, template InspectionTemplate) ([]InspectionResult, error) {
	cluster = NormalizeResource(cluster)
	if err := ValidateInspectionTemplate(template); err != nil {
		return nil, err
	}
	if i.executor == nil {
		return nil, fmt.Errorf("pg query executor is required")
	}
	_, _ = i.executor.QueryPrimary(ctx, cluster, template.PrimarySQL)
	replicas := ResourceReplicaEndpoints(cluster)
	results := make([]InspectionResult, 0, len(replicas))
	for _, replica := range replicas {
		result := InspectionResult{
			SnapshotID: fmt.Sprintf("snap-%d", time.Now().UnixNano()),
			ResourceID: cluster.ID,
			TargetID:   replica.ID,
			Target:     replica,
			ClusterID:  cluster.ID,
			ReplicaID:  replica.ID,
			Replica:    replica,
			Fields: map[string]FieldValue{
				FieldReplicaReachable: KnownBool(true),
			},
			ObservedAt: time.Now().UTC(),
		}
		row, err := i.executor.QueryReplica(ctx, cluster, replica, template.ReplicaSQL)
		if err != nil {
			result.Fields[FieldReplicaReachable] = KnownBool(false)
			result.Errors = append(result.Errors, err.Error())
			results = append(results, result)
			continue
		}
		result.Fields[FieldReplicaInRecovery] = boolField(row, "in_recovery")
		result.Fields[FieldReplicaReceiverRunning] = boolField(row, "receiver_running")
		result.Fields[FieldReplicaReplayLagSeconds] = numberField(row, "replay_lag_seconds")
		result.Fields[FieldReplicaReplayLagBytes] = numberField(row, "replay_lag_bytes")
		result.Fields[FieldReplicaReceiveLSN] = stringField(row, "receive_lsn")
		result.Fields[FieldReplicaReplayLSN] = stringField(row, "replay_lsn")
		results = append(results, result)
	}
	return results, nil
}

func boolField(row map[string]any, key string) FieldValue {
	if value, ok := row[key].(bool); ok {
		return KnownBool(value)
	}
	return Unknown(FieldTypeBool)
}

func numberField(row map[string]any, key string) FieldValue {
	value, ok := row[key]
	if !ok || value == nil {
		return Unknown(FieldTypeNumber)
	}
	switch typed := value.(type) {
	case int:
		return KnownNumber(float64(typed))
	case int64:
		return KnownNumber(float64(typed))
	case float64:
		return KnownNumber(typed)
	case float32:
		return KnownNumber(float64(typed))
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		if err != nil {
			return Unknown(FieldTypeNumber)
		}
		return KnownNumber(parsed)
	default:
		return Unknown(FieldTypeNumber)
	}
}

func stringField(row map[string]any, key string) FieldValue {
	value, ok := row[key]
	if !ok || value == nil {
		return Unknown(FieldTypeString)
	}
	return KnownString(fmt.Sprint(value))
}
