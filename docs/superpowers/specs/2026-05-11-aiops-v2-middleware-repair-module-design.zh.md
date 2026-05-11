# aiops-v2 Middleware Repair 模块设计

日期：2026-05-11
状态：模块详细设计
所属总纲：[2026-05-11-aiops-v2-enterprise-control-plane-design.zh.md](2026-05-11-aiops-v2-enterprise-control-plane-design.zh.md)

## 1. 模块定位

Middleware Repair 模块负责 PG、Redis、MQ、Elasticsearch、Kafka、MySQL 等中间件异常的根因分析、修复计划、受治理执行、验证和经验沉淀。它把“帮我修复 xxx 的 PG 集群”这类自然语言请求转成可审计的 `middleware_repair` case 和 `RepairPlan`。

## 2. 设计目标

- 先 RCA，再修复。不能把“修复集群”直接解释为重启、failover 或删数据。
- 优先复用已审核经验包、Capsule、Runbook 或 Workflow。
- 未命中经验时生成 RepairPlan 草稿，用户确认后才执行。
- 中间件修复必须比普通服务修复更严格地处理数据安全、角色、复制、备份和回滚。
- 成功和失败都沉淀为经验包候选。

## 3. 核心对象

```text
MiddlewareResource
  id
  kind: postgres | redis | mq | elasticsearch | kafka | mysql | other
  clusterName
  environment
  topology
  roleMap
  criticality
  safetyProfile
  opsGraphEntityIds
```

```text
RepairPlan
  id
  caseId
  middlewareResourceId
  source: experience_pack | capsule | runbook | generated
  rootCauseHypothesisIds
  assumptions
  diagnosticSteps
  remediationSteps
  riskLevel
  approvalPolicy
  rollbackPlan
  verificationSpec
  disabledConditions
  experiencePackRef
  status: draft | awaiting_confirmation | approved | running | succeeded | failed
```

```text
RecoveryAttempt
  id
  repairPlanId
  workflowRunId
  actionProposalRefs
  status
  stepResults
  failedPoint
  rollbackResult
  verificationRefs
```

## 4. PG 集群诊断基线

PG 修复前必须采集：

- 集群拓扑：primary、standby、replica、proxy。
- 复制状态：replication lag、slot、WAL、sender/receiver。
- 锁等待：blocking query、blocked query、lock type、duration。
- 连接：active、idle、idle in transaction、max connections。
- 存储：disk usage、WAL usage、tablespace。
- 性能：slow query、CPU、IO、buffer、checkpoint。
- 备份：最近备份、备份可用性、PITR 能力。
- failover 风险：数据丢失窗口、应用连接、只读/读写路由。

没有这些证据时，只能生成“证据不足”的只读诊断建议。

## 5. 修复流程

```text
User request
  -> identify middleware resource
  -> create middleware_repair case
  -> collect readonly evidence
  -> query OpsGraph for business impact
  -> match Activation Index
  -> if approved experience matched:
       show source, success rate, disabled conditions, verification
     else:
       generate RepairPlan draft
  -> user confirms
  -> create ActionProposal / Workflow run
  -> execute with HostLease and Approval
  -> verify middleware and business recovery
  -> produce experience candidate
```

## 6. 风险分级

`readonly`：

- 查看状态、指标、日志、锁、复制、磁盘。

`low`：

- 清理安全缓存、调整非生产限流、刷新只读配置。

`medium`：

- reload 配置、扩容副本、调整连接池参数。

`high`：

- 重启实例、切换流量、触发 failover、变更数据库参数。

`destructive`：

- 删除数据、强制 promote、重建副本、清理 WAL、修复数据。

`high` 和 `destructive` 必须有明确审批、回滚和验证。

## 7. 经验复用

命中已审核经验时，展示：

- 经验来源 case。
- 适用环境。
- 成功率和最近失败。
- 禁用条件。
- 所需权限。
- 风险和回滚。
- 验证项。

如果当前环境与经验不匹配，只能使用只读诊断部分，不能直接复用修复动作。

## 8. RepairPlan 示例

```yaml
title: "PG lock wait 导致 checkout 写入延迟"
assumptions:
  - checkout-api p95 latency increase is caused by postgres lock wait
diagnosticSteps:
  - inspect pg_stat_activity
  - inspect blocking locks
  - correlate trace slow span with SQL fingerprint
remediationSteps:
  - terminate only confirmed idle-in-transaction blocker
  - notify service owner
rollbackPlan:
  - no data mutation
  - if termination impacts job, restart affected worker under approval
verificationSpec:
  - lock wait below threshold
  - checkout p95 latency recovered
  - ERP order submission success rate recovered
disabledConditions:
  - blocker is migration process
  - no recent backup
  - replication lag above threshold
```

## 9. API 草案

```text
GET    /api/v1/middleware-resources
GET    /api/v1/middleware-resources/{id}
POST   /api/v1/middleware-resources/{id}/diagnose
POST   /api/v1/middleware-repair-cases
GET    /api/v1/middleware-repair-cases/{case_id}
POST   /api/v1/middleware-repair-cases/{case_id}/match-experience
POST   /api/v1/middleware-repair-cases/{case_id}/repair-plans
POST   /api/v1/repair-plans/{id}/confirm
POST   /api/v1/repair-plans/{id}/execute
POST   /api/v1/repair-plans/{id}/verify
```

## 10. UI 设计

中间件修复页面展示：

- 集群拓扑和角色。
- 业务影响。
- RCA 假设和证据。
- 已命中经验或新 RepairPlan。
- 风险分级、审批要求、回滚方案。
- 执行步骤和每步结果。
- 修复验证和失败点。
- 生成经验包候选入口。

## 11. 安全要求

- 默认只读诊断。
- 高风险动作必须 DBA 或对应资源 owner 审批。
- 数据破坏性动作默认禁止自动化，除非 break-glass 且强审计。
- RepairPlan 必须检查备份状态和回滚可行性。
- 未审核经验不能降低风险等级或审批要求。

## 12. 验收标准

- 用户说“帮我修复 xxx 的 PG 集群”时，系统创建 middleware_repair case 并先诊断。
- 系统能识别 PG 集群角色、复制、锁、连接、磁盘、WAL、备份和 failover 风险。
- 命中已审核经验时展示来源、适用环境、成功率、禁用条件和验证项。
- 未命中经验时生成 RepairPlan，并等待用户确认。
- 修复完成后验证中间件健康和业务恢复。
- 修复成功或失败都会生成经验包候选。
