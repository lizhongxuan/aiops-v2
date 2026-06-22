package operatorruntime

import (
	"encoding/json"
	"testing"
	"time"
)

func validPGCluster() PGCluster {
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	return PGCluster{
		ID:                   "pg-order",
		Name:                 "order-postgres",
		MonitorCredentialRef: "cred-monitor",
		RepairCredentialRef:  "cred-repair",
		Primary: PGInstance{
			ID:          "pg-order-primary",
			Role:        PGRolePrimary,
			Host:        "10.0.0.10",
			Port:        5432,
			ServiceName: "postgresql",
			Labels:      map[string]string{"az": "a"},
		},
		Replicas: []PGInstance{
			{
				ID:          "pg-order-replica-a",
				Role:        PGRoleReplica,
				Host:        "10.0.0.11",
				Port:        5432,
				ServiceName: "postgresql",
				Labels:      map[string]string{"az": "b"},
			},
		},
		Tags:      []string{"production", "orders"},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func validInspectionTemplate() InspectionTemplate {
	return InspectionTemplate{
		ID:              "postgres.replication.basic.v1",
		Name:            "PG 主从复制基础巡检",
		ObjectKind:      ObjectKindPostgresReplication,
		IntervalSeconds: 60,
		PrimarySQL:      "select application_name, state, sent_lsn, replay_lsn from pg_stat_replication",
		ReplicaSQL:      "select pg_is_in_recovery() as in_recovery, true as receiver_running, 1 as replay_lag_seconds",
		OutputFields: []InspectionField{
			{Name: FieldReplicaReachable, Type: FieldTypeBool},
			{Name: FieldReplicaInRecovery, Type: FieldTypeBool},
			{Name: FieldReplicaReceiverRunning, Type: FieldTypeBool},
			{Name: FieldReplicaReplayLagSeconds, Type: FieldTypeNumber},
		},
	}
}

func validAction() ActionCatalogItem {
	return ActionCatalogItem{
		ID:          "postgres.replication.reconnect_replica.v1",
		DisplayName: "重连 PG 从库复制",
		RiskLevel:   RiskMedium,
		TargetKind:  TargetKindPostgresReplica,
		InputSchema: map[string]string{
			"resourceId":  "string",
			"replicaHost": "string",
		},
		Steps: []ActionStep{
			{ID: "check_service", Kind: ActionStepCheckService},
			{ID: "reload_config", Kind: ActionStepReloadConfig},
			{ID: "restart_service", Kind: ActionStepRestartService, RequiresApproval: true},
		},
		ConfirmationRequiredSteps: []string{"restart_service"},
	}
}

func validWorkflowBinding() WorkflowBinding {
	return WorkflowBinding{
		ID:              "binding-pg-reconnect",
		ActionRef:       "postgres.replication.reconnect_replica.v1",
		WorkflowRef:     "builtin.postgres.replication_reconnect_replica.v1",
		WorkflowVersion: "v1",
		Capabilities:    []string{"preflight", "act", "verify"},
		InputMapping: map[string]string{
			"resourceId":  "guard.resourceId",
			"replicaHost": "event.target.host",
		},
		VerifyPolicy: VerifyPolicy{
			ReceiverRunningRequired: true,
			MaxReplayLagSeconds:     10,
			TimeoutSeconds:          300,
			IntervalSeconds:         30,
		},
	}
}

func validLagProblem() ProblemType {
	return ProblemType{
		ID:                "pg.replication.lag_high",
		DisplayName:       "PG 复制延迟过高",
		Severity:          SeverityWarning,
		ForSeconds:        180,
		AutoRepairAllowed: true,
		Conditions: []ProblemCondition{
			{Field: FieldReplicaReachable, Operator: OperatorEqual, Value: KnownBool(true)},
			{Field: FieldReplicaReplayLagSeconds, Operator: OperatorGreaterThan, Value: KnownNumber(60)},
		},
		RecommendedActionRefs: []string{"postgres.replication.reconnect_replica.v1"},
	}
}

func validReceiverStoppedProblem() ProblemType {
	return ProblemType{
		ID:                "pg.replication.receiver_stopped",
		DisplayName:       "PG WAL receiver 停止",
		Severity:          SeverityCritical,
		ForSeconds:        60,
		AutoRepairAllowed: true,
		Conditions: []ProblemCondition{
			{Field: FieldReplicaReachable, Operator: OperatorEqual, Value: KnownBool(true)},
			{Field: FieldReplicaReceiverRunning, Operator: OperatorEqual, Value: KnownBool(false)},
		},
		RecommendedActionRefs: []string{"postgres.replication.reconnect_replica.v1"},
	}
}

func validGuardRule() GuardRule {
	return GuardRule{
		ID:                              "guard.pg-order.replication",
		Name:                            "order-postgres 主从复制守护",
		ResourceRef:                     "pg-order",
		ClusterRef:                      "pg-order",
		TemplateRef:                     "postgres.replication.basic.v1",
		ProblemTypeRefs:                 []string{"pg.replication.lag_high", "pg.replication.receiver_stopped"},
		ActionRefs:                      []string{"postgres.replication.reconnect_replica.v1"},
		WorkflowBindingRefs:             []string{"binding-pg-reconnect"},
		ScheduleSeconds:                 60,
		CooldownSeconds:                 1800,
		MaxConcurrency:                  1,
		DisableAfterConsecutiveFailures: 3,
		Enabled:                         true,
		Policy: GuardPolicy{
			MaxAutoRisk:              RiskMedium,
			RequireApprovalStepKinds: []ActionStepKind{ActionStepRestartService},
		},
	}
}

func TestModelJSONRoundTrip(t *testing.T) {
	cluster := validPGCluster()
	raw, err := json.Marshal(cluster)
	if err != nil {
		t.Fatalf("marshal cluster: %v", err)
	}
	var decoded PGCluster
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal cluster: %v", err)
	}
	if decoded.ID != cluster.ID || decoded.Primary.Role != PGRolePrimary || decoded.Replicas[0].Role != PGRoleReplica {
		t.Fatalf("decoded cluster lost important fields: %#v", decoded)
	}
}
