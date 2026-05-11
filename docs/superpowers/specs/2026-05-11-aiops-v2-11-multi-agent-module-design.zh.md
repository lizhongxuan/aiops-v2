# aiops-v2 Multi-Agent Collaboration 模块设计

日期：2026-05-11
状态：模块详细设计
所属总纲：[2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md](2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md)

## 1. 模块定位

Multi-Agent 是协作形态，不是独立事实源。多个 Agent 可以分工收集证据、诊断、推进 Runbook、生成计划、执行受治理动作、验证恢复和沉淀经验，但必须共享同一个 case、event log、policy、host lease、prompt trace 和 audit。

## 2. 设计目标

- 多 Agent 提升复杂事故处理效率，而不是制造多套状态。
- 每个 Agent 有明确职责、输入、输出和权限。
- 所有 Agent 输出都进入 case timeline 或 Prompt Trace。
- 执行动作仍由 Governed Action Plane 控制。
- 支持串行协作和并行协作，但最终由 Incident Commander Agent 或人工 owner 收敛。

## 3. Agent 角色

`Incident Commander Agent`：

- 维护 case 状态。
- 决定下一步需要什么证据。
- 汇总各 Agent 输出。
- 向用户呈现计划和风险。

`Observer Agent`：

- 收集 Coroot、ERP、trace、logs、K8s、host、中间件证据。
- 生成 EvidenceRef，不给最终根因定论。

`Diagnosis Agent`：

- 维护 Hypothesis。
- 对支持和反证做归因。
- 输出根因路径和置信度。

`Runbook Agent`：

- 匹配 Runbook、Capsule、Experience Pack。
- 推进 Runbook step。
- 生成下一步 ActionProposal draft。

`Execution Agent`：

- 只执行已批准 ActionToken 或 Workflow node。
- 不能自行发明新动作。

`Verification Agent`：

- 运行 VerificationSpec。
- 判断恢复、失败、部分恢复或需要回滚。

`Learning Agent`：

- 从 case、工具结果、验证和复盘生成经验包候选。
- 不发布生产资产。

## 4. 共享状态

```text
SharedCaseContext
  caseId
  eventLog
  evidenceRefs
  hypotheses
  actionProposals
  workflowRuns
  verificationRecords
  hostLeases
  promptTraceRefs
  auditRefs
```

Agent 不允许维护无法回放的私有事实。临时 scratchpad 可以存在，但结论必须落到共享上下文。

## 5. 协作流程

```text
case created
  -> Incident Commander assigns tasks
  -> Observer collects evidence
  -> Diagnosis updates hypotheses
  -> Runbook Agent matches assets
  -> Commander asks user for confirmation if needed
  -> Governed Action creates proposal/token
  -> Execution Agent runs approved action
  -> Verification Agent validates recovery
  -> Learning Agent creates candidates
```

并行规则：

- Observer 可以并行收集多源证据。
- Diagnosis 可以在 evidence 更新时重新评分。
- Execution Agent 不能和另一个执行 Agent 同时操作同一 exclusive host lease。
- Verification 必须等待相关动作完成。

## 6. Agent 输出协议

```text
AgentMessage
  id
  caseId
  agentRole
  messageType:
    - evidence_summary
    - hypothesis_update
    - plan_proposal
    - action_proposal_draft
    - execution_status
    - verification_result
    - learning_candidate
  refs
  confidence
  requiresHumanDecision
  createdAt
```

所有 AgentMessage 都进入 Prompt Trace 或 case timeline。

## 7. 冲突处理

冲突类型：

- 根因假设冲突。
- 计划冲突。
- 资源锁冲突。
- 经验匹配冲突。
- 验证结果冲突。

处理规则：

- 假设冲突用证据和反证解决，不靠 Agent 优先级。
- 计划冲突由 Incident Commander 汇总后让用户或审批人决策。
- 资源锁冲突由 HostLease 硬拒绝。
- 验证冲突时 case 不能进入 resolved。

## 8. API 草案

```text
POST   /api/v1/agents/tasks
GET    /api/v1/agents/tasks/{id}
POST   /api/v1/agents/tasks/{id}/messages
GET    /api/v1/cases/{case_id}/agent-messages
POST   /api/v1/cases/{case_id}/agent-coordination
```

## 9. UI 设计

Case 页面中多 Agent 展示为协作面板：

- 当前参与 Agent。
- 每个 Agent 的任务、状态和最新输出。
- 需要用户决策的事项。
- 冲突和未解决问题。
- 可展开的 Prompt Trace。

普通用户不需要理解内部多 Agent 机制，只看到更清晰的证据、计划和状态。

## 10. 安全与治理

- Agent 权限继承发起用户和 case resource scope。
- Execution Agent 只接受 ActionToken。
- Agent 不能读取未授权 evidence。
- Agent 输出如果包含敏感信息，进入 Prompt Trace 前必须脱敏。
- Agent 失败必须可见，不允许静默降级为“AI 已处理”。

## 11. 验收标准

- 多 Agent 共享同一个 case 和 event log。
- Observer、Diagnosis、Runbook、Execution、Verification、Learning 的输出都能追溯。
- Execution Agent 无 ActionToken 时不能执行工具。
- 两个 Agent 同时操作同一主机时 HostLease 能阻止冲突。
- 根因假设冲突能展示支持证据和反证。
- Learning Agent 只能生成候选经验，不能发布生产资产。
