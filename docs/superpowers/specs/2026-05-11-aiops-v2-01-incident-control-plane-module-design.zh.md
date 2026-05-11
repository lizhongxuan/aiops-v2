# aiops-v2 Incident Control Plane 模块设计

日期：2026-05-11
状态：模块详细设计
所属总纲：[2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md](2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md)
前端设计：[2026-05-11-aiops-v2-01a-incident-control-plane-frontend-design.zh.md](2026-05-11-aiops-v2-01a-incident-control-plane-frontend-design.zh.md)
实施清单：[2026-05-11-aiops-v2-01b-incident-control-plane-frontend-todo.zh.md](2026-05-11-aiops-v2-01b-incident-control-plane-frontend-todo.zh.md)

## 0. 场景边界

本模块本阶段优先服务两条场景闭环：浏览器插件慢请求 `DebugCase` 和 PG 集群 `middleware_repair` case。`operation`、`maintenance`、`drill`、复杂工单同步和跨来源自动合并规则保留为后续能力，不作为本次场景改造的主路径。

关键产物是 case 事实源、EvidenceRef 引用、timeline、状态机和关闭门禁。它不诊断、不执行、不审核经验，只把 04/05/06/07/08/09/10 的输出挂回同一个 case。

## 1. 模块定位

Incident Control Plane 是 `aiops-v2` 的生产事实入口。它把 Coroot 告警、ERP 异常、用户对话、用户侧 DebugEvent、中间件异常、工单和人工操作统一规整为 `Case`，并围绕 case 管理证据、假设、动作、审批、执行、验证、复盘和经验沉淀。

这个模块不负责直接诊断和执行。它负责定义“这件事是什么、当前状态是什么、哪些证据可信、哪些动作被批准、最终是否恢复”。

## 2. 设计目标

- 所有生产运维行为必须归属到统一 `Case`，本阶段优先支持 `DebugCase` 和 `middleware_repair` case。
- Case 是审计根对象，任何证据、计划、动作、审批、工具结果、验证和复盘都能回溯到 case。
- 支持多来源合并：同一 trace、同一服务、同一中间件集群、同一 ERP 业务能力的事件应能合并或关联。
- 支持长任务恢复：服务重启后 case、workflow run、approval、host lease 和 verification 能恢复或失败收敛。
- 为 AI、Runbook、Workflow、经验包提供统一上下文，而不是让各模块各自维护事实。

## 3. 非目标

- 不直接调用生产工具。
- 不做复杂模型推理，只保存 AI Reasoning Plane 输出的假设和计划。
- 不保存敏感原文；大日志、trace、命令输出只保存引用、摘要和 hash。
- 不替代工单系统，但可以与工单双向同步状态和链接。

## 4. 核心领域对象

```text
Case
  id
  type: incident | operation | debug | middleware_repair | maintenance | drill
  status: new | triaging | diagnosing | planning | awaiting_approval | executing | verifying | mitigated | resolved | failed | closed
  severity: sev0 | sev1 | sev2 | sev3 | info
  source: coroot | erp | user | frontend_debug | middleware | ticket | manual
  title
  summary
  environment
  ownerUserId
  ownerTeamId
  affectedBusinessCapabilityIds
  affectedServiceIds
  affectedHostIds
  affectedMiddlewareIds
  traceContextRefs
  evidenceRefs
  hypothesisRefs
  actionRecordRefs
  approvalRefs
  verificationRefs
  postmortemId
  createdAt
  updatedAt
```

```text
EvidenceRef
  id
  caseId
  source: coroot | erp | tool | workflow | user | frontend_debug | change | log | trace | middleware
  entityType
  entityId
  rawRef
  summary
  confidence
  redactionStatus
  digest
  observedAt
  createdBy
```

```text
TimelineEvent
  id
  caseId
  type: case.created | evidence.added | hypothesis.updated | action.proposed | approval.changed | workflow.event | verification.completed | case.closed
  actorType: user | ai | system | workflow | integration
  actorId
  payloadRef
  summary
  createdAt
```

## 5. Case 类型

`incident`：告警、业务异常、用户报障等已经影响生产的问题。

`operation`：用户主动发起的运维操作，例如“检查 checkout 服务状态”。

`debug`：浏览器插件 Debug Mode 产生的慢请求或功能异常调试事件。

`middleware_repair`：PG、Redis、MQ、Elasticsearch 等中间件修复流程。

`maintenance`：计划内维护、巡检、变更前检查。

`drill`：演练流程，用生产治理链路但隔离真实写动作。

## 6. 状态机

```text
new
  -> triaging
  -> diagnosing
  -> planning
  -> awaiting_approval
  -> executing
  -> verifying
  -> mitigated
  -> resolved
  -> closed
```

允许失败分支：

```text
diagnosing -> failed
planning -> failed
executing -> failed
verifying -> failed
failed -> planning
failed -> closed
```

关键约束：

