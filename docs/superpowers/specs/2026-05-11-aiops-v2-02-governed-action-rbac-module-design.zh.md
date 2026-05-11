# aiops-v2 Governed Action & Multi-user RBAC 模块设计

日期：2026-05-11
状态：模块详细设计
所属总纲：[2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md](2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md)

## 1. 模块定位

Governed Action 模块是生产写动作的硬边界。它把 AI 建议、Runbook 下一步、Workflow 节点和人工操作统一转成 `ActionProposal`，再通过 RBAC、Policy、Approval、HostLease、ActionToken 和 Audit 决定是否允许执行。

多用户能力不是简单用户表，而是生产资源权限、会话隔离、审批职责、主机锁和审计的组合。

## 2. 设计目标

- AI、Runbook、Workflow 和 UI 都不能绕过 Action governance。
- 审批对象必须是具体动作，不是“同意 AI 修复”这种模糊意图。
- 同一时间一台主机只允许一个不兼容会话持有写锁。
- 用户只能对有权限的资源发起、审批和执行动作。
- 所有动作都有审计：谁提议、谁审批、谁执行、在哪些资源上执行、结果是什么。

## 3. 核心对象

```text
ActionProposal
  id
  caseId
  sessionId
  turnId
  source: ai | runbook | workflow | manual | break_glass
  toolName
  toolInput
  normalizedInputHash
  targetResources
  risk: readonly | low | medium | high | destructive
  approvalRequired
  hostLeaseRequired
  expectedEffect
  rollback
  verificationSpec
  evidenceRefs
  status: draft | pending_policy | pending_approval | approved | rejected | expired | executed | failed
  createdBy
  expiresAt
```

```text
ActionToken
  id
  proposalId
  caseId
  sessionId
  turnId
  userId
  toolName
  normalizedInputHash
  targetResources
  risk
  expiresAt
  status: active | used | expired | revoked
```

```text
HostLease
  id
  hostId
  caseId
  sessionId
  turnId
  userId
  mode: exclusive | shared_readonly
  status: active | released | expired
  reason
  acquiredAt
  heartbeatAt
  expiresAt
  releasedAt
```

## 4. RBAC 模型

```text
User -> Team -> Role -> Permission
Permission -> ResourceScope
ResourceScope:
  environment
  service
  host
  namespace
  middleware
  runbook
  workflow
  experience_pack
```

典型角色：

- `viewer`：查看 case、证据摘要和只读结果。
- `operator`：发起只读诊断和低风险动作。
- `sre`：审批中风险动作，执行受控修复。
- `service_owner`：审批业务服务相关变更。
- `dba`：审批数据库和中间件高风险动作。
- `admin`：管理权限和策略。
- `break_glass_admin`：紧急授权，强审计。

## 5. Policy 判断

PolicyEngine 输入：

```text
user
case
actionProposal
targetResources
risk
timeWindow
environment
evidenceQuality
activatedExperience
```

输出：

```text
PolicyDecision
  allow
  reasons
  requiredApprovals
  requiredLeases
  requiredPrechecks
  deniedFields
  auditLevel
```

典型策略：

- 生产环境高风险动作必须审批。
- 数据库 failover 必须 DBA + service owner 双审批。
- 证据质量不足时禁止自动修复，只允许只读诊断。
- 未审核经验包不能降低审批级别。
- 变更窗口外禁止非紧急写动作。

## 6. HostLease 流程

```text
resolve target host group
  -> acquire leases atomically
  -> if any host conflict, release all acquired leases
  -> bind leases to session, turn, case, action
  -> execute
  -> heartbeat during long run
  -> release on success, failure, cancel, timeout
```

冲突规则：

- `exclusive` 与任何 active lease 冲突。
- `shared_readonly` 可与 shared_readonly 共存。
- Workflow fanout 前必须先申请目标集合锁，避免半启动。
- Agent 离线时 lease 不能永久占用，必须 TTL + reconcile。

## 7. 审批流程

```text
ActionProposal
  -> policy check
  -> approval request
  -> approver reviews evidence, risk, rollback, verification
  -> approved creates ActionToken
  -> rejected records reason
  -> expired releases pending state
```

审批页面必须展示：

- 动作来源：AI、Runbook、Workflow、人工、break-glass。
- 目标资源。
- 标准化工具输入。
- 证据和根因假设。
- 预期效果。
- 风险等级。
- 回滚方案。
- 验证方式。

## 8. Break-glass

Break-glass 只用于紧急恢复，必须满足：

- 用户具备 break-glass 权限。
- 输入紧急原因。
- 自动通知审计渠道。
- 强制创建 postmortem follow-up。
- 不能跳过记录 ActionToken、ToolResult、Verification。

## 9. API 草案

```text
POST   /api/v1/action-proposals
GET    /api/v1/action-proposals/{id}
POST   /api/v1/action-proposals/{id}/policy-check
POST   /api/v1/action-proposals/{id}/approve
POST   /api/v1/action-proposals/{id}/reject
POST   /api/v1/action-proposals/{id}/issue-token
POST   /api/v1/host-leases/acquire
POST   /api/v1/host-leases/{id}/heartbeat
POST   /api/v1/host-leases/{id}/release
GET    /api/v1/audit/actions
GET    /api/v1/rbac/me
```

## 10. 审计

AuditLog 必须 append-only：

```text
AuditRecord
  id
  caseId
  actorId
  actorType
  action
  resourceRefs
  decision
  inputHash
  outputRef
  ipHash
  createdAt
```

审计不能只记录成功动作。被拒绝、过期、权限不足、锁冲突、策略拒绝都要记录。

## 11. 前端体验

- Chat 中的高风险建议显示为“待审批动作”，不是普通按钮。
- Case 页面展示当前锁、待审批、执行中动作。
- Workflow 页面展示每个节点对应的 ActionProposal 和 Approval 状态。
- 管理页支持角色、资源范围、审批策略、break-glass 权限配置。

## 12. 验收标准

- 非只读动作没有 ActionToken 时不能进入 ToolDispatcher。
- 用户无资源权限时，ActionProposal 创建或审批会被拒绝。
- 同一主机已有 exclusive lease 时，新会话发起写动作失败并释放已申请锁。
- Workflow 目标主机组有任一锁冲突时整体拒绝启动。
- 审批记录能回溯到 case、turn、proposal、evidence 和 actor。
- break-glass 动作有强审计和后续复盘要求。
