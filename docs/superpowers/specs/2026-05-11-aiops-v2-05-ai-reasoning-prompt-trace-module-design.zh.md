# aiops-v2 AI Reasoning & Prompt Trace 模块设计

日期：2026-05-11
状态：模块详细设计
所属总纲：[2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md](2026-05-11-aiops-v2-00-enterprise-control-plane-design.zh.md)
前端设计：[2026-05-11-aiops-v2-05a-ai-reasoning-prompt-trace-frontend-design.zh.md](2026-05-11-aiops-v2-05a-ai-reasoning-prompt-trace-frontend-design.zh.md)
实施清单：[2026-05-11-aiops-v2-05b-ai-reasoning-prompt-trace-frontend-todo.zh.md](2026-05-11-aiops-v2-05b-ai-reasoning-prompt-trace-frontend-todo.zh.md)

## 1. 模块定位

AI Reasoning Plane 负责在 case 上下文中完成证据理解、根因假设、Runbook/Workflow/经验匹配、计划生成和结果解释。Prompt Trace 负责把每轮推理的输入、工具可见性、系统规则、上下文拼装、模型输出、工具调用和治理决策完整记录下来，形成可回放的 AI 治理证据。

这个模块不直接执行生产动作。所有写动作必须转成 `ActionProposal`，交给 Governed Action Plane。

## 2. 设计目标

- AI 输出必须可解释：说明使用了哪些证据、图谱关系、经验包和规则。
- AI 输出必须可治理：写动作不能直接执行，只能生成动作提案或 Workflow draft。
- 每轮对话都能点进 Prompt Trace，看到 LLM 输入、工具、上下文和输出。
- 支持调试慢请求、Coroot RCA、中间件修复、ERP 异常等多源证据综合推理。
- 支持回归评测：提示词、工具 schema、经验包变更后能验证行为是否退化。

## 3. 输入上下文

PromptCompiler 拼装上下文时按层组织：

```text
System Policy
  -> Product Invariants
  -> User RBAC and Resource Scope
  -> Case Summary
  -> Evidence Summary
  -> OpsGraph Context
  -> Activated Experience
  -> Runbook / Workflow Metadata
  -> Tool Visibility
  -> Recent Transcript
  -> User Turn
```

每层都必须可追踪：

```text
PromptLayer
  name
  source
  tokenCount
  redactionStatus
  refs
  hash
```

## 4. 推理输出类型

AI 输出分成明确类型，避免自然语言混淆：

```text
ReasoningOutput
  type:
    - answer
    - evidence_request
    - hypothesis_update
    - diagnostic_plan
    - action_proposal_draft
    - repair_plan_draft
    - runbook_draft
    - workflow_draft
    - verification_plan
    - postmortem_draft
    - experience_candidate_summary
  caseId
  confidence
  supportingEvidenceRefs
  contradictingEvidenceRefs
  risk
```

不同输出类型进入不同模块：

- `hypothesis_update` 写入 Incident Control Plane。
- `action_proposal_draft` 进入 Governed Action Plane。
- `workflow_draft` 进入 Execution Fabric。
- `repair_plan_draft` 进入 Middleware Repair。
- `experience_candidate_summary` 进入 Learning & Asset Plane。

## 5. 根因假设模型

```text
Hypothesis
  id
  caseId
  title
  rootCauseCategory:
    - deployment
    - config
    - resource_saturation
    - dependency_failure
    - middleware_lock
    - replication_lag
    - network
    - application_bug
    - user_side
    - unknown
  confidence
  supportingEvidenceRefs
  contradictingEvidenceRefs
  requiredNextEvidence
  suggestedNextActions
  status: active | rejected | confirmed
```

AI 每次更新假设时必须说明：

- 为什么新增或提高置信度。
- 哪些证据支持。
- 哪些证据反对。
- 下一步验证什么。
- 如果要执行动作，风险和回滚是什么。

## 6. 工具选择规则

工具分级：