- `awaiting_approval` 必须有具体 `ActionProposal`。
- `executing` 必须存在有效 `ActionToken`，必要时存在 `HostLease`。
- `resolved` 必须至少有一条成功 `VerificationRecord`。
- `closed` 必须记录关闭原因、根因结论、遗留风险和经验沉淀状态。

## 7. 输入归一化

### Coroot 告警

输入字段：

- project、application、service、SLO、RCA、alert rule、timeline。

归一化规则：

- service 映射到 OpsGraph `Service`。
- SLO 和 RCA 转成 `EvidenceRef`。
- 严重级别映射到 case severity。
- 同一 service、同一 alert window、同一 root cause signature 的告警合并。

### ERP 异常

输入字段：

- ERP module、business capability、tenant、job、order、SLA、error code。

归一化规则：

- ERP module 映射到 OpsGraph `BusinessCapability`。
- tenant 和 order 只保存脱敏标识或引用。
- 业务异常可以创建 case，也可以附加到已有技术 case。

### 用户对话

输入字段：

- userId、sessionId、message、目标服务、时间窗口、用户意图。

归一化规则：

- 用户自然语言先解析为 `OperationIntent`，再决定创建新 case 或绑定已有 case。
- 对生产写操作意图只创建计划，不直接执行。

### DebugEvent

输入字段：

- page route、action name、trace id、frontend timings、backend span summary。

归一化规则：

- 有 trace id 时创建 `DebugCase` 或绑定同 trace 的 `IncidentCase`。
- 无 trace id 时创建证据不足 case，禁止自动修复结论。

### 中间件异常

输入字段：

- cluster、kind、role、replication、lock、disk、WAL、connections、failover state。

归一化规则：

- 创建 `middleware_repair` case。
- 必须先进入 diagnosing，不允许从 new 直接进入 executing。

## 8. Case 合并与关联

合并条件按强弱排序：

1. 相同 trace id。
2. 相同 Coroot incident id。
3. 相同 ERP business event id。
4. 相同 middleware cluster + 同一异常窗口。
5. 相同 service + 相同 root cause signature + 时间窗口重叠。

弱关联不自动合并，只建立 `relatedCaseIds`，避免把独立事故误合并。

## 9. API 草案

```text
GET    /api/v1/cases
POST   /api/v1/cases
GET    /api/v1/cases/{case_id}
PATCH  /api/v1/cases/{case_id}
POST   /api/v1/cases/{case_id}/evidence
POST   /api/v1/cases/{case_id}/hypotheses
POST   /api/v1/cases/{case_id}/actions
POST   /api/v1/cases/{case_id}/verifications
POST   /api/v1/cases/{case_id}/close
POST   /api/v1/cases/{case_id}/merge
POST   /api/v1/cases/{case_id}/relate
GET    /api/v1/cases/{case_id}/timeline
```

## 10. 事件契约

所有模块都通过 case event 写入事实：

```json
{
  "event_id": "evt_20260511_001",
  "case_id": "case_prod_checkout_001",
  "type": "evidence.added",
  "actor_type": "integration",
  "actor_id": "coroot",
  "payload_ref": "obj://evidence/coroot_rca_001",
  "summary": "checkout p95 latency exceeded SLO; postgres lock wait suspected",
  "created_at": "2026-05-11T10:15:00+08:00"
}
```

事件必须 append-only。状态对象可以更新，但 timeline 不能改写。

## 11. 前端体验

事故工作台围绕 case 展示：

- Header：标题、严重级别、状态、负责人、环境、更新时间。
- Impact：业务能力、租户影响、服务影响、SLO。
- Evidence：按来源和可信度分组。
- Hypothesis：根因候选、支持证据、反证、置信度。
- Actions：待审批、执行中、已完成、失败动作。
- Verification：自动验证、人工确认、业务恢复。
- Timeline：不可变事件流。
- Assets：可生成的 Runbook、Workflow、经验包、复盘。

## 12. 权限与安全

- 用户只能查看自己有资源权限的 case。
- Case 详情中的证据按字段做脱敏和权限裁剪。
- 高风险 case 的审批记录和操作记录不可删除。
- DebugCase 不能展示用户请求体、cookie、token 或敏感输入。
- Break-glass case 必须强制记录原因、审批人、执行人和后续复盘。

## 13. 验收标准

- Coroot webhook 能创建或更新 `IncidentCase`，并写入 evidence 和 timeline。
- 用户从现有 AI Chat 发起 PG 修复时，后端能创建或绑定 `middleware_repair` case，不要求修改 AI Chat 页面。
- DebugEvent 能创建 `DebugCase` 并关联 trace evidence。
- “修复 PG 集群”会创建 `middleware_repair` case，并停留在 diagnosing 或 planning，不会直接执行。
- 所有 ActionProposal、Approval、WorkflowRun、Verification 都能从 case 页面追溯。
- Case 关闭时能生成 postmortem draft 和 experience candidate。
