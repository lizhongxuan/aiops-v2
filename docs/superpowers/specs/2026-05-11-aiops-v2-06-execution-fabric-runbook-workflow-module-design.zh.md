# aiops-v2 Execution Fabric, Runbook & Workflow 模块设计

日期：2026-05-11
状态：模块详细设计
所属总纲：[2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md](2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md)

## 1. 模块定位

Execution Fabric 是 `aiops-v2` 的统一执行面。它承载本地命令、远程主机、Host Agent、MCP tools、Coroot、K8s、ERP action、脚本和 Runner workflow。Runbook 提供知识型处置路径，Workflow 提供可执行编排，但两者都必须通过 Governed Action 模块进入生产写动作。

## 2. 设计目标

- 所有工具调用共享 tool schema、权限、事件、trace、审计和结果引用。
- Runbook 只生成下一步 `ActionProposal`，不直接执行工具。
- Workflow 表达 DAG、并行、循环、审批、变量、输出和回调，但不绕过 RBAC、Policy、ActionToken 和 HostLease。
- 支持手动创建和 AI 创建 Runbook/Workflow。
- 支持多主机标签选择、fanout、失败阈值、批次和验证。

## 3. 执行链路

```text
ActionToken / Approved Workflow Node
  -> ToolDispatcher / Runner Engine
  -> Host Agent / MCP / K8s / ERP / Script
  -> ToolResult / RunEvent / Artifact
  -> EvidenceRef
  -> Verification
```

所有执行结果必须回写：

- case timeline。
- workflow run event。
- prompt trace tool call。
- audit log。
- evidence store。

## 4. ToolDispatcher

```text
ToolDefinition
  name
  description
  inputSchema
  outputSchema
  risk
  requiredPermissions
  requiredCapabilities
  idempotency
  timeout
  redactionRules
```

执行前检查：

- ActionToken 是否有效。
- normalizedInputHash 是否匹配。
- 用户和资源权限是否仍有效。
- HostLease 是否满足。
- SecretRef 是否可解析。
- 工具是否在 allowlist。

## 5. Runbook 模型

```text
Runbook
  id
  title
  version
  status: draft | reviewed | published | deprecated
  match:
    problemSignatures
    entityTypes
    environmentProfile
    disabledConditions
  steps:
    - id
      type: observe | decide | propose_action | verify | rollback
      title
      instructions
      evidenceRequirements
      actionTemplate
      risk
      expectedResult
      next
  verification
  rollback
  owners
  lineage
```

Runbook instance：

```text
RunbookInstance
  id
  caseId
  runbookId
  currentStepId
  status
  observations
  actionProposalRefs
  verificationRefs
```

## 6. Workflow 模型

```text
Workflow
  id
  title
  version
  status: draft | validated | dry_run_passed | published | deprecated
  start:
    targetSelector
    variables
    lockPolicy
  graph:
    nodes
    edges
  end:
    outputs
    callbacks
    verification
  riskSummary
  lineage
```

节点类型：

- `tool`：执行受治理工具。
- `approval`：等待人工审批。
- `condition`：基于变量或证据分支。
- `fanout`：按主机或标签循环。
- `verify`：执行验证。
- `callback`：写 case、ERP、webhook。
- `manual`：人工确认或人工步骤。

## 7. 目标选择和 Fanout

Start 支持：

```yaml
targetSelector:
  labels:
    role: web
    env: prod
  explicitHosts: []
lockPolicy:
  mode: exclusive
  acquire: before_run
fanout:
  mode: sequential | parallel | batch
  batchSize: 10
  failureThreshold: 1
```

规则：

- 目标解析必须在 dry-run 中可见。
- 生产写动作启动前必须申请目标锁。
- 多台主机 fanout 时，每个目标都要独立记录结果。
- 任一目标失败时按 failureThreshold 决定继续、暂停或回滚。

## 8. Workflow 生命周期

```text
draft
  -> validate
  -> dry_run
  -> risk_review
  -> publish
  -> run
  -> verify
  -> close
```

AI 创建的 workflow 默认只能进入 draft。发布必须经过 validate、dry-run、risk review 和人工确认。

## 9. 输出与回调

End 支持：

```yaml
outputs:
  - key: recovered
    source: verification.slo
  - key: failed_hosts
    source: fanout.failures
callbacks:
  - type: incident.update
  - type: erp.update
  - type: webhook
  - type: experience.candidate
```

回调必须幂等。外部系统不可用时，workflow run 状态不能丢失，应进入 callback retry。

## 10. API 草案

```text
GET    /api/v1/tools
POST   /api/v1/tools/{name}/execute
GET    /api/v1/runbooks
GET    /api/v1/runbooks/{id}
POST   /api/v1/runbook-instances
POST   /api/v1/runbook-instances/{id}/next
GET    /api/v1/workflows
POST   /api/v1/workflows
POST   /api/v1/workflows/{id}/validate
POST   /api/v1/workflows/{id}/dry-run
POST   /api/v1/workflows/{id}/publish
POST   /api/v1/workflows/{id}/runs
GET    /api/v1/workflow-runs/{run_id}
```

## 11. 可靠性

- 长运行 workflow 必须持久化 run state。
- Runner 重启后能从上次节点恢复或明确失败。
- Tool output 大于上下文限制时写入 object store，只保留摘要和 ref。
- 非幂等工具禁止自动重试，除非工具声明 retry safety。
- Agent 离线时节点进入 `agent_unavailable`，不是无限 pending。

## 12. 验收标准

- Runbook step 生成 ActionProposal，不直接执行工具。
- Workflow 节点执行高风险工具前必须有 ActionToken。
- Start 能定义主机组、标签、变量和锁策略。
- 每个步骤能指定目标标签，多目标按 fanout 策略运行。
- End 能展示输出变量、验证结果和回调结果。
- AI 生成 workflow 默认是 draft，不能直接发布或执行。
- Workflow run 事件能回写 case timeline 和 evidence。