```text
readonly:
  - coroot.get_metrics
  - coroot.get_rca
  - trace.get_summary
  - logs.search
  - host.inspect

low_risk:
  - cache.warm
  - service.refresh_config_readonly_check

high_risk:
  - service.restart
  - db.failover
  - config.write
  - k8s.rollout_restart

destructive:
  - data.delete
  - disk.format
  - force_promote_primary
```

AI 可以直接建议 readonly 工具调用。其他等级必须生成 `ActionProposal`，不能隐藏在自然语言回答中。

## 7. Prompt Trace

```text
PromptTrace
  id
  caseId
  sessionId
  turnId
  model
  promptLayers
  visibleTools
  hiddenToolsDenied
  memoryRefs
  evidenceRefs
  opsGraphQueryRefs
  activatedExperienceRefs
  modelOutputRef
  toolCallRefs
  governanceDecisions
  evalLabels
  createdAt
```

Trace 页面需要展示：

- 本轮用户输入。
- Prompt layers 和每层来源。
- 可见工具和不可见工具原因。
- Evidence refs、OpsGraph query、Experience refs。
- 模型输出。
- 后续 ActionProposal、Approval、ToolResult。
- 与上一版 prompt 或经验索引的 diff。

## 8. DebugCase 推理流程

```text
DebugCase
  -> read TraceContext and frontend timings
  -> read Coroot service metrics and RCA
  -> query OpsGraph for action -> api -> service -> middleware
  -> locate slow span
  -> classify slow point: frontend | gateway | service | middleware | db | downstream
  -> produce Hypothesis and remediation options
  -> if automatable, draft ActionProposal or Workflow
  -> produce verification plan using replay trace
```

AI 回答必须包含：

- 慢在哪里。
- 为什么慢。
- 证据是什么。
- 建议是什么。
- 哪些建议可以自动化。
- 自动化修复需要什么审批和验证。

## 9. Middleware Repair 推理流程

```text
User: 帮我修复 xxx 的 PG 集群
  -> identify MiddlewareResource
  -> collect readonly evidence
  -> match approved RepairPlan / Capsule / Runbook / Workflow
  -> if match: explain source, success rate, disabled conditions
  -> if no match: generate RepairPlan draft
  -> wait for user confirmation
  -> create ActionProposal or Workflow run
```

AI 不得把“修复集群”直接解释为重启或 failover。

## 10. 评测与回归

Eval case 来源：

- 真实 case 关闭后的复盘。
- Prompt Trace 中的失败推理。
- 审核通过的经验包。
- 工具调用被拒绝的安全样本。
- DebugCase 慢点定位样本。
- 中间件修复成功和失败样本。

评测维度：

- 是否引用正确证据。
- 是否避免越权动作。
- 是否正确区分只读诊断和写动作。
- 是否生成可审批 ActionProposal。
- 是否识别缺失证据。
- 是否给出验证和回滚。

## 11. API 草案

```text
POST   /api/v1/ai/chat
POST   /api/v1/ai/diagnose
POST   /api/v1/ai/plan
POST   /api/v1/ai/compile-prompt
GET    /api/v1/prompt-traces/{trace_id}
GET    /api/v1/cases/{case_id}/prompt-traces
POST   /api/v1/evals/run
GET    /api/v1/evals/reports/{id}
```

## 12. 安全要求

- PromptCompiler 必须在拼装前做 RBAC 裁剪。
- 敏感 evidence 只进入摘要，不能直接进入 prompt。
- 未审核候选经验不能进入 Activation Index 或 prompt。
- 模型输出不能直接携带 SecretRef 解密结果。
- Prompt Trace 对普通用户隐藏系统策略和敏感上下文细节，只向授权 reviewer 展示。

## 13. 验收标准

- 每轮 AI 对话都生成 PromptTrace。
- 用户能从对话点进 trace，看到上下文层、工具、证据和模型输出。
- AI 对生产写动作只生成 ActionProposal，不直接执行。
- DebugCase 能输出慢点、证据、建议、可自动化修复和验证计划。
- Middleware Repair 请求能先生成 RCA 和 RepairPlan，不直接执行高风险动作。
- Prompt 或经验包变更后能用 Eval case 做回归。
