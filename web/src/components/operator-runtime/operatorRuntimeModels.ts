import type { OperatorRuntimeItem } from "@/api/operatorRuntime";

export type RuntimeCollectionKey =
  | "resources"
  | "inspectionTemplates"
  | "problemTypes"
  | "actions"
  | "workflowBindings"
  | "rules";

export type RuntimeCollections = Record<RuntimeCollectionKey, OperatorRuntimeItem[]>;

export function itemId(item: OperatorRuntimeItem | undefined, fallback = "") {
  const value = item?.id ?? item?.name ?? item?.key;
  return value === undefined || value === null ? fallback : String(value);
}

export function itemLabel(item: OperatorRuntimeItem | undefined, fallback = "未命名") {
  const value = item?.name ?? item?.displayName ?? item?.title ?? item?.id ?? item?.key;
  return value === undefined || value === null || value === "" ? fallback : String(value);
}

export function valueText(value: unknown) {
  if (value === undefined || value === null) return "";
  if (typeof value === "boolean") return value ? "true" : "false";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

export function ruleEnabled(rule: OperatorRuntimeItem) {
  return rule.enabled === true || rule.status === "enabled" || rule.state === "enabled";
}

export function runStatus(run: OperatorRuntimeItem) {
  return String(run.status ?? run.state ?? run.phase ?? run.decision ?? "pending");
}

export function createSamplePayloads(host: string, clusterName: string) {
  const clusterKey = clusterName || "postgres-prod-primary";
  const replicaHost = host || "120.77.239.90";
  return {
    resource: {
      id: clusterKey,
      name: clusterKey,
      kind: "postgresql",
      provider: "self-managed",
      environment: "production",
      endpoints: [
        {
          id: `${clusterKey}-primary`,
          role: "primary",
          host,
          port: 5432,
          serviceName: "postgresql",
        },
        {
          id: `${clusterKey}-replica-a`,
          role: "replica",
          host: replicaHost,
          port: 5432,
          serviceName: "postgresql",
        },
      ],
      primary: {
        id: `${clusterKey}-primary`,
        role: "primary",
        host,
        port: 5432,
        serviceName: "postgresql",
      },
      replicas: [
        {
          id: `${clusterKey}-replica-a`,
          role: "replica",
          host: replicaHost,
          port: 5432,
          serviceName: "postgresql",
        },
      ],
      monitorCredentialRef: "pg-monitor-ref",
      repairCredentialRef: "pg-repair-ref",
      credentialRefs: { monitor: "pg-monitor-ref", repair: "pg-repair-ref" },
      tags: ["production", "postgres"],
    },
    inspectionTemplate: {
      id: "postgres.replication.basic.v1",
      name: "PG 主从复制基础巡检",
      objectKind: "postgres_replication",
      intervalSeconds: 60,
      checks: [
        {
          id: "primary_replication_status",
          kind: "sql",
          targetRole: "primary",
          query: "select application_name, client_addr, state, sync_state, sent_lsn, write_lsn, flush_lsn, replay_lsn from pg_stat_replication",
          timeoutSeconds: 10,
        },
        {
          id: "replica_receiver_status",
          kind: "sql",
          targetRole: "replica",
          query:
            "select pg_is_in_recovery() as in_recovery, true as receiver_running, 0 as replay_lag_seconds, 0 as replay_lag_bytes, pg_last_wal_receive_lsn() as receive_lsn, pg_last_wal_replay_lsn() as replay_lsn",
          timeoutSeconds: 10,
        },
      ],
      primarySql: "select application_name, client_addr, state, sync_state, sent_lsn, write_lsn, flush_lsn, replay_lsn from pg_stat_replication",
      replicaSql:
        "select pg_is_in_recovery() as in_recovery, true as receiver_running, 0 as replay_lag_seconds, 0 as replay_lag_bytes, pg_last_wal_receive_lsn() as receive_lsn, pg_last_wal_replay_lsn() as replay_lsn",
      outputFields: [
        { name: "replica.reachable", type: "bool" },
        { name: "replica.inRecovery", type: "bool" },
        { name: "replica.receiverRunning", type: "bool" },
        { name: "replica.replayLagSeconds", type: "number" },
        { name: "replica.replayLagBytes", type: "number" },
        { name: "replica.receiveLsn", type: "string" },
        { name: "replica.replayLsn", type: "string" },
      ],
    },
    problemType: createProblemTypePresetPayload("lag_high"),
    action: {
      id: "postgres.replication.reconnect_replica.v1",
      displayName: "重连 PG 从库复制",
      riskLevel: "medium",
      targetKind: "postgres_replica",
      inputSchema: { resourceId: "string", primaryHost: "string", replicaHost: "string" },
      steps: [
        { id: "check_service", kind: "check_service" },
        { id: "reload_config", kind: "reload_config" },
        { id: "restart_service", kind: "restart_service", requiresApproval: true },
      ],
      confirmationRequiredSteps: ["restart_service"],
    },
    workflowBinding: {
      id: "builtin.postgres.replication_reconnect_replica.v1",
      actionRef: "postgres.replication.reconnect_replica.v1",
      workflowRef: "builtin.postgres.replication_reconnect_replica.v1",
      workflowVersion: "v1",
      capabilities: ["preflight", "act", "verify"],
      inputMapping: {
        resourceId: "guard.resourceId",
        primaryHost: "resource.endpoint.primary.host",
        replicaHost: "event.target.host",
      },
      verifyPolicy: {
        receiverRunningRequired: true,
        maxReplayLagSeconds: 10,
        timeoutSeconds: 300,
        intervalSeconds: 30,
      },
    },
  };
}

export type ProblemPreset = "lag_high" | "receiver_stopped";

export function createProblemTypePresetPayload(preset: ProblemPreset) {
  if (preset === "receiver_stopped") {
    return {
      id: "pg.replication.receiver_stopped",
      displayName: "PG WAL receiver 停止",
      severity: "critical",
      forSeconds: 60,
      autoRepairAllowed: true,
      conditions: [
        { field: "replica.reachable", operator: "==", value: { known: true, type: "bool", bool: true } },
        { field: "replica.receiverRunning", operator: "==", value: { known: true, type: "bool", bool: false } },
      ],
      recommendedActionRefs: ["postgres.replication.reconnect_replica.v1"],
    };
  }
  return {
    id: "pg.replication.lag_high",
    displayName: "PG 复制延迟过高",
    severity: "warning",
    forSeconds: 180,
    autoRepairAllowed: true,
    conditions: [
      { field: "replica.reachable", operator: "==", value: { known: true, type: "bool", bool: true } },
      { field: "replica.replayLagSeconds", operator: ">", value: { known: true, type: "number", number: 60 } },
    ],
    recommendedActionRefs: ["postgres.replication.reconnect_replica.v1"],
  };
}

export function createRulePayload({
  resource,
  inspectionTemplate,
  problemType,
  action,
  workflowBinding,
}: {
  resource?: OperatorRuntimeItem;
  inspectionTemplate?: OperatorRuntimeItem;
  problemType?: OperatorRuntimeItem;
  action?: OperatorRuntimeItem;
  workflowBinding?: OperatorRuntimeItem;
}) {
  return {
    id: "pg-runtime-autoheal-rule",
    name: "pg-runtime-autoheal-rule",
    enabled: false,
    resourceRef: itemId(resource),
    clusterRef: itemId(resource),
    templateRef: itemId(inspectionTemplate),
    problemTypeRefs: [itemId(problemType)].filter(Boolean),
    actionRefs: [itemId(action)].filter(Boolean),
    workflowBindingRefs: [itemId(workflowBinding)].filter(Boolean),
    scheduleSeconds: 60,
    cooldownSeconds: 1800,
    maxConcurrency: 1,
    disableAfterConsecutiveFailures: 3,
    policy: {
      maxAutoRisk: "medium",
      requireApprovalStepKinds: ["restart_service"],
    },
  };
}
