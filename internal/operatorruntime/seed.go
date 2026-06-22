package operatorruntime

import "context"

func DefaultInspectionTemplate() InspectionTemplate {
	return InspectionTemplate{
		ID:              "postgres.replication.basic.v1",
		Name:            "PG 主从复制基础巡检",
		ObjectKind:      ObjectKindPostgresReplication,
		IntervalSeconds: 60,
		PrimarySQL:      "select application_name, client_addr, state, sync_state, sent_lsn, write_lsn, flush_lsn, replay_lsn from pg_stat_replication",
		ReplicaSQL:      "select pg_is_in_recovery() as in_recovery, true as receiver_running, 0 as replay_lag_seconds, 0 as replay_lag_bytes, pg_last_wal_receive_lsn() as receive_lsn, pg_last_wal_replay_lsn() as replay_lsn",
		OutputFields: []InspectionField{
			{Name: FieldReplicaReachable, Type: FieldTypeBool},
			{Name: FieldReplicaInRecovery, Type: FieldTypeBool},
			{Name: FieldReplicaReceiverRunning, Type: FieldTypeBool},
			{Name: FieldReplicaReplayLagSeconds, Type: FieldTypeNumber},
			{Name: FieldReplicaReplayLagBytes, Type: FieldTypeNumber},
			{Name: FieldReplicaReceiveLSN, Type: FieldTypeString},
			{Name: FieldReplicaReplayLSN, Type: FieldTypeString},
		},
	}
}

func DefaultLagHighProblem() ProblemType {
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

func DefaultReceiverStoppedProblem() ProblemType {
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

func DefaultReconnectAction() ActionCatalogItem {
	return ActionCatalogItem{
		ID:          "postgres.replication.reconnect_replica.v1",
		DisplayName: "重连 PG 从库复制",
		RiskLevel:   RiskMedium,
		TargetKind:  TargetKindPostgresReplica,
		InputSchema: map[string]string{
			"resourceId":  "string",
			"primaryHost": "string",
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

func DefaultWorkflowBinding() WorkflowBinding {
	return WorkflowBinding{
		ID:              "builtin.postgres.replication_reconnect_replica.v1",
		ActionRef:       "postgres.replication.reconnect_replica.v1",
		WorkflowRef:     "builtin.postgres.replication_reconnect_replica.v1",
		WorkflowVersion: "v1",
		Capabilities:    []string{"preflight", "act", "verify"},
		InputMapping: map[string]string{
			"resourceId":  "guard.resourceId",
			"primaryHost": "resource.endpoint.primary.host",
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

func SeedDefaultCatalog(ctx context.Context, store Store) error {
	if err := store.SaveInspectionTemplate(ctx, DefaultInspectionTemplate()); err != nil {
		return err
	}
	if err := store.SaveProblemType(ctx, DefaultLagHighProblem()); err != nil {
		return err
	}
	if err := store.SaveProblemType(ctx, DefaultReceiverStoppedProblem()); err != nil {
		return err
	}
	if err := store.SaveAction(ctx, DefaultReconnectAction()); err != nil {
		return err
	}
	if err := store.SaveWorkflowBinding(ctx, DefaultWorkflowBinding()); err != nil {
		return err
	}
	return nil
}
